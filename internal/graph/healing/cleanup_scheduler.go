package healing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSchedulerNotRunning = errors.New("scheduler not running")
	ErrSchedulerRunning    = errors.New("scheduler already running")
)

type FixValidatorInterface interface {
	ValidateFix(ctx context.Context, fixAttempt *FixAttempt) (*ValidationResult, error)
	TrackFixAttempt(ctx context.Context, issueID uuid.UUID, fix *FixAttempt, outcome FixOutcome) error
	AutoUndo(ctx context.Context, fixAttempt *FixAttempt) error
	GetFixHistory(ctx context.Context, issueID uuid.UUID) (*FixHistory, error)
	ShouldRetry(ctx context.Context, issueID uuid.UUID) (bool, error)
}

type ContextCleanupInterface interface {
	CleanupStaleContext(ctx context.Context, bucketID uuid.UUID) error
	PruneStaleNodes(ctx context.Context) (int64, error)
	CleanupInjectedPrompts(ctx context.Context) (int64, error)
	DeduplicateContext(ctx context.Context, bucketID uuid.UUID) error
	TrackInjectedPrompt(ctx context.Context, prompt *InjectedPrompt) error
	MarkPromptUsed(ctx context.Context, promptID uuid.UUID) error
	GetContextStats(bucketID uuid.UUID) map[string]interface{}
}

type GraphMaintenanceInterface interface {
	PruneStaleNodes(ctx context.Context, ttl time.Duration) (int64, error)
	ConsolidateMemories(ctx context.Context) (int64, error)
	UpdateStalenessScores(ctx context.Context) error
	RebuildIndexes(ctx context.Context) (int64, error)
	CompactGraph(ctx context.Context) error
	RunFullMaintenance(ctx context.Context) (*CleanupStats, error)
	GetLastStats() *CleanupStats
	GetLastRun() time.Time
	IsRunning() bool
}

type CleanupScheduler struct {
	mu              sync.RWMutex
	store           GraphStore
	config          CleanupConfig
	fixValidator    FixValidatorInterface
	contextCleanup  ContextCleanupInterface
	graphMaintenance GraphMaintenanceInterface
	running         bool
	stopCh          chan struct{}
	wg              sync.WaitGroup
	lastRun         time.Time
	lastStats       *CleanupStats
	runCount        int64
	errorCount      int64
}

func NewCleanupScheduler(
	store GraphStore,
	config CleanupConfig,
	fixValidator FixValidatorInterface,
	contextCleanup ContextCleanupInterface,
	graphMaintenance GraphMaintenanceInterface,
) *CleanupScheduler {
	if config.Interval <= 0 {
		config.Interval = DefaultCleanupConfig().Interval
	}
	return &CleanupScheduler{
		store:            store,
		config:           config,
		fixValidator:     fixValidator,
		contextCleanup:   contextCleanup,
		graphMaintenance: graphMaintenance,
		stopCh:           make(chan struct{}),
	}
}

func (cs *CleanupScheduler) ScheduleCleanup(interval time.Duration) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return ErrSchedulerRunning
	}

	if interval <= 0 {
		interval = cs.config.Interval
	}

	cs.config.Interval = interval
	cs.running = true

	cs.wg.Add(1)
	go cs.runCleanupLoop()

	return nil
}

func (cs *CleanupScheduler) runCleanupLoop() {
	defer cs.wg.Done()

	ticker := time.NewTicker(cs.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-cs.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			stats, err := cs.runCleanup(ctx)
			cancel()
			if err != nil {
				cs.mu.Lock()
				cs.errorCount++
				cs.mu.Unlock()
			} else {
				cs.mu.Lock()
				cs.runCount++
				cs.lastRun = stats.CompletedAt
				cs.lastStats = stats
				cs.mu.Unlock()
			}
		}
	}
}

func (cs *CleanupScheduler) runCleanup(ctx context.Context) (*CleanupStats, error) {
	startedAt := time.Now()
	stats := &CleanupStats{
		StartedAt: startedAt,
		Errors:    []string{},
	}

	if cs.config.EnableContextCleanup && cs.contextCleanup != nil {
		promptsRemoved, err := cs.contextCleanup.CleanupInjectedPrompts(ctx)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("cleanup prompts: %v", err))
		}
		stats.PromptsRemoved = promptsRemoved

		nodesRemoved, err := cs.contextCleanup.PruneStaleNodes(ctx)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("prune stale nodes: %v", err))
		}
		stats.NodesRemoved += nodesRemoved
	}

	if cs.config.EnableGraphMaintenance && cs.graphMaintenance != nil {
		maintStats, err := cs.graphMaintenance.RunFullMaintenance(ctx)
		if err != nil {
			stats.Errors = append(stats.Errors, fmt.Sprintf("graph maintenance: %v", err))
		} else {
			stats.NodesRemoved += maintStats.NodesRemoved
			stats.MemoriesConsolidated += maintStats.MemoriesConsolidated
			stats.IndexesRebuilt += maintStats.IndexesRebuilt
			stats.GraphCompacted = maintStats.GraphCompacted
		}
	}

	stats.CompletedAt = time.Now()
	stats.Duration = stats.CompletedAt.Sub(startedAt)

	return stats, nil
}

