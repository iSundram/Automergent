package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// Token-budget continuation ("+500k" / "use 2M tokens") — a minimal port of
// the reference agent's utils/tokenBudget.ts + query/tokenBudget.ts.
//
// When the user sets an output-token target for the turn, the loop does not
// accept the model's first natural stop: it injects a continuation nudge and
// keeps the turn alive until the budget is (nearly) spent — or until the
// model is producing nothing of value (diminishing returns), because burning
// the remaining budget on restatements helps no one.

// budgetShorthandStartRE matches "+500k" anchored at the start of the
// message. Anchoring avoids false positives on amounts in natural language
// ("add 300k to the limit" must not set a budget).
var budgetShorthandStartRE = regexp.MustCompile(`^\s*\+(\d+(?:\.\d+)?)\s*(k|m|b)\b`)

// budgetShorthandEndRE matches the shorthand anchored at the end.
var budgetShorthandEndRE = regexp.MustCompile(`\s\+(\d+(?:\.\d+)?)\s*(k|m|b)\s*[.!?]?\s*$`)

// budgetVerboseRE matches the verbose form anywhere in the message.
var budgetVerboseRE = regexp.MustCompile(`(?i)\b(?:use|spend)\s+(\d+(?:\.\d+)?)\s*(k|m|b)\s*tokens?\b`)

var budgetMultipliers = map[string]float64{"k": 1_000, "m": 1_000_000, "b": 1_000_000_000}

// parseTokenBudget extracts a user-specified token target from a message.
// Returns 0 when the message sets none.
func parseTokenBudget(text string) int {
	if m := budgetShorthandStartRE.FindStringSubmatch(text); m != nil {
		return int(parseFloat(m[1]) * budgetMultipliers[strings.ToLower(m[2])])
	}
	if m := budgetShorthandEndRE.FindStringSubmatch(text); m != nil {
		return int(parseFloat(m[1]) * budgetMultipliers[strings.ToLower(m[2])])
	}
	if m := budgetVerboseRE.FindStringSubmatch(text); m != nil {
		return int(parseFloat(m[1]) * budgetMultipliers[strings.ToLower(m[2])])
	}
	return 0
}

func parseFloat(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%g", &v)
	return v
}

// budgetTracker carries the token-budget continuation state for one Run.
type budgetTracker struct {
	budget            int
	spent             int
	continuations     int
	lastDelta         int
}

// Token-budget continuation knobs (reference values).
const (
	// budgetCompletionThreshold: stop continuing once this fraction of the
	// budget is spent.
	budgetCompletionThreshold = 0.9
	// budgetDiminishingThreshold: consecutive continuations adding fewer
	// output tokens than this signal the model is restating, not working.
	budgetDiminishingThreshold = 500
	// budgetDiminishingContinuations is how many continuations must show
	// diminishing deltas before the loop gives up on the budget.
	budgetDiminishingContinuations = 3
	// budgetMaxContinuations bounds the whole mechanism regardless.
	budgetMaxContinuations = 50
)

// budgetDecision is what the loop should do at a natural stop.
type budgetDecision struct {
	continueTurn bool
	nudge        string
}

// checkBudget runs at every natural stop (no tool calls). spentDelta is the
// output tokens produced since the last check.
func (t *budgetTracker) checkBudget(spentDelta int) budgetDecision {
	if t == nil || t.budget <= 0 {
		return budgetDecision{}
	}
	t.spent += spentDelta
	t.continuations++

	diminishing := t.continuations > budgetDiminishingContinuations &&
		spentDelta < budgetDiminishingThreshold &&
		t.lastDelta < budgetDiminishingThreshold
	t.lastDelta = spentDelta

	pct := t.spent * 100 / t.budget
	if diminishing {
		return budgetDecision{}
	}
	if t.spent >= int(float64(t.budget)*budgetCompletionThreshold) {
		return budgetDecision{}
	}
	if t.continuations > budgetMaxContinuations {
		return budgetDecision{}
	}
	return budgetDecision{
		continueTurn: true,
		nudge: fmt.Sprintf(
			"Stopped at %d%% of the token target (%s / %s tokens). Keep working — do not summarize or wrap up. Continue the task with the next concrete step.",
			pct, formatTokens(t.spent), formatTokens(t.budget)),
	}
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%dK", n/1_000)
	}
	return fmt.Sprintf("%d", n)
}
