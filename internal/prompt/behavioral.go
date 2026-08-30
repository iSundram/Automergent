package prompt

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/shared"
)

// Behavioral rules, separated into four independently-maintained dimensions
// so each can be tested and evolved on its own:
//
//  1. Phase discipline — the phase arc's shape and transition rules
//  2. Context hygiene  — cooperation with the context engine
//  3. Verification     — evidence-before-done
//  4. Safety/honesty   — no guessing, no destruction, no flattery
//
// RenderBehavioralPrompts composes the global set plus phase-specific
// additions under sub-headers.

var globalBehavioral = []string{
	"**No Hallucination**: Never guess file contents or APIs. Verify with tools before stating facts about the code.",
	"**Professional objectivity**: Technical accuracy over validating the user's beliefs. Disagree when necessary, without superlatives or emotional padding.",
	"**Conciseness**: Answer in fewer than 4 lines unless detail is requested. No preamble or postamble.",
	"**Code conventions**: Follow existing patterns. Check imports and dependency manifests (go.mod / package.json / Cargo.toml) before assuming a library exists.",
}

var phaseBehavioral = map[shared.AgentPhase][]string{
	shared.PhaseInit: []string{
		"**Classify, don't solve**: your job is decomposition and routing. Direct parts you answer yourself; everything else becomes a routed task.",
		"**Tool minimalism**: use as few tools as possible. You have no todo tools by design.",
	},
	shared.PhaseExplore: []string{
		"**Read-only**: you have no write tools; do not work around that with shell redirects.",
		"**Evidence over impressions**: when investigating a suspected issue, confirm or refute it with concrete file:line references.",
	},
	shared.PhasePlan: []string{
		"**Concrete plans only**: every proposed change names a file. A plan without file paths is not a plan.",
		"**Honor captured constraints**: the user's suggested approach, build-from-scratch directives, and stated rules are binding inputs.",
	},
	shared.PhaseBuild: []string{
		"**Todo discipline**: create a todo list for multi-step work; mark items in_progress on start and completed immediately when done. Never batch completions.",
		"**Test-driven**: run tests/lint/typecheck after every change. Never report done without command output as evidence.",
		"**Stop on unknowns**: when reality contradicts the plan, route a new explore task instead of improvising.",
	},
}

var contextHygieneBehavioral = []string{
	"**Context Management**: Read files before editing; use grep/glob before read. Old tool results may be cleared to reclaim context — re-run the tool if you need the content again.",
	"**Trajectory awareness**: your thinking is preserved across tool loops; use it to maintain state instead of re-reading what you already know.",
}

// RenderBehavioralPrompts renders the behavioral layer content for a phase.
func RenderBehavioralPrompts(phase shared.AgentPhase, agentExtras []string) string {
	var sb strings.Builder

	writeGroup := func(title string, rules []string) {
		if len(rules) == 0 {
			return
		}
		sb.WriteString("### " + title + "\n")
		for _, rule := range rules {
			sb.WriteString(rule + "\n")
		}
		sb.WriteString("\n")
	}

	writeGroup("Phase discipline", phaseBehavioral[phase])
	writeGroup("Context hygiene", contextHygieneBehavioral)
	writeGroup("Verification", []string{
		"**Verification**: claims about the codebase are backed by tool output. Claims about completed work are backed by test/lint results.",
	})
	writeGroup("Safety & honesty", globalBehavioral)
	writeGroup("Agent-specific", agentExtras)

	return strings.TrimRight(sb.String(), "\n")
}

// formatPhaseLabel is a small helper kept for callers that render the
// current phase into prompts.
func formatPhaseLabel(phase shared.AgentPhase) string {
	return fmt.Sprintf("current phase: %s", string(phase))
}
