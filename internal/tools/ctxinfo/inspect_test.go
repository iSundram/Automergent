package ctxinfo

import (
	"context"
	"strings"
	"testing"
)

func TestCtxInspectUsesInspectorHook(t *testing.T) {
	defer SetInspector(nil)
	SetInspector(func() string { return "context limit: 1.0M\nestimated in use: 12.3k (1.2%)" })

	res, err := (&CtxInspectTool{}).Execute(context.Background(), nil)
	if err != nil || res.IsError {
		t.Fatalf("inspect failed: %v %s", err, res.Content)
	}
	if !strings.Contains(res.Content, "1.0M") {
		t.Fatalf("summary missing: %s", res.Content)
	}

	// Without a hook the tool degrades gracefully.
	SetInspector(nil)
	res, _ = (&CtxInspectTool{}).Execute(context.Background(), nil)
	if !res.IsError {
		t.Fatal("expected error without inspector")
	}
}

func TestFormatSummaryRendersHumanUnits(t *testing.T) {
	out := FormatSummary(1000000, 12345, 9000, 1500, 800, 1045, 1.2)
	for _, want := range []string{"1.0M", "12.3k", "1.2%", "remaining: 987.7k"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
}
