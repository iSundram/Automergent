package app

import (
	"strings"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tui/commands"
	cmdtips "github.com/iSundram/Automergent/internal/tui/commands/tips"
	"github.com/iSundram/Automergent/internal/tui/tips"
)

func TestTipRotatorShowsEveryCommandThenFactsThenLoops(t *testing.T) {
	r := commands.Default()
	rot := newTipRotator(r)

	cmdCount := 0
	for _, cmd := range r.List() {
		if ct, ok := r.Tip(cmd.Name); ok && ct.InfolineTip() != "" {
			cmdCount++
		}
	}
	if cmdCount == 0 {
		t.Fatal("no command tips found to rotate")
	}

	// The first cmdCount entries are tips, one per command, in List() order.
	seen := map[string]bool{}
	for i := 0; i < cmdCount; i++ {
		entry := rot.Current()
		if !strings.HasPrefix(entry, "tip: /") {
			t.Fatalf("entry %d is %q, want a command tip", i, entry)
		}
		cmd := commands.Command{}
		for _, c := range r.List() {
			if strings.HasPrefix(entry, "tip: /"+c.Name+" ") {
				cmd = c
				break
			}
		}
		if seen[cmd.Name] {
			t.Fatalf("command %q appeared twice before all tips were shown", cmd.Name)
		}
		seen[cmd.Name] = true
		rot.advance()
	}

	// Facts follow the tips.
	for i := 0; i < len(cmdtips.Facts); i++ {
		if !strings.HasPrefix(rot.Current(), "fact: ") {
			t.Fatalf("entry after tips is %q, want a fact", rot.Current())
		}
		rot.advance()
	}

	// After tips + facts the sequence loops back to the first tip.
	if !strings.HasPrefix(rot.Current(), "tip: /") {
		t.Fatalf("after one full cycle got %q, want the first tip again", rot.Current())
	}
}

func TestTipRotatorEmptyRegistry(t *testing.T) {
	var nilRegistry *commands.Registry
	rot := newTipRotator(nilRegistry)
	if rot.Current() != "" {
		t.Fatal("empty rotator must render nothing")
	}
	// advance on an empty rotator must not panic.
	rot.advance()

	var nilRotator *tipRotator
	if nilRotator.Current() != "" {
		t.Fatal("nil rotator must render nothing")
	}
	nilRotator.advance()
}

func TestMaybeRotateTipGating(t *testing.T) {
	a := &App{tipRotate: newTipRotator(commands.Default())}
	start := time.Now()
	a.thinking = false // idle

	// First tick only records the baseline; nothing advances yet.
	if a.maybeRotateTip(start) {
		t.Fatal("first tick must not advance, only set the baseline")
	}
	if a.maybeRotateTip(start.Add(14 * time.Second)) {
		t.Fatal("a tick inside the 15s window must not advance")
	}
	// Crossing the interval advances exactly once.
	if !a.maybeRotateTip(start.Add(15 * time.Second)) {
		t.Fatal("a tick at the interval must advance")
	}
	if a.maybeRotateTip(start.Add(16 * time.Second)) {
		t.Fatal("the next tick after an advance must not advance again")
	}
	if !a.maybeRotateTip(start.Add(31 * time.Second)) {
		t.Fatal("the second interval boundary must advance")
	}

	// While running, rotation pauses: no advance even past the boundary.
	a.thinking = true
	if a.maybeRotateTip(start.Add(120 * time.Second)) {
		t.Fatal("rotation must pause while the agent is running")
	}
}

func TestIdleTipReachesInfoLine(t *testing.T) {
	a := &App{cfg: &config.Config{}, tipRotate: newTipRotator(commands.Default())}
	a.thinking = false
	ctx := a.tipContext()
	if ctx.IdleTip == "" {
		t.Fatal("idle context must carry the rotating tip")
	}
	if got := tips.Info(tips.StateIdle, ctx); got != ctx.IdleTip {
		t.Fatalf("idle info line is %q, want the rotating tip %q", got, ctx.IdleTip)
	}
	if !strings.HasPrefix(ctx.IdleTip, "tip: /") {
		t.Fatalf("the first rotation entry must be a command tip, got %q", ctx.IdleTip)
	}
}
