package prompt

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tools"
)

type fakeTool struct {
	name string
	desc string
	meta *tools.ToolMeta
	tools.BaseTool
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return f.desc }
func (f *fakeTool) Schema() map[string]any {
	return map[string]any{"type": "object"}
}
func (f *fakeTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (f *fakeTool) RequiresConfirmation(mode string) bool { return false }
func (f *fakeTool) Meta() *tools.ToolMeta                 { return f.meta }

func TestRenderToolSectionsGroupingAndOrder(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&fakeTool{name: "bash", desc: "run it"})
	reg.Register(&fakeTool{name: "read_file", desc: "read it", meta: &tools.ToolMeta{
		Category:    "read",
		DisplayName: "Read file",
		WhenToUse:   "before editing",
	}})
	reg.Register(&fakeTool{name: "grep", desc: "grep it", meta: &tools.ToolMeta{
		Category:    "search",
		DisplayName: "Search content",
	}})

	out := RenderToolSections(reg, "")
	if out == "" {
		t.Fatal("expected non-empty sections")
	}
	if !strings.Contains(out, "#### Read file (`read_file`)") {
		t.Errorf("missing explicit-meta tool section:\n%s", out)
	}
	if !strings.Contains(out, "#### Bash (`bash`)") {
		t.Errorf("missing inferred-default section:\n%s", out)
	}
	// Category ordering: read < search < shell.
	iRead := strings.Index(out, "Read tools")
	iSearch := strings.Index(out, "Search tools")
	iShell := strings.Index(out, "Shell tools")
	if !(0 <= iRead && iRead < iSearch && iSearch < iShell) {
		t.Fatalf("category order wrong: read=%d search=%d shell=%d\n%s", iRead, iSearch, iShell, out)
	}
}

func TestRenderToolSectionsGeminiFamilyNotes(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&fakeTool{name: "read_file", desc: "read", meta: &tools.ToolMeta{
		Category:      "read",
		DisplayName:   "Read file",
		UsageByFamily: map[string]string{"gemini3": "batch parallel reads"},
	}})

	withGemini3 := RenderToolSections(reg, "gemini-3-pro")
	if !strings.Contains(withGemini3, "Model notes (gemini3)") || !strings.Contains(withGemini3, "batch parallel reads") {
		t.Errorf("gemini3 note missing for gemini-3-pro model:\n%s", withGemini3)
	}

	withOther := RenderToolSections(reg, "gemini-2.5-pro")
	if strings.Contains(withOther, "batch parallel reads") {
		t.Errorf("gemini3 note must not leak into other families:\n%s", withOther)
	}

	withUnknown := RenderToolSections(reg, "")
	if strings.Contains(withUnknown, "Model notes") {
		t.Errorf("no family notes expected for unknown model:\n%s", withUnknown)
	}
}

func TestRenderToolSectionsEmptyRegistry(t *testing.T) {
	if out := RenderToolSections(tools.NewRegistry(), ""); out != "" {
		t.Errorf("expected empty output for empty registry, got %q", out)
	}
	if out := RenderToolSections(nil, ""); out != "" {
		t.Errorf("expected empty output for nil registry, got %q", out)
	}
}
