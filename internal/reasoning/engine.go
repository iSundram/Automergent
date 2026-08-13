package reasoning

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/recovery"
	"github.com/iSundram/Automergent/internal/diagnostics/types"
	"github.com/iSundram/Automergent/internal/learning"
	planningPkg "github.com/iSundram/Automergent/internal/planning"
	"github.com/iSundram/Automergent/internal/verification"
)

// Engine is the core reasoning system that orchestrates task analysis,
// planning, execution, and verification.
type Engine struct {
	planner         *planningPkg.Planner
	executor        *Executor
	verifier        *verification.Engine
	learningSystem  *learning.PatternRecognizer
	learningStorage learning.Storage
	strategies      map[TaskType]Strategy
	mu              sync.RWMutex
	logger          Logger

	// Configuration
	maxRetries             int
	parallelWorkers        int
	enableExtendedThinking bool
	thinkingBudget         int

	// State
	currentState *ExecutionState
	trace        *ReasoningTrace
}

// Logger defines a simple logging interface
type Logger interface {
	Warn(format string, args ...interface{})
	Info(format string, args ...interface{})
}

// EngineConfig configures the reasoning engine.
type EngineConfig struct {
	MaxRetries             int
	ParallelWorkers        int
	EnableExtendedThinking bool
	ThinkingBudget         int // token budget for thinking
	DefaultTimeout         time.Duration
}

// DefaultEngineConfig returns sensible defaults.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxRetries:             3,
		ParallelWorkers:        3,
		EnableExtendedThinking: true,
		ThinkingBudget:         10000,
		DefaultTimeout:         5 * time.Minute,
	}
}

// NewEngine creates a new reasoning engine.
func NewEngine(cfg *EngineConfig) *Engine {
	if cfg == nil {
		cfg = DefaultEngineConfig()
	}

	e := &Engine{
		planner:                planningPkg.NewPlanner("."),
		executor:               NewExecutor(cfg.ParallelWorkers),
		verifier:               verification.NewDefaultEngine(),
		learningSystem:         learning.NewPatternRecognizer(),
		learningStorage:        mustCreateLearningStorage(),
		strategies:             make(map[TaskType]Strategy),
		maxRetries:             cfg.MaxRetries,
		parallelWorkers:        cfg.ParallelWorkers,
		enableExtendedThinking: cfg.EnableExtendedThinking,
		thinkingBudget:         cfg.ThinkingBudget,
		trace:                  &ReasoningTrace{StartedAt: time.Now()},
	}

	// Register default strategies
	e.registerDefaultStrategies()

	return e
}

// Process is the main entry point that orchestrates the entire reasoning flow.
func (e *Engine) Process(ctx context.Context, userRequest string) (*ExecutionPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.trace = &ReasoningTrace{StartedAt: time.Now()}

	// Phase 1: Analyze the problem
	e.addThought(PhaseAnalysis, "Analyzing user request to determine intent and scope", "", "")
	analysis, err := e.analyze(ctx, userRequest)
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}
	e.addThought(PhaseAnalysis, "Analysis complete", "classified_task",
		fmt.Sprintf("Type=%s, Scope=%s, Complexity=%s", analysis.TaskType, analysis.Scope, analysis.Complexity))

	// Phase 2: Create execution plan
	e.addThought(PhasePlanning, "Creating structured execution plan", "", "")
	plan, err := e.plan(ctx, analysis)
	if err != nil {
		return nil, fmt.Errorf("planning failed: %w", err)
	}
	e.addThought(PhasePlanning, "Plan created", "task_count", fmt.Sprintf("%d tasks", len(plan.Tasks)))

	// Check if user confirmation is needed
	if e.shouldAskUser(analysis, plan) {
		e.addThought(PhasePlanning, "User confirmation required", "confidence",
			fmt.Sprintf("%.2f (below threshold)", weightedAverage(
				[]float64{analysis.Confidence, plan.Confidence, e.strategyConfidence(analysis.TaskType)},
				[]float64{0.4, 0.4, 0.2},
			)))
		plan.Metadata["requires_confirmation"] = "true"
		return plan, nil
	}

	// Phase 3: Execute the plan
	e.addThought(PhaseExecution, "Beginning task execution", "", "")
	executionStarted := time.Now()
	if err := e.execute(ctx, plan); err != nil {
		return nil, fmt.Errorf("execution failed: %w", err)
	}

	// Phase 4: Verify results
	e.addThought(PhaseVerification, "Verifying execution results", "", "")
	verificationResult, err := e.verify(ctx, plan)
	if err != nil {
		// Verification failure - attempt recovery
		e.addThought(PhaseVerification, "Verification failed, attempting recovery", "error", err.Error())
		if err := e.recover(ctx, plan, err); err != nil {
			return nil, fmt.Errorf("verification failed and recovery unsuccessful: %w", err)
		}
	} else {
		_ = e.refineStrategy(ctx, plan, verificationResult, time.Since(executionStarted))
	}

	e.addThought(PhaseComplete, "Task completed successfully", "", "")
	e.trace.Duration = time.Since(e.trace.StartedAt)

	return plan, nil
}

// Analyze performs problem analysis to understand the user's intent.
func (e *Engine) Analyze(ctx context.Context, userRequest string) (*TaskAnalysis, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.analyze(ctx, userRequest)
}

func (e *Engine) analyze(ctx context.Context, userRequest string) (*TaskAnalysis, error) {
	analysis := &TaskAnalysis{
		Intent:     extractIntent(userRequest),
		Metadata:   make(map[string]string),
		AnalyzedAt: time.Now(),
	}

	// Classify task type
	analysis.TaskType = e.classifyTask(userRequest)

	// Determine scope
	analysis.Scope = e.determineScope(userRequest)

	// Assess complexity
	analysis.Complexity = e.assessComplexity(userRequest, analysis.TaskType, analysis.Scope)

	// Estimate time
	analysis.EstimatedTime = e.estimateTime(analysis.Complexity)

	// Identify risks
	analysis.Risks = e.identifyRisks(analysis)

	// Extract required context
	analysis.RequiredFiles = e.extractRequiredFiles(userRequest)
	analysis.Dependencies = e.extractDependencies(userRequest)

	// Record assumptions
	analysis.Assumptions = e.recordAssumptions(analysis)

	// Calculate confidence in analysis
	analysis.Confidence = e.calculateAnalysisConfidence(analysis)

	analysis.Metadata["thinking_budget"] = fmt.Sprintf("%d", e.thinkingBudget)
	analysis.Metadata["extended_thinking"] = fmt.Sprintf("%t", e.enableExtendedThinking)

	return analysis, nil
}

// Plan creates a structured execution plan.
func (e *Engine) Plan(ctx context.Context, analysis *TaskAnalysis) (*ExecutionPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.plan(ctx, analysis)
}

func (e *Engine) plan(ctx context.Context, analysis *TaskAnalysis) (*ExecutionPlan, error) {
	if analysis == nil {
		return nil, fmt.Errorf("analysis is nil")
	}
	if analysis.Metadata == nil {
		analysis.Metadata = make(map[string]string)
	}

	plan, err := e.planner.GeneratePlan(ctx, analysis.Intent)
	if err != nil {
		return nil, err
	}
	execPlan := convertPlan(plan, analysis)
	analysis.Metadata["plan_summary"] = plan.Summary()
	if execPlan.Metadata == nil {
		execPlan.Metadata = make(map[string]string)
	}
	execPlan.Metadata["plan_summary"] = plan.Summary()
	execPlan.Metadata["context_signals"] = ""
	execPlan.Metadata["thinking_budget"] = fmt.Sprintf("%d", e.thinkingBudget)
	execPlan.Metadata["extended_thinking"] = fmt.Sprintf("%t", e.enableExtendedThinking)

	// Calculate plan confidence
	execPlan.Confidence = e.calculatePlanConfidence(execPlan)

	return execPlan, nil
}

