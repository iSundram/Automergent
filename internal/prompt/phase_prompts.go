package prompt

import (
	_ "embed"

	"github.com/iSundram/Automergent/internal/shared"
)

// Per-phase prompt files. Each follows the same three-section structure —
// MISSION (what this phase is for), RULES (hard behavioral rules), EXIT
// (what "done" means and what the next phase receives) — so the concerns
// stay separable and every phase reads consistently.

//go:embed phases/init.txt
var phaseInitPrompt string

//go:embed phases/explore.txt
var phaseExplorePrompt string

//go:embed phases/plan.txt
var phasePlanPrompt string

//go:embed phases/build.txt
var phaseBuildPrompt string

// PhasePrompt returns the default prompt for a phase, or "" when the phase
// is unknown. Agent definitions may override via PhasePrompts; this is the
// platform default layer.
func PhasePrompt(phase shared.AgentPhase) string {
	switch phase {
	case shared.PhaseInit:
		return phaseInitPrompt
	case shared.PhaseExplore:
		return phaseExplorePrompt
	case shared.PhasePlan:
		return phasePlanPrompt
	case shared.PhaseBuild:
		return phaseBuildPrompt
	default:
		return ""
	}
}
