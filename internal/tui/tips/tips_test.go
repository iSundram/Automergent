package tips

import (
	"strings"
	"testing"
	"time"
)

// TestEveryStateHasHints guarantees that a new State cannot be added without
// giving it footer hints — the table lookup falls back to idle, which would
// silently advertise the wrong keys.
func TestEveryStateHasHints(t *testing.T) {
	for _, s := range AllStates() {
		hints, ok := hintTable[s]
		if !ok {
			t.Errorf("state %q has no hintTable entry", s)
			continue
		}
		if len(hints) == 0 {
			t.Errorf("state %q has an empty hint list", s)
		}
		var hasAnchor bool
		for _, h := range hints {
			if h.Key == "" {
				t.Errorf("state %q has a hint with an empty key", s)
			}
			if h.Priority == 0 {
				hasAnchor = true
			}
		}
		if !hasAnchor {
			t.Errorf("state %q has no priority-0 hint: nothing survives a narrow terminal", s)
		}
	}
}

// TestEveryStateHasInfo checks the `└─` line is populated for every state,
// both with an empty Context and a fully-populated one.
func TestEveryStateHasInfo(t *testing.T) {
	full := Context{
		Queued: 2, Attempt: 3, MaxAttempts: 10,
		ErrCode: "529", Tool: "read_file", Detail: "overloaded",
		ToolsRun: 4, Elapsed: 3 * time.Second, NextRetryIn: 8 * time.Second,
	}
	for _, s := range AllStates() {
		for name, ctx := range map[string]Context{"zero": {}, "full": full} {
			got := Info(s, ctx)
			if strings.TrimSpace(got) == "" {
				t.Errorf("Info(%q, %s) is empty", s, name)
			}
		}
	}
}

func TestHintsReturnsCopy(t *testing.T) {
	first := Hints(StateIdle)
	if len(first) == 0 {
		t.Fatal("idle hints are empty")
	}
	first[0].Key = "MUTATED"
	if Hints(StateIdle)[0].Key == "MUTATED" {
		t.Fatal("Hints returned a slice aliasing the package table")
	}
}

func TestHintsUnknownStateFallsBackToIdle(t *testing.T) {
	got := Hints(State("no-such-state"))
	want := Hints(StateIdle)
	if len(got) != len(want) {
		t.Fatalf("unknown state returned %d hints, want idle's %d", len(got), len(want))
	}
}

func TestInfoUnknownStateFallsBackToIdle(t *testing.T) {
	if got, want := Info(State("no-such-state"), Context{}), Info(StateIdle, Context{}); got != want {
		t.Fatalf("unknown state info = %q, want idle's %q", got, want)
	}
}

func TestRetryingInfoIncludesCodeAttemptAndDelay(t *testing.T) {
	got := Info(StateRetrying, Context{
		ErrCode: "529", Detail: "overloaded",
		Attempt: 3, MaxAttempts: 10, NextRetryIn: 8 * time.Second,
	})
	for _, want := range []string{"529", "overloaded", "3/10", "8s", "/error"} {
		if !strings.Contains(got, want) {
			t.Errorf("retrying info %q missing %q", got, want)
		}
	}
}

func TestStopFirstInfoTellsUserToStopBeforeExiting(t *testing.T) {
	got := Info(StateStopFirst, Context{})
	for _, want := range []string{"still running", "esc", "ctrl+c"} {
		if !strings.Contains(got, want) {
			t.Errorf("stop-first info %q missing %q", got, want)
		}
	}
}

func TestQueuedInfoPluralises(t *testing.T) {
	one := Info(StateQueued, Context{Queued: 1})
	if !strings.Contains(one, "1 message ") {
		t.Errorf("single queued message not singular: %q", one)
	}
	two := Info(StateQueued, Context{Queued: 2})
	if !strings.Contains(two, "2 messages") {
		t.Errorf("two queued messages not plural: %q", two)
	}
}

func TestInterruptedInfoReportsToolCount(t *testing.T) {
	got := Info(StateInterrupted, Context{ToolsRun: 2})
	if !strings.Contains(got, "2 tools") {
		t.Errorf("interrupted info %q should mention the tool count", got)
	}
	if !strings.Contains(got, "/rewind") {
		t.Errorf("interrupted info %q should offer /rewind", got)
	}
}

func TestHintString(t *testing.T) {
	if got := (Hint{Key: "ESC", Action: "cancel"}).String(); got != "ESC cancel" {
		t.Errorf("Hint.String() = %q, want %q", got, "ESC cancel")
	}
	if got := (Hint{Key: "?"}).String(); got != "?" {
		t.Errorf("actionless Hint.String() = %q, want %q", got, "?")
	}
}

func TestRoundDuration(t *testing.T) {
	cases := map[time.Duration]string{
		500 * time.Millisecond: "500ms",
		8 * time.Second:        "8s",
		90 * time.Second:       "1m30s",
	}
	for in, want := range cases {
		if got := roundDuration(in); got != want {
			t.Errorf("roundDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
