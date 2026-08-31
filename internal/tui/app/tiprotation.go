package app

// The idle info-line tip rotator.
//
// While the app is idle and nothing else claims the info line (no command
// highlighted in the palette, no typed "/command"), the line cycles through a
// personalized one-line tip for every command — prefixed "tip: " — and then
// the coding facts — prefixed "fact: " — before looping back to the tips.

import (
	"fmt"
	"time"

	"github.com/iSundram/Automergent/internal/tui/commands"
	cmdtips "github.com/iSundram/Automergent/internal/tui/commands/tips"
	"github.com/iSundram/Automergent/internal/tui/tips"
)

// tipRotateInterval is how long the idle line shows one entry before moving
// to the next. The live tick checks the clock; this is not a new timer.
const tipRotateInterval = 15 * time.Second

// tipRotator cycles the idle info line through every command's one-line tip
// and then the coding facts, endlessly.
type tipRotator struct {
	// entries are the fully formatted lines: "tip: /compact — …" then
	// "fact: …", in rotation order. Placeholders in the tip text are left
	// raw here and personalized at display time.
	entries []string
	// idx is the current entry's position.
	idx int
}

// newTipRotator builds the rotation sequence from the registry: one entry per
// command in List() order, then one per coding fact.
func newTipRotator(r *commands.Registry) *tipRotator {
	t := &tipRotator{}
	if r == nil {
		return t
	}
	for _, cmd := range r.List() {
		ct, ok := r.Tip(cmd.Name)
		if !ok {
			continue
		}
		line := ct.InfolineTip()
		if line == "" {
			continue
		}
		t.entries = append(t.entries, fmt.Sprintf("tip: /%s — %s", cmd.Name, line))
	}
	for _, f := range cmdtips.Facts {
		t.entries = append(t.entries, "fact: "+f)
	}
	return t
}

// Current returns the line currently in rotation, or "" when there is
// nothing to show (the info line then falls back to the static idle hints).
func (t *tipRotator) Current() string {
	if t == nil || len(t.entries) == 0 {
		return ""
	}
	return t.entries[t.idx%len(t.entries)]
}

// advance moves to the next entry, wrapping past the end back to the first
// tip so the sequence loops forever.
func (t *tipRotator) advance() {
	if t == nil || len(t.entries) == 0 {
		return
	}
	t.idx = (t.idx + 1) % len(t.entries)
}

// maybeRotateTip advances the rotator when the idle line's interval elapsed.
// It returns true when the line changed, so the caller can limit repaints.
func (a *App) maybeRotateTip(now time.Time) bool {
	if a.tipRotate == nil || a.uiState() != tips.StateIdle {
		return false
	}
	if a.tipLastRotate.IsZero() {
		a.tipLastRotate = now
		return false
	}
	if now.Sub(a.tipLastRotate) < tipRotateInterval {
		return false
	}
	a.tipLastRotate = now
	a.tipRotate.advance()
	return true
}

// idleTipText returns the current rotation entry with its {placeholders}
// filled from live state, for the info line's IdleTip context field.
func (a *App) idleTipText() string {
	if a.tipRotate == nil {
		return ""
	}
	return a.personalizeCommandTip(a.tipRotate.Current())
}
