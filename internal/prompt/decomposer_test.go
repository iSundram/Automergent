package prompt

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
)

// decomposerFixture is a vision example: a message and the parts the
// decomposer must find in it.
type decomposerFixture struct {
	name    string
	message string
	parts   string // JSON array fragment for the parts
}

func runDecomposer(t *testing.T, fx decomposerFixture) *Decomposition {
	t.Helper()
	client := &MockLLMClient{Response: `{"parts": ` + fx.parts + `, "constraints": []}`}
	d := NewInitDecomposer(client)
	got := d.Decompose(context.Background(), fx.message, "/tmp", nil)
	if got == nil {
		t.Fatalf("decomposer returned nil for %q", fx.message)
	}
	return got
}

func TestDecomposerWhoAreYou(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "hey who are you",
		parts:   `[{"id":"p1","text":"hey who are you","kind":"direct","answer_style":"about-me"}]`,
	})
	direct := got.DirectParts()
	if len(direct) != 1 || direct[0].AnswerStyle != "about-me" {
		t.Fatalf("expected one about-me direct part, got %+v", direct)
	}
	if len(got.TaskParts()) != 0 {
		t.Fatalf("who-are-you must not route tasks, got %+v", got.TaskParts())
	}
}

func TestDecomposerWhatDoesXDo(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "hey what does X do",
		parts:   `[{"id":"p1","text":"hey what does X do","kind":"direct","answer_style":"concise"}]`,
	})
	if len(got.DirectParts()) != 1 {
		t.Fatalf("expected direct part, got %+v", got.Parts)
	}
}

func TestDecomposerTellMeWhatGoogleDoes(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "tell me what Google does",
		parts:   `[{"id":"p1","text":"tell me what Google does","kind":"question","scope":"general"}]`,
	})
	// General-knowledge question: INIT answers it, no tasks.
	if len(got.DirectParts()) != 1 {
		t.Fatalf("expected one general question answered by init, got %+v", got.Parts)
	}
	if len(got.TaskParts()) != 0 {
		t.Fatalf("general question must not route tasks, got %+v", got.TaskParts())
	}
}

func TestDecomposerCloneRepoAndSeeHowXWorks(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "hey clone repo X and see how X is working, I think Y is wrong, and never use tabs",
		parts: `[{"id":"p1","text":"clone repo X","kind":"direct"},
			{"id":"p2","text":"see how X is working","kind":"task","task_type":"explore","phase":"explore","agent":"explore","priority":1},
			{"id":"p3","text":"I think Y is wrong and has issues","kind":"task","task_type":"explore","phase":"explore","agent":"explore","priority":1,"dependencies":["p2"]},
			{"id":"p4","text":"never use tabs","kind":"rule","rule":"never use tabs","rule_action":"add"}]`,
	})

	if len(got.DirectParts()) != 1 {
		t.Fatalf("expected clone to be direct, got %+v", got.DirectParts())
	}
	tasks := got.TaskParts()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 explore tasks, got %+v", tasks)
	}
	if tasks[0].Phase != "explore" || tasks[0].Agent != "explore" {
		t.Fatalf("explore task misrouted: %+v", tasks[0])
	}
	if len(got.RuleParts()) != 1 || got.RuleParts()[0].RuleAction != "add" {
		t.Fatalf("expected one rule add, got %+v", got.RuleParts())
	}
}

func TestDecomposerViolationSuspect(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "make a tool that hacks X",
		parts: `[{"id":"p1","text":"make a tool that hacks X","kind":"violation_suspect",
			"violation_type":"hacking","needs_confirmation":true}]`,
	})
	violations := got.ViolationParts()
	if len(violations) != 1 || !violations[0].NeedsConfirmation {
		t.Fatalf("expected one violation_suspect needing confirmation, got %+v", violations)
	}
}

func TestDecomposerClarifyAmbiguous(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "see feature X and tell comprehensive plan",
		parts: `[{"id":"p1","text":"see feature X and tell comprehensive plan","kind":"clarify",
			"options":["explore X then produce a plan","produce a plan from what you already know"]}]`,
	})
	clarify := got.ClarifyParts()
	if len(clarify) != 1 || len(clarify[0].Options) != 2 {
		t.Fatalf("expected clarify with 2 options, got %+v", clarify)
	}
}

