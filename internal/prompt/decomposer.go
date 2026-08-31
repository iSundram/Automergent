package prompt

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/shared"
)

//go:embed bases/init-decomposer.txt
var initDecomposerPrompt string

// InitDecomposer splits a first message into atomic parts and routes each
// one: direct answers, questions, phased tasks, rules, noise, suspected
// violations, and clarification requests. It is the LLM-driven replacement
// for the keyword PhaseClassifier; the keyword router remains the fallback.
type InitDecomposer struct {
	llmClient LLMClient
}

// NewInitDecomposer creates a decomposer over an LLM client.
func NewInitDecomposer(llmClient LLMClient) *InitDecomposer {
	return &InitDecomposer{llmClient: llmClient}
}

// PartKind classifies one atomic part of the first message.
type PartKind string

const (
	PartKindDirect     PartKind = "direct"
	PartKindQuestion   PartKind = "question"
	PartKindTask       PartKind = "task"
	PartKindRule       PartKind = "rule"
	PartKindNoise      PartKind = "noise"
	PartKindViolation  PartKind = "violation_suspect"
	PartKindClarify    PartKind = "clarify"
)

// DecomposedPart is one atomic piece of the first message with its routing.
type DecomposedPart struct {
	ID                string   `json:"id"`
	Text              string   `json:"text"`
	Kind              PartKind `json:"kind"`
	TaskType          string   `json:"task_type,omitempty"`
	Phase             string   `json:"phase,omitempty"`
	Agent             string   `json:"agent,omitempty"`
	Priority          int      `json:"priority,omitempty"`
	Dependencies      []string `json:"dependencies,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	Scope             string   `json:"scope,omitempty"`      // question: codebase|general
	AnswerStyle       string   `json:"answer_style,omitempty"` // direct: about-me|concise
	Rule              string   `json:"rule,omitempty"`
	RuleAction        string   `json:"rule_action,omitempty"` // add|remove
	ViolationType     string   `json:"violation_type,omitempty"`
	NeedsConfirmation bool     `json:"needs_confirmation,omitempty"`
	Options           []string `json:"options,omitempty"`
}

// Decomposition is the decomposer's full output: parts plus the constraints
// the user attached to the work.
type Decomposition struct {
	Parts                 []DecomposedPart `json:"parts"`
	RequiresClarification bool             `json:"requires_clarification"`
	ClarificationQuestion string           `json:"clarification_question"`
	Constraints           []string         `json:"constraints"`
	Summary               string           `json:"summary"`
}

// TaskParts returns the parts routed as phased tasks, in priority order.
func (d *Decomposition) TaskParts() []DecomposedPart {
	var tasks []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindTask {
			tasks = append(tasks, p)
		}
	}
	// Stable sort by priority (1 first); equal priorities keep input order.
	for i := 1; i < len(tasks); i++ {
		for j := i; j > 0 && tasks[j].Priority < tasks[j-1].Priority; j-- {
			tasks[j], tasks[j-1] = tasks[j-1], tasks[j]
		}
	}
	return tasks
}

// DirectParts returns the parts INIT fulfills itself (direct + general
// questions + about-me).
func (d *Decomposition) DirectParts() []DecomposedPart {
	var out []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindDirect || (p.Kind == PartKindQuestion && p.Scope != "codebase") {
			out = append(out, p)
		}
	}
	return out
}

// RuleParts returns the rule add/remove requests.
func (d *Decomposition) RuleParts() []DecomposedPart {
	var out []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindRule {
			out = append(out, p)
		}
	}
	return out
}

// NoiseParts returns the discarded personal filler.
func (d *Decomposition) NoiseParts() []DecomposedPart {
	var out []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindNoise {
			out = append(out, p)
		}
	}
	return out
}

// ViolationParts returns suspected violations.
func (d *Decomposition) ViolationParts() []DecomposedPart {
	var out []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindViolation {
			out = append(out, p)
		}
	}
	return out
}

// ClarifyParts returns the parts that need user clarification.
func (d *Decomposition) ClarifyParts() []DecomposedPart {
	var out []DecomposedPart
	for _, p := range d.Parts {
		if p.Kind == PartKindClarify {
			out = append(out, p)
		}
	}
	return out
}

// ToTaskSpecs converts the routed task parts into TaskSpecs for the phase
// machinery, preserving agent, priority, and dependencies. Part IDs are the
// spec IDs, so dependencies already reference the right nodes.
func (d *Decomposition) ToTaskSpecs() []shared.TaskSpec {
	specs := make([]shared.TaskSpec, 0)
	for _, p := range d.Parts {
		if p.Kind != PartKindTask {
			continue
		}
		specs = append(specs, newTaskSpecFromPart(p, d.Constraints))
	}
	return specs
}

// IntentSetFromDecomposition derives the prompt system's IntentSet from the
// decomposer's output instead of running a SECOND LLM call. The decomposer
// already classified every part; mapping part kinds onto intent types is
// deterministic. This halves the per-message classification cost and removes
// a whole class of decomposer/intent-disagreement bugs.
func IntentSetFromDecomposition(d *Decomposition) *shared.IntentSet {
	if d == nil {
		return nil
	}
	set := &shared.IntentSet{OriginalPrompt: d.Summary}
	if set.OriginalPrompt == "" && len(d.Parts) > 0 {
		set.OriginalPrompt = d.Parts[0].Text
	}
	kindToIntent := map[PartKind]shared.IntentType{
		PartKindTask:      shared.IntentImplement,
		PartKindDirect:    shared.IntentDirect,
		PartKindQuestion:  shared.IntentQuestion,
		PartKindRule:      shared.IntentPlan, // standing instructions ride as plan-intent
		PartKindClarify:   shared.IntentQuestion,
		PartKindViolation: shared.IntentReview,
	}
	for _, p := range d.Parts {
		it, ok := kindToIntent[p.Kind]
		if !ok {
			continue // noise: no intent
		}
		// A task part's phase refines the intent type.
		if p.Kind == PartKindTask {
			switch p.Phase {
			case "explore":
				it = shared.IntentExplore
			case "plan":
				it = shared.IntentPlan
			case "build":
				it = shared.IntentImplement
			}
		}
		priority := p.Priority
		if priority == 0 {
			priority = 1
		}
		set.Intents = append(set.Intents, shared.Intent{
			ID:         p.ID,
			Type:       it,
			Priority:   priority,
			RawText:    p.Text,
			Confidence: p.Confidence,
		})
		set.RequiresInit = set.RequiresInit || p.Kind == PartKindTask
	}
	return set
}

// newTaskSpecFromPart builds a TaskSpec from a routed task part.
// phaseForPart resolves the arc phase for a task part: explicit phase wins;
// otherwise the task type maps (explore→explore, plan→plan, anything else
// →build). Never returns an invalid phase string.
func phaseForPart(p DecomposedPart) shared.AgentPhase {
	if p.Phase != "" {
		switch shared.AgentPhase(p.Phase) {
		case shared.PhaseExplore, shared.PhasePlan, shared.PhaseBuild:
			return shared.AgentPhase(p.Phase)
		}
	}
	switch p.TaskType {
	case "explore":
		return shared.PhaseExplore
	case "plan":
		return shared.PhasePlan
	default:
		return shared.PhaseBuild
	}
}

func newTaskSpecFromPart(p DecomposedPart, constraints []string) shared.TaskSpec {
	spec := shared.TaskSpec{
		ID:           p.ID,
		Type:         p.TaskType,
		Role:         p.Agent,
		Agent:        p.Agent,
		Description:  p.Text,
		Priority:     p.Priority,
		Dependencies: p.Dependencies,
		Phase:        phaseForPart(p),
		Context:      map[string]any{"reason": p.Reason},
	}
	if len(constraints) > 0 {
		spec.Context["constraints"] = constraints
	}
	return spec
}

// Decompose runs the decomposer on a first message. Returns nil (with a nil
// error) when the LLM is unavailable or its output is unparseable — callers
// fall back to the keyword router.
func (d *InitDecomposer) Decompose(ctx context.Context, message string, workingDir string, availableFiles []string) *Decomposition {
	if d == nil || d.llmClient == nil {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("User Message: ")
	sb.WriteString(message)
	sb.WriteString("\n\nWorking Directory: ")
	sb.WriteString(workingDir)
	if len(availableFiles) > 0 {
		sb.WriteString("\nAvailable Files (sample): ")
		sb.WriteString(strings.Join(availableFiles, ", "))
	}
	sb.WriteString("\n\nDecompose into parts and return JSON only.")

	response, err := d.llmClient.Complete(ctx, initDecomposerPrompt, sb.String())
	if err != nil {
		return nil
	}

	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil
	}
	var result Decomposition
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	if len(result.Parts) == 0 {
		return nil
	}

	// Normalize: every part gets an ID and a kind; tasks get defaults.
	for i := range result.Parts {
		p := &result.Parts[i]
		if p.ID == "" {
			p.ID = fmt.Sprintf("p%d", i+1)
		}
		if p.Priority == 0 {
			p.Priority = 1
		}
		if p.Kind == PartKindTask {
			if p.Agent == "" {
				p.Agent = "main"
			}
			if p.Phase == "" {
				p.Phase = p.TaskType
			}
			switch p.Phase {
			case string(shared.PhaseExplore), string(shared.PhasePlan), string(shared.PhaseBuild):
			default:
				p.Phase = string(shared.PhaseExplore)
			}
		}
	}
	return &result
}
