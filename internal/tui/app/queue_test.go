package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
)

var enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}

// TestEnterQueuesWhileRunning is the fix for the silent dead-end: Enter used to
// be gated on !thinking, so typing mid-run did nothing at all.
func TestEnterQueuesWhileRunning(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.input.SetValue("also update the tests")

	app.handleKey(enterKey)

	if len(app.msgQueue) != 1 {
		t.Fatalf("expected 1 queued message, got %d", len(app.msgQueue))
	}
	if got := app.msgQueue[0].text; got != "also update the tests" {
		t.Errorf("queued text = %q", got)
	}
	if app.input.Value() != "" {
		t.Errorf("input should be cleared after queueing, got %q", app.input.Value())
	}
	if app.msgQueue[0].boundary {
		t.Error("Enter should queue for end-of-turn, not boundary delivery")
	}
}

func TestQueuedCountReachesFooter(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.input.SetValue("one")
	app.handleKey(enterKey)
	app.input.SetValue("two")
	app.handleKey(enterKey)

	if len(app.msgQueue) != 2 {
		t.Fatalf("expected 2 queued, got %d", len(app.msgQueue))
	}
	if !strings.Contains(app.statusBar.View(), "queued") {
		t.Error("footer should show the queue count")
	}
}

// TestEnterSendsImmediatelyWhenIdle: queueing must only happen mid-run.
func TestEnterSendsImmediatelyWhenIdle(t *testing.T) {
	app := sizedTestApp(t)
	app.input.SetValue("/version")

	app.handleKey(enterKey)

	if len(app.msgQueue) != 0 {
		t.Error("an idle Enter should send, not queue")
	}
	if app.input.Value() != "" {
		t.Error("input should be cleared on send")
	}
}

// TestAskUserReplyBypassesQueue: the agent is blocked waiting for exactly this
// answer, so queueing it would deadlock the run.
func TestAskUserReplyBypassesQueue(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	reply := make(chan string, 1)
	app.askUserReplyCh = reply
	app.input.SetValue("the answer")

	app.handleKey(enterKey)

	if len(app.msgQueue) != 0 {
		t.Error("an ask_user reply must not be queued")
	}
	select {
	case got := <-reply:
		if got != "the answer" {
			t.Errorf("reply = %q", got)
		}
	default:
		t.Fatal("reply was not delivered to the waiting agent")
	}
}

func TestCtrlJMarksBoundaryDelivery(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.input.SetValue("steer this in")

	app.handleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})

	// The message either reached the agent's steer channel (and so left the
	// queue) or stayed queued marked for boundary delivery. Both are correct;
	// what must not happen is it being dropped or queued as end-of-turn.
	if len(app.msgQueue) == 0 {
		return // accepted by the agent
	}
	if !app.msgQueue[0].boundary {
		t.Error("ctrl+j should mark the message for boundary delivery")
	}
}

func TestCtrlJPromotesExistingQueue(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.msgQueue = []queuedMessage{{text: "earlier"}}

	app.handleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})

	if len(app.msgQueue) > 0 && !app.msgQueue[0].boundary {
		t.Error("ctrl+j with an empty prompt should promote the existing queue")
	}
}

func TestCtrlJWithNothingToSendReportsIt(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true

	app.handleKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})

	if status := strings.ToLower(app.statusBar.Status()); !strings.Contains(status, "nothing to send") {
		t.Errorf("status should say there is nothing to send, got %q", status)
	}
}

func TestSlashCommandsNeverSteer(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true

	// A slash command is local dispatch, so it cannot be injected into the
	// model's context — it must wait for the turn to end.
	app.enqueueMessage("/diff", true)

	if len(app.msgQueue) != 1 {
		t.Fatalf("slash command should stay queued, got %d entries", len(app.msgQueue))
	}
	if !app.msgQueue[0].isCmd {
		t.Error("queued slash command should be flagged isCmd")
	}
}

