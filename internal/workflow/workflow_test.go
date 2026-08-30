package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakePorts records agent calls and can replay scripted results.
type fakePorts struct {
	mu      sync.Mutex
	calls   []AgentParams
	results map[string]AgentResult // keyed by prompt prefix
	errFor  map[string]error
	events  []ProgressEvent
	journal JournalStore
	// live tracks concurrent executions to verify the concurrency cap.
	live, maxLive int
	cap           int
}

func newFakePorts(dir string) *fakePorts {
	return &fakePorts{
		results: map[string]AgentResult{},
		errFor:  map[string]error{},
		journal: NewFileJournalStore(dir),
	}
}

func (f *fakePorts) RunAgent(ctx context.Context, params AgentParams) (AgentResult, error) {
	f.mu.Lock()
	f.live++
	if f.live > f.maxLive {
		f.maxLive = f.live
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.live--
		f.mu.Unlock()
	}()

	if f.cap > 0 {
		f.mu.Lock()
		over := f.live > f.cap
		f.mu.Unlock()
		if over {
			return AgentResult{}, errors.New("concurrency cap exceeded")
		}
	}

	f.mu.Lock()
	f.calls = append(f.calls, params)
	res, hasResult := f.results[params.Prompt]
	err := f.errFor[params.Prompt]
	f.mu.Unlock()
	if err != nil {
		return AgentResult{}, err
	}
	if hasResult {
		return res, nil
	}
	return AgentResult{Output: "out:" + params.Prompt, OutputTokens: 10}, nil
}

func (f *fakePorts) Journal() JournalStore { return f.journal }

func (f *fakePorts) Progress(ev ProgressEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
}

func (f *fakePorts) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

const linearYAML = `
name: linear
steps:
  - id: first
    prompt: "do the first thing"
  - id: second
    prompt: "then do ${first} and more"
    dependsOn: [first]
`

func TestRunExecutesInDependencyOrder(t *testing.T) {
	dir := t.TempDir()
	p := newFakePorts(dir)
	spec, err := ParseSpec([]byte(linearYAML))
	if err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), spec, "", p)
	if res.Status != "completed" {
		t.Fatalf("status = %q, err = %q", res.Status, res.Error)
	}
	if got := p.calls[0].Prompt; got != "do the first thing" {
		t.Fatalf("first call = %q", got)
	}
	if got := p.calls[1].Prompt; got != "then do out:do the first thing and more" {
		t.Fatalf("dependency output not substituted: %q", got)
	}
	if res.Outputs["second"] != "out:then do out:do the first thing and more" {
		t.Fatalf("final output missing: %v", res.Outputs)
	}
	if res.TotalOutputTokens != 20 {
		t.Fatalf("tokens = %d, want 20", res.TotalOutputTokens)
	}
}

func TestRunResumesFromJournal(t *testing.T) {
	dir := t.TempDir()
	p := newFakePorts(dir)
	spec, err := ParseSpec([]byte(linearYAML))
	if err != nil {
		t.Fatal(err)
	}

	first := Run(context.Background(), spec, "", p, WithRunID("resume-test"))
	if first.Status != "completed" {
		t.Fatalf("first run failed: %q", first.Error)
	}
	if n := p.callCount(); n != 2 {
		t.Fatalf("first run made %d calls, want 2", n)
	}

	// Second run, same run ID, resume: every key matches the journal, so no
	// agent is invoked and outputs are replayed.
	second := Run(context.Background(), spec, "", p, WithRunID("resume-test"), WithResume(true))
	if second.Status != "completed" {
		t.Fatalf("resumed run failed: %q", second.Error)
	}
	if n := p.callCount(); n != 2 {
		t.Fatalf("resume re-ran agents: %d calls, want 2", n)
	}
	if second.Outputs["second"] != first.Outputs["second"] {
		t.Fatalf("replayed output differs: %q vs %q", second.Outputs["second"], first.Outputs["second"])
	}
	if second.TotalOutputTokens != 0 {
		t.Fatalf("replayed steps must cost nothing, got %d tokens", second.TotalOutputTokens)
	}
}

const fanoutYAML = `
name: fanout
concurrency: 2
steps:
  - id: a
    prompt: "task a"
  - id: b
    prompt: "task b"
  - id: c
    prompt: "task c"
  - id: join
    prompt: "join ${a} ${b} ${c}"
    dependsOn: [a, b, c]
`