func (cs *CleanupScheduler) RunCleanupNow(ctx context.Context) (*CleanupStats, error) {
	cs.mu.RLock()
	if !cs.running {
		cs.mu.RUnlock()
		return nil, ErrSchedulerNotRunning
	}
	cs.mu.RUnlock()

	return cs.runCleanup(ctx)
}

func (cs *CleanupScheduler) Stop() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return ErrSchedulerNotRunning
	}

	close(cs.stopCh)
	cs.wg.Wait()
	cs.running = false
	cs.stopCh = make(chan struct{})

	return nil
}

func (cs *CleanupScheduler) GetCleanupStats() *CleanupStats {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.lastStats == nil {
		return &CleanupStats{}
	}

	stats := *cs.lastStats
	stats.NodesRemoved += cs.getNodeCleanupCount()
	return &stats
}

func (cs *CleanupScheduler) getNodeCleanupCount() int64 {
	return cs.runCount
}

func (cs *CleanupScheduler) GetSchedulerStats() map[string]interface{} {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	return map[string]interface{}{
		"running":       cs.running,
		"interval":      cs.config.Interval.String(),
		"last_run":      cs.lastRun,
		"run_count":     cs.runCount,
		"error_count":   cs.errorCount,
		"last_duration": cs.lastStats.Duration.String(),
	}
}

func (cs *CleanupScheduler) ConfigureCleanup(config CleanupConfig) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return ErrSchedulerRunning
	}

	cs.config = config
	return nil
}

func (cs *CleanupScheduler) UpdateInterval(interval time.Duration) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	cs.config.Interval = interval

	if cs.running {
		close(cs.stopCh)
		cs.wg.Wait()
		cs.stopCh = make(chan struct{})
		cs.running = true
		cs.wg.Add(1)
		go cs.runCleanupLoop()
	}

	return nil
}

func (cs *CleanupScheduler) EnablePeriodicCleanup(enable bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.EnablePeriodicCleanup = enable
}

func (cs *CleanupScheduler) EnableFixValidation(enable bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.EnableFixValidation = enable
}

func (cs *CleanupScheduler) EnableContextCleanup(enable bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.EnableContextCleanup = enable
}

func (cs *CleanupScheduler) EnableGraphMaintenance(enable bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.EnableGraphMaintenance = enable
}

func (cs *CleanupScheduler) SetStalenessConfig(config StalenessConfig) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.StalenessConfig = config
}

func (cs *CleanupScheduler) SetFixValidatorConfig(config FixValidatorConfig) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.config.FixValidatorConfig = config
}

func (cs *CleanupScheduler) TriggerFixValidation(ctx context.Context, issueID uuid.UUID, fix *FixAttempt) (*ValidationResult, error) {
	if cs.fixValidator == nil {
		return nil, fmt.Errorf("fix validator not configured")
	}

	if !cs.config.EnableFixValidation {
		return nil, fmt.Errorf("fix validation disabled")
	}

	result, err := cs.fixValidator.ValidateFix(ctx, fix)
	if err != nil {
		return nil, err
	}

	outcome := FixOutcomeFailure
	if result.Valid {
		if result.AcceptanceMet && result.TestsFailed == 0 {
			outcome = FixOutcomeSuccess
		} else {
			outcome = FixOutcomePartial
		}
	} else if result.Confidence < 0.3 {
		outcome = FixOutcomeWorsened
	}

	if err := cs.fixValidator.TrackFixAttempt(ctx, issueID, fix, outcome); err != nil {
		return result, fmt.Errorf("track attempt: %w", err)
	}

	if outcome != FixOutcomeSuccess && cs.config.FixValidatorConfig.AutoUndoOnFailure {
		if err := cs.fixValidator.AutoUndo(ctx, fix); err != nil {
			return result, fmt.Errorf("auto undo: %w", err)
		}
	}

	return result, nil
}

func (cs *CleanupScheduler) GetConfig() CleanupConfig {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.config
}

func (cs *CleanupScheduler) IsRunning() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.running
}

func (cs *CleanupScheduler) GetNextRun() time.Time {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.lastRun.IsZero() {
		return time.Now().Add(cs.config.Interval)
	}
	return cs.lastRun.Add(cs.config.Interval)
}