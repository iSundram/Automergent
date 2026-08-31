package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	"github.com/iSundram/Automergent/internal/config"
)

// titleMaxChars bounds generated session titles.
const titleMaxChars = 60

// titleModelLadder is the cascade of cheap Google models tried, in order, for
// auto-generated session titles. Each rung is only a few tokens, so the ladder
// costs almost nothing next to the turn that triggered it; when every rung
// fails (or no Google credentials are resolvable), the user's active model is
// tried as the last resort.
var titleModelLadder = []string{
	"gemini-2.0-flash",
	"gemini-2.5-flash",
	"gemini-3.5-flash-lite",
}

// titleRequestFormat is the prompt/system pair shared by every ladder rung.
const (
	titleRequestSystem = "You write terse, specific session titles in the user's language. Plain text only."
	titleMaxTokens     = 32
)

// GenerateSessionTitle produces a short title for a conversation's opening
// exchange, used to auto-name sessions in the picker. It walks the cheap-model
// ladder above and falls back to the agent's active provider; any failure is
// silent and yields "" so callers can keep their deterministic fallback (the
// first user message) instead.
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

	for _, p := range a.titleProviders() {
		if title := completeTitle(ctx, p, prompt); title != "" {
			return title
		}
	}
	return ""
}

// titleProviders returns the ladder rungs followed by the user's active
// provider. The Google rungs are built once from the stored Google provider
// config (API key, base URL, headers) and reused across turns; when Google
// credentials can't be resolved the ladder is empty and only the active
// provider remains.
func (a *Agent) titleProviders() []ai.Provider {
	a.titleOnce.Do(func() {
		a.titleLadder = buildTitleLadder(a.cfg)
	})
	ladder := append([]ai.Provider{}, a.titleLadder...)
	if a.provider != nil {
		ladder = append(ladder, a.provider)
	}
	return ladder
}

// buildTitleLadder constructs one lightweight Google client per ladder model.
// It mirrors buildProviderForConfig in the TUI layer (credential resolution
// included) but keeps the clients private to title generation: short timeout,
// no retries worth mentioning, never wrapped in fallback/cache decorators.
func buildTitleLadder(cfg *config.Config) []ai.Provider {
	if cfg == nil {
		return nil
	}
	pc, name := titleGoogleConfig(cfg)
	if name == "" {
		return nil
	}
	apiKey := pc.APIKey
	if apiKey == "" {
		if key, err := config.GetProviderAPIKey(cfg, name); err == nil && key != "" {
			apiKey = key
		}
	}
	// Without a key the Gemini API is unreachable; a custom base URL may carry
	// its own auth, so that still qualifies.
	if apiKey == "" && pc.BaseURL == "" {
		return nil
	}

	providers := make([]ai.Provider, 0, len(titleModelLadder))
	for _, model := range titleModelLadder {
		aiCfg := ai.ProviderConfig{
			APIKey:       apiKey,
			BaseURL:      pc.BaseURL,
			DefaultModel: model,
			OrgID:        pc.OrgID,
			Project:      pc.Project,
			Location:     pc.Location,
			Backend:      pc.Backend,
			Headers:      pc.Headers,
			// Titles are a background nicety: fail fast and quietly so a
			// dead rung never delays the next one (or the active model).
			TimeoutSeconds: 15,
			MaxRetries:     1,
		}
		providers = append(providers, googleProvider.New(aiCfg))
	}
	return providers
}

// titleGoogleConfig picks the Google provider entry to run the ladder on,
// preferring the current catalog name over the legacy alias.
func titleGoogleConfig(cfg *config.Config) (config.ProviderConfig, string) {
	for _, name := range []string{"google-aistudio", "google"} {
		if pc, ok := cfg.Providers[name]; ok {
			return pc, name
		}
	}
	return config.ProviderConfig{}, ""
}

// completeTitle runs one ladder attempt. Any error — transport, stream, or an
// empty/unusable reply — returns "" so the caller advances to the next rung.
func completeTitle(ctx context.Context, p ai.Provider, prompt string) string {
	if p == nil {
		return ""
	}
	req := ai.CompletionRequest{
		Messages:    []ai.Message{ai.NewTextMessage(ai.RoleUser, prompt)},
		System:      titleRequestSystem,
		Temperature: 0.2,
		MaxTokens:   titleMaxTokens,
		Stream:      true,
	}
	resp, err := p.Complete(ctx, req)
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
