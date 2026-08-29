package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tui/render"
)

func TestTaskBoardRendersTodos(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	b.SetTodos([]shared.TodoItem{
		{ID: "t1", Description: "wire auth flow", Status: shared.TodoStatusCompleted},
		{ID: "t2", Description: "add tests", Status: shared.TodoStatusInProgress},
		{ID: "t3", Description: "ship it", Status: shared.TodoStatusPending},
	})
	out := b.View()
	for _, want := range []string{"BOARD", "todos", "wire auth flow", "add tests", "ship it", "1/3 done"} {
		if !strings.Contains(out, want) {
			t.Errorf("board missing %q:\n%s", want, out)
		}
	}
}

// Pending and completed must draw different marks: the old board reused one
// glyph for both and told them apart only by colour.
func TestTaskBoardPendingAndCompletedDiffer(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	b.SetTodos([]shared.TodoItem{
		{ID: "t1", Description: "done thing", Status: shared.TodoStatusCompleted},
		{ID: "t2", Description: "pending thing", Status: shared.TodoStatusPending},
	})
	out := b.View()
	if !strings.Contains(out, render.GlyphOK) || strings.Contains(render.GlyphIdle, render.GlyphOK) {
		t.Errorf("completed row should carry its own mark:\n%s", out)
	}
	if !strings.Contains(out, render.GlyphIdle) {
		t.Errorf("pending row should carry the idle mark:\n%s", out)
	}
}

func TestTaskBoardFocusMovesWithinTodos(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	b.SetTodos([]shared.TodoItem{
		{ID: "t1", Description: "x", Status: shared.TodoStatusPending},
		{ID: "t2", Description: "y", Status: shared.TodoStatusPending},
	})
	if !b.MoveFocus(1) {
		t.Fatal("MoveFocus(1) should be handled")
	}
	// Moving past the end clamps rather than escaping into a section that no
	// longer exists.
	if !b.MoveFocus(5) {
		t.Fatal("MoveFocus(5) should be handled")
	}
	out := b.View()
	if !strings.Contains(out, "y") || !strings.Contains(out, render.GlyphCursor) {
		t.Errorf("highlight should stay on the last todo:\n%s", out)
	}
}
