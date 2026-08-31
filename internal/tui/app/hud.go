package app

// HUD/status plumbing: tokens, git branch, context detail.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	googleProvider "github.com/iSundram/Automergent/internal/ai/google"
	openaiProvider "github.com/iSundram/Automergent/internal/ai/openai"
	"github.com/iSundram/Automergent/internal/cache"
	"github.com/iSundram/Automergent/internal/config"
	"os/exec"
	"strings"
	"time"
)

func (a *App) refreshGitBranch() {
	if time.Since(a.lastBranchCheck) < 5*time.Second {
		return
	}
	a.lastBranchCheck = time.Now()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if out, err := cmd.Output(); err == nil {
		a.statusBar.SetGitBranch(strings.TrimSpace(string(out)))
	} else {
		a.statusBar.SetGitBranch("")
	}
}

func (a *App) updateActiveTokens() {
	if a.ag == nil {
		return
	}
	mgr := a.ag.ContextManager()
	if mgr == nil {
		return
	}
	calc := mgr.AdaptiveCalculator()
	if calc == nil {
		return
	}
	if a.sess == nil {
		return
	}
	active := calc.EstimateMessages(a.sess.Messages)

	// Add currently-being-typed prompt if it's non-empty
	if pending := a.input.Value(); pending != "" {
		active += calc.Estimate(pending)
	}

	// The header's meter is the single source of truth for context usage
	// (tokens + bar). The status bar no longer duplicates it.
	a.header.SetActiveTokens(active)
	// Cumulative session usage rides alongside: the real, provider-reported
	// sum of every request so far (not the window estimate).
	a.header.SetTotalTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)

	// HUD: pending-edit review counter.
	if store := a.ag.EditReviewStore(); store != nil {
		a.statusBar.SetPendingEdits(store.PendingCount())
	}

	// Keep the bottom dock live (background shells + agents).
	a.refreshDock()
}

