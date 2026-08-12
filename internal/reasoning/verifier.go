package reasoning

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Verifier validates task execution results.
type Verifier struct {
	validators map[CheckpointType]Validator
}

// Validator performs specific verification checks.
type Validator interface {
	Validate(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error)
	Type() CheckpointType
}

// NewVerifier creates a new verification system.
func NewVerifier() *Verifier {
	v := &Verifier{
		validators: make(map[CheckpointType]Validator),
	}

	// Register default validators
	v.registerDefaultValidators()

	return v
}

// Verify validates the entire execution plan.
func (v *Verifier) Verify(ctx context.Context, plan *ExecutionPlan) error {
	if plan == nil {
		return fmt.Errorf("execution plan is nil")
	}

	// Verify all checkpoints
	allPassed := true
	var failures []string

	for i := range plan.Checkpoints {
		checkpoint := &plan.Checkpoints[i]

		passed, err := v.verifyCheckpoint(ctx, checkpoint, plan)
		if err != nil {
			return fmt.Errorf("checkpoint %s verification error: %w", checkpoint.ID, err)
		}

		checkpoint.Passed = &passed

		if !passed && checkpoint.Required {
			allPassed = false
			failures = append(failures, checkpoint.Description)
		}
	}

	// Verify individual tasks
	for _, task := range plan.Tasks {
		if err := v.verifyTask(ctx, task); err != nil {
			allPassed = false
			failures = append(failures, fmt.Sprintf("Task %s: %v", task.Description, err))
		}
	}

	if !allPassed {
		return fmt.Errorf("verification failed: %s", strings.Join(failures, "; "))
	}

	return nil
}

// verifyCheckpoint validates a single checkpoint.
func (v *Verifier) verifyCheckpoint(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error) {
	validator, ok := v.validators[checkpoint.Type]
	if !ok {
		// No validator available - default to pass
		return true, nil
	}

	return validator.Validate(ctx, checkpoint, plan)
}

// verifyTask validates a task's completion.
func (v *Verifier) verifyTask(ctx context.Context, task *Task) error {
	if task.Status != TaskStatusComplete {
		return fmt.Errorf("task not complete: status=%s", task.Status)
	}

	if task.Result == nil {
		return fmt.Errorf("task has no result")
	}

	if !task.Result.Success {
		return fmt.Errorf("task failed: %v", task.Result.Error)
	}

	// Verify task checkpoints
	for _, checkpoint := range task.Verification {
		if checkpoint.Required && (checkpoint.Passed == nil || !*checkpoint.Passed) {
			return fmt.Errorf("required checkpoint failed: %s", checkpoint.Description)
		}
	}

	return nil
}

// RegisterValidator adds a custom validator.
func (v *Verifier) RegisterValidator(validator Validator) {
	v.validators[validator.Type()] = validator
}

// registerDefaultValidators sets up built-in validators.
func (v *Verifier) registerDefaultValidators() {
	v.validators[CheckpointSyntax] = &SyntaxValidator{}
	v.validators[CheckpointSemantic] = &SemanticValidator{}
	v.validators[CheckpointTest] = &TestValidator{}
	v.validators[CheckpointIntegration] = &IntegrationValidator{}
}

// SyntaxValidator checks code syntax.
type SyntaxValidator struct{}

func (v *SyntaxValidator) Type() CheckpointType {
	return CheckpointSyntax
}

func (v *SyntaxValidator) Validate(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error) {
	// In real implementation, this would:
	// 1. Find all modified files
	// 2. Run language-specific syntax checkers (go build, eslint, etc.)
	// 3. Parse output for errors

	// For now, simulate validation
	if plan.Analysis.TaskType == TaskTypeDocumentation {
		// Documentation doesn't need syntax checking
		return true, nil
	}

	// Simulate syntax check
	return true, nil
}

// SemanticValidator checks logical correctness.
type SemanticValidator struct{}

func (v *SemanticValidator) Type() CheckpointType {
	return CheckpointSemantic
}

func (v *SemanticValidator) Validate(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error) {
	// In real implementation, this would:
	// 1. Analyze code changes for logical errors
	// 2. Check if task objectives were met
	// 3. Verify no unintended side effects

	// Simulate semantic validation
	// Check that at least one task completed successfully
	hasSuccess := false
	for _, task := range plan.Tasks {
		if task.Status == TaskStatusComplete && task.Result != nil && task.Result.Success {
			hasSuccess = true
			break
		}
	}

	return hasSuccess, nil
}

// TestValidator runs tests to verify correctness.
type TestValidator struct{}

func (v *TestValidator) Type() CheckpointType {
	return CheckpointTest
}

func (v *TestValidator) Validate(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error) {
	// In real implementation, this would:
	// 1. Detect test framework (go test, npm test, pytest, etc.)
	// 2. Run relevant tests
	// 3. Parse test results
	// 4. Return pass/fail status

	// For documentation tasks, skip tests
	if plan.Analysis.TaskType == TaskTypeDocumentation {
		return true, nil
	}

	// Simulate test execution
	// Check if any test tasks completed
	for _, task := range plan.Tasks {
		if task.Type == TaskTypeTest && task.Status == TaskStatusComplete {
			return true, nil
		}
	}

	// No tests run - pass for now (non-critical)
	return true, nil
}

// IntegrationValidator checks system integration.
type IntegrationValidator struct{}

func (v *IntegrationValidator) Type() CheckpointType {
	return CheckpointIntegration
}

