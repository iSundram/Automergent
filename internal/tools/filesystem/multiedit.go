package filesystem

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

// MultiEditTool applies several exact string replacements to one file in a
// single call. Sequential application means later edits see earlier ones'
// output — order the edits accordingly.
type MultiEditTool struct {
	cfg *config.Config
}

func NewMultiEditTool(cfg *config.Config) *MultiEditTool {
	return &MultiEditTool{cfg: cfg}
}

func (*MultiEditTool) Name() string { return "multi_edit" }
func (*MultiEditTool) Description() string {
	return "Apply multiple exact replacements to one file in a single call."
}
func (*MultiEditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "File to edit"},
			"edits": map[string]any{
				"type":        "array",
				"description": "Applied in order; each old_str must match exactly and uniquely at apply time.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old_str":     map[string]any{"type": "string"},
						"new_str":     map[string]any{"type": "string"},
						"replace_all": map[string]any{"type": "boolean"},
					},
					"required": []string{"old_str", "new_str"},
				},
			},
		},
		"required": []string{"path", "edits"},
	}
}
func (*MultiEditTool) RequiresConfirmation(mode string) bool { return mode == "edit" || mode == "plan" }
func (*MultiEditTool) IsConcurrencySafe(map[string]any) bool { return false }
func (*MultiEditTool) IsReadOnly(map[string]any) bool        { return false }
func (*MultiEditTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 150, LatencyMs: 60, RiskLevel: "medium"}
}

func (t *MultiEditTool) IsDestructive(args map[string]any) bool {
	raw, ok := args["edits"].([]any)
	if !ok {
		return false
	}
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if old, ok := tools.StringArg(m, "old_str"); ok && strings.Count(old, "\n") > 10 {
			return true // removing substantial blocks, same rule as edit_file
		}
	}
	return false
}

func (*MultiEditTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "edit",
		DisplayName: "Multi edit",
		InjectOrder: 15,
		WhenToUse:   "Two or more changes in the SAME file. One call replaces a noisy sequence of edit_file calls and keeps every edit atomic together.",
		WhenNotTo:   "Single change → `edit_file`. Changes across different files → separate calls so each result is attributable.",
		UsageByFamily: map[string]string{
			"gemini3": "Gemini 3: batch same-file fixes here in one turn instead of chaining single edits across turns.",
		},
	}
}

func (t *MultiEditTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	if path == "" {
		return tools.Result{IsError: true, Content: "multi_edit requires `path`"}, nil
	}

	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("multi_edit: %v", err)}, nil
	}
	content := string(contentBytes)

	raw, ok := args["edits"].([]any)
	if !ok || len(raw) == 0 {
		return tools.Result{IsError: true, Content: "multi_edit requires a non-empty `edits` array"}, nil
	}

	applied := 0
	for i, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			return tools.Result{IsError: true, Content: fmt.Sprintf("edit #%d is not an object", i+1)}, nil
		}
		oldStr, _ := tools.StringArg(m, "old_str")
		newStr, _ := tools.StringArg(m, "new_str")
		replaceAll, _ := m["replace_all"].(bool)

		if oldStr == "" {
			return tools.Result{IsError: true, Content: fmt.Sprintf("edit #%d: old_str must not be empty", i+1)}, nil
		}
		count := strings.Count(content, oldStr)
		if count == 0 {
			return tools.Result{IsError: true, Content: fmt.Sprintf("edit #%d: old_str not found in %s (no changes written)", i+1, path)}, nil
		}
		if count > 1 && !replaceAll {
			return tools.Result{IsError: true, Content: fmt.Sprintf("edit #%d: old_str matches %d times — add context or set replace_all (no changes written)", i+1, count)}, nil
		}
		if replaceAll {
			content = strings.ReplaceAll(content, oldStr, newStr)
		} else {
			content = strings.Replace(content, oldStr, newStr, 1)
		}
		applied++
	}

	if t.cfg != nil {
		if err := validateWritePath(path, t.cfg.Security.BlockedWritePaths, t.cfg.Security.AllowedWritePaths); err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
	}
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("multi_edit write: %v", err)}, nil
	}
	return tools.Result{
		Content: fmt.Sprintf("applied %d/%d edits to %s", applied, len(raw), path),
		Summary: fmt.Sprintf("%d edits", applied),
	}, nil
}