func (a *App) showContextDetail() {
	var b strings.Builder
	b.WriteString("# Context Telemetry\n\n")

	// Live context-window engine state (thresholds, usage, compaction health)
	if stats := a.ag.ContextWindowStats(a.ag.Provider()); stats.EffectiveWindow > 0 {
		b.WriteString(fmt.Sprintf("## Context Window\n- Raw limit: %d tokens\n- Effective window (summary reserved): %d\n- Auto-compact threshold: %d\n- Blocking limit: %d\n- Estimated usage: %d tokens (%d%%)\n",
			stats.RawLimit, stats.EffectiveWindow, stats.AutoCompactThreshold, stats.BlockingLimit,
			stats.EstimatedTokens, stats.EstimatedTokens*100/stats.EffectiveWindow))
		if !stats.LastCompactedAt.IsZero() {
			b.WriteString(fmt.Sprintf("- Last compaction: %s ago\n", time.Since(stats.LastCompactedAt).Round(time.Second)))
		}
		if stats.CircuitBreakerTripped {
			b.WriteString(fmt.Sprintf("- ⚠ Compaction circuit breaker tripped (%d consecutive failures)\n", stats.CompactionFailures))
		}
		b.WriteString("\n")
	}

	// Adaptive token weight
	if calc := a.ag.AdaptiveCalculator(); calc != nil {
		b.WriteString(fmt.Sprintf("## Adaptive Token Estimation\n- Model: %s\n- Learned Weight: %.2f\n- Samples: %d\n\n",
			calc.Model(), calc.Weight(), calc.Samples()))
	}

	// Telemetry collector
	if tel := a.ag.Telemetry(); tel != nil {
		breakdowns := tel.GetBreakdowns(1)
		if len(breakdowns) > 0 {
			bd := breakdowns[0]
			b.WriteString(fmt.Sprintf("## Latest Context Breakdown\n- Total: %d tokens\n- System Prompt: %d\n- Tool Definitions: %d\n- Conversation: %d\n- Context Files: %d\n- Tool Calls: %d\n- Thinking: %d\n- Output Reserve: %d\n- Safety Margin: %d\n- Provider Actual: %d\n- Est. Weight: %.2f\n\n",
				bd.TotalTokens, bd.SystemPrompt, bd.ToolDefinitions, bd.Conversation,
				bd.ContextFiles, bd.ToolCalls, bd.Thinking, bd.OutputReserve, bd.SafetyMargin,
				bd.ProviderActual, bd.EstimationWeight))
		}

		compacts := tel.GetCompactionEvents(5)
		if len(compacts) > 0 {
			b.WriteString("## Recent Compaction Events\n")
			for _, c := range compacts {
				b.WriteString(fmt.Sprintf("- %s: %s (tier: %s) %d→%d tokens (%dms) %s\n",
					c.Timestamp.Format("15:04:05"), c.Reason, c.Strategy,
					c.TokensBefore, c.TokensAfter, c.DurationMs,
					map[bool]string{true: "✓", false: "✗"}[c.Success]))
			}
			b.WriteString("\n")
		}

		cost := tel.GetCostSummary()
		if cost.TotalCostUSD > 0 || cost.TotalInputTokens > 0 {
			b.WriteString(fmt.Sprintf("## Cost Summary\n- Total: $%.4f\n- Input Tokens: %d\n- Output Tokens: %d\n",
				cost.TotalCostUSD, cost.TotalInputTokens, cost.TotalOutputTokens))
			for model, mc := range cost.ByModel {
				if mc.TotalCost > 0 || mc.InputTokens > 0 || mc.OutputTokens > 0 {
					b.WriteString(fmt.Sprintf("  - %s: $%.4f (%d in / %d out)\n", model, mc.TotalCost, mc.InputTokens, mc.OutputTokens))
				}
			}
			b.WriteString("\n")
		}

		// Transcript info
		if mgr := a.ag.ContextManager(); mgr != nil {
			if tm := mgr.TranscriptManager(); tm != nil {
				items := tm.RawMessages()
				b.WriteString(fmt.Sprintf("## Transcript\n- Total messages: %d\n", len(items)))
				if pt := tm.PristineMessages(); len(pt) != len(items) {
					b.WriteString(fmt.Sprintf("- Pristine (never compacted): %d\n", len(pt)))
				}
			}
		}
	}

	a.conversation.AddMessage("system", b.String(), false)
}

func (a *App) compactContext() tea.Cmd {
	ctx := a.ctx
	return func() tea.Msg {
		if a.ag == nil {
			return nil
		}
		compacted := a.ag.CompactSessionMessages(ctx, a.sess.Messages)
		a.sess.SetMessages(compacted)
		// The usage anchor described the pre-compaction prefix; drop it so
		// token estimates are recomputed from the compacted history.
		a.ag.InvalidateUsageAnchor()
		// Update transcript with compacted messages
		if mgr := a.ag.ContextManager(); mgr != nil {
			if tm := mgr.TranscriptManager(); tm != nil {
				// Rebuild transcript from compacted messages
				tm.Rollback(0)
				for _, m := range compacted {
					tm.Append(m)
				}
			}
		}
		return agentEventMsg{ev: agent.Event{Type: agent.EventCompacted}}
	}
}

func buildProviderFromConfig(cfg *config.Config) (ai.Provider, error) {
	primary, err := buildProviderForConfig(cfg, cfg.Provider, cfg.Model)
	if err != nil {
		return nil, err
	}

	// Build fallback chain when configured.
	var fallbacks []ai.Provider
	var labels []string
	for _, fb := range cfg.ProviderFallback {
		fp, err := buildProviderForConfig(cfg, fb.Provider, fb.Model)
		if err != nil {
			continue // skip invalid entries; validated at /provider fallback add time
		}
		fallbacks = append(fallbacks, fp)
		labels = append(labels, fb.Provider+"/"+fb.Model)
	}
	if len(fallbacks) > 0 {
		chain := append([]ai.Provider{primary}, fallbacks...)
		chainLabels := append([]string{cfg.Provider + "/" + cfg.Model}, labels...)
		primary = ai.NewFallbackChain(chain, chainLabels)
	}

	// Prompt cache wrapper.
	if shouldWrapPromptCacheProvider(cfg, primary) {
		primary = cache.NewCachingProvider(primary, cache.NewPromptCache())
	}

	return primary, nil
}