func TestRunParallelStepsAndJoin(t *testing.T) {
	dir := t.TempDir()
	p := newFakePorts(dir)
	p.cap = 2 // ports itself enforces the cap to verify the engine respects it
	spec, err := ParseSpec([]byte(fanoutYAML))
	if err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), spec, "", p)
	if res.Status != "completed" {
		t.Fatalf("status = %q, err = %q", res.Status, res.Error)
	}
	if got := p.calls[3].Prompt; !strings.Contains(got, "out:task a") || !strings.Contains(got, "out:task c") {
		t.Fatalf("join step missing dependency outputs: %q", got)
	}
	if p.maxLive > 2 {
		t.Fatalf("concurrency cap violated: %d simultaneous", p.maxLive)
	}
}

func TestRunStepFailureFailsRun(t *testing.T) {
	dir := t.TempDir()
	p := newFakePorts(dir)
	p.errFor["task a"] = errors.New("agent exploded")
	spec, err := ParseSpec([]byte(fanoutYAML))
	if err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), spec, "", p)
	if res.Status != "failed" {
		t.Fatalf("status = %q, want failed", res.Status)
	}
	if !strings.Contains(res.Error, `"a"`) || !strings.Contains(res.Error, "agent exploded") {
		t.Fatalf("error should name the failed step and cause: %q", res.Error)
	}
}

func TestRunBudgetAborts(t *testing.T) {
	dir := t.TempDir()
	p := newFakePorts(dir)
	specYAML := strings.Replace(fanoutYAML, "concurrency: 2", "concurrency: 2\nbudget: 5", 1)
	spec, err := ParseSpec([]byte(specYAML))
	if err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), spec, "", p)
	if res.Status == "completed" {
		t.Fatal("budget of 5 tokens with 10-token steps must not complete")
	}
}

func TestParseSpecValidation(t *testing.T) {
	cases := []struct {
		name, yaml, wantErr string
	}{
		{"no name", "steps:\n  - id: a\n    prompt: x", "name"},
		{"no steps", "name: empty", "no steps"},
		{"dup id", "name: d\nsteps:\n  - id: a\n    prompt: x\n  - id: a\n    prompt: y", "duplicate"},
		{"unknown dep", "name: u\nsteps:\n  - id: a\n    prompt: x\n    dependsOn: [ghost]", "unknown"},
		{"cycle", "name: c\nsteps:\n  - id: a\n    prompt: x\n    dependsOn: [b]\n  - id: b\n    prompt: y\n    dependsOn: [a]", "cycle"},
	}
	for _, tc := range cases {
		_, err := ParseSpec([]byte(tc.yaml))
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestJournalTornLineTolerated(t *testing.T) {
	dir := t.TempDir()
	store := NewFileJournalStore(dir)
	if err := store.Append("r1", JournalEntry{Key: "k1", Seq: 1, Step: "a", Output: "hello"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-write: half a JSON line.
	path := filepath.Join(dir, "r1", "journal.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"key":"k2","seq":2,"ste`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	entries, err := store.Read("r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key != "k1" {
		t.Fatalf("torn line should be skipped, got %v", entries)
	}
}

func TestAgentCallKeyDeterministic(t *testing.T) {
	a := agentCallKey("same prompt", AgentParams{AgentType: "explore"})
	b := agentCallKey("same prompt", AgentParams{AgentType: "explore"})
	if a != b {
		t.Fatal("identical params must produce identical keys")
	}
	c := agentCallKey("same prompt", AgentParams{AgentType: "review"})
	if a == c {
		t.Fatal("different agent types must produce different keys")
	}
	d := agentCallKey("other prompt", AgentParams{AgentType: "explore"})
	if a == d {
		t.Fatal("different prompts must produce different keys")
	}
}

func TestExpandPrompt(t *testing.T) {
	out := expandPrompt("use ${a} and ${b} with $ARGUMENTS", "extra",
		map[string]string{"a": "A", "b": "B"})
	if out != "use A and B with extra" {
		t.Fatalf("expansion = %q", out)
	}
}

var _ = fmt.Sprint // keep fmt for future assertions