func (e *Engine) shouldAskUser(analysis *TaskAnalysis, plan *ExecutionPlan) bool {
	if analysis == nil || plan == nil {
		return true
	}

	if e.isDestructiveRequest(analysis, plan) {
		return true
	}

	confidence := weightedAverage(
		[]float64{analysis.Confidence, plan.Confidence, e.strategyConfidence(analysis.TaskType)},
		[]float64{0.4, 0.4, 0.2},
	)

	if confidence < 0.50 {
		return true
	}
	if confidence < 0.70 && len(analysis.Risks) > 0 {
		return true
	}
	if confidence < 0.80 && analysis.Scope == ScopeProjectWide {
		return true
	}

	return false
}

func weightedAverage(values, weights []float64) float64 {
	if len(values) == 0 || len(values) != len(weights) {
		return 0
	}

	var total, totalWeight float64
	for i := range values {
		if weights[i] <= 0 {
			continue
		}
		total += values[i] * weights[i]
		totalWeight += weights[i]
	}
	if totalWeight == 0 {
		return 0
	}
	return clampConfidence(total / totalWeight)
}

// Execute runs the execution plan.
func (e *Engine) Execute(ctx context.Context, plan *ExecutionPlan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.execute(ctx, plan)
}

func (e *Engine) execute(ctx context.Context, plan *ExecutionPlan) error {
	if plan == nil {
		return fmt.Errorf("execution plan is nil")
	}
	if e.executor == nil {
		e.executor = NewExecutor(e.parallelWorkers)
	}
	e.currentState = &ExecutionState{
		PlanID:       plan.ID,
		CurrentPhase: PhaseExecution,
		Attempts:     make(map[string]int),
		UpdatedAt:    time.Now(),
	}
	if err := e.executor.Execute(ctx, plan, e.currentState); err != nil {
		// Mark tasks as failed, not complete
		for _, task := range plan.Tasks {
			if task.Status != TaskStatusComplete {
				task.Status = TaskStatusFailed
				task.Result.Success = false
				task.Result.Error = err
			}
		}
		// Return the error so verification/recovery can run
		return fmt.Errorf("execution failed: %w", err)
	}
	return nil
}

func (e *Engine) strategyConfidence(taskType TaskType) float64 {
	if strategy, ok := e.strategies[taskType]; ok && strategy != nil {
		return clampConfidence(strategy.Confidence())
	}
	return 0.5
}

// Verify validates execution results.
func (e *Engine) Verify(ctx context.Context, plan *ExecutionPlan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.verify(ctx, plan)
	return err
}

func (e *Engine) verify(ctx context.Context, plan *ExecutionPlan) (*verification.Result, error) {
	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}
	if e.verifier == nil {
		e.verifier = verification.NewDefaultEngine()
	}
	vctx := &verification.Context{
		WorkingDir:      plan.Metadata["working_dir"],
		Files:           collectTaskFiles(plan),
		ChangedFiles:    collectChangedFiles(plan),
		Operation:       "reasoning",
		ToolName:        "reasoning-engine",
		ExpectedOutcome: plan.Analysis.Intent,
	}
	res, err := e.verifier.Verify(vctx)
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}
	plan.Metadata["verification_status"] = string(res.Status)
	plan.Metadata["verification_can_proceed"] = fmt.Sprintf("%t", res.CanProceed)
	if !res.CanProceed {
		return res, fmt.Errorf("verification gate failed: %s", res.Recommendation)
	}
	return res, nil
}

// GetTrace returns the current reasoning trace.
func (e *Engine) GetTrace() *ReasoningTrace {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.trace
}

// GetState returns the current execution state.
func (e *Engine) GetState() *ExecutionState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentState
}

// RegisterStrategy adds a custom strategy for a task type.
func (e *Engine) RegisterStrategy(taskType TaskType, strategy Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies[taskType] = newConcreteStrategy(taskType, strategy)
}

// Helper methods for analysis

func (e *Engine) classifyTask(request string) TaskType {
	req := strings.ToLower(request)

	if containsAny(req, []string{"test", "unit test", "integration test", "coverage"}) {
		return TaskTypeTest
	}
	if containsAny(req, []string{"document", "readme", "docs", "comment", "comments", "explain"}) {
		return TaskTypeDocumentation
	}
	if containsAny(req, []string{"why", "how does", "explain", "investigate", "analyze", "find out"}) {
		return TaskTypeInvestigation
	}

	// Bug fix indicators
	if containsAny(req, []string{"fix", "bug", "error", "issue", "crash", "broken"}) {
		return TaskTypeBugFix
	}

	// Feature indicators
	if containsAny(req, []string{"add", "create", "implement", "new feature", "build"}) {
		return TaskTypeFeature
	}

	// Refactor indicators
	if containsAny(req, []string{"refactor", "restructure", "improve", "optimize", "clean up"}) {
		return TaskTypeRefactor
	}

	// Multi-file indicators
	if containsAny(req, []string{"across", "all files", "multiple", "project-wide", "entire"}) {
		return TaskTypeMultiFile
	}

	// Default to feature if unclear
	return TaskTypeFeature
}

func (e *Engine) determineScope(request string) Scope {
	req := strings.ToLower(request)

	// Project-wide indicators
	if containsAny(req, []string{"entire project", "all files", "codebase", "project-wide", "everywhere"}) {
		return ScopeProjectWide
	}

	// Multi-file indicators
	if containsAny(req, []string{"multiple files", "across files", "several", "files"}) {
		return ScopeMultiFile
	}

	// External indicators
	if containsAny(req, []string{"external", "library", "package", "dependency", "integrate with", "third-party"}) {
		return ScopeExternal
	}

	// Default to single file
	return ScopeSingleFile
}

func (e *Engine) assessComplexity(request string, taskType TaskType, scope Scope) Complexity {
	// Base complexity on scope
	baseComplexity := ComplexitySimple

	switch scope {
	case ScopeProjectWide:
		baseComplexity = ComplexityComplex
	case ScopeMultiFile:
		baseComplexity = ComplexityModerate
	case ScopeExternal:
		baseComplexity = ComplexityModerate
	}

	// Check for complexity indicators in request
	req := strings.ToLower(request)
	if containsAny(req, []string{"complex", "difficult", "challenging", "architecture", "redesign"}) {
		baseComplexity = upgradeComplexity(baseComplexity)
	}

	if containsAny(req, []string{"simple", "quick", "easy", "trivial", "just"}) {
		baseComplexity = downgradeComplexity(baseComplexity)
	}

	return baseComplexity
}

func (e *Engine) estimateTime(complexity Complexity) time.Duration {
	switch complexity {
	case ComplexityTrivial:
		return 5 * time.Minute
	case ComplexitySimple:
		return 15 * time.Minute
	case ComplexityModerate:
		return 45 * time.Minute
	case ComplexityComplex:
		return 2 * time.Hour
	case ComplexityMajor:
		return 6 * time.Hour
	default:
		return 30 * time.Minute
	}
}

func (e *Engine) identifyRisks(analysis *TaskAnalysis) []string {
	risks := []string{}

	if analysis.Scope == ScopeProjectWide {
		risks = append(risks, "Project-wide changes may affect multiple systems")
	}

	if analysis.Complexity == ComplexityComplex || analysis.Complexity == ComplexityMajor {
		risks = append(risks, "High complexity increases chance of introducing bugs")
	}

	if analysis.TaskType == TaskTypeRefactor {
		risks = append(risks, "Refactoring may break existing functionality")
		risks = append(risks, "Tests required to ensure behavior preservation")
	}

	if len(analysis.Dependencies) > 0 {
		risks = append(risks, "External dependencies may introduce version conflicts")
	}

	return risks
}

func (e *Engine) extractRequiredFiles(request string) []string {
	// Simple heuristic: look for file patterns
	// In real implementation, this would use better NLP/pattern matching
	files := []string{}

	// Look for common patterns like "in file.go" or "file.go"
	words := strings.Fields(request)
	for _, word := range words {
		if strings.Contains(word, ".go") || strings.Contains(word, ".md") ||
			strings.Contains(word, ".yaml") || strings.Contains(word, ".json") {
			files = append(files, strings.Trim(word, ".,;:"))
		}
	}

	return files
}

