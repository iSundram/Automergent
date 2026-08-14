package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExecutionStrategy is the scheduling lane selected for a call.
type ExecutionStrategy string

const (
	ExecutionStrategySequential ExecutionStrategy = "sequential"
	ExecutionStrategyParallel   ExecutionStrategy = "parallel"
)

// ReasonCode explains why a specific scheduling decision was made.
type ReasonCode string

const (
	ReasonCodeParallelSafe          ReasonCode = "parallel_safe"
	ReasonCodeRequiresConfirmation  ReasonCode = "requires_confirmation"
	ReasonCodeNotReadOnly           ReasonCode = "not_read_only"
	ReasonCodeNotConcurrencySafe    ReasonCode = "not_concurrency_safe"
	ReasonCodeHighRisk              ReasonCode = "high_risk"
	ReasonCodeUnknownRisk           ReasonCode = "unknown_risk"
	ReasonCodeDeterministicFallback ReasonCode = "deterministic_fallback"
	ReasonCodeUnknownTool           ReasonCode = "unknown_tool"
)

// OrchestrationCall is the normalized shape used by the orchestration kernel.
type OrchestrationCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ExecutionRequest is the orchestration kernel input.
type ExecutionRequest struct {
	Calls            []OrchestrationCall
	Mode             string
	MaxParallelBatch int
}

// ExecutionRecord captures scheduling and runtime output for one call.
type ExecutionRecord struct {
	Call       OrchestrationCall
	BatchIndex int
	Strategy   ExecutionStrategy
	Reasons    []ReasonCode
	Fallback   ExecutionStrategy

	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Result     Result
	Error      string
}

// ExecutionResponse is the orchestration kernel output.
type ExecutionResponse struct {
	Records []ExecutionRecord
}

type toolLookup func(name string) (Tool, bool)
type toolExecutor func(ctx context.Context, call OrchestrationCall) (Result, error)

// Orchestrator is the unified tool orchestration entrypoint.
type Orchestrator struct {
	lookup  toolLookup
	execute toolExecutor
	now     func() time.Time
}

// NewOrchestrator creates a composable orchestration kernel.
func NewOrchestrator(lookup toolLookup, execute toolExecutor) *Orchestrator {
	return &Orchestrator{
		lookup:  lookup,
		execute: execute,
		now:     time.Now,
	}
}

type plannedCall struct {
	index    int
	call     OrchestrationCall
	strategy ExecutionStrategy
	reasons  []ReasonCode
	fallback ExecutionStrategy
}

type plannedBatch struct {
	index    int
	parallel bool
	calls    []plannedCall
}

// Execute runs the orchestration request and returns structured execution records.
func (o *Orchestrator) Execute(ctx context.Context, req ExecutionRequest) ExecutionResponse {
	if len(req.Calls) == 0 {
		return ExecutionResponse{}
	}

	batches := o.planBatches(req)
	records := make([]ExecutionRecord, len(req.Calls))

	for _, batch := range batches {
		if !batch.parallel {
			for _, call := range batch.calls {
				records[call.index] = o.executeCall(ctx, batch.index, call)
			}
			continue
		}

		var wg sync.WaitGroup
		wg.Add(len(batch.calls))

		for _, call := range batch.calls {
			call := call
			go func() {
				defer wg.Done()
				records[call.index] = o.executeCall(ctx, batch.index, call)
			}()
		}

		wg.Wait()
	}

	return ExecutionResponse{Records: records}
}

func (o *Orchestrator) planBatches(req ExecutionRequest) []plannedBatch {
	maxParallel := req.MaxParallelBatch
	if maxParallel <= 0 {
		maxParallel = 10
	}

	batches := make([]plannedBatch, 0, len(req.Calls))
	current := plannedBatch{parallel: true}
	nextBatchIndex := 0

	flush := func() {
		if len(current.calls) == 0 {
			return
		}
		current.index = nextBatchIndex
		nextBatchIndex++
		batches = append(batches, current)
		current = plannedBatch{parallel: true}
	}

	for i, call := range req.Calls {
		strategy, reasons, fallback := o.classifyCall(req.Mode, call)
		planned := plannedCall{
			index:    i,
			call:     call,
			strategy: strategy,
			reasons:  reasons,
			fallback: fallback,
		}

		if strategy == ExecutionStrategySequential {
			flush()
			batches = append(batches, plannedBatch{
				index:    nextBatchIndex,
				parallel: false,
				calls:    []plannedCall{planned},
			})
			nextBatchIndex++
			continue
		}

		if !current.parallel || len(current.calls) == maxParallel {
			flush()
		}
		current.parallel = true
		current.calls = append(current.calls, planned)
	}

	flush()
	return batches
}

func (o *Orchestrator) classifyCall(mode string, call OrchestrationCall) (ExecutionStrategy, []ReasonCode, ExecutionStrategy) {
	if o.lookup == nil {
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeUnknownTool, ReasonCodeDeterministicFallback}, ExecutionStrategySequential
	}

	tool, ok := o.lookup(call.Name)
	if !ok {
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeUnknownTool, ReasonCodeDeterministicFallback}, ExecutionStrategySequential
	}

	if tool.RequiresConfirmation(mode) {
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeRequiresConfirmation}, ""
	}
	if !tool.IsReadOnly(call.Args) {
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeNotReadOnly}, ""
	}
	if !tool.IsConcurrencySafe(call.Args) {
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeNotConcurrencySafe}, ""
	}

	switch strings.ToLower(strings.TrimSpace(tool.EstimatedCost().RiskLevel)) {
	case "", "low", "medium":
		return ExecutionStrategyParallel, []ReasonCode{ReasonCodeParallelSafe}, ""
	case "high":
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeHighRisk}, ""
	default:
		return ExecutionStrategySequential, []ReasonCode{ReasonCodeUnknownRisk, ReasonCodeDeterministicFallback}, ExecutionStrategySequential
	}
}

func (o *Orchestrator) executeCall(ctx context.Context, batchIndex int, call plannedCall) ExecutionRecord {
	startedAt := o.now()
	result, err := o.run(ctx, call.call)
	finishedAt := o.now()

	record := ExecutionRecord{
		Call:       call.call,
		BatchIndex: batchIndex,
		Strategy:   call.strategy,
		Reasons:    call.reasons,
		Fallback:   call.fallback,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Duration:   finishedAt.Sub(startedAt),
		Result:     result,
	}

	if err != nil {
		record.Error = err.Error()
		if !record.Result.IsError {
			record.Result.IsError = true
		}
		if record.Result.Content == "" {
			record.Result.Content = err.Error()
		}
	}

	return record
}

func (o *Orchestrator) run(ctx context.Context, call OrchestrationCall) (Result, error) {
	if o.execute == nil {
		return Result{}, fmt.Errorf("orchestrator executor is nil")
	}
	return o.execute(ctx, call)
}
