package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
)

// titleMaxChars bounds generated session titles.
const titleMaxChars = 60

// GenerateSessionTitle produces a short title for a conversation's opening
// exchange, used to auto-name sessions in the picker. It returns "" when the
// provider call fails or yields nothing usable, so callers can keep their
// deterministic fallback (the first user message) instead.
func (a *Agent) GenerateSessionTitle(ctx context.Context, messages []ai.Message) string {
	var user, assistant string
	for _, m := range messages {
		if user == "" && m.Role == ai.RoleUser {
			user = truncateTitle(m.PlaintextForHistory(), 800)
		}
		if assistant == "" && m.Role == ai.RoleAssistant {
			assistant = truncateTitle(m.PlaintextForHistory(), 800)
		}
		if user != "" && assistant != "" {
			break
		}
	}
	if user == "" {
		return ""
	}
	prompt := fmt.Sprintf(
		"Write a title of at most 6 words summarizing what this coding session is about.\n"+
			"Reply with the title only: no quotes, no punctuation at the end, no prefix.\n\n"+
			"User request:\n%s\n\nAssistant reply (for context):\n%s",
		user, assistant)

	req := ai.CompletionRequest{
		Messages:    []ai.Message{ai.NewTextMessage(ai.RoleUser, prompt)},
		System:      "You write terse, specific session titles in the user's language. Plain text only.",
		Temperature: 0.2,
		MaxTokens:   32,
		Stream:      true,
	}
	resp, err := a.provider.Complete(ctx, req)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for chunk := range resp.Stream() {
		if chunk.Error != nil {
			return ""
		}
		sb.WriteString(chunk.Text)
	}
	return sanitizeTitle(sb.String())
}

// sanitizeTitle normalizes a model reply into a displayable single-line title.
func sanitizeTitle(raw string) string {
	title := strings.TrimSpace(raw)
	title = strings.Trim(title, "\"'`")
	if idx := strings.IndexAny(title, "\n"); idx >= 0 {
		title = strings.TrimSpace(title[:idx])
	}
	if len(title) > titleMaxChars {
		title = strings.TrimSpace(title[:titleMaxChars])
	}
	return title
}

func truncateTitle(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}