func TestDecomposerNoiseAndConstraints(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "see files related to X and I like coffee and I don't sleep",
		parts: `[{"id":"p1","text":"see files related to X","kind":"task","task_type":"explore","phase":"explore","agent":"explore","priority":1},
			{"id":"p2","text":"I like coffee","kind":"noise"},
			{"id":"p3","text":"I don't sleep","kind":"noise"}]`,
	})
	if len(got.NoiseParts()) != 2 {
		t.Fatalf("expected 2 noise parts, got %+v", got.NoiseParts())
	}
	if len(got.TaskParts()) != 1 {
		t.Fatalf("expected 1 task, got %+v", got.TaskParts())
	}
}

func TestDecomposerUnparseableFallsBack(t *testing.T) {
	client := &MockLLMClient{Response: "I think you should explore the codebase first."}
	d := NewInitDecomposer(client)
	if got := d.Decompose(context.Background(), "anything", "/tmp", nil); got != nil {
		t.Fatalf("expected nil fallback on unparseable response, got %+v", got)
	}
}

func TestDecomposerErrorFallsBack(t *testing.T) {
	client := &MockLLMClient{Error: context.Canceled}
	d := NewInitDecomposer(client)
	if got := d.Decompose(context.Background(), "anything", "/tmp", nil); got != nil {
		t.Fatalf("expected nil fallback on error, got %+v", got)
	}
}

func TestDecomposerNormalizesMissingFields(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "fix the parser",
		parts:   `[{"text":"fix the parser","kind":"task","task_type":"build"}]`,
	})
	tasks := got.TaskParts()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %+v", tasks)
	}
	if tasks[0].ID == "" {
		t.Fatal("normalization must assign part IDs")
	}
	if tasks[0].Agent != "main" {
		t.Fatalf("default agent = %q, want main", tasks[0].Agent)
	}
	if tasks[0].Phase != string(shared.PhaseBuild) {
		t.Fatalf("default phase = %q, want build", tasks[0].Phase)
	}
}

func TestToTaskSpecsCarriesRouting(t *testing.T) {
	got := runDecomposer(t, decomposerFixture{
		message: "see files related to X and tell upgrade plan",
		parts: `[{"id":"p1","text":"see files related to X","kind":"task","task_type":"explore","phase":"explore","agent":"explore","priority":1},
			{"id":"p2","text":"tell upgrade plan","kind":"task","task_type":"plan","phase":"plan","agent":"main","priority":2,"dependencies":["p1"]}]`,
	})
	specs := got.ToTaskSpecs()
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Agent != "explore" || specs[0].ID != "p1" {
		t.Fatalf("first spec misrouted: %+v", specs[0])
	}
	// PHASE is the field executeDecomposition routes on. It used to be
	// dropped entirely, sending every task to build — explore never ran.
	if specs[0].Phase != shared.PhaseExplore {
		t.Fatalf("explore task phase = %q, want explore", specs[0].Phase)
	}
	if specs[1].Phase != shared.PhasePlan {
		t.Fatalf("plan task phase = %q, want plan", specs[1].Phase)
	}
	if len(specs[1].Dependencies) != 1 || specs[1].Dependencies[0] != "p1" {
		t.Fatalf("dependency lost: %+v", specs[1])
	}
}

func TestToTaskSpecsPhaseFallsBackToTaskType(t *testing.T) {
	// The LLM may omit "phase" and set only "task_type"; the spec must
	// still route to the right arc phase instead of defaulting to build.
	got := runDecomposer(t, decomposerFixture{
		message: "find the auth flow",
		parts:   `[{"id":"p1","text":"find the auth flow","kind":"task","task_type":"explore","agent":"explore","priority":1}]`,
	})
	specs := got.ToTaskSpecs()
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Phase != shared.PhaseExplore {
		t.Fatalf("task without explicit phase routed to %q, want explore", specs[0].Phase)
	}
}