func TestDrainQueueDeliversOnePerTurn(t *testing.T) {
	app := sizedTestApp(t)
	app.msgQueue = []queuedMessage{{text: "first"}, {text: "second"}}

	cmd := app.drainQueue()
	if cmd == nil {
		t.Fatal("drainQueue should return a command for the first message")
	}
	if len(app.msgQueue) != 1 {
		t.Errorf("one message should remain queued, got %d", len(app.msgQueue))
	}
	if app.msgQueue[0].text != "second" {
		t.Errorf("wrong message left queued: %q", app.msgQueue[0].text)
	}
}

func TestDrainQueueSkipsBlankEntries(t *testing.T) {
	app := sizedTestApp(t)
	app.msgQueue = []queuedMessage{{text: "   "}, {text: "real"}}

	if cmd := app.drainQueue(); cmd == nil {
		t.Fatal("drainQueue should skip the blank and deliver the real message")
	}
	if len(app.msgQueue) != 0 {
		t.Errorf("queue should be empty, got %d", len(app.msgQueue))
	}
}

func TestDrainQueueEmptyReturnsNil(t *testing.T) {
	app := sizedTestApp(t)
	if cmd := app.drainQueue(); cmd != nil {
		t.Error("draining an empty queue should return nil")
	}
}

func TestEnqueueRejectsBlank(t *testing.T) {
	app := sizedTestApp(t)
	if app.enqueueMessage("   \n  ", false) {
		t.Error("blank text should not be queued")
	}
	if len(app.msgQueue) != 0 {
		t.Errorf("queue should be empty, got %d", len(app.msgQueue))
	}
}

// TestCancelClearsQueue: a message queued for the cancelled run must not leak
// into the next, unrelated one.
func TestCancelClearsQueue(t *testing.T) {
	app := sizedTestApp(t)
	app.thinking = true
	app.msgQueue = []queuedMessage{{text: "for the cancelled run"}}

	app.cancelActiveRun("Interrupted")

	if len(app.msgQueue) != 0 {
		t.Errorf("cancelling should clear the queue, got %d entries", len(app.msgQueue))
	}
}

func TestClearQueueReportsCount(t *testing.T) {
	app := sizedTestApp(t)
	app.msgQueue = []queuedMessage{{text: "a"}, {text: "b"}, {text: "c"}}
	if got := app.clearQueue(); got != 3 {
		t.Errorf("clearQueue() = %d, want 3", got)
	}
	if len(app.msgQueue) != 0 {
		t.Error("queue should be empty after clearQueue")
	}
}

// TestAgentSteerAndDrain exercises the agent-side half of boundary delivery.
func TestAgentSteerAndDrain(t *testing.T) {
	app := sizedTestApp(t)

	if !app.ag.Steer("mid-run instruction") {
		t.Fatal("Steer should accept a message into the buffer")
	}
	if app.ag.Steer("   ") {
		t.Error("Steer should reject blank text")
	}

	app.ag.ClearSteerQueue()
	// After clearing, the buffer has room again.
	if !app.ag.Steer("another") {
		t.Error("Steer should accept after the queue is cleared")
	}
}

func TestAgentSteerBufferBackpressure(t *testing.T) {
	app := sizedTestApp(t)
	// The buffer is bounded; once full, Steer must report false rather than
	// blocking the UI goroutine.
	accepted := 0
	for i := 0; i < 100; i++ {
		if app.ag.Steer("msg") {
			accepted++
			continue
		}
		break
	}
	if accepted == 0 {
		t.Fatal("Steer accepted nothing")
	}
	if accepted >= 100 {
		t.Error("Steer should apply backpressure rather than accepting unboundedly")
	}
	if app.ag.Steer("overflow") {
		t.Error("a full buffer should reject further messages")
	}
}

func TestModeCanonicalisedAtStartup(t *testing.T) {
	app := newTestApp(t)
	// newTestApp uses config.Default(); whatever it holds must be canonical
	// after construction so the chip and the approval gate agree.
	if got := app.cfg.Mode; got != agent.CanonicalMode(got) {
		t.Errorf("cfg.Mode = %q is not canonical", got)
	}
	if app.cfg.Mode == "" {
		t.Error("cfg.Mode should default to a real mode, not empty")
	}
	if app.statusBar.Mode() != app.cfg.Mode {
		t.Errorf("footer chip %q out of sync with cfg.Mode %q", app.statusBar.Mode(), app.cfg.Mode)
	}
}
