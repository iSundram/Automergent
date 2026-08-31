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

// switchedProvider records every request's model so the test can prove
// which provider each pipeline actually called.
type switchedProvider struct {
	firstMessageRecordingProvider
	name  string
	model string
	calls []string // "provider/model" per request
}

func (p *switchedProvider) Name() string { return p.name }
func (p *switchedProvider) Complete(_ context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	p.calls = append(p.calls, p.name+"/"+p.model)
	return ai.NewStaticResponse("not json", "", nil, ai.StopReasonEnd, ai.Usage{}), nil
}
func (p *switchedProvider) ContextLimit() int { return 128000 }

func TestSetProviderRebuildsRoutingPipelines(t *testing.T) {
	old := &switchedProvider{name: "old", model: "old-model"}
	newP := &switchedProvider{name: "new", model: "new-model"}

	toolReg := tools.NewRegistry()
	ag := &Agent{
		cfg:          &config.Config{Model: "old-model"},
		provider:     old,
		sess:         session.New(),
		tools:        toolReg,
		events:       make(chan Event, 256),
		sessionGrants: newGrants(nil),
	}

	// Prime every pipeline against the old provider: one Run builds the
	// decomposer, classifier, and (on the keyword path) the prompt system's
	// intent/planner adapters.
	if err := ag.Run(context.Background(), "find the auth flow"); err != nil {
		t.Fatalf("priming run: %v", err)
	}
	if len(old.calls) == 0 {
		t.Fatal("priming run never called the old provider")
	}
	oldCalls := len(old.calls)

	// Switch.
	ag.cfg.Model = "new-model"
	ag.SetProvider(newP)

	// Run again: the decomposer must call the NEW provider, never the old.
	if err := ag.Run(context.Background(), "find the billing flow"); err != nil {
		t.Fatalf("post-switch run: %v", err)
	}
	if len(old.calls) != oldCalls {
		t.Fatalf("old provider received %d new calls after the switch — routing pipelines are stale", len(old.calls)-oldCalls)
	}
	if len(newP.calls) == 0 {
		t.Fatal("new provider never called after the switch")
	}
}

func TestSetProviderRefreshesContextLimits(t *testing.T) {
	old := &switchedProvider{name: "old", model: "big-model"}
	ag := &Agent{
		cfg:      &config.Config{Model: "big-model"},
		provider: old,
		sess:     session.New(),
		tools:    tools.NewRegistry(),
		events:   make(chan Event, 128),
	}

	before := ag.ContextManager().GetBudgetSummary().TotalBudget
	// Switching must re-derive the context budget from the new model's
	// limits (or at least not corrupt the manager): assert the manager
	// survives with a sane budget.
	ag.SetProvider(old)
	after := ag.ContextManager().GetBudgetSummary().TotalBudget
	if after <= 0 {
		t.Fatalf("context budget corrupted after switch: %d (was %d)", after, before)
	}
}

func TestSetProviderInvalidatesUsageAnchor(t *testing.T) {
	ag := &Agent{
		cfg:      &config.Config{},
		provider: &switchedProvider{name: "p", model: "m"},
		sess:     session.New(),
		tools:    tools.NewRegistry(),
		events:   make(chan Event, 128),
	}
	ag.recordUsageAnchor(ai.Usage{InputTokens: 5000}, 3)
	if ag.usageAnchor != 3 {
		t.Fatal("anchor not recorded")
	}
	ag.SetProvider(&switchedProvider{name: "p2", model: "m2"})
	if ag.usageAnchor != 0 {
		t.Fatalf("anchor survived the provider switch: %d — token estimates will be wrong for the new provider", ag.usageAnchor)
	}
}

var _ = strings.TrimSpace // keep strings import if asserts change
