package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
)

// sharedPhaseExplore is a local alias keeping the mask test readable.
var (
	sharedPhaseExplore = shared.PhaseExplore
	sharedPhaseInit    = shared.PhaseInit
)

// toolSetForPhaseOrDefault mirrors the phase config's ToolSet resolution
// for the mask test.
func toolSetForPhaseOrDefault(phase shared.AgentPhase) shared.ToolSet {
	switch phase {
	case shared.PhaseExplore:
		return shared.ToolSetReadOnly
	case shared.PhaseInit:
		return shared.ToolSetBasic
	case shared.PhasePlan:
		return shared.ToolSetModerate
	default:
		return shared.ToolSetFull
	}
}

// decomposerRecordingProvider answers the INIT decomposer call with valid
// JSON (a direct part), and "ok" for everything else, recording every
// system prompt it saw.
type decomposerRecordingProvider struct {
	firstMessageRecordingProvider
	decomposerJSON string
	sawDecomposer  int
}

func (p *decomposerRecordingProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	if strings.Contains(req.System, "INIT decomposer") {
		p.sawDecomposer++
		return ai.NewStaticResponse(p.decomposerJSON, "", nil, ai.StopReasonEnd, ai.Usage{}), nil
	}
	return ai.NewStaticResponse("ok", "", nil, ai.StopReasonEnd, ai.Usage{}), nil
}

func newDecomposerTestAgent(provider *decomposerRecordingProvider) *Agent {
	toolReg := tools.NewRegistry()
	ag := &Agent{
		cfg:          &config.Config{},
		provider:     provider,
		sess:         session.New(),
		tools:        toolReg,
		events:       make(chan Event, 256),
		sessionGrants: newGrants(nil),
	}
	toolReg.Register(testSchemaTool{name: "read_file"})
	return ag
}

const directOnlyDecomposition = `{
  "parts": [{"id":"p1","text":"who are you","kind":"direct","answer_style":"about-me"}],
  "constraints": [],
  "summary": "direct question"
}`

func TestDecomposerRunsOnEveryMessage(t *testing.T) {
	provider := &decomposerRecordingProvider{
		decomposerJSON: directOnlyDecomposition,
	}
	ag := newDecomposerTestAgent(provider)

	// First message: decomposer fires and answers the direct part itself.
	if err := ag.Run(context.Background(), "who are you"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if provider.sawDecomposer == 0 {
		t.Fatal("decomposer did not fire on the first message")
	}
	first := provider.sawDecomposer

	// Second message: the arc must decompose AGAIN — per-message routing,
	// not once per session. This was the bug that left the arc dead after
	// the first turn.
	if err := ag.Run(context.Background(), "what can you do"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if provider.sawDecomposer <= first {
		t.Fatalf("decomposer did not fire on the second message (%d → %d) — the arc is dead after turn 1", first, provider.sawDecomposer)
	}
}

func TestChildrenBypassDecomposer(t *testing.T) {
	provider := &decomposerRecordingProvider{
		decomposerJSON: directOnlyDecomposition,
	}
	ag := newDecomposerTestAgent(provider)
	ag.decomposeDisabled = true // what Execute sets on children

	if err := ag.Run(context.Background(), "find the auth flow"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if provider.sawDecomposer != 0 {
		t.Fatalf("child agent ran the decomposer (%d calls) — children inherit their parent's routing", provider.sawDecomposer)
	}
}

func TestKeywordFallbackChainsBuildAfterExplore(t *testing.T) {
	// Provider returns "ok" for the decomposer: unparseable JSON → nil →
	// the keyword router takes over. A build request must then chain
	// EXPLORE → BUILD instead of stranding the arc at explore.
	provider := &decomposerRecordingProvider{
		decomposerJSON: "I will just answer directly without JSON",
	}
	ag := newDecomposerTestAgent(provider)

	if err := ag.Run(context.Background(), "build a tool that parses logs"); err != nil {
		t.Fatalf("run: %v", err)
	}

	classified := ag.phaseManager
	_ = classified
	// The phase history must show explore followed by build.
	var sawExplore, sawBuild bool
	for _, tr := range ag.phaseManager.PhaseHistory() {
		if tr.To == "explore" {
			sawExplore = true
		}
		if tr.To == "build" && sawExplore {
			sawBuild = true
		}
	}
	if !sawBuild {
		history := make([]string, 0)
		for _, tr := range ag.phaseManager.PhaseHistory() {
			history = append(history, string(tr.From)+"→"+string(tr.To))
		}
		t.Fatalf("keyword fallback did not reach BUILD; transitions: %v", history)
	}
}

func TestExplorePhaseMaskExcludesWriteTools(t *testing.T) {
	ag := newDecomposerTestAgent(&decomposerRecordingProvider{})
	ag.tools.Register(testSchemaTool{name: "edit_file"})
	ag.tools.Register(testSchemaTool{name: "write_file"})
	ag.tools.Register(testSchemaTool{name: "bash"})
	ag.tools.Register(testSchemaTool{name: "read_file"})
	ag.tools.Register(testSchemaTool{name: "grep"})

	profile := ag.toolSetToProfile(toolSetForPhaseOrDefault(sharedPhaseExplore))
	if profile["edit_file"] || profile["write_file"] {
		t.Fatal("EXPLORE profile exposes write tools — read-only phase is not enforced")
	}
	if !profile["read_file"] || !profile["grep"] {
		t.Fatal("EXPLORE profile lost its read tools")
	}

	// INIT has edits and task, never todos.
	initProfile := ag.toolSetToProfile(toolSetForPhaseOrDefault(sharedPhaseInit))
	if !initProfile["edit_file"] {
		t.Fatal("INIT profile missing edit_file — direct actions need it")
	}
	if initProfile["todo_write"] || initProfile["todo_list"] {
		t.Fatal("INIT profile exposes todo tools — the vision forbids them")
	}
}
