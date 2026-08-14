package agent

import (
	"testing"
	"time"
)

func resetAgentManagerForTest() *AgentManager {
	globalAgentManager = &AgentManager{agents: make(map[string]*AgentInstance)}
	return GetAgentManager()
}

func TestAgentManagerUpdateStatusHooks(t *testing.T) {
	mgr := resetAgentManagerForTest()

	agent := &AgentInstance{
		ID:        "agent-1",
		Name:      "worker",
		Type:      AgentTypeTask,
		Status:    AgentStatusRunning,
		StartedAt: time.Now().Add(-2 * time.Second),
		done:      make(chan struct{}),
	}
	mgr.Create(agent)

	var got AgentNotification
	mgr.RegisterCompletionHook(func(n AgentNotification) {
		got = n
	})

	ok := mgr.UpdateStatus(agent.ID, AgentStatusCompleted, "done", nil)
	if !ok {
		t.Fatalf("expected status update to succeed")
	}
	if got.AgentID != agent.ID || got.Status != AgentStatusCompleted {
		t.Fatalf("unexpected hook payload: %+v", got)
	}
	if got.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", got.Duration)
	}
}

func TestAgentManagerListExcludesCompletedWhenRequested(t *testing.T) {
	mgr := resetAgentManagerForTest()

	mgr.Create(&AgentInstance{ID: "running", Status: AgentStatusRunning, StartedAt: time.Now(), done: make(chan struct{})})
	mgr.Create(&AgentInstance{ID: "completed", Status: AgentStatusCompleted, StartedAt: time.Now(), done: make(chan struct{})})
	mgr.Create(&AgentInstance{ID: "cancelled", Status: AgentStatusCancelled, StartedAt: time.Now(), done: make(chan struct{})})

	active := mgr.List(false)
	if len(active) != 1 || active[0].ID != "running" {
		t.Fatalf("expected only running agent, got %v entries", len(active))
	}
}
