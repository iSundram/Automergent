package editreview

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// ProposalTools wraps the mutating file tools so that, when review mode is
// enabled, writes become PROPOSALS instead of disk mutations. Reads and
// everything else pass through untouched. The wrapper preserves the wrapped
// tool's full Tool interface by delegation.
type ProposalTool struct {
	wrapped tools.Tool
	store   *Store

	// writeTool is true when this tool's args carry full new file content.
	writeTool bool
}

// WrapWriteTools registers proposal-aware versions of the mutating file tools
// into reg (replacing same-name entries). Tools not found are skipped so the
// helper degrades gracefully with registry evolution.
func WrapWriteTools(reg *tools.Registry, store *Store) {
	if reg == nil || store == nil {
		return
	}
	for _, name := range []string{"edit_file", "write_file", "create_file", "multi_edit"} {
		if base, ok := reg.Get(name); ok {
			reg.Register(&ProposalTool{wrapped: base, store: store})
		}
	}
}

// --- delegated core Tool interface ---

func (p *ProposalTool) Name() string           { return p.wrapped.Name() }
func (p *ProposalTool) Description() string    { return p.wrapped.Description() }
func (p *ProposalTool) Schema() map[string]any { return p.wrapped.Schema() }
func (p *ProposalTool) RequiresConfirmation(mode string) bool {
	return p.wrapped.RequiresConfirmation(mode)
}
func (p *ProposalTool) IsConcurrencySafe(args map[string]any) bool {
	return p.wrapped.IsConcurrencySafe(args)
}
func (p *ProposalTool) IsReadOnly(args map[string]any) bool    { return false }
func (p *ProposalTool) IsDestructive(args map[string]any) bool { return p.wrapped.IsDestructive(args) }
func (p *ProposalTool) EstimatedCost() tools.ToolCost          { return p.wrapped.EstimatedCost() }

// Meta passes through explicit metadata, adding review guidance.
func (p *ProposalTool) Meta() *tools.ToolMeta {
	if mp, ok := p.wrapped.(tools.MetaProvider); ok {
		if meta := mp.Meta(); meta != nil {
			cp := *meta
			cp.Usage = strings.TrimSpace(cp.Usage) + "\nReview mode: your edit is PROPOSED, not applied — the user reviews it in the diff pane."
			return &cp
		}
	}
	return nil
}

// Execute intercepts mutating calls and records proposals.
func (p *ProposalTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path := pathOf(args)
	if path == "" {
		return p.wrapped.Execute(ctx, args)
	}

	switch p.wrapped.Name() {
	case "create_file":
		content := contentOf(args)
		id := p.store.Add("create_file", path, "", content, "new file")
		return proposed(id, path), nil

	case "write_file":
		original := readIfExists(path)
		content := contentOf(args)
		id := p.store.Add("write_file", path, original, content, "full rewrite")
		return proposed(id, path), nil

	case "edit_file", "multi_edit":
		currentBytes, err := os.ReadFile(path)
		if err != nil {
			return p.wrapped.Execute(ctx, args) // let real tool surface the error
		}
		// Dry-run the real tool against a temp shadow of the file to compute
		// the proposed result without touching the workspace.
		proposedContent, err := dryRun(ctx, p.wrapped, path, string(currentBytes), args)
		if err != nil {
			return tools.Result{IsError: true, Content: err.Error()}, nil
		}
		label := "edit"
		if p.wrapped.Name() == "multi_edit" {
			label = "multi-edit"
		}
		id := p.store.Add(p.wrapped.Name(), path, string(currentBytes), proposedContent, label)
		return proposed(id, path), nil
	}

	return p.wrapped.Execute(ctx, args)
}

// proposed builds the standard awaiting-review result.
func proposed(id, path string) tools.Result {
	return tools.Result{
		Content:  fmt.Sprintf("proposed %s → %s\nAwaiting user review in the diff pane.", id, path),
		Summary:  fmt.Sprintf("%s awaiting review", id),
		Metadata: map[string]any{"proposal_id": id},
	}
}

// Apply writes an accepted proposal to disk atomically.
func Apply(p *Proposal) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(p.Path); err == nil {
		mode = info.Mode()
	}
	return atomicWrite(p.Path, []byte(p.Proposed), mode)
}

// RevertNote is the conversation feedback injected on rejection so the model
// adapts rather than re-proposing blindly.
func RevertNote(p *Proposal) string {
	return fmt.Sprintf("[user rejected the proposed %s to %s (%s). Ask or adjust your approach instead of re-proposing the same change.]",
		p.Summary, p.Path, p.ID)
}

// pathOf extracts the target path from common arg shapes.
func pathOf(args map[string]any) string {
	if args == nil {
		return ""
	}
	if v, ok := args["path"].(string); ok {
		return v
	}
	return ""
}

func contentOf(args map[string]any) string {
	if v, ok := args["content"].(string); ok {
		return v
	}
	return ""
}

func readIfExists(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// dryRun applies the tool's string-replacement semantics against a copy of
// the content, mirroring edit_file/multi_edit rules exactly.
func dryRun(ctx context.Context, t tools.Tool, path, current string, args map[string]any) (string, error) {
	switch t.Name() {
	case "edit_file":
		oldStr, _ := args["old_str"].(string)
		newStr, _ := args["new_str"].(string)
		replaceAll, _ := args["replace_all"].(bool)
		if oldStr == "" {
			return "", fmt.Errorf("edit_file: old_str required")
		}
		count := strings.Count(current, oldStr)
		if count == 0 {
			return "", fmt.Errorf("edit_file: old_str not found in %s", path)
		}
		if count > 1 && !replaceAll {
			return "", fmt.Errorf("edit_file: old_str matches %d times — add context", count)
		}
		if replaceAll {
			return strings.ReplaceAll(current, oldStr, newStr), nil
		}
		return strings.Replace(current, oldStr, newStr, 1), nil

	case "multi_edit":
		raw, ok := args["edits"].([]any)
		if !ok || len(raw) == 0 {
			return "", fmt.Errorf("multi_edit: edits array required")
		}
		content := current
		for i, r := range raw {
			m, ok := r.(map[string]any)
			if !ok {
				return "", fmt.Errorf("edit #%d invalid", i+1)
			}
			oldStr, _ := m["old_str"].(string)
			newStr, _ := m["new_str"].(string)
			replaceAll, _ := m["replace_all"].(bool)
			if oldStr == "" {
				return "", fmt.Errorf("edit #%d: empty old_str", i+1)
			}
			count := strings.Count(content, oldStr)
			if count == 0 {
				return "", fmt.Errorf("edit #%d: old_str not found", i+1)
			}
			if count > 1 && !replaceAll {
				return "", fmt.Errorf("edit #%d: old_str matches %d times", i+1, count)
			}
			if replaceAll {
				content = strings.ReplaceAll(content, oldStr, newStr)
			} else {
				content = strings.Replace(content, oldStr, newStr, 1)
			}
		}
		return content, nil
	}
	return "", fmt.Errorf("unsupported proposal tool %q", t.Name())
}
