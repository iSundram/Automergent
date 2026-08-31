package agent

import (
	"context"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
)

// fakeTitleProvider records whether Complete was called and returns a
// configurable error or streamed text.
type fakeTitleProvider struct {
	name    string
	err     error
	text    string
	called  bool
	lastReq ai.CompletionRequest
}

func (f *fakeTitleProvider) Name() string { return f.name }
func (f *fakeTitleProvider) Models(ctx context.Context) ([]ai.Model, error) {
	return nil, nil
}
func (f *fakeTitleProvider) TokenCount(messages []ai.Message) (int, error) {
	return 0, nil
}
func (f *fakeTitleProvider) ContextLimit() int { return 1000 }

func (f *fakeTitleProvider) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	f.called = true
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return &fakeTitleResponse{provider: f}, nil
}

type fakeTitleResponse struct {
	provider *fakeTitleProvider
}

func (r *fakeTitleResponse) Stream() <-chan ai.Chunk {
	ch := make(chan ai.Chunk, 1)
	if r.provider.text != "" {
		ch <- ai.Chunk{Text: r.provider.text, Done: true}
	} else {
		ch <- ai.Chunk{Error: context.Canceled, Done: true}
	}
	close(ch)
	return ch
}
func (r *fakeTitleResponse) ToolCalls() []ai.ToolCall { return nil }
func (r *fakeTitleResponse) StopReason() ai.StopReason {
	return ai.StopReasonEnd
}
func (r *fakeTitleResponse) Usage() ai.Usage             { return ai.Usage{} }
func (r *fakeTitleResponse) GetMetadata() map[string]any { return nil }

// TestTitleRequestPinsMinimalThinking pins the request shape that makes title
// generation work at all: reasoning models spend their entire MaxOutputTokens
// budget thinking before writing a word, so a title request without minimal
// thinking returns empty text and every ladder rung silently fails.
func TestTitleRequestPinsMinimalThinking(t *testing.T) {
	a := &Agent{cfg: &config.Config{}}
	rung := &fakeTitleProvider{name: "rung", text: "Fix login bug"}
	a.titleLadder = []ai.Provider{rung}
	a.provider = &fakeTitleProvider{name: "active", text: "nope"}
	a.titleOnce.Do(func() {})

	a.GenerateSessionTitle(context.Background(), []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "fix the login bug"),
	})

	req := rung.lastReq
	if req.Thinking == nil {
		t.Fatal("title request must pin Thinking — without it reasoning models burn MaxOutputTokens before emitting text")
	}
	if req.Thinking.BudgetTokens > 1 {
		t.Fatalf("BudgetTokens must be ≤1 for Gemini 2.5 rungs, got %d", req.Thinking.BudgetTokens)
	}
	if req.Thinking.ThinkingLevel != "minimal" && req.Thinking.Effort != "minimal" {
		t.Fatalf("a minimal effort/level must be set for Gemini 3.x rungs, got level=%q effort=%q",
			req.Thinking.ThinkingLevel, req.Thinking.Effort)
	}
	if req.MaxTokens < 32 {
		t.Fatalf("MaxTokens must leave room for the title after thinking, got %d", req.MaxTokens)
	}
	if req.MaxTokens > 256 {
		t.Fatalf("title requests stay cheap, got MaxTokens=%d", req.MaxTokens)
	}
}

// TestGenerateSessionTitleLadderOrder verifies the cheap-model cascade: the
// first rung that produces usable text wins and no later rung (including the
// active provider) is tried.
func TestGenerateSessionTitleLadderOrder(t *testing.T) {
	a := &Agent{cfg: &config.Config{}}

	rung1 := &fakeTitleProvider{name: "rung1", err: context.Canceled}
	rung2 := &fakeTitleProvider{name: "rung2", text: "Fix login bug"}
	rung3 := &fakeTitleProvider{name: "rung3", text: "should not be reached"}
	active := &fakeTitleProvider{name: "active", text: "should not be reached"}

	a.titleLadder = []ai.Provider{rung1, rung2, rung3}
	a.provider = active
	// Mark the lazy ladder build as done so the injected rungs survive
	// titleProviders' titleOnce.Do (which would otherwise rebuild from the
	// empty test config and drop them).
	a.titleOnce.Do(func() {})

	messages := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "Please fix the login bug in auth.go"),
		ai.NewTextMessage(ai.RoleAssistant, "I'll look at the auth code."),
	}
	title := a.GenerateSessionTitle(context.Background(), messages)

	if title != "Fix login bug" {
		t.Fatalf("expected title from rung2, got %q", title)
	}
	if !rung1.called || !rung2.called {
		t.Fatalf("expected rungs 1 and 2 to be tried")
	}
	if rung3.called || active.called {
		t.Fatalf("later rungs must not be tried after a rung succeeds")
	}
}

// TestGenerateSessionTitleFallsBackToActiveProvider verifies the last resort:
// when every ladder rung fails, the user's active model is used.
func TestGenerateSessionTitleFallsBackToActiveProvider(t *testing.T) {
	a := &Agent{cfg: &config.Config{}}

	rung1 := &fakeTitleProvider{name: "rung1", err: context.Canceled}
	rung2 := &fakeTitleProvider{name: "rung2", text: ""} // empty stream -> unusable
	active := &fakeTitleProvider{name: "active", text: "Active model title"}

	a.titleLadder = []ai.Provider{rung1, rung2}
	a.provider = active
	a.titleOnce.Do(func() {})

	messages := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "Refactor the config loader"),
	}
	title := a.GenerateSessionTitle(context.Background(), messages)

	if title != "Active model title" {
		t.Fatalf("expected title from active provider, got %q", title)
	}
	if !active.called {
		t.Fatal("active provider must be tried when the ladder fails")
	}
}

// TestGenerateSessionTitleNoUserMessage verifies the guard: without a user
// message there is nothing to summarize, and no provider is contacted.
func TestGenerateSessionTitleNoUserMessage(t *testing.T) {
	a := &Agent{cfg: &config.Config{}}
	active := &fakeTitleProvider{name: "active", text: "nope"}
	a.provider = active

	title := a.GenerateSessionTitle(context.Background(), []ai.Message{
		ai.NewTextMessage(ai.RoleAssistant, "hello"),
	})
	if title != "" {
		t.Fatalf("expected empty title, got %q", title)
	}
	if active.called {
		t.Fatal("no provider should be contacted without a user message")
	}
}

// TestGenerateSessionTitleAllFailSilent verifies total failure yields "" so
// callers keep their deterministic fallback (first user line).
func TestGenerateSessionTitleAllFailSilent(t *testing.T) {
	a := &Agent{cfg: &config.Config{}}
	rung1 := &fakeTitleProvider{name: "rung1", err: context.Canceled}
	active := &fakeTitleProvider{name: "active", err: context.Canceled}
	a.titleLadder = []ai.Provider{rung1}
	a.provider = active
	a.titleOnce.Do(func() {})

	title := a.GenerateSessionTitle(context.Background(), []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "do something"),
	})
	if title != "" {
		t.Fatalf("expected empty title on total failure, got %q", title)
	}
}
