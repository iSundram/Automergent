package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

type firstMessageRecordingProvider struct {
	userPrompts []string
	toolCounts  []int
	systems     []string
	routerJSON  string
}

func (p *firstMessageRecordingProvider) Name() string { return "test-provider" }

func (p *firstMessageRecordingProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	p.userPrompts = append(p.userPrompts, firstUserPrompt(req.Messages))
	p.toolCounts = append(p.toolCounts, len(req.Tools))
	p.systems = append(p.systems, req.System)
	if strings.Contains(req.System, "tool personalization only") {
		category := p.routerJSON
		if category == "" {
			category = `{"category":"question"}`
		}
		return ai.NewStaticResponse(category, "", nil, ai.StopReasonEnd, ai.Usage{}), nil
	}
	return ai.NewStaticResponse("ok", "", nil, ai.StopReasonEnd, ai.Usage{}), nil
}

func (p *firstMessageRecordingProvider) Models(context.Context) ([]ai.Model, error) {
	return nil, nil
}

func (p *firstMessageRecordingProvider) TokenCount(messages []ai.Message) (int, error) {
	return ai.ApproximateTokenCount(messages), nil
}

func (p *firstMessageRecordingProvider) ContextLimit() int { return 128000 }

func firstUserPrompt(messages []ai.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == ai.RoleUser {
			return msg.TextContent()
		}
	}
	return ""
}

func newFirstMessageTestAgent(provider ai.Provider) *Agent {
	return &Agent{
		cfg:                 &config.Config{PromptSystemEnabled: true},
		provider:            provider,
		sess:                session.New(),
		tools:               tools.NewRegistry(),
		events:              make(chan Event, 128),
		sessionAllowedTools: map[string]bool{},
		promptSystem:        promptpkg.NewPromptSystem(),
	}
}

func TestRunFirstMessagePreservesOriginalPrompt(t *testing.T) {
	provider := &firstMessageRecordingProvider{}
	ag := newFirstMessageTestAgent(provider)

	firstPrompt := "Summarize repository architecture."
	if err := ag.Run(context.Background(), firstPrompt); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if err := ag.Run(context.Background(), "Second request."); err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	if len(provider.userPrompts) != 2 {
		t.Fatalf("expected one main provider call for each tool-less request, got %d", len(provider.userPrompts))
	}
	if provider.userPrompts[0] != firstPrompt {
		t.Fatalf("expected original first prompt on router request, got %q", provider.userPrompts[0])
	}
	if provider.userPrompts[1] != "Second request." {
		t.Fatalf("main prompt mismatch: %v", provider.userPrompts)
	}

	if got := ag.sess.Messages[0].TextContent(); got != firstPrompt {
		t.Fatalf("expected first stored user message to be restored prompt %q, got %q", firstPrompt, got)
	}
}

func TestGreetingDoesNotCallProviderOrExposeTools(t *testing.T) {
	provider := &firstMessageRecordingProvider{}
	ag := newFirstMessageTestAgent(provider)
	ag.cfg.PromptSystemEnabled = true

	if err := ag.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(provider.userPrompts) != 0 || len(provider.toolCounts) != 0 {
		t.Fatalf("greeting reached provider: prompts=%v tools=%v", provider.userPrompts, provider.toolCounts)
	}
	if len(ag.sess.Messages) != 2 {
		t.Fatalf("message count = %d, want user and assistant only", len(ag.sess.Messages))
	}
	if got := ag.sess.Messages[0].TextContent(); got != "hi" {
		t.Fatalf("stored greeting = %q", got)
	}
}

func TestInvestigationKeepsReadToolsAvailable(t *testing.T) {
	provider := &firstMessageRecordingProvider{routerJSON: `{"category":"issue_investigation"}`}
	ag := newFirstMessageTestAgent(provider)
	for _, name := range []string{"read_file", "search", "context_bucket_list"} {
		ag.tools.Register(testSchemaTool{name: name})
	}
	ag.toolProfile = ag.selectToolProfile(context.Background(), "Read files related to the search tool and tell issues if any")
	schemas := ag.buildActiveToolSchemas()
	if len(schemas) == 0 {
		t.Fatal("investigation exposed no tools")
	}
	want := map[string]bool{"read_file": true, "search": true, "context_bucket_list": true}
	got := map[string]bool{}
	for _, schema := range schemas {
		got[schema.Name] = true
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("investigation missing tool %q; tools=%v", name, got)
		}
	}
}

func TestToolCategoryIsEphemeral(t *testing.T) {
	provider := &firstMessageRecordingProvider{routerJSON: `{"category":"bug_fix"}`}
	ag := newFirstMessageTestAgent(provider)
	ag.tools.Register(testSchemaTool{name: "edit_file"})
	if err := ag.Run(context.Background(), "Fix the broken parser"); err != nil {
		t.Fatal(err)
	}
	if ag.toolProfile != nil {
		t.Fatal("tool category/profile survived the request")
	}
	if len(provider.toolCounts) != 2 || provider.toolCounts[0] != 0 || provider.toolCounts[1] != 1 {
		t.Fatalf("expected private router call then personalized main call, tool counts=%v", provider.toolCounts)
	}
	for _, message := range ag.sess.Messages {
		if strings.Contains(message.TextContent(), "bug_fix") {
			t.Fatalf("category leaked into session: %q", message.TextContent())
		}
	}
	if len(provider.systems) != 2 || strings.Contains(provider.systems[1], "bug_fix") {
		t.Fatalf("category leaked into main system prompt: %v", provider.systems)
	}
}

type testSchemaTool struct{ name string }

func (t testSchemaTool) Name() string           { return t.name }
func (t testSchemaTool) Description() string    { return t.name }
func (t testSchemaTool) Schema() map[string]any { return map[string]any{"type": "object"} }
func (t testSchemaTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (t testSchemaTool) RequiresConfirmation(string) bool      { return false }
func (t testSchemaTool) IsConcurrencySafe(map[string]any) bool { return true }
func (t testSchemaTool) IsReadOnly(map[string]any) bool        { return true }
func (t testSchemaTool) IsDestructive(map[string]any) bool     { return false }
func (t testSchemaTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{} }

func TestRunFirstMessagePreservesDelimiterTextInUserPrompt(t *testing.T) {
	provider := &firstMessageRecordingProvider{}
	ag := newFirstMessageTestAgent(provider)

	prompt := "Please keep literal text User Request: in output."
	if err := ag.Run(context.Background(), prompt); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if got := ag.sess.Messages[0].TextContent(); got != prompt {
		t.Fatalf("expected exact prompt preservation, got %q", got)
	}
}
