package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/shared"
)

func TestTaskBoardRendersTodosAndAgents(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	b.SetTodos([]shared.TodoItem{
		{ID: "t1", Description: "wire auth flow", Status: shared.TodoStatusCompleted},
		{ID: "t2", Description: "add tests", Status: shared.TodoStatusInProgress},
	})
	b.SetAgents([]AgentRow{
		{ID: "a-1", Name: "auth-worker", Type: "general-purpose", Status: "running", Elapsed: "1m02s"},
	})
	out := b.View()
	for _, want := range []string{"BOARD", "todos", "wire auth flow", "add tests", "1/2 done", "agents", "auth-worker", "running"} {
		if !strings.Contains(out, want) {
			t.Errorf("board missing %q:\n%s", want, out)
		}
	}
}

func TestTaskBoardFocusAndAgentLookup(t *testing.T) {
	b := NewTaskBoard(testStyles())
	b.SetSize(40, 24)
	b.Toggle()
	b.SetTodos([]shared.TodoItem{{ID: "t1", Description: "x", Status: shared.TodoStatusPending}})
	b.SetAgents([]AgentRow{{ID: "a-9", Name: "w", Type: "task", Status: "running"}})
	b.MoveFocus(2) // past the single todo onto the agent row
	if ag, ok := b.FocusedAgent(); !ok || ag.ID != "a-9" {
		t.Fatalf("FocusedAgent = %+v ok=%v", ag, ok)
	}
}