// buildProviderForConfig constructs a provider for an arbitrary name+model
// from the stored config. It resolves credentials (config key → env → secret
// store), applies per-model credential overrides and returns the raw provider
// without fallback/cache/debug wrappers.
func buildProviderForConfig(cfg *config.Config, name, model string) (ai.Provider, error) {
	pc := cfg.Providers[name]

	apiKey := pc.APIKey
	if apiKey == "" {
		if key, err := config.GetProviderAPIKey(cfg, name); err == nil && key != "" {
			apiKey = key
		}
	}
	baseURL := pc.BaseURL
	if mc, ok := pc.Models[model]; ok {
		if mc.APIKey != "" {
			apiKey = mc.APIKey
		}
		if mc.BaseURL != "" {
			baseURL = mc.BaseURL
		}
	}

	enablePromptCache := shouldEnablePromptCache(cfg, name)
	aiCfg := ai.ProviderConfig{
		APIKey:             apiKey,
		BaseURL:            baseURL,
		DefaultModel:       model,
		OrgID:              pc.OrgID,
		Project:            pc.Project,
		Location:           pc.Location,
		Backend:            pc.Backend,
		Effort:             pc.Effort,
		ThinkingLevel:      pc.ThinkingLevel,
		PromptCacheEnabled: &enablePromptCache,
		Headers:            pc.Headers,
		Temperature:        pc.Temperature,
		MaxTokens:          pc.MaxTokens,
		TimeoutSeconds:     pc.TimeoutSeconds,
		MaxRetries:         pc.MaxRetries,
	}

	// Vertex-expressing providers: force the vertex backend regardless of
	// any stale config value.
	switch name {
	case "google", "google-aistudio":
		if name == "google-aistudio" {
			aiCfg.Backend = "aistudio"
		}
		return googleProvider.New(aiCfg), nil
	case "google-vertex":
		aiCfg.Backend = "vertex"
		return googleProvider.New(aiCfg), nil
	default:
		// Custom providers and legacy catalog names: route by ApiType.
		return buildCustomProvider(name, aiCfg, pc.ApiType)
	}
}

// buildCustomProvider constructs a provider for a custom or legacy name.
// apiType ("gemini" | "openai" | "anthropic" | "") picks the protocol: gemini
// → the Google client pointed at the custom base URL; anything else → the
// OpenAI-compatible client (the lingua franca of custom endpoints). This is
// what makes the "custom" provider real: register a name with a baseUrl, API
// key, and apiType, and it builds.
func buildCustomProvider(name string, aiCfg ai.ProviderConfig, apiType string) (ai.Provider, error) {
	if strings.EqualFold(apiType, "gemini") {
		return googleProvider.New(aiCfg), nil
	}
	if aiCfg.BaseURL == "" {
		return nil, fmt.Errorf("custom provider %q needs a baseUrl (/provider set %s baseUrl <url>)", name, name)
	}
	return openaiProvider.New(aiCfg), nil
}

func shouldEnablePromptCache(cfg *config.Config, provider string) bool {
	if !cfg.Cache.Prompt.Enabled {
		return false
	}
	switch provider {
	default:
		return false
	}
}

func shouldWrapPromptCacheProvider(cfg *config.Config, provider ai.Provider) bool {
	return shouldEnablePromptCache(cfg, provider.Name())
}