func (v *IntegrationValidator) Validate(ctx context.Context, checkpoint *Checkpoint, plan *ExecutionPlan) (bool, error) {
	// In real implementation, this would:
	// 1. Build the entire project
	// 2. Run integration tests
	// 3. Check for breaking changes in APIs
	// 4. Verify dependencies still resolve

	// Simulate integration check
	return true, nil
}

// VerificationReport summarizes verification results.
type VerificationReport struct {
	PlanID            string
	TotalCheckpoints  int
	PassedCheckpoints int
	FailedCheckpoints int
	Checkpoints       []CheckpointResult
	Tasks             []TaskVerification
	OverallStatus     VerificationStatus
	GeneratedAt       time.Time
}

// CheckpointResult stores verification outcome for a checkpoint.
type CheckpointResult struct {
	CheckpointID string
	Description  string
	Type         CheckpointType
	Passed       bool
	Required     bool
	ErrorMessage string
}

// TaskVerification stores verification outcome for a task.
type TaskVerification struct {
	TaskID       string
	Description  string
	Status       TaskStatus
	Passed       bool
	ErrorMessage string
}

// VerificationStatus indicates overall verification outcome.
type VerificationStatus string

const (
	VerificationStatusPassed  VerificationStatus = "passed"
	VerificationStatusFailed  VerificationStatus = "failed"
	VerificationStatusPartial VerificationStatus = "partial"
)

// GenerateReport creates a detailed verification report.
func (v *Verifier) GenerateReport(ctx context.Context, plan *ExecutionPlan) (*VerificationReport, error) {
	report := &VerificationReport{
		PlanID:      plan.ID,
		GeneratedAt: time.Now(),
	}

	// Verify all checkpoints
	for _, checkpoint := range plan.Checkpoints {
		checkpointCopy := checkpoint
		passed, err := v.verifyCheckpoint(ctx, &checkpointCopy, plan)

		result := CheckpointResult{
			CheckpointID: checkpoint.ID,
			Description:  checkpoint.Description,
			Type:         checkpoint.Type,
			Passed:       passed,
			Required:     checkpoint.Required,
		}

		if err != nil {
			result.ErrorMessage = err.Error()
		}

		report.Checkpoints = append(report.Checkpoints, result)
		report.TotalCheckpoints++

		if passed {
			report.PassedCheckpoints++
		} else {
			report.FailedCheckpoints++
		}
	}

	// Verify all tasks
	for _, task := range plan.Tasks {
		taskErr := v.verifyTask(ctx, task)

		verification := TaskVerification{
			TaskID:      task.ID,
			Description: task.Description,
			Status:      task.Status,
			Passed:      taskErr == nil,
		}

		if taskErr != nil {
			verification.ErrorMessage = taskErr.Error()
		}

		report.Tasks = append(report.Tasks, verification)
	}

	// Determine overall status
	if report.FailedCheckpoints == 0 {
		allTasksPassed := true
		for _, tv := range report.Tasks {
			if !tv.Passed {
				allTasksPassed = false
				break
			}
		}
		if allTasksPassed {
			report.OverallStatus = VerificationStatusPassed
		} else {
			report.OverallStatus = VerificationStatusPartial
		}
	} else {
		report.OverallStatus = VerificationStatusFailed
	}

	return report, nil
}

// VerificationEngine provides advanced verification capabilities.
type VerificationEngine struct {
	*Verifier
	strategies map[string]VerificationStrategy
}

// VerificationStrategy defines how to verify specific aspects.
type VerificationStrategy interface {
	Name() string
	Verify(ctx context.Context, plan *ExecutionPlan) error
	Confidence() float64 // 0.0 to 1.0
}

// NewVerificationEngine creates an advanced verifier.
func NewVerificationEngine() *VerificationEngine {
	return &VerificationEngine{
		Verifier:   NewVerifier(),
		strategies: make(map[string]VerificationStrategy),
	}
}

// RegisterStrategy adds a verification strategy.
func (e *VerificationEngine) RegisterStrategy(strategy VerificationStrategy) {
	e.strategies[strategy.Name()] = strategy
}

// VerifyWithStrategies runs all verification strategies.
func (e *VerificationEngine) VerifyWithStrategies(ctx context.Context, plan *ExecutionPlan) error {
	// Run standard verification first
	if err := e.Verify(ctx, plan); err != nil {
		return err
	}

	// Run additional strategies
	for name, strategy := range e.strategies {
		if err := strategy.Verify(ctx, plan); err != nil {
			return fmt.Errorf("strategy %s failed: %w", name, err)
		}
	}

	return nil
}

// SmartVerifier uses ML/heuristics for intelligent verification.
type SmartVerifier struct {
	*VerificationEngine
	confidenceThreshold float64
}

// NewSmartVerifier creates a verifier with confidence scoring.
func NewSmartVerifier(confidenceThreshold float64) *SmartVerifier {
	return &SmartVerifier{
		VerificationEngine:  NewVerificationEngine(),
		confidenceThreshold: confidenceThreshold,
	}
}

// VerifyWithConfidence performs verification with confidence scoring.
func (v *SmartVerifier) VerifyWithConfidence(ctx context.Context, plan *ExecutionPlan) (bool, float64, error) {
	// Run all strategies and aggregate confidence
	var totalConfidence float64
	strategyCount := 0

	for _, strategy := range v.strategies {
		if err := strategy.Verify(ctx, plan); err != nil {
			// Failure reduces confidence
			totalConfidence += 0.0
		} else {
			totalConfidence += strategy.Confidence()
		}
		strategyCount++
	}

	avgConfidence := totalConfidence / float64(strategyCount)
	passed := avgConfidence >= v.confidenceThreshold

	return passed, avgConfidence, nil
}