func TestDecomposerJSONRoundTrip(t *testing.T) {
	// The struct tags must match the documented schema exactly.
	raw := `{"parts":[{"id":"p1","text":"t","kind":"task","task_type":"explore","phase":"explore","agent":"explore","priority":1,"dependencies":["p0"],"confidence":0.9,"reason":"r","scope":"codebase","answer_style":"concise","rule":"x","rule_action":"add","violation_type":"hacking","needs_confirmation":true,"options":["a","b"]}],"requires_clarification":false,"clarification_question":"","constraints":["c"],"summary":"s"}`
	var d Decomposition
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("schema mismatch: %v", err)
	}
	if d.Parts[0].NeedsConfirmation != true || d.Parts[0].RuleAction != "add" {
		t.Fatalf("round trip lost fields: %+v", d.Parts[0])
	}
}

func TestPhasePromptFilesContent(t *testing.T) {
	cases := []struct {
		phase shared.AgentPhase
		want  []string
	}{
		{shared.PhaseInit, []string{"MISSION", "no todo tools", "EXIT"}},
		{shared.PhaseExplore, []string{"MISSION", "read-only", "path:line"}},
		{shared.PhasePlan, []string{"MISSION", "### Objective", "Risks"}},
		{shared.PhaseBuild, []string{"MISSION", "todo", "tests"}},
	}
	for _, tc := range cases {
		p := PhasePrompt(tc.phase)
		if p == "" {
			t.Fatalf("phase %s has no prompt file", tc.phase)
		}
		lower := strings.ToLower(p)
		for _, want := range tc.want {
			if !strings.Contains(lower, strings.ToLower(want)) {
				t.Errorf("phase %s prompt missing %q", tc.phase, want)
			}
		}
	}
	if PhasePrompt(shared.AgentPhase("nonexistent")) != "" {
		t.Error("unknown phase must return empty prompt")
	}
}

func TestRenderBehavioralPromptsSplitsDimensions(t *testing.T) {
	out := RenderBehavioralPrompts(shared.PhaseBuild, []string{"custom rule"})
	for _, want := range []string{"Phase discipline", "Context hygiene", "Verification", "Safety & honesty", "Agent-specific", "custom rule"} {
		if !strings.Contains(out, want) {
			t.Errorf("behavioral layer missing %q", want)
		}
	}
	// Build-specific rules appear only for build.
	outInit := RenderBehavioralPrompts(shared.PhaseInit, nil)
	if strings.Contains(outInit, "Todo discipline") {
		t.Error("init must not carry build's todo discipline")
	}
}

func TestRenderToolPromptsRegistryPriority(t *testing.T) {
	// A tool with Meta() in the registry is skipped from the fallback layer.
	reg := newTestRegistryWithMeta()
	out := RenderToolPromptsFromRegistry(reg, []string{"meta_tool", "bash"}, nil)
	if strings.Contains(out, "meta_tool") {
		t.Error("self-documented tool must not appear in fallback layer")
	}
	if !strings.Contains(out, "### bash") {
		t.Error("bash guidance must appear")
	}
}

// --- test helpers for the registry-priority test ---

type metaTestTool struct{ name string }

func (t *metaTestTool) Name() string                          { return t.name }
func (t *metaTestTool) Description() string                   { return t.name }
func (t *metaTestTool) Schema() map[string]any                { return map[string]any{"type": "object"} }
func (t *metaTestTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (t *metaTestTool) RequiresConfirmation(string) bool      { return false }
func (t *metaTestTool) IsConcurrencySafe(map[string]any) bool { return true }
func (t *metaTestTool) IsReadOnly(map[string]any) bool        { return true }
func (t *metaTestTool) IsDestructive(map[string]any) bool     { return false }
func (t *metaTestTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{} }

// Meta marks the tool as self-documenting: the registry layer owns its
// guidance, so the fallback layer must skip it.
func (t *metaTestTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{Category: "read", DisplayName: "Meta Tool", Usage: "self documented"}
}

type plainTestTool struct{ metaTestTool }

func (t *plainTestTool) Meta() *tools.ToolMeta { return nil }

func newTestRegistryWithMeta() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(&metaTestTool{name: "meta_tool"})
	reg.Register(&plainTestTool{metaTestTool{name: "plain_tool"}})
	return reg
}
