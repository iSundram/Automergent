package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
)

type mockProvider struct {
	ai.Provider
	summaryResponse string
}

func (m *mockProvider) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	return &mockResponse{text: m.summaryResponse}, nil
}

func (m *mockProvider) TokenCount(messages []ai.Message) (int, error) {
	return ai.ApproximateTokenCount(messages), nil
}

func (m *mockProvider) ContextLimit() int {
	return 100000
}

type mockResponse struct {
	ai.CompletionResponse
	text string
}

func (m *mockResponse) Stream() <-chan ai.Chunk {
	ch := make(chan ai.Chunk, 1)
	ch <- ai.Chunk{Text: m.text, Done: true}
	close(ch)
	return ch
}

func (m *mockResponse) ToolCalls() []ai.ToolCall    { return nil }
func (m *mockResponse) StopReason() ai.StopReason   { return ai.StopReasonEnd }
func (m *mockResponse) Usage() ai.Usage             { return ai.Usage{} }
func (m *mockResponse) GetMetadata() map[string]any { return nil }

func TestGhostLargeOutputs(t *testing.T) {
	ag := &Agent{
		cfg: &config.Config{MaxToolOutputChars: 100},
	}

	messages := []ai.Message{
		{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{
				{
					Type: ai.ContentTypeToolResult,
					ToolResult: &ai.ToolResult{
						ToolCallID: "call_1",
						Content:    strings.Repeat("a", 10000),
					},
				},
			},
		},
	}

	ghosted := ag.GhostLargeOutputs(messages)
	content := ghosted[0].Content[0].ToolResult.Content
	if !strings.Contains(content, "Output too large") {
		t.Fatalf("expected output to be ghosted, got: %s", content)
	}
	if len(content) >= 10000 {
		t.Fatalf("expected ghosted content to be smaller than original, got len %d", len(content))
	}
}

func TestCompactSessionMessagesNeural(t *testing.T) {
	mock := &mockProvider{summaryResponse: "Neural Summary of coding session"}
	ag := &Agent{
		cfg:      &config.Config{CompressionKeepRecent: 2},
		provider: mock,
		sess:     session.New(),
	}

	// Create 15 messages to trigger compaction (needs > 12)
	messages := make([]ai.Message, 15)
	messages[0] = ai.NewTextMessage(ai.RoleUser, "Initial intent")
	for i := 1; i < 14; i++ {
		messages[i] = ai.NewTextMessage(ai.RoleAssistant, "Normal message")
	}
	// Add an important message in the middle
	messages[5] = ai.NewTextMessage(ai.RoleUser, "This is a constraint that must be followed")
	messages[14] = ai.NewTextMessage(ai.RoleAssistant, "Most recent message")

	compacted := ag.CompactSessionMessages(context.Background(), messages)

	// Check structure: [0] Initial, [1] Neural Summary, [2] Preserved Context Header, [3] Important Message, [4] Recent 1, [5] Recent 2
	if len(compacted) >= 15 {
		t.Fatalf("expected fewer messages after compaction, got %d", len(compacted))
	}

	if compacted[0].TextContent() != "Initial intent" {
		t.Fatalf("expected first message to be preserved")
	}

	foundSummary := false
	foundImportant := false
	for _, m := range compacted {
		if strings.Contains(m.TextContent(), "Neural Context Summary") {
			foundSummary = true
		}
		if strings.Contains(m.TextContent(), "This is a constraint") {
			foundImportant = true
		}
	}

	if !foundSummary {
		t.Errorf("Neural Summary message not found in compacted history")
	}
	if !foundImportant {
		t.Errorf("Important message (constraint) not preserved in compacted history")
	}

	// Check recent messages are preserved at the end
	lastMsg := compacted[len(compacted)-1].TextContent()
	if lastMsg != "Most recent message" {
		t.Errorf("expected most recent message to be at the end, got %q", lastMsg)
	}
}
