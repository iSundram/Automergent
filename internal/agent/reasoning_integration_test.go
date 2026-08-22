package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/config"
)

func TestRunReasoningPreAnalysisEnabledInvokesAnalyzerAndEmitsStatus(t *testing.T) {
	provider := &firstMessageRecordingProvider{}
	ag := newFirstMessageTestAgent(provider)
	ag.cfg = &config.Config{ReasoningPreAnalysis: true}

	called := 0
	var analyzedPrompt string
	ag.reasoningPreAnalyze = func(_ context.Context, prompt string) (string, error) {
		called++
		analyzedPrompt = prompt
		return "feature/multi_file", nil
	}

	prompt := "Implement feature X."
	if err := ag.Run(context.Background(), prompt); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected analyzer to be called once, got %d", called)
	}
	if analyzedPrompt != prompt {
		t.Fatalf("expected analyzer prompt %q, got %q", prompt, analyzedPrompt)
	}
	// Prompt system makes 3 calls: intent identification, task planning, then main call
	if len(provider.userPrompts) != 3 {
		t.Fatalf("expected 3 provider calls (intent+planning+main), got %d", len(provider.userPrompts))
	}

	statuses := drainStatusEvents(ag.events)
	assertContainsStatus(t, statuses, "reasoning: pre-analyzing prompt")
	assertContainsStatus(t, statuses, "reasoning: feature/multi_file")
}

func TestRunReasoningPreAnalysisErrorDoesNotBlockProvider(t *testing.T) {
	provider := &firstMessageRecordingProvider{}
	ag := newFirstMessageTestAgent(provider)
	ag.cfg = &config.Config{ReasoningPreAnalysis: true}

	called := 0
	ag.reasoningPreAnalyze = func(_ context.Context, _ string) (string, error) {
		called++
		return "", errors.New("boom")
	}

	if err := ag.Run(context.Background(), "Still continue."); err != nil {
		t.Fatalf("run should continue on reasoning error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected analyzer to be called once, got %d", called)
	}
	// Prompt system makes 3 calls: intent identification, task planning, then main call
	if len(provider.userPrompts) != 3 {
		t.Fatalf("expected 3 provider calls (intent+planning+main), got %d", len(provider.userPrompts))
	}

	statuses := drainStatusEvents(ag.events)
	assertContainsStatus(t, statuses, "reasoning: unavailable, continuing")
}

func drainStatusEvents(ch <-chan Event) []string {
	statuses := []string{}
	for {
		select {
		case evt := <-ch:
			if evt.Type != EventStatus {
				continue
			}
			if s, ok := evt.Payload.(string); ok {
				statuses = append(statuses, s)
			}
		default:
			return statuses
		}
	}
}

func assertContainsStatus(t *testing.T, statuses []string, want string) {
	t.Helper()
	for _, s := range statuses {
		if strings.Contains(s, want) {
			return
		}
	}
	t.Fatalf("expected status containing %q, got %v", want, statuses)
}
