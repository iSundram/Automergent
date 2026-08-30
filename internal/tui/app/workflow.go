package app

// Workflow wiring: discovers YAML pipeline specs under
// .automergent/workflows/, executes them on the engine, and streams step
// progress back into the conversation.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	"github.com/iSundram/Automergent/internal/tui/commands"
	"github.com/iSundram/Automergent/internal/workflow"
)

// workflowRunsDir holds journals and run state under the project.
func (a *App) workflowRunsDir() string {
	return filepath.Join(a.workDir, ".automergent", "workflow-runs")
}

// workflowSpecsDir is where user-authored pipeline specs live.
func (a *App) workflowSpecsDir() string {
	return filepath.Join(a.workDir, ".automergent", "workflows")
}

// WorkflowSpecs lists available pipeline specs (backs /workflow list).
func (a *App) WorkflowSpecs() []commands.WorkflowSpecInfo {
	entries, err := os.ReadDir(a.workflowSpecsDir())
	if err != nil {
		return nil
	}
	var out []commands.WorkflowSpecInfo
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(a.workflowSpecsDir(), e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		spec, err := workflow.ParseSpec(data)
		if err != nil {
			out = append(out, commands.WorkflowSpecInfo{
				Name:        strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
				Description: "invalid spec: " + err.Error(),
				Path:        path,
			})
			continue
		}
		out = append(out, commands.WorkflowSpecInfo{Name: spec.Name, Description: spec.Description, Path: path})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// workflowRun records one run for /workflow history.
type workflowRun struct {
	runID  string
	name   string
	status string
	steps  int
	detail string
}

// appWorkflowPorts adapts the App to the engine's Ports: agent calls go
// through the agent executor, progress lands as system messages.
type appWorkflowPorts struct {
	a *App
}

func (p appWorkflowPorts) RunAgent(ctx context.Context, params workflow.AgentParams) (workflow.AgentResult, error) {
	agentType := toolsagent.AgentTypeGeneralPurpose
	if params.AgentType != "" {
		agentType = toolsagent.AgentType(params.AgentType)
	}
	out, err := p.a.ag.Execute(ctx, agentType, params.Prompt, params.Model)
	// Output tokens are unknown from the executor string; the journal
	// records what it can and the budget gate uses it as a floor.
	return workflow.AgentResult{Output: out}, err
}

func (p appWorkflowPorts) Journal() workflow.JournalStore {
	return workflow.NewFileJournalStore(p.a.workflowRunsDir())
}

func (p appWorkflowPorts) Progress(ev workflow.ProgressEvent) {
	var msg string
	switch ev.Status {
	case "started":
		msg = fmt.Sprintf("⚙ workflow %s: step %q started", ev.RunID, ev.Step)
	case "replayed":
		msg = fmt.Sprintf("↺ workflow %s: step %q replayed from journal", ev.RunID, ev.Step)
	case "done":
		msg = fmt.Sprintf("✓ workflow %s: step %q done", ev.RunID, ev.Step)
	case "failed":
		msg = fmt.Sprintf("✗ workflow %s: step %q failed", ev.RunID, ev.Step)
	default:
		return
	}
	if p.a.sendToProgram != nil {
		p.a.sendToProgram(workflowProgressMsg{text: msg})
	}
}

// workflowProgressMsg carries a workflow progress line to the main loop.
type workflowProgressMsg struct{ text string }

// RunWorkflow executes a pipeline spec (backs /workflow run|resume). The
// engine runs in a background goroutine; progress and the terminal result
// arrive as messages.
func (a *App) RunWorkflow(path string, args []string, resume bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	spec, err := workflow.ParseSpec(data)
	if err != nil {
		return err
	}

	runID := ""
	if resume {
		// The newest journal for this spec's name.
		runID = a.latestWorkflowRunID(spec.Name)
		if runID == "" {
			return fmt.Errorf("no previous run found for workflow %q", spec.Name)
		}
	}

	go func() {
		ports := appWorkflowPorts{a: a}
		var res workflow.RunResult
		if resume {
			res = workflow.Run(a.ctx, spec, strings.Join(args, " "), ports,
				workflow.WithRunID(runID), workflow.WithResume(true))
		} else {
			res = workflow.Run(a.ctx, spec, strings.Join(args, " "), ports)
		}

		outcome := fmt.Sprintf("workflow %s %s — %d steps, %d tokens",
			spec.Name, res.Status, len(spec.Steps), res.TotalOutputTokens)
		if res.Error != "" {
			outcome += "\nerror: " + res.Error
		}
		a.recordWorkflowRun(workflowRun{runID: res.RunID, name: spec.Name, status: res.Status, steps: len(spec.Steps), detail: res.Error})
		if a.sendToProgram != nil {
			a.sendToProgram(workflowProgressMsg{text: outcome, })
		}
	}()
	return nil
}

// recordWorkflowRun appends to the in-memory run history.
func (a *App) recordWorkflowRun(r workflowRun) {
	a.workflowRuns = append(a.workflowRuns, r)
	if len(a.workflowRuns) > 50 {
		a.workflowRuns = a.workflowRuns[len(a.workflowRuns)-50:]
	}
}

// latestWorkflowRunID scans the runs dir for the most recent journal
// belonging to the named workflow.
func (a *App) latestWorkflowRunID(name string) string {
	entries, err := os.ReadDir(a.workflowRunsDir())
	if err != nil {
		return ""
	}
	var candidates []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), name+"-") {
			candidates = append(candidates, e.Name())
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1]
}

// WorkflowRunHistory backs /workflow history.
func (a *App) WorkflowRunHistory() []commands.WorkflowRunInfo {
	out := make([]commands.WorkflowRunInfo, 0, len(a.workflowRuns))
	for i := len(a.workflowRuns) - 1; i >= 0; i-- {
		r := a.workflowRuns[i]
		out = append(out, commands.WorkflowRunInfo{RunID: r.runID, Name: r.name, Status: r.status, Steps: r.steps, Detail: r.detail})
	}
	return out
}
