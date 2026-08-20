package analysis

import "testing"

func TestAnalyzeRequestFeatureIncludesWiringAndVerification(t *testing.T) {
	result := AnalyzeRequest("Add thinking effort support to the provider and wire it into the TUI", nil)
	if result.Category != RequestCategoryFeature {
		t.Fatalf("category = %q, want %q", result.Category, RequestCategoryFeature)
	}
	if !result.NeedsWiring || !result.NeedsVerification || !result.RequiresCoder {
		t.Fatalf("feature analysis missing execution requirements: %+v", result)
	}
	if len(result.EntryPointHints) == 0 || result.EntryPointHints[0] != "tui" {
		t.Fatalf("entry point hints = %v, want tui", result.EntryPointHints)
	}
	if len(result.Todos) != 3 || result.Todos[2].Stage != "verification" {
		t.Fatalf("todos = %+v, want analysis, execution, verification", result.Todos)
	}
}

func TestAnalyzeRequestFollowUpSharesFullContext(t *testing.T) {
	result := AnalyzeRequest("Continue, you missed wiring it into the TUI", []string{"add thinking effort support"})
	if result.Relation != RequestRelationFollowUp {
		t.Fatalf("relation = %q, want follow-up", result.Relation)
	}
	if len(result.Context) != 1 || result.Context[0].Mode != ContextShareFull {
		t.Fatalf("context = %+v, want full sharing", result.Context)
	}
}

func TestAnalyzeRequestIndependentTaskDoesNotShareContext(t *testing.T) {
	result := AnalyzeRequest("New task: review the session picker", []string{"add thinking effort support"})
	if result.Relation != RequestRelationNew {
		t.Fatalf("relation = %q, want new task", result.Relation)
	}
	if len(result.Context) != 1 || result.Context[0].Mode != ContextShareNone {
		t.Fatalf("context = %+v, want no sharing", result.Context)
	}
}
