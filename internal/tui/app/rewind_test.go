package app

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/session"
)

// Rewind contract: checkpoints derive from message history (so they survive
// restarts and resume), never leak across session switches, and rewind
// refuses to run while the agent is mid-turn.

func msg(role ai.Role, text string) ai.Message {
	return ai.NewTextMessage(role, text)
}

func TestRebuildCheckpointsFromHistory(t *testing.T) {
	messages := []ai.Message{
		msg(ai.RoleUser, "first prompt"),
		msg(ai.RoleAssistant, "first reply"),
		msg(ai.RoleUser, "second prompt"),
		msg(ai.RoleAssistant, "second reply"),
		msg(ai.RoleUser, "third prompt"),
	}
	cps := rebuildCheckpoints(messages)

	if len(cps) != 2 {
		t.Fatalf("want 2 checkpoints (before turns 2 and 3), got %d", len(cps))
	}
	if cps[0].label != "second prompt" {
		t.Fatalf("checkpoint 1 label = %q, want the second turn's prompt", cps[0].label)
	}
	if len(cps[0].messages) != 2 {
		t.Fatalf("checkpoint 1 must hold the state after turn 1, got %d messages", len(cps[0].messages))
	}
	if cps[1].label != "third prompt" || len(cps[1].messages) != 4 {
		t.Fatalf("checkpoint 2 wrong: label=%q msgs=%d", cps[1].label, len(cps[1].messages))
	}
}

func TestRebuildCheckpointsSingleTurnIsEmpty(t *testing.T) {
	cps := rebuildCheckpoints([]ai.Message{
		msg(ai.RoleUser, "only prompt"),
		msg(ai.RoleAssistant, "only reply"),
	})
	if len(cps) != 0 {
		t.Fatalf("a single turn has nothing to rewind to, got %d", len(cps))
	}
}

func TestRebuildCheckpointsTruncatesLongLabels(t *testing.T) {
	long := strings.Repeat("x", 200)
	cps := rebuildCheckpoints([]ai.Message{
		msg(ai.RoleUser, "a"),
		msg(ai.RoleAssistant, "b"),
		msg(ai.RoleUser, long),
	})
	if len(cps) != 1 || len(cps[0].label) > 80 {
		t.Fatalf("label not truncated: %d", len(cps[0].label))
	}
}

func TestRestoreSessionReplacesStaleCheckpoints(t *testing.T) {
	app := newTestApp(t)

	// Simulate a prior session's live checkpoints.
	app.checkpoints = []conversationCheckpoint{
		{label: "foreign", messages: []ai.Message{msg(ai.RoleUser, "other session")}},
	}

	resumed := session.New()
	resumed.SetMessages([]ai.Message{
		msg(ai.RoleUser, "resumed turn 1"),
		msg(ai.RoleAssistant, "resumed reply 1"),
		msg(ai.RoleUser, "resumed turn 2"),
	})
	if err := app.restoreSession(resumed); err != nil {
		t.Fatal(err)
	}

	if len(app.checkpoints) != 1 {
		t.Fatalf("restore must rebuild checkpoints from the resumed history, got %d", len(app.checkpoints))
	}
	if app.checkpoints[0].label != "resumed turn 2" {
		t.Fatalf("stale checkpoint leaked: %q", app.checkpoints[0].label)
	}
	for _, m := range app.checkpoints[0].messages {
		if m.TextContent() == "other session" {
			t.Fatal("foreign session messages must never survive a session switch")
		}
	}
}

func TestRewindToRefusesWhileAgentRunning(t *testing.T) {
	app := newTestApp(t)
	app.sess.SetMessages([]ai.Message{
		msg(ai.RoleUser, "one"),
		msg(ai.RoleAssistant, "reply"),
		msg(ai.RoleUser, "two"),
	})
	app.checkpoints = rebuildCheckpoints(app.sess.Messages)

	app.thinking = true
	if err := app.rewindTo(1); err == nil {
		t.Fatal("rewind must refuse while the agent is running")
	}

	app.thinking = false
	if err := app.rewindTo(1); err != nil {
		t.Fatalf("rewind should succeed when idle: %v", err)
	}
	// rebuildCheckpoints derives one checkpoint per later user turn: with
	// user/assistant/user, checkpoint 1 holds everything before "two" —
	// the first user message and its reply.
	if len(app.sess.Messages) != 2 || app.sess.Messages[0].TextContent() != "one" ||
		app.sess.Messages[1].TextContent() != "reply" {
		t.Fatalf("rewind restored wrong state: %+v", app.sess.Messages)
	}
}

func TestRewindToRestoresViewAndPersistsShape(t *testing.T) {
	app := newTestApp(t)
	app.sess.SetMessages([]ai.Message{
		msg(ai.RoleUser, "one"),
		msg(ai.RoleAssistant, "reply one"),
		msg(ai.RoleUser, "two"),
		msg(ai.RoleAssistant, "reply two"),
		msg(ai.RoleUser, "three"),
	})
	app.checkpoints = rebuildCheckpoints(app.sess.Messages)

	if err := app.rewindTo(2); err != nil {
		t.Fatal(err)
	}
	if len(app.sess.Messages) != 4 {
		t.Fatalf("rewind to checkpoint 2 must restore the pre-turn-3 state, got %d messages", len(app.sess.Messages))
	}
	if app.sess.Messages[3].TextContent() != "reply two" {
		t.Fatalf("restored tail wrong: %q", app.sess.Messages[3].TextContent())
	}
	// Checkpoints after the rewind point are discarded; earlier ones remain.
	if len(app.checkpoints) != 1 {
		t.Fatalf("checkpoint truncation wrong: %d remain", len(app.checkpoints))
	}
}