func (e *Engine) extractDependencies(request string) []string {
	deps := []string{}
	req := strings.ToLower(request)

	if strings.Contains(req, "database") || strings.Contains(req, "db") {
		deps = append(deps, "database")
	}

	if strings.Contains(req, "api") || strings.Contains(req, "rest") {
		deps = append(deps, "api")
	}

	return deps
}

func (e *Engine) recordAssumptions(analysis *TaskAnalysis) []string {
	assumptions := []string{
		"User has necessary permissions to modify files",
		"Development environment is properly configured",
	}

	if analysis.Scope == ScopeProjectWide {
		assumptions = append(assumptions, "All project files are accessible")
	}

	return assumptions
}

func (e *Engine) recover(ctx context.Context, plan *ExecutionPlan, verifyErr error) error {
	// Self-correction loop: analyze failure → revise plan → retry
	recoveryStarted := time.Now()
	for attempt := 1; attempt <= e.maxRetries; attempt++ {
		e.addThought(PhaseExecution, "Attempting recovery", "attempt", fmt.Sprintf("%d/%d", attempt, e.maxRetries))

		// Analyze what went wrong
		failureAnalysis := e.analyzeFailure(plan, verifyErr)
		e.addThought(PhaseExecution, "Failure analyzed", "cause", failureAnalysis.RootCause)
		e.addThought(PhaseExecution, "Fix suggestions generated", "count", fmt.Sprintf("%d", len(failureAnalysis.SuggestedFixes)))

		// Record error pattern for learning
		if e.learningSystem != nil {
			e.learningSystem.RecordError(failureAnalysis.RootCause, false, "")
		}

		// Revise strategy
		revisedPlan, err := e.reviseStrategy(ctx, plan, &failureAnalysis)
		if err != nil {
			continue
		}
		plan = revisedPlan

		// Retry execution
		if err := e.execute(ctx, plan); err != nil {
			continue
		}

		// Verify again
		if verificationResult, err := e.verify(ctx, plan); err == nil {
			e.addThought(PhaseExecution, "Recovery successful", "attempt", fmt.Sprintf("%d", attempt))
			_ = e.refineStrategy(ctx, plan, verificationResult, time.Since(recoveryStarted))
			// Record successful resolution
			if e.learningSystem != nil {
				resolution := fmt.Sprintf("Applied %d fixes, retry attempt %d", len(failureAnalysis.SuggestedFixes), attempt)
				e.learningSystem.RecordError(failureAnalysis.RootCause, true, resolution)
			}
			return nil
		}
	}

	return fmt.Errorf("recovery failed after %d attempts", e.maxRetries)
}

func (e *Engine) analyzeFailure(plan *ExecutionPlan, err error) FailureAnalysis {
	analysis := FailureAnalysis{
		RootCause:      "unknown",
		Confidence:     0.0,
		ErrorPatterns:  []ErrorPattern{},
		FailedTasks:    []FailedTask{},
		SuggestedFixes: []Fix{},
		Retryable:      false,
		RequiresManual: false,
		AnalyzedAt:     time.Now(),
	}

	diagnostics := e.collectDiagnostics(plan, err)

	// Step 1: Parse error messages for patterns.
	errorPatterns := e.extractErrorPatterns(plan, err)
	analysis.ErrorPatterns = errorPatterns

	// Step 2: Use diagnostics recovery classification to identify the root cause.
	if len(diagnostics) > 0 {
		report := recovery.Summarize(diagnostics)
		if report.Primary.RootCauseHint != "" {
			analysis.RootCause = report.Primary.RootCauseHint
		} else if report.Summary != "" {
			analysis.RootCause = report.Summary
		}
		analysis.Confidence = report.Primary.Confidence
		analysis.Retryable = report.Primary.Retry.Retryable

		for i, suggestion := range report.Primary.FixSuggestions {
			analysis.SuggestedFixes = append(analysis.SuggestedFixes, Fix{
				Description: suggestion,
				Action:      "manual",
				Priority:    len(report.Primary.FixSuggestions) - i,
				Confidence:  report.Primary.Confidence,
				Automated:   false,
			})
		}
	} else if err != nil {
		analysis.RootCause = e.inferRootCause(err, errorPatterns)
		analysis.Confidence = 0.45
	}

	// Step 3: Query learning system for similar past failures.
	if e.learningSystem != nil {
		historicalPatterns := e.learningSystem.GetPatterns(learning.PatternTypeError)
		matchedPattern := e.matchHistoricalPattern(analysis.RootCause, errorPatterns, historicalPatterns)
		if matchedPattern != nil {
			analysis.HistoricalMatch = true
			if analysis.RootCause == "unknown" || analysis.RootCause == "" {
				if matchedPattern.Description != "" {
					analysis.RootCause = matchedPattern.Description
				} else {
					analysis.RootCause = matchedPattern.Name
				}
			}
			analysis.Confidence = clampConfidence((analysis.Confidence + matchedPattern.Confidence) / 2)

			if matchedPattern.Data.ResolutionPath != "" {
				analysis.SuggestedFixes = append([]Fix{{
					Description: "Previously successful resolution: " + matchedPattern.Data.ResolutionPath,
					Action:      "retry_with_adjustment",
					Priority:    10,
					Confidence:  clampConfidence(matchedPattern.Confidence),
					Automated:   true,
				}}, analysis.SuggestedFixes...)
			}
		}
	}

	// Step 4: Identify failed tasks and determine fixable/retryable flags.
	failedTasks := e.identifyFailedTasks(plan, diagnostics)
	analysis.FailedTasks = failedTasks
	for _, ft := range failedTasks {
		if ft.Retryable {
			analysis.Retryable = true
		}
		if !ft.Fixable {
			analysis.RequiresManual = true
		}
	}

	// Step 5: Generate fix suggestions with confidence scores.
	analysis.SuggestedFixes = append(analysis.SuggestedFixes, e.generateContextualFixes(plan, failedTasks, errorPatterns)...)
	analysis.SuggestedFixes = dedupFixes(analysis.SuggestedFixes)

	if analysis.Confidence == 0 {
		analysis.Confidence = 0.35
	}
	if len(analysis.ErrorPatterns) > 0 {
		analysis.Confidence = clampConfidence(analysis.Confidence + 0.05*float64(minInt(len(analysis.ErrorPatterns), 3)))
	}

	analysis.RequiresManual = analysis.RequiresManual || e.requiresManualIntervention(analysis)
	return analysis
}

// extractErrorPatterns identifies error patterns from verification results and error messages
func (e *Engine) extractErrorPatterns(plan *ExecutionPlan, err error) []ErrorPattern {
	patterns := make([]ErrorPattern, 0)
	if err != nil {
		patterns = append(patterns, parseErrorMessagePatterns(err.Error())...)
	}

	for _, diag := range e.collectDiagnostics(plan, err) {
		patterns = append(patterns, ErrorPattern{
			Pattern:    diag.Code,
			Severity:   diag.Severity,
			Category:   inferCategory(diag.Message),
			Location:   fmt.Sprintf("%s:%d:%d", diag.FilePath, diag.Line, diag.Column),
			Message:    diag.Message,
			Confidence: clampConfidence(0.85 + confidenceBonusForSeverity(diag.Severity)),
		})
	}

	return dedupErrorPatterns(patterns)
}

// collectDiagnostics gathers all diagnostic information from failed tasks
func (e *Engine) collectDiagnostics(plan *ExecutionPlan, err error) []types.Diagnostic {
	diagnostics := []types.Diagnostic{}
	if plan == nil {
		if err != nil {
			return []types.Diagnostic{{
				Severity: "error",
				Message:  err.Error(),
				Source:   "reasoning_engine",
			}}
		}
		return diagnostics
	}

	for _, task := range plan.Tasks {
		if task != nil && task.Status == TaskStatusFailed && task.VerificationResult != nil {
			diagnostics = append(diagnostics, diagnosticsFromVerificationResult(task.VerificationResult)...)
		}
	}

	// If no task diagnostics, create one from the error
	if len(diagnostics) == 0 && err != nil {
		diag := types.Diagnostic{
			Severity: "error",
			Message:  err.Error(),
			Source:   "reasoning_engine",
		}
		diagnostics = append(diagnostics, diag)
	}

	return diagnostics
}

