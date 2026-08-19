package healing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

var (
	ErrFixAttemptNotFound = errors.New("fix attempt not found")
	ErrIssueNotFound      = errors.New("issue not found")
	ErrValidationTimeout  = errors.New("validation timeout")
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

type TestRunner interface {
	RunTests(ctx context.Context, fixData json.RawMessage) (passed, failed int, err error)
}

type AcceptanceChecker interface {
	CheckAcceptance(ctx context.Context, fixData json.RawMessage) (bool, error)
}

type FixApplier interface {
	ApplyFix(ctx context.Context, fixData json.RawMessage) error
	RevertFix(ctx context.Context, fixData json.RawMessage) error
}

type FixValidator struct {
	mu               sync.RWMutex
	store            GraphStore
	config           FixValidatorConfig
	testRunner       TestRunner
	acceptanceChecker AcceptanceChecker
	fixApplier       FixApplier
	fixHistory       map[uuid.UUID]*FixHistory
	attemptCounts    map[uuid.UUID]int
}

func NewFixValidator(
	store GraphStore,
	config FixValidatorConfig,
	testRunner TestRunner,
	acceptanceChecker AcceptanceChecker,
	fixApplier FixApplier,
) *FixValidator {
	return &FixValidator{
		store:             store,
		config:            config,
		testRunner:        testRunner,
		acceptanceChecker: acceptanceChecker,
		fixApplier:        fixApplier,
		fixHistory:        make(map[uuid.UUID]*FixHistory),
		attemptCounts:     make(map[uuid.UUID]int),
	}
}

func (fv *FixValidator) ValidateFix(ctx context.Context, fixAttempt *FixAttempt) (*ValidationResult, error) {
	if fv.config.ValidationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, fv.config.ValidationTimeout)
		defer cancel()
	}

	result := &ValidationResult{
		Valid: false,
	}

	acceptanceMet, err := fv.acceptanceChecker.CheckAcceptance(ctx, fixAttempt.FixData)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("acceptance check failed: %v", err))
	} else {
		result.AcceptanceMet = acceptanceMet
	}

	testsPassed, testsFailed, err := fv.testRunner.RunTests(ctx, fixAttempt.FixData)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("test run failed: %v", err))
	} else {
		result.TestsPassed = testsPassed
		result.TestsFailed = testsFailed
	}

	result.Confidence = fv.calculateConfidence(result)

	result.Valid = fv.isValid(result)
	result.Duration = time.Since(fixAttempt.CreatedAt)

	return result, nil
}

func (fv *FixValidator) calculateConfidence(result *ValidationResult) float64 {
	if result.TestsPassed+result.TestsFailed == 0 {
		return 0.5
	}
	testScore := float64(result.TestsPassed) / float64(result.TestsPassed+result.TestsFailed)
	acceptanceScore := 0.0
	if result.AcceptanceMet {
		acceptanceScore = 1.0
	}
	return (testScore + acceptanceScore) / 2.0
}

func (fv *FixValidator) isValid(result *ValidationResult) bool {
	if fv.config.RequireTestsPass && result.TestsFailed > 0 {
		return false
	}
	if fv.config.RequireAcceptanceMet && !result.AcceptanceMet {
		return false
	}
	if result.Confidence < fv.config.MinConfidence {
		return false
	}
	return true
}

func (fv *FixValidator) TrackFixAttempt(ctx context.Context, issueID uuid.UUID, fix *FixAttempt, outcome FixOutcome) error {
	fv.mu.Lock()
	defer fv.mu.Unlock()

	history, exists := fv.fixHistory[issueID]
	if !exists {
		history = &FixHistory{
			IssueID:    issueID,
			Attempts:   []*FixAttempt{},
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		fv.fixHistory[issueID] = history
	}

	fix.Outcome = outcome
	fix.CompletedAt = ptrTime(time.Now())
	history.Attempts = append(history.Attempts, fix)
	history.TotalAttempts++
	history.LastOutcome = outcome
	history.UpdatedAt = time.Now()

	if outcome.Score() > history.BestOutcome.Score() {
		history.BestOutcome = outcome
	}

	fv.attemptCounts[issueID] = history.TotalAttempts

	if fv.config.TrackAttempts {
		if err := fv.persistFixAttempt(ctx, fix); err != nil {
			return fmt.Errorf("persist fix attempt: %w", err)
		}
	}

	return nil
}

func (fv *FixValidator) persistFixAttempt(ctx context.Context, attempt *FixAttempt) error {
	node, err := fv.attemptToNode(attempt)
	if err != nil {
		return err
	}
	return fv.store.CreateNode(ctx, node)
}

func (fv *FixValidator) attemptToNode(attempt *FixAttempt) (interface{}, error) {
	data, err := json.Marshal(attempt)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":         attempt.ID.String(),
		"type":       "fix_attempt",
		"data":       data,
		"created_at": attempt.CreatedAt,
		"updated_at": attempt.CompletedAt,
	}, nil
}

func (fv *FixValidator) AutoUndo(ctx context.Context, fixAttempt *FixAttempt) error {
	if fixAttempt.Outcome == FixOutcomeSuccess {
		return nil
	}

	if !fv.config.AutoUndoOnFailure {
		return nil
	}

	if fv.fixApplier == nil {
		return errors.New("no fix applier available for undo")
	}

	err := fv.fixApplier.RevertFix(ctx, fixAttempt.FixData)
	if err != nil {
		return fmt.Errorf("auto-undo failed: %w", err)
	}

	now := time.Now()
	fixAttempt.RevertedAt = &now

	return nil
}

func (fv *FixValidator) GetFixHistory(ctx context.Context, issueID uuid.UUID) (*FixHistory, error) {
	fv.mu.RLock()
	defer fv.mu.RUnlock()

	history, exists := fv.fixHistory[issueID]
	if !exists {
		return nil, ErrIssueNotFound
	}

	return history, nil
}

func (fv *FixValidator) ShouldRetry(ctx context.Context, issueID uuid.UUID) (bool, error) {
	fv.mu.RLock()
	defer fv.mu.RUnlock()

	history, exists := fv.fixHistory[issueID]
	if !exists {
		return true, nil
	}

	if history.TotalAttempts >= fv.config.MaxRetries {
		return false, ErrMaxRetriesExceeded
	}

	if history.LastOutcome == FixOutcomeWorsened {
		return false, nil
	}

	if history.BestOutcome == FixOutcomeSuccess {
		return false, nil
	}

	avgConfidence := fv.averageConfidence(history)
	if avgConfidence < fv.config.MinConfidence {
		return false, nil
	}

	return true, nil
}

func (fv *FixValidator) averageConfidence(history *FixHistory) float64 {
	if len(history.Attempts) == 0 {
		return 0
	}
	sum := 0.0
	for _, a := range history.Attempts {
		sum += a.Confidence
	}
	return sum / float64(len(history.Attempts))
}

func (fv *FixValidator) GetAttemptCount(issueID uuid.UUID) int {
	fv.mu.RLock()
	defer fv.mu.RUnlock()
	return fv.attemptCounts[issueID]
}

func (fv *FixValidator) ClearHistory(issueID uuid.UUID) {
	fv.mu.Lock()
	defer fv.mu.Unlock()
	delete(fv.fixHistory, issueID)
	delete(fv.attemptCounts, issueID)
}

func (fv *FixValidator) ExportHistory(ctx context.Context) ([]byte, error) {
	fv.mu.RLock()
	defer fv.mu.RUnlock()

	data := make(map[string]*FixHistory)
	for k, v := range fv.fixHistory {
		data[k.String()] = v
	}
	return yaml.Marshal(data)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}