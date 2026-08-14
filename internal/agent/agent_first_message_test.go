package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

type firstMessageRecordingProvider struct {
	userPrompts []string
}

func (p *firstMessageRecordingProvider) Name() string { return "test-provider" }

func (p *firstMessageRecordingProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	p.userPrompts = append(p.userPrompts, firstUserPrompt(req.Messages))
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
		cfg:                 &config.Config{},
		provider:            provider,
		sess:                session.New(),
		tools:               tools.NewRegistry(),
		events:              make(chan Event, 128),
		sessionAllowedTools: map[string]bool{},
	}
}

func TestRunFirstMessageUsesTemporaryTriageAndRestoresOriginalPrompt(t *testing.T) {
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
		t.Fatalf("expected 2 provider calls, got %d", len(provider.userPrompts))
	}
	if !strings.HasPrefix(provider.userPrompts[0], TriageInstruction+"\n\nUser Request: ") {
		t.Fatalf("expected triage wrapper on first provider prompt, got %q", provider.userPrompts[0])
	}
	if provider.userPrompts[1] != "Second request." {
		t.Fatalf("expected no triage wrapper on second provider prompt, got %q", provider.userPrompts[1])
	}

	if got := ag.sess.Messages[0].TextContent(); got != firstPrompt {
		t.Fatalf("expected first stored user message to be restored prompt %q, got %q", firstPrompt, got)
	}
}

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