// inferRootCause attempts to determine root cause from error and patterns
func (e *Engine) inferRootCause(err error, patterns []ErrorPattern) string {
	if len(patterns) > 0 {
		// Use the first high-confidence pattern
		for _, p := range patterns {
			if p.Confidence > 0.7 {
				return fmt.Sprintf("%s error: %s", p.Category, p.Message)
			}
		}
		return fmt.Sprintf("%s error detected", patterns[0].Category)
	}

	// Fallback to error message analysis
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "syntax") {
		return "Syntax error in source code"
	} else if strings.Contains(errMsg, "import") || strings.Contains(errMsg, "cannot find") {
		return "Import or dependency resolution error"
	} else if strings.Contains(errMsg, "permission") {
		return "Permission or access error"
	} else if strings.Contains(errMsg, "timeout") {
		return "Operation timeout (possibly transient)"
	}

	return fmt.Sprintf("Execution failed: %v", err)
}

// matchHistoricalPattern finds similar patterns from learning history
func (e *Engine) matchHistoricalPattern(rootCause string, currentPatterns []ErrorPattern, historicalPatterns []*learning.Pattern) *learning.Pattern {
	var bestMatch *learning.Pattern
	bestScore := 0.0

	rootCauseLower := strings.ToLower(rootCause)

	for _, hp := range historicalPatterns {
		score := 0.0

		// Match by error type
		for _, errorType := range hp.Data.ErrorTypes {
			if strings.Contains(rootCauseLower, strings.ToLower(errorType)) {
				score += 0.5
			}
		}

		// Match by pattern category
		for _, cp := range currentPatterns {
			for _, errorType := range hp.Data.ErrorTypes {
				if strings.Contains(strings.ToLower(errorType), strings.ToLower(cp.Category)) {
					score += 0.3
				}
			}
		}

		// Weight by confidence and frequency
		score *= hp.Confidence * (float64(hp.Frequency) / 10.0)

		if score > bestScore {
			bestScore = score
			bestMatch = hp
		}
	}

	// Only return if score is significant
	if bestScore > 0.3 {
		return bestMatch
	}

	return nil
}

// identifyFailedTasks collects information about failed tasks
func (e *Engine) identifyFailedTasks(plan *ExecutionPlan, diagnostics []types.Diagnostic) []FailedTask {
	failedTasks := []FailedTask{}
	if plan == nil {
		return failedTasks
	}

	for _, task := range plan.Tasks {
		if task == nil || task.Status != TaskStatusFailed {
			continue
		}

		ft := FailedTask{
			TaskID:       task.ID,
			Description:  task.Description,
			Error:        "",
			Fixable:      false,
			Retryable:    false,
			FailureCount: 1,
		}

		if task.Result != nil {
			if task.Result.Error != nil {
				ft.Error = task.Result.Error.Error()
			}
			if task.Result.Attempts > 0 {
				ft.FailureCount = task.Result.Attempts
			}
		}

		var relatedDiagnostics []types.Diagnostic
		if task.VerificationResult != nil {
			relatedDiagnostics = diagnosticsFromVerificationResult(task.VerificationResult)
		} else if task.Result != nil && task.Result.Error != nil {
			relatedDiagnostics = []types.Diagnostic{{
				Severity: "error",
				Message:  task.Result.Error.Error(),
				Source:   "task_result",
			}}
		} else if len(diagnostics) == 1 {
			relatedDiagnostics = diagnostics
		}

		for _, diag := range relatedDiagnostics {
			if ft.Error == "" {
				ft.Error = diag.Message
			}
			classification := recovery.ClassifyDiagnostic(diag)
			ft.Diagnostics = append(ft.Diagnostics, diag.Message)

			switch classification.Cause {
			case recovery.CauseTransient:
				ft.Retryable = true
				ft.Fixable = false
			case recovery.CauseSyntax, recovery.CauseImport, recovery.CauseDependency, recovery.CauseConfig:
				ft.Fixable = true
			case recovery.CauseMissingFile, recovery.CausePermission:
				ft.Fixable = false
			default:
				if strings.Contains(strings.ToLower(diag.Message), "test failed") ||
					strings.Contains(strings.ToLower(diag.Message), "build failed") {
					ft.Fixable = true
				}
			}

			if classification.Retry.Retryable {
				ft.Retryable = true
			}
		}

		if ft.Error == "" && task.Result != nil && task.Result.Error != nil {
			ft.Error = task.Result.Error.Error()
		}
		if ft.Error == "" {
			ft.Error = "task failed without structured diagnostic"
		}
		if !ft.Fixable && !ft.Retryable && len(ft.Diagnostics) > 0 {
			if !containsAny(strings.ToLower(ft.Error), []string{"permission denied", "missing file", "no such file"}) {
				ft.Fixable = true
			}
		}

		failedTasks = append(failedTasks, ft)
	}

	return failedTasks
}

// generateContextualFixes creates fix suggestions based on task context
func (e *Engine) generateContextualFixes(plan *ExecutionPlan, failedTasks []FailedTask, patterns []ErrorPattern) []Fix {
	fixes := []Fix{}

	// Generate fixes based on error patterns
	for _, pattern := range patterns {
		switch pattern.Category {
		case "syntax":
			fixes = append(fixes, Fix{
				Description: "Review and correct syntax errors in the affected files",
				Action:      "syntax_check",
				Priority:    8,
				Confidence:  0.85,
				Automated:   true,
				Steps: []string{
					"Run linter or formatter",
					"Check for unclosed brackets, quotes, or parentheses",
					"Validate against language grammar",
				},
			})

		case "import":
			fixes = append(fixes, Fix{
				Description: "Resolve missing imports or dependencies",
				Action:      "dependency_resolution",
				Priority:    7,
				Confidence:  0.8,
				Automated:   true,
				Steps: []string{
					"Check import statements for typos",
					"Install missing dependencies",
					"Verify module paths and availability",
				},
			})

		case "type":
			fixes = append(fixes, Fix{
				Description: "Fix type mismatches and type errors",
				Action:      "type_correction",
				Priority:    6,
				Confidence:  0.75,
				Automated:   false,
				Steps: []string{
					"Review type annotations and declarations",
					"Ensure type compatibility in assignments",
					"Check function signatures",
				},
			})

		case "test":
			fixes = append(fixes, Fix{
				Description: "Address test failures",
				Action:      "test_fix",
				Priority:    5,
				Confidence:  0.7,
				Automated:   false,
				Steps: []string{
					"Review test output for failure reasons",
					"Update test expectations if behavior changed",
					"Fix implementation to match test requirements",
				},
			})
		}
	}

	// Add task-specific fixes
	for _, ft := range failedTasks {
		if !ft.Fixable {
			fixes = append(fixes, Fix{
				Description: fmt.Sprintf("Manual investigation required for task: %s", ft.Description),
				Action:      "manual_review",
				Priority:    3,
				Confidence:  0.9,
				Automated:   false,
				Steps: []string{
					"Review error details: " + ft.Error,
					"Check task dependencies and prerequisites",
					"Validate execution environment",
				},
			})
		} else if ft.Retryable {
			fixes = append(fixes, Fix{
				Description: fmt.Sprintf("Retry task with adjusted parameters: %s", ft.TaskID),
				Action:      "retry_task",
				Priority:    4,
				Confidence:  0.65,
				Automated:   true,
				Steps: []string{
					"Reset task state",
					"Apply environmental fixes",
					"Re-execute with backoff",
				},
			})
		}
	}

	return fixes
}

