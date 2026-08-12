package planning

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/iSundram/Automergent/internal/tools"
)

// Tool exposes the planner to the agent workflow.
type Tool struct {
	planner *Planner
}

// NewTool creates a planning tool rooted at the current directory.
func NewTool(rootDir string) *Tool {
	return &Tool{planner: NewPlanner(rootDir)}
}

func (t *Tool) Name() string { return "plan" }

func (t *Tool) Description() string {
	return "Analyze a request, discover relevant files, and generate a dependency-aware execution plan."
}

func (t *Tool) RequiresConfirmation(mode string) bool { return false }

func (t *Tool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request":         map[string]any{"type": "string", "description": "User request to plan for."},
			"context_signals": map[string]any{"type": "string", "description": "Optional JSON-encoded context signals from the context layer."},
		},
		"required": []string{"request"},
	}
}

func (t *Tool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	request, ok := tools.StringArg(args, "request")
	if !ok || request == "" {
		return tools.Result{IsError: true, Content: "request is required"}, nil
	}
	analysis := t.planner.AnalyzeRequest(request)
	if rawSignals, ok := tools.StringArg(args, "context_signals"); ok && rawSignals != "" {
		_ = json.Unmarshal([]byte(rawSignals), &analysis.ContextSignals)
	}
	plan, err := t.planner.GeneratePlanWithAnalysis(ctx, analysis)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("planning failed: %v", err)}, nil
	}
	return tools.Result{
		Content:  plan.Summary(),
		Summary:  fmt.Sprintf("generated %d steps", len(plan.Steps)),
		Metadata: map[string]any{"plan": plan},
	}, nil
}

// ReplanTool updates an existing plan based on feedback.
type ReplanTool struct {
	planner *Planner
}

func NewReplanTool(rootDir string) *ReplanTool {
	return &ReplanTool{planner: NewPlanner(rootDir)}
}

func (t *ReplanTool) Name() string { return "replan" }
func (t *ReplanTool) Description() string {
	return "Revise an existing plan using feedback or new information."
}
func (t *ReplanTool) RequiresConfirmation(mode string) bool { return false }

func (t *ReplanTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"feedback":     map[string]any{"type": "string", "description": "What changed or what needs to be revised."},
			"current_plan": map[string]any{"type": "string", "description": "Current plan summary or notes."},
		},
		"required": []string{"feedback"},
	}
}

func (t *ReplanTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	feedback, ok := tools.StringArg(args, "feedback")
	if !ok || feedback == "" {
		return tools.Result{IsError: true, Content: "feedback is required"}, nil
	}
	current, _ := tools.StringArg(args, "current_plan")
	existing := &Plan{
		Analysis:    RequestAnalysis{RawRequest: current, Intent: current},
		ReplanCount: 0,
	}
	plan, err := t.planner.Replan(ctx, existing, feedback)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("replanning failed: %v", err)}, nil
	}
	return tools.Result{
		Content:  plan.Summary(),
		Summary:  fmt.Sprintf("replanned to %d steps", len(plan.Steps)),
		Metadata: map[string]any{"plan": plan},
	}, nil
}
