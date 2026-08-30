package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AgentResult is the outcome of one agent call.
type AgentResult struct {
	Output       string
	OutputTokens int
}

// Ports are the host-supplied capabilities the engine needs. Keeping them
// behind an interface keeps the engine free of any dependency on the agent
// stack, so it is testable with fakes.
type Ports interface {
	// RunAgent executes one agent call.
	RunAgent(ctx context.Context, params AgentParams) (AgentResult, error)
	// Journal persists run results for resume.
	Journal() JournalStore
	// Progress reports step-level state transitions.
	Progress(event ProgressEvent)
}

// ProgressEvent is one observable state transition in a run.
type ProgressEvent struct {
	RunID  string
	Step   string
	Status string // "started" | "replayed" | "done" | "failed" | "skipped"
	Detail string
}

// RunResult is the terminal outcome of a workflow run.
type RunResult struct {
	RunID  string
	Status string // "completed" | "failed" | "aborted"
	Error  string
	// Outputs maps step id → final output (completed runs only).
	Outputs map[string]string
	// TotalOutputTokens is the sum of live agent usage; replayed steps cost
	// nothing and add nothing.
	TotalOutputTokens int
}

// RunOption customizes a run's identity or resume mode.
type RunOption func(*runOptions)

type runOptions struct {
	runID  string
	resume bool
}

// WithRunID sets an explicit run identity (resume path).
func WithRunID(id string) RunOption { return func(o *runOptions) { o.runID = id } }

// WithResume replays the journal for the given run instead of starting cold.
func WithResume(resume bool) RunOption { return func(o *runOptions) { o.resume = resume } }

// Run executes the workflow: a step becomes runnable when every dependency
// has an output; runnable steps execute in parallel bounded by the spec's
// concurrency; each agent call is journaled, and on resume a journaled key
// is replayed instead of re-run.
func Run(ctx context.Context, spec *Spec, arguments string, ports Ports, opts ...RunOption) RunResult {
	ro := runOptions{runID: newRunID(spec.Name)}
	for _, opt := range opts {
		opt(&ro)
	}

	var replay map[string]JournalEntry
	if ro.resume {
		if entries, err := ports.Journal().Read(ro.runID); err == nil {
			replay = indexEntries(entries)
		}
	}

	e := &engineRun{
		spec:      spec,
		ports:     ports,
		runID:     ro.runID,
		ctx:       ctx,
		args:      arguments,
		outputs:   map[string]string{},
		replay:    replay,
		remaining: map[string]bool{},
	}
	for _, st := range spec.Steps {
		e.remaining[st.ID] = true
	}
	return e.execute()
}

func indexEntries(entries []JournalEntry) map[string]JournalEntry {
	out := make(map[string]JournalEntry, len(entries))
	for _, en := range entries {
		out[en.Key] = en
	}
	return out
}

func newRunID(name string) string {
	return fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
}

// engineRun is the mutable state of one execution.
type engineRun struct {
	spec    *Spec
	ports   Ports
	runID   string
	ctx     context.Context
	args    string
	outputs map[string]string
	replay  map[string]JournalEntry

	mu        sync.Mutex
	seq       int
	tokens    int
	failed    string
	remaining map[string]bool
}

// execute schedules waves of runnable steps until none remain or the run
// dies. Within a wave, steps run in parallel bounded by the concurrency cap.
func (e *engineRun) execute() RunResult {
	for {
		e.mu.Lock()
		if e.failed != "" {
			e.mu.Unlock()
			break
		}
		runnable := e.findRunnableLocked()
		e.mu.Unlock()

		if len(runnable) == 0 {
			e.mu.Lock()
			done := len(e.remaining) == 0
			e.mu.Unlock()
			if done {
				break
			}
			// Nothing runnable but steps remain: unreachable in a validated
			// DAG, defended anyway.
			e.fail("no runnable steps remain (internal scheduling error)")
			break
		}

		// Cap the wave at the concurrency limit; unlaunched steps become
		// runnable again on the next loop iteration.
		if len(runnable) > e.spec.Concurrency {
			runnable = runnable[:e.spec.Concurrency]
		}
		var wg sync.WaitGroup
		for _, st := range runnable {
			wg.Add(1)
			go func(st Step) {
				defer wg.Done()
				e.runStep(st)
			}(st)
		}
		wg.Wait()
	}

	res := RunResult{RunID: e.runID, TotalOutputTokens: e.tokens}
	if e.failed != "" {
		if e.ctx.Err() != nil {
			res.Status = "aborted"
		} else {
			res.Status = "failed"
		}
		res.Error = e.failed
		return res
	}
	res.Status = "completed"
	res.Outputs = e.outputs
	return res
}

func (e *engineRun) findRunnableLocked() []Step {
	var runnable []Step
	for _, st := range e.spec.Steps {
		if !e.remaining[st.ID] {
			continue
		}
		ready := true
		for _, dep := range st.DependsOn {
			if _, ok := e.outputs[dep]; !ok {
				ready = false
				break
			}
		}
		if ready {
			runnable = append(runnable, st)
		}
	}
	return runnable
}

func (e *engineRun) runStep(st Step) {
	e.ports.Progress(ProgressEvent{RunID: e.runID, Step: st.ID, Status: "started"})

	prompt := expandPrompt(st.Prompt, e.args, e.outputs)
	params := AgentParams{Prompt: prompt, AgentType: st.AgentType, Model: st.Model}
	key := agentCallKey(prompt, params)

	// Budget gate before spending.
	e.mu.Lock()
	over := e.spec.Budget > 0 && e.tokens >= e.spec.Budget
	e.mu.Unlock()
	if over {
		e.fail(fmt.Sprintf("token budget exhausted before step %q (%d/%d)", st.ID, e.tokens, e.spec.Budget))
		e.finishStep(st.ID, "skipped")
		return
	}

	// Replay from the journal when the key matches a prior successful
	// result — resume costs nothing for unchanged steps.
	if entry, ok := e.replay[key]; ok && entry.Error == "" {
		e.mu.Lock()
		e.outputs[st.ID] = entry.Output
		delete(e.remaining, st.ID)
		e.mu.Unlock()
		e.ports.Progress(ProgressEvent{RunID: e.runID, Step: st.ID, Status: "replayed"})
		return
	}

	result, err := e.ports.RunAgent(e.ctx, params)

	e.mu.Lock()
	e.seq++
	entry := JournalEntry{
		Key:          key,
		Seq:          e.seq,
		Step:         st.ID,
		Output:       result.Output,
		OutputTokens: result.OutputTokens,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	e.mu.Unlock()
	_ = e.ports.Journal().Append(e.runID, entry)

	if err != nil {
		e.fail(fmt.Sprintf("step %q failed: %v", st.ID, err))
		e.finishStep(st.ID, "failed")
		return
	}

	e.mu.Lock()
	e.outputs[st.ID] = result.Output
	e.tokens += result.OutputTokens
	delete(e.remaining, st.ID)
	e.mu.Unlock()
	e.ports.Progress(ProgressEvent{RunID: e.runID, Step: st.ID, Status: "done"})
}

func (e *engineRun) finishStep(id, status string) {
	e.mu.Lock()
	delete(e.remaining, id)
	e.mu.Unlock()
	e.ports.Progress(ProgressEvent{RunID: e.runID, Step: id, Status: status})
}

func (e *engineRun) fail(msg string) {
	e.mu.Lock()
	if e.failed == "" {
		e.failed = msg
	}
	e.mu.Unlock()
}