// requiresManualIntervention determines if automatic recovery is unlikely
func (e *Engine) requiresManualIntervention(analysis FailureAnalysis) bool {
	// Low confidence suggests uncertainty
	if analysis.Confidence < 0.4 {
		return true
	}

	// Check if any failed tasks are not fixable
	for _, ft := range analysis.FailedTasks {
		if !ft.Fixable {
			return true
		}
	}

	// If not retryable and no high-confidence fixes
	if !analysis.Retryable {
		highConfidenceFixes := 0
		for _, fix := range analysis.SuggestedFixes {
			if fix.Confidence > 0.7 && fix.Automated {
				highConfidenceFixes++
			}
		}
		if highConfidenceFixes == 0 {
			return true
		}
	}

	return false
}

func diagnosticsFromVerificationResult(result *verification.Result) []types.Diagnostic {
	if result == nil {
		return nil
	}

	diagnostics := make([]types.Diagnostic, 0)
	for _, layer := range result.Layers {
		for _, issue := range layer.Issues {
			diagnostics = append(diagnostics, types.Diagnostic{
				FilePath: issue.File,
				Line:     issue.Line,
				Column:   issue.Column,
				Severity: string(issue.Severity),
				Code:     issue.Code,
				Message:  issue.Message,
				Source:   issue.Source,
			})
		}
	}
	return diagnostics
}

func (e *Engine) calculateAnalysisConfidence(analysis *TaskAnalysis) float64 {
	if analysis == nil {
		return 0
	}

	confidence := 0.65
	switch analysis.Scope {
	case ScopeProjectWide:
		confidence -= 0.18
	case ScopeMultiFile:
		confidence -= 0.08
	case ScopeExternal:
		confidence -= 0.10
	}

	switch analysis.Complexity {
	case ComplexityTrivial:
		confidence += 0.10
	case ComplexitySimple:
		confidence += 0.05
	case ComplexityComplex:
		confidence -= 0.10
	case ComplexityMajor:
		confidence -= 0.15
	}

	if len(analysis.Risks) > 0 {
		confidence -= 0.03 * float64(minInt(len(analysis.Risks), 3))
	}
	if len(analysis.Dependencies) > 0 {
		confidence -= 0.04
	}
	if len(analysis.RequiredFiles) > 0 {
		confidence += 0.03
	}

	return clampConfidence(confidence)
}

func (e *Engine) calculatePlanConfidence(plan *ExecutionPlan) float64 {
	if plan == nil {
		return 0
	}

	confidence := 0.60
	if plan.Analysis != nil {
		confidence = plan.Analysis.Confidence
	}

	switch {
	case len(plan.Tasks) == 0:
		confidence = 0.1
	case len(plan.Tasks) <= 3:
		confidence += 0.05
	case len(plan.Tasks) > 8:
		confidence -= 0.08
	}

	if len(plan.ExecutionOrder) > 1 {
		confidence += 0.03
	}
	if plan.Analysis != nil {
		if plan.Analysis.Scope == ScopeProjectWide {
			confidence -= 0.05
		}
		if len(plan.Analysis.Risks) > 0 {
			confidence -= 0.02 * float64(minInt(len(plan.Analysis.Risks), 3))
		}
	}

	return clampConfidence(confidence)
}

func (e *Engine) isDestructiveRequest(analysis *TaskAnalysis, plan *ExecutionPlan) bool {
	keywords := []string{"delete", "remove", "destroy", "drop", "overwrite", "wipe", "purge", "reset"}
	checkText := func(text string) bool {
		return containsAny(strings.ToLower(text), keywords)
	}

	if analysis != nil && checkText(analysis.Intent) {
		return true
	}
	if plan != nil {
		for _, task := range plan.Tasks {
			if task != nil && checkText(task.Description) {
				return true
			}
		}
	}

	return false
}

func clampConfidence(confidence float64) float64 {
	switch {
	case confidence < 0:
		return 0
	case confidence > 1:
		return 1
	default:
		return confidence
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func confidenceBonusForSeverity(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 0.12
	case "error":
		return 0.08
	case "warning":
		return 0.04
	default:
		return 0
	}
}

// inferCategory infers error category from diagnostic message
func inferCategory(message string) string {
	msgLower := strings.ToLower(message)
	if strings.Contains(msgLower, "syntax") {
		return "syntax"
	} else if strings.Contains(msgLower, "import") || strings.Contains(msgLower, "cannot find") {
		return "import"
	} else if strings.Contains(msgLower, "type") {
		return "type"
	} else if strings.Contains(msgLower, "test") {
		return "test"
	} else if strings.Contains(msgLower, "permission") {
		return "permission"
	}
	return "unknown"
}

func parseErrorMessagePatterns(message string) []ErrorPattern {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}

	matchers := []struct {
		regex      *regexp.Regexp
		category   string
		severity   string
		confidence float64
	}{
		{regexp.MustCompile(`(?i)syntax error[^,\n]*`), "syntax", "error", 0.95},
		{regexp.MustCompile(`(?i)undefined|not defined|cannot find`), "reference", "error", 0.9},
		{regexp.MustCompile(`(?i)type mismatch|type error`), "type", "error", 0.9},
		{regexp.MustCompile(`(?i)import.*failed|cannot import|module not found`), "import", "error", 0.92},
		{regexp.MustCompile(`(?i)permission denied|access denied`), "permission", "error", 0.88},
		{regexp.MustCompile(`(?i)timeout|timed out|request timeout`), "timeout", "warning", 0.78},
		{regexp.MustCompile(`(?i)file not found|no such file`), "missing_file", "error", 0.9},
		{regexp.MustCompile(`(?i)compilation failed|build failed`), "compilation", "error", 0.86},
		{regexp.MustCompile(`(?i)test.*failed|tests? failed`), "test", "error", 0.84},
	}

	patterns := make([]ErrorPattern, 0)
	for _, matcher := range matchers {
		for _, match := range matcher.regex.FindAllString(message, -1) {
			patterns = append(patterns, ErrorPattern{
				Pattern:    match,
				Severity:   matcher.severity,
				Category:   matcher.category,
				Message:    message,
				Confidence: clampConfidence(matcher.confidence),
			})
		}
	}

	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if containsAny(lower, []string{"error", "failed", "cannot", "not found", "timeout"}) {
			patterns = append(patterns, ErrorPattern{
				Pattern:    line,
				Severity:   "error",
				Category:   inferCategory(line),
				Message:    line,
				Confidence: 0.6,
			})
		}
	}

	return dedupErrorPatterns(patterns)
}

