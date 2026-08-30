package filesystem

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

// ArtifactTool writes a deliverable document (plan, review, design, summary)
// as an artifact: the file lands on disk exactly like write_file, but the
// call carries the metadata the review UI needs (title, kind) and can ask
// for user feedback before the agent continues.
type ArtifactTool struct {
	cfg *config.Config
}

// NewArtifactTool creates the artifact tool. Path validation mirrors
// write_file so artifacts obey the same write policy.
func NewArtifactTool(cfg *config.Config) *ArtifactTool {
	return &ArtifactTool{cfg: cfg}
}

func (t *ArtifactTool) Name() string { return "artifact" }
func (t *ArtifactTool) Description() string {
	return `Write a deliverable document (plan, review, design, summary, report) as a Markdown artifact file.
- Prefer .automergent/artifacts/<topic>.md unless the user asked for a specific path.
- Include a "# title" heading and a short lead paragraph stating what the document is.
- Set request_feedback=true when the user should review before you continue: write the artifact, stop calling tools, and reply with a one-paragraph summary.
- The user approves or rejects artifacts via /artifact; never assume approval.`
}
func (t *ArtifactTool) RequiresConfirmation(mode string) bool {
	return mode == "edit" || mode == "plan"
}
func (t *ArtifactTool) IsConcurrencySafe(args map[string]any) bool { return false }
func (t *ArtifactTool) IsReadOnly(args map[string]any) bool         { return false }
func (t *ArtifactTool) IsDestructive(args map[string]any) bool      { return false }

func (t *ArtifactTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 400, LatencyMs: 50, RiskLevel: "low"}
}

func (t *ArtifactTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "edit",
		Usage:      "Use for any deliverable the user asked to see as a document. The title should be a few words describing the deliverable; kind is one of plan, review, design, summary, document.",
		WhenToUse:  "The user asks for a plan, review, design, summary, or any written deliverable — or a phase prompt instructs an artifact.",
		WhenNotTo:  "Do not use for code files or config; write_file is the right tool there. Do not create artifacts nobody asked for.",
		InjectOrder: 30,
	}
}

func (t *ArtifactTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path for the artifact, e.g. .automergent/artifacts/plan.md.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Full Markdown content of the artifact, starting with a '# title' heading.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Short human title for the review list (a few words).",
			},
			"kind": map[string]any{
				"type":        "string",
				"description": "Artifact kind: plan | review | design | summary | document.",
			},
			"request_feedback": map[string]any{
				"type":        "boolean",
				"description": "True when the user should review this artifact before you continue (default false).",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *ArtifactTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return tools.Result{IsError: true, Content: "path is required"}, nil
	}
	content, ok := tools.StringArg(args, "content")
	if !ok {
		return tools.Result{IsError: true, Content: "content is required (string)"}, nil
	}
	title, _ := tools.StringArg(args, "title")
	kind, _ := tools.StringArg(args, "kind")
	requestFeedback, _ := tools.ArgBool(args, "request_feedback")

	if t.cfg != nil {
		if err := validateWritePath(path, t.cfg.Security.BlockedWritePaths, t.cfg.Security.AllowedWritePaths); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
	}

	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("write: %v", err)}, nil
	}

	lineCount := strings.Count(content, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		lineCount++
	}

	result := fmt.Sprintf("Artifact written: %s (%d lines)", path, lineCount)
	if requestFeedback {
		result += "\n\nUser review requested. Stop calling tools now and reply with a one-paragraph summary pointing at the artifact. The user will review it via /artifact; do not continue with follow-up work until they approve."
	}

	return tools.Result{
		Content: result,
		Summary: fmt.Sprintf("artifact %s (%d lines)", filepath.Base(path), lineCount),
		Metadata: map[string]any{
			"artifact":         true,
			"artifact_title":   title,
			"artifact_kind":    kind,
			"request_feedback": requestFeedback,
		},
	}, nil
}