func dedupErrorPatterns(patterns []ErrorPattern) []ErrorPattern {
	seen := make(map[string]struct{}, len(patterns))
	out := make([]ErrorPattern, 0, len(patterns))
	for _, p := range patterns {
		key := strings.ToLower(strings.TrimSpace(p.Pattern)) + "|" + strings.ToLower(strings.TrimSpace(p.Category)) + "|" + strings.ToLower(strings.TrimSpace(p.Location)) + "|" + strings.ToLower(strings.TrimSpace(p.Message))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

func dedupFixes(fixes []Fix) []Fix {
	seen := make(map[string]struct{}, len(fixes))
	out := make([]Fix, 0, len(fixes))
	for _, fix := range fixes {
		key := strings.ToLower(strings.TrimSpace(fix.Description)) + "|" + strings.ToLower(strings.TrimSpace(fix.Action))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		fix.Confidence = clampConfidence(fix.Confidence)
		out = append(out, fix)
	}
	return out
}

func (e *Engine) reviseStrategy(ctx context.Context, plan *ExecutionPlan, failureAnalysis *FailureAnalysis) (*ExecutionPlan, error) {
	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}
	if failureAnalysis == nil {
		return nil, fmt.Errorf("failure analysis is nil")
	}
	if plan.Metadata == nil {
		plan.Metadata = make(map[string]string)
	}

	retryCount := 0
	if rc, ok := plan.Metadata["retry_count"]; ok {
		fmt.Sscanf(rc, "%d", &retryCount)
	}
	retryCount++
	plan.Metadata["retry_count"] = fmt.Sprintf("%d", retryCount)

	if retryCount > 3 {
		plan.Metadata["revision_aborted"] = "true"
		return nil, fmt.Errorf("maximum retry attempts (3) exceeded")
	}

	planner := NewPlanner()
	revisedTasks := make([]*Task, 0, len(plan.Tasks)+4)
	failedTasks := make([]*Task, 0)
	for _, task := range plan.Tasks {
		if task != nil && task.Status == TaskStatusFailed {
			failedTasks = append(failedTasks, task)
		}
	}

	for _, failedTask := range failedTasks {
		confidence := e.analyzeFailureConfidence(failedTask, failureAnalysis.RootCause)

		if e.isNonFixable(failedTask, failureAnalysis.RootCause) {
			failedTask.Status = TaskStatusSkipped
			if failedTask.Context == nil {
				failedTask.Context = make(map[string]string)
			}
			failedTask.Context["skip_reason"] = "Non-fixable: " + failureAnalysis.RootCause
			failedTask.Context["skipped_at"] = time.Now().Format(time.RFC3339)
			e.addThought(PhasePlanning, "Skipping non-fixable task", "task", failedTask.Description)
			continue
		}

		if confidence < 0.6 {
			diagnosticTask := e.createDiagnosticTask(failedTask, failureAnalysis.RootCause)
			revisedTasks = append(revisedTasks, diagnosticTask)
			failedTask.Dependencies = appendUniqueString(failedTask.Dependencies, diagnosticTask.ID)
			e.addThought(PhasePlanning, "Adding diagnostic task", "reason", "low confidence in root cause")
		}

		if failureAnalysis.Confidence > 0.7 && e.hasLearningEngine() {
			repairTask := e.createRepairTaskFromLearning(ctx, failedTask, failureAnalysis)
			if repairTask != nil {
				revisedTasks = append(revisedTasks, repairTask)
				failedTask.Dependencies = appendUniqueString(failedTask.Dependencies, repairTask.ID)
				e.addThought(PhasePlanning, "Applying learned fix", "task", repairTask.Description)
			}
		}

		failedTask.Status = TaskStatusPending
		if failedTask.Result == nil {
			failedTask.Result = &TaskResult{}
		}
		failedTask.Result.Attempts++
	}

	newTasks := make([]*Task, 0, len(plan.Tasks)+len(revisedTasks))
	for _, task := range plan.Tasks {
		if task == nil {
			continue
		}
		if task.Status == TaskStatusComplete || task.Status == TaskStatusSkipped || task.Status == TaskStatusPending {
			newTasks = append(newTasks, task)
		}
	}
	newTasks = append(newTasks, revisedTasks...)
	plan.Tasks = newTasks
	plan.ExecutionOrder = planner.determineExecutionOrder(plan.Tasks)

	conf := failureAnalysis.Confidence
	if currentConf, ok := plan.Metadata["confidence"]; ok {
		fmt.Sscanf(currentConf, "%f", &conf)
	} else if plan.Analysis != nil && plan.Analysis.Confidence > 0 {
		conf = plan.Analysis.Confidence
	}
	conf *= 0.8
	plan.Metadata["confidence"] = fmt.Sprintf("%.2f", conf)
	plan.Metadata["revision_reason"] = failureAnalysis.RootCause
	plan.Metadata["failure_confidence"] = fmt.Sprintf("%.2f", failureAnalysis.Confidence)
	plan.Metadata["revision_time"] = time.Now().Format(time.RFC3339)
	plan.UpdatedAt = time.Now()

	e.addThought(PhasePlanning, "Strategy revised", "retry_count", fmt.Sprintf("%d", retryCount))
	return plan, nil
}

// analyzeFailureConfidence estimates confidence in understanding the failure
func (e *Engine) analyzeFailureConfidence(task *Task, failureAnalysis string) float64 {
	// Simple heuristic: longer, more detailed errors = higher confidence
	confidence := 0.5

	analysis := strings.ToLower(failureAnalysis)

	// Known error patterns increase confidence
	knownPatterns := []string{
		"syntax error", "undefined reference", "import error",
		"compilation failed", "type mismatch", "missing dependency",
	}

	for _, pattern := range knownPatterns {
		if strings.Contains(analysis, pattern) {
			confidence += 0.2
			break
		}
	}

	// Generic errors reduce confidence
	if strings.Contains(analysis, "unknown") || strings.Contains(analysis, "unexpected") {
		confidence -= 0.2
	}

	// Normalize to 0-1 range
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}

// isNonFixable determines if a task failure is non-recoverable
func (e *Engine) isNonFixable(task *Task, failureAnalysis string) bool {
	analysis := strings.ToLower(failureAnalysis)

	// Permission errors typically can't be fixed automatically
	if strings.Contains(analysis, "permission denied") {
		return true
	}

	// Missing external resources
	if strings.Contains(analysis, "network unreachable") ||
		strings.Contains(analysis, "connection refused") {
		return true
	}

	// Exceeded retry attempts in task itself
	if task.Result != nil && task.Result.Attempts >= 3 {
		return true
	}

	return false
}

// createDiagnosticTask creates a task to investigate the failure
func (e *Engine) createDiagnosticTask(failedTask *Task, failureAnalysis string) *Task {
	return &Task{
		ID:           generateTaskID(),
		Description:  fmt.Sprintf("Diagnose failure in: %s", failedTask.Description),
		Type:         TaskTypeInvestigation,
		Dependencies: []string{}, // No dependencies
		Parallel:     false,
		Priority:     failedTask.Priority + 10, // Higher priority than original
		Estimated:    5 * time.Minute,
		Tools:        []string{"grep", "read", "bash"},
		Context: map[string]string{
			"phase":          "diagnostic",
			"failed_task_id": failedTask.ID,
			"failure_reason": failureAnalysis,
		},
		Verification: []Checkpoint{
			{
				ID:          generateTaskID(),
				Description: "Root cause identified",
				Type:        CheckpointSemantic,
				Validator:   "diagnostic_check",
				Required:    true,
			},
		},
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
	}
}

// createRepairTaskFromLearning creates a repair task based on learned patterns
func (e *Engine) createRepairTaskFromLearning(ctx context.Context, failedTask *Task, failureAnalysis *FailureAnalysis) *Task {
	_ = ctx
	if failureAnalysis == nil || failureAnalysis.Confidence < 0.7 || e.learningSystem == nil {
		return nil
	}

	rootCause := strings.ToLower(failureAnalysis.RootCause)
	var resolution string
	var matchedPattern *learning.Pattern
	for _, pattern := range e.learningSystem.GetPatterns(learning.PatternTypeError) {
		if pattern == nil {
			continue
		}
		for _, errorType := range pattern.Data.ErrorTypes {
			if strings.Contains(rootCause, strings.ToLower(errorType)) {
				matchedPattern = pattern
				resolution = pattern.Data.ResolutionPath
				break
			}
		}
		if matchedPattern != nil {
			break
		}
	}
	if matchedPattern == nil {
		for _, pattern := range e.learningSystem.GetPatterns(learning.PatternTypeError) {
			if pattern != nil && pattern.Data.ResolutionPath != "" && pattern.Confidence > 0.7 {
				matchedPattern = pattern
				resolution = pattern.Data.ResolutionPath
				break
			}
		}
	}
	if matchedPattern == nil {
		return nil
	}

	tools := []string{"edit", "bash"}
	switch failedTask.Type {
	case TaskTypeFeature:
		tools = []string{"create", "edit", "bash"}
	case TaskTypeBugFix, TaskTypeRefactor:
		tools = []string{"edit", "bash"}
	}

	description := fmt.Sprintf("Apply learned fix pattern for: %s", failedTask.Description)
	if resolution != "" {
		description = fmt.Sprintf("Apply learned resolution: %s", resolution)
	}

	context := map[string]string{
		"phase":          "repair",
		"failed_task_id": failedTask.ID,
		"learned_fix":    "true",
		"fix_confidence": fmt.Sprintf("%.2f", failureAnalysis.Confidence),
		"root_cause":     failureAnalysis.RootCause,
	}
	if matchedPattern != nil {
		context["learning_pattern"] = matchedPattern.Name
	}
	if resolution != "" {
		context["resolution_path"] = resolution
	}

	return &Task{
		ID:           generateTaskID(),
		Description:  description,
		Type:         failedTask.Type,
		Dependencies: []string{},
		Parallel:     false,
		Priority:     failedTask.Priority + 5,
		Estimated:    10 * time.Minute,
		Tools:        tools,
		Context:      context,
		Verification: []Checkpoint{
			{
				ID:          generateTaskID(),
				Description: "Fix applied successfully",
				Type:        CheckpointSemantic,
				Validator:   "fix_check",
				Required:    true,
			},
		},
		Status:    TaskStatusPending,
		CreatedAt: time.Now(),
	}
}

func newConcreteStrategy(taskType TaskType, strategy Strategy) *ConcreteStrategy {
	concrete := &ConcreteStrategy{
		StrategyName:    strategy.Name(),
		ConfidenceScore: strategy.Confidence(),
		Handler:         strategy,
	}
	if concrete.StrategyName == "" {
		concrete.StrategyName = string(taskType)
	}
	return concrete
}

func mustCreateLearningStorage() learning.Storage {
	storage, err := learning.NewFileStorage(filepath.Join(".automergent", "learning"))
	if err != nil {
		return nil
	}
	return storage
}

func (e *Engine) refineStrategy(ctx context.Context, plan *ExecutionPlan, verificationResult *verification.Result, executionDuration time.Duration) error {
	if plan == nil || plan.Analysis == nil {
		return nil
	}

	e.mu.Lock()
	strategy, ok := e.strategies[plan.Analysis.TaskType]
	if !ok || strategy == nil {
		e.mu.Unlock()
		return nil
	}

	concrete, ok := strategy.(*ConcreteStrategy)
	if !ok || concrete == nil {
		e.mu.Unlock()
		return nil
	}

	successRate := 1.0
	if verificationResult != nil {
		successRate = verificationResult.SuccessRate
	}

	concrete.TimesUsed++
	if successRate >= 0.5 {
		concrete.TotalSuccesses++
	}
	if concrete.TimesUsed == 1 {
		concrete.AvgExecutionTime = executionDuration
	} else {
		totalDuration := concrete.AvgExecutionTime*time.Duration(concrete.TimesUsed-1) + executionDuration
		concrete.AvgExecutionTime = totalDuration / time.Duration(concrete.TimesUsed)
	}

	if successRate > 0.95 {
		concrete.ConfidenceScore += 0.02
	} else if successRate < 0.70 {
		concrete.ConfidenceScore -= 0.05
	}

	if concrete.ConfidenceScore < 0.5 {
		concrete.ConfidenceScore = 0.5
	}
	if concrete.ConfidenceScore > 1.0 {
		concrete.ConfidenceScore = 1.0
	}

	historicalSuccessRate := float64(concrete.TotalSuccesses) / float64(concrete.TimesUsed)
	concrete.NeedsReview = concrete.TimesUsed >= 10 && historicalSuccessRate < 0.6
	snapshot := *concrete
	e.mu.Unlock()

	if e.learningStorage != nil {
		_ = e.learningStorage.SaveStrategy(ctx, learning.Strategy{
			ID:          string(plan.Analysis.TaskType),
			Name:        snapshot.StrategyName,
			Description: fmt.Sprintf("Adaptive reasoning strategy for %s", plan.Analysis.TaskType),
			ProjectType: string(plan.Analysis.TaskType),
			SuccessRate: historicalSuccessRate,
			AvgDuration: snapshot.AvgExecutionTime,
			UseCount:    snapshot.TimesUsed,
			LastUsed:    time.Now(),
			Configuration: map[string]interface{}{
				"times_used":         snapshot.TimesUsed,
				"total_successes":    snapshot.TotalSuccesses,
				"avg_execution_time": snapshot.AvgExecutionTime.String(),
				"needs_review":       snapshot.NeedsReview,
				"confidence":         snapshot.ConfidenceScore,
				"verification_rate":  successRate,
			},
		})
	}

	return nil
}

// hasLearningEngine checks if learning system is available
func (e *Engine) hasLearningEngine() bool {
	// In full implementation, check if learning engine is configured
	// For now, return true to enable the feature
	return true
}

// generateTaskID creates a unique task identifier
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func (e *Engine) addThought(phase Phase, thought, action, result string) {
	if e.trace == nil {
		e.trace = &ReasoningTrace{StartedAt: time.Now()}
	}
	step := ReasoningStep{
		Phase:     phase,
		Thought:   thought,
		Action:    action,
		Result:    result,
		Timestamp: time.Now(),
	}
	e.trace.Steps = append(e.trace.Steps, step)
}

func (e *Engine) registerDefaultStrategies() {
	if e.strategies == nil {
		e.strategies = make(map[TaskType]Strategy)
	}

	strategies := []*defaultStrategy{
		{
			name:     "Investigation",
			taskType: TaskTypeInvestigation,
			steps: []string{
				"grep the codebase for relevant symbols and patterns",
				"read the matching files and surrounding context",
				"analyze dependencies, data flow, and behavior",
				"synthesize findings into a concise explanation",
			},
			recommendedTools:  []string{"grep", "glob", "view"},
			confidence:        0.92,
			estimatedDuration: 15 * time.Minute,
		},
		{
			name:     "Bug Fix",
			taskType: TaskTypeBugFix,
			steps: []string{
				"reproduce the bug in a controlled environment",
				"diagnose the root cause from logs, code, and tests",
				"apply the smallest safe fix",
				"test the fix and add a regression check",
			},
			recommendedTools:  []string{"bash", "view", "edit"},
			confidence:        0.91,
			estimatedDuration: 30 * time.Minute,
		},
		{
			name:     "Feature",
			taskType: TaskTypeFeature,
			steps: []string{
				"write or update tests that define the desired behavior",
				"implement the feature to satisfy the tests",
				"refactor for clarity and maintainability",
				"update documentation and usage examples",
			},
			recommendedTools:  []string{"create", "edit", "bash"},
			confidence:        0.94,
			estimatedDuration: 45 * time.Minute,
		},
		{
			name:     "Refactor",
			taskType: TaskTypeRefactor,
			steps: []string{
				"preserve existing tests and establish a baseline",
				"make incremental changes without changing behavior",
				"verify after each change and before proceeding",
				"confirm the final result with the full test suite",
			},
			recommendedTools:  []string{"bash", "edit", "view"},
			confidence:        0.90,
			estimatedDuration: 40 * time.Minute,
		},
		{
			name:     "Test",
			taskType: TaskTypeTest,
			steps: []string{
				"measure coverage and identify missing cases",
				"write focused tests for the uncovered behavior",
				"verify the new tests and the existing suite",
			},
			recommendedTools:  []string{"bash", "create", "edit"},
			confidence:        0.93,
			estimatedDuration: 25 * time.Minute,
		},
		{
			name:     "Documentation",
			taskType: TaskTypeDocumentation,
			steps: []string{
				"understand the code, workflow, and audience",
				"add examples that show common usage patterns",
				"write API documentation for contracts and inputs",
			},
			recommendedTools:  []string{"view", "create", "edit"},
			confidence:        0.95,
			estimatedDuration: 20 * time.Minute,
		},
		{
			name:     "Build/Config",
			taskType: TaskTypeBuild,
			steps: []string{
				"read configuration, manifests, and build files",
				"validate settings and toolchain assumptions",
				"run a build or config check to confirm correctness",
				"test the resulting setup end to end",
			},
			recommendedTools:  []string{"view", "bash", "edit"},
			confidence:        0.89,
			estimatedDuration: 35 * time.Minute,
		},
		{
			name:     "Deployment",
			taskType: TaskTypeDeployment,
			steps: []string{
				"run pre-deployment checks and validation gates",
				"perform a staged rollout or dry run first",
				"monitor post-deployment signals and health checks",
			},
			recommendedTools:  []string{"bash", "view", "edit"},
			confidence:        0.87,
			estimatedDuration: 40 * time.Minute,
		},
	}

	for _, s := range strategies {
		e.strategies[s.taskType] = newConcreteStrategy(s.taskType, s)
	}
}

// defaultStrategy is a concrete implementation of the Strategy interface.
type defaultStrategy struct {
	name              string
	taskType          TaskType
	steps             []string
	recommendedTools  []string
	confidence        float64
	estimatedDuration time.Duration
}

func (s *defaultStrategy) Name() string {
	return s.name
}

func (s *defaultStrategy) CanHandle(taskType TaskType) bool {
	return s.taskType == taskType
}

func (s *defaultStrategy) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	tasks := make([]*Task, 0, len(s.steps))

	for i, step := range s.steps {
		task := &Task{
			ID:          fmt.Sprintf("%s-step-%d", s.taskType, i+1),
			Description: step,
			Type:        s.taskType,
			Priority:    len(s.steps) - i, // Earlier steps have higher priority
			Estimated:   s.estimatedDuration / time.Duration(len(s.steps)),
			Tools:       s.recommendedTools,
			Context:     make(map[string]string),
			Status:      TaskStatusPending,
			CreatedAt:   time.Now(),
		}

		// Add dependencies for sequential steps
		if i > 0 {
			task.Dependencies = []string{tasks[i-1].ID}
		}

		// Mark parallel-safe tasks
		if s.taskType == TaskTypeInvestigation && i < 2 {
			task.Parallel = true
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (s *defaultStrategy) EstimateEffort(analysis *TaskAnalysis) time.Duration {
	// Adjust estimate based on complexity
	baseTime := s.estimatedDuration

	switch analysis.Complexity {
	case ComplexityTrivial:
		return baseTime / 4
	case ComplexitySimple:
		return baseTime / 2
	case ComplexityModerate:
		return baseTime
	case ComplexityComplex:
		return baseTime * 2
	case ComplexityMajor:
		return baseTime * 4
	default:
		return baseTime
	}
}

func (s *defaultStrategy) Confidence() float64 {
	return clampConfidence(s.confidence)
}

func (s *ConcreteStrategy) CanHandle(taskType TaskType) bool {
	if s == nil {
		return false
	}
	if s.Handler != nil {
		return s.Handler.CanHandle(taskType)
	}
	return true
}

func (s *ConcreteStrategy) Name() string {
	if s == nil {
		return ""
	}
	if s.StrategyName != "" {
		return s.StrategyName
	}
	if s.Handler != nil {
		return s.Handler.Name()
	}
	return ""
}

func (s *ConcreteStrategy) Decompose(ctx context.Context, analysis *TaskAnalysis) ([]*Task, error) {
	if s == nil || s.Handler == nil {
		return nil, fmt.Errorf("strategy handler unavailable")
	}
	return s.Handler.Decompose(ctx, analysis)
}

func (s *ConcreteStrategy) EstimateEffort(analysis *TaskAnalysis) time.Duration {
	if s == nil {
		return 0
	}
	if s.Handler != nil {
		return s.Handler.EstimateEffort(analysis)
	}
	return s.AvgExecutionTime
}

func (s *ConcreteStrategy) Confidence() float64 {
	if s == nil {
		return 0
	}
	return clampConfidence(s.ConfidenceScore)
}

// Helper functions

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func upgradeComplexity(c Complexity) Complexity {
	switch c {
	case ComplexityTrivial:
		return ComplexitySimple
	case ComplexitySimple:
		return ComplexityModerate
	case ComplexityModerate:
		return ComplexityComplex
	case ComplexityComplex:
		return ComplexityMajor
	default:
		return c
	}
}

func downgradeComplexity(c Complexity) Complexity {
	switch c {
	case ComplexityMajor:
		return ComplexityComplex
	case ComplexityComplex:
		return ComplexityModerate
	case ComplexityModerate:
		return ComplexitySimple
	case ComplexitySimple:
		return ComplexityTrivial
	default:
		return c
	}
}

func extractIntent(request string) string {
	// Simple intent extraction - first sentence or up to 100 chars
	sentences := strings.Split(request, ".")
	if len(sentences) > 0 && len(sentences[0]) < 100 {
		return strings.TrimSpace(sentences[0])
	}
	if len(request) <= 100 {
		return request
	}
	return strings.TrimSpace(request[:100])
}

func convertPlan(plan *planningPkg.Plan, analysis *TaskAnalysis) *ExecutionPlan {
	execPlan := &ExecutionPlan{
		ID:        plan.ID,
		Analysis:  analysis,
		Tasks:     make([]*Task, 0, len(plan.Steps)),
		Metadata:  map[string]string{},
		CreatedAt: plan.CreatedAt,
		UpdatedAt: plan.UpdatedAt,
	}
	for _, step := range plan.Steps {
		task := &Task{
			ID:           step.ID,
			Description:  step.Description,
			Type:         mapRequestType(step.Title, analysis.TaskType),
			Dependencies: append([]string(nil), step.DependsOn...),
			Parallel:     step.Parallel,
			Priority:     step.Priority,
			Estimated:    step.Estimated,
			Tools:        []string{"planning", "verification"},
			Context:      map[string]string{"title": step.Title},
			Verification: convertCheckpoints(step.Verification),
			Status:       TaskStatusPending,
			CreatedAt:    plan.CreatedAt,
		}
		execPlan.Tasks = append(execPlan.Tasks, task)
	}

	execPlan.ExecutionOrder = make([][]string, 0, len(plan.ExecutionOrder))
	for _, group := range plan.ExecutionOrder {
		if len(group) == 0 {
			continue
		}
		execPlan.ExecutionOrder = append(execPlan.ExecutionOrder, append([]string(nil), group...))
	}
	if len(execPlan.ExecutionOrder) == 0 && len(execPlan.Tasks) > 0 {
		order := make([]string, 0, len(execPlan.Tasks))
		for _, task := range execPlan.Tasks {
			order = append(order, task.ID)
		}
		execPlan.ExecutionOrder = [][]string{order}
	}

	for _, task := range execPlan.Tasks {
		for _, v := range task.Verification {
			execPlan.Checkpoints = append(execPlan.Checkpoints, v)
		}
	}
	if len(execPlan.Checkpoints) == 0 && analysis != nil {
		execPlan.Checkpoints = append(execPlan.Checkpoints, Checkpoint{
			ID:          "default-verification",
			Description: "plan executed successfully",
			Type:        CheckpointSemantic,
			Validator:   "verification-engine",
			Required:    true,
		})
	}
	execPlan.Metadata["planning_summary"] = plan.Summary()
	execPlan.Metadata["step_count"] = fmt.Sprintf("%d", len(execPlan.Tasks))
	return execPlan
}

func convertCheckpoints(values []string) []Checkpoint {
	checkpoints := make([]Checkpoint, 0, len(values))
	for i, v := range values {
		checkpoints = append(checkpoints, Checkpoint{
			ID:          fmt.Sprintf("checkpoint-%d", i+1),
			Description: v,
			Type:        CheckpointSemantic,
			Validator:   "planning-check",
			Required:    true,
		})
	}
	return checkpoints
}

func mapRequestType(title string, fallback TaskType) TaskType {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "verify"), strings.Contains(lower, "validate"):
		return TaskTypeTest
	case strings.Contains(lower, "inspect"), strings.Contains(lower, "investigate"), strings.Contains(lower, "research"):
		return TaskTypeInvestigation
	case strings.Contains(lower, "document"):
		return TaskTypeDocumentation
	case fallback != "":
		return fallback
	default:
		return TaskTypeFeature
	}
}

func collectTaskFiles(plan *ExecutionPlan) []string {
	files := make([]string, 0)
	if plan == nil {
		return files
	}
	for _, task := range plan.Tasks {
		if task == nil {
			continue
		}
		for _, f := range task.Context {
			if f != "" {
				files = append(files, f)
			}
		}
	}
	return files
}

func collectChangedFiles(plan *ExecutionPlan) []string {
	if plan == nil {
		return nil
	}
	changed := make([]string, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		if task == nil {
			continue
		}
		changed = append(changed, task.Dependencies...)
	}
	return changed
}
