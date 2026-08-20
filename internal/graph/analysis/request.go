package analysis

import (
	"regexp"
	"strings"
)

// RequestCategory is the stable intent vocabulary shared by the coordinator,
// prompt system, and graph planner.
type RequestCategory string

const (
	RequestCategoryFeature       RequestCategory = "feature_addition"
	RequestCategoryBugFix        RequestCategory = "bug_fix"
	RequestCategoryInvestigation RequestCategory = "issue_investigation"
	RequestCategoryReview        RequestCategory = "review"
	RequestCategoryTest          RequestCategory = "test"
	RequestCategoryPlan          RequestCategory = "plan"
	RequestCategoryQuestion      RequestCategory = "question"
	RequestCategoryDirect        RequestCategory = "direct_command"
	RequestCategoryUnknown       RequestCategory = "unknown"
)

// RequestRelation describes how a message relates to existing work.
type RequestRelation string

const (
	RequestRelationNew      RequestRelation = "new_task"
	RequestRelationFollowUp RequestRelation = "follow_up"
	RequestRelationRelated  RequestRelation = "related"
)

// ContextShareMode is deliberately explicit: context is never copied merely
// because it exists in the graph.
type ContextShareMode string

const (
	ContextShareNone    ContextShareMode = "none"
	ContextShareSummary ContextShareMode = "summary"
	ContextSharePartial ContextShareMode = "partial"
	ContextShareFull    ContextShareMode = "full"
)

type ContextShareDecision struct {
	SourceTaskID string           `json:"source_task_id,omitempty"`
	Mode         ContextShareMode `json:"mode"`
	Keys         []string         `json:"keys,omitempty"`
	Reason       string           `json:"reason"`
}

type ToolSuggestion struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
	Deferred   bool    `json:"deferred"`
}

type TodoRecommendation struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Stage        string   `json:"stage"`
	Dependencies []string `json:"dependencies,omitempty"`
	ContextKeys  []string `json:"context_keys,omitempty"`
	Tools        []string `json:"tools,omitempty"`
}

// RequestAnalysis is the graph's recommendation packet. It contains no
// imperative file or tool operation; callers decide whether and when to apply
// recommendations.
type RequestAnalysis struct {
	Category          RequestCategory        `json:"category"`
	Relation          RequestRelation        `json:"relation"`
	Confidence        float64                `json:"confidence"`
	Intent            string                 `json:"intent"`
	Scope             string                 `json:"scope"`
	Risk              string                 `json:"risk"`
	RequiresCoder     bool                   `json:"requires_coder"`
	NeedsVerification bool                   `json:"needs_verification"`
	NeedsWiring       bool                   `json:"needs_wiring"`
	Context           []ContextShareDecision `json:"context,omitempty"`
	Tools             []ToolSuggestion       `json:"tools,omitempty"`
	Todos             []TodoRecommendation   `json:"todos,omitempty"`
	EntryPointHints   []string               `json:"entry_point_hints,omitempty"`
	CleanupKeys       []string               `json:"cleanup_keys,omitempty"`
}

var wordPattern = regexp.MustCompile(`[a-zA-Z0-9_./:-]+`)

// AnalyzeRequest performs deterministic routing before an LLM is invoked.
// This is intentionally conservative: uncertain requests stay unknown and
// request read-only investigation instead of inventing edits.
func AnalyzeRequest(message string, previousMessages []string) RequestAnalysis {
	lower := strings.ToLower(message)
	category := RequestCategoryQuestion
	switch {
	case containsAny(lower, "tell me", "which files", "what files", "where is", "how does", "read files", "show me", "explain"):
		if containsAny(lower, "issue", "issues", "error", "broken", "not working") {
			category = RequestCategoryInvestigation
		} else {
			category = RequestCategoryQuestion
		}
	case containsAny(lower, "add ", "create ", "add feature", "new feature", "implement", "introduce", "support", "wire in", "expose "):
		category = RequestCategoryFeature
	case containsAny(lower, "bug", "fix", "broken", "regression", "doesn't work", "does not work", "issue"):
		category = RequestCategoryBugFix
	case containsAny(lower, "investigate", "diagnose", "find root cause", "why is"):
		category = RequestCategoryInvestigation
	case containsAny(lower, "review", "audit"):
		category = RequestCategoryReview
	case containsAny(lower, "test", "verify", "validation", "check that"):
		category = RequestCategoryTest
	case containsAny(lower, "plan", "design", "architecture"):
		category = RequestCategoryPlan
	case containsAny(lower, "run ", "build ", "format ", "install ", "execute "):
		category = RequestCategoryDirect
	case lower == "" || !containsAny(lower, "what", "how", "which", "where", "explain", "show", "tell"):
		category = RequestCategoryUnknown
	}

	relation := RequestRelationNew
	confidence := 0.72
	if len(previousMessages) > 0 {
		if containsAny(lower, "new task", "separate task", "unrelated task", "fresh task", "clear context") {
			relation = RequestRelationNew
			confidence = 0.95
		} else if containsAny(lower, "continue", "resume", "again", "missed", "you didn't", "follow up", "also", "undo") {
			relation = RequestRelationFollowUp
			confidence = 0.9
		} else {
			relation = RequestRelationRelated
			confidence = 0.62
		}
	}

	result := RequestAnalysis{
		Category:          category,
		Relation:          relation,
		Confidence:        confidence,
		Intent:            strings.TrimSpace(message),
		Scope:             inferScope(lower),
		Risk:              inferRisk(lower, category),
		RequiresCoder:     category == RequestCategoryFeature || category == RequestCategoryBugFix || category == RequestCategoryReview,
		NeedsVerification: category == RequestCategoryFeature || category == RequestCategoryBugFix || category == RequestCategoryTest,
		NeedsWiring:       category == RequestCategoryFeature,
		EntryPointHints:   entryPointHints(lower),
		CleanupKeys:       []string{"injected_prompts", "stale_context", "duplicate_context"},
	}

	if relation == RequestRelationFollowUp {
		mode := ContextShareFull
		if containsAny(lower, "summary", "briefly", "just the result") {
			mode = ContextShareSummary
		}
		result.Context = append(result.Context, ContextShareDecision{Mode: mode, Reason: "follow-up request resumes the active task"})
	} else if relation == RequestRelationRelated {
		result.Context = append(result.Context, ContextShareDecision{Mode: ContextSharePartial, Keys: []string{"decisions", "relevant_files", "verification"}, Reason: "related request shares only task-relevant graph facts"})
	} else {
		result.Context = append(result.Context, ContextShareDecision{Mode: ContextShareNone, Reason: "new task starts with an isolated task context"})
	}

	result.Tools = suggestTools(category, lower)
	result.Todos = recommendTodos(result)
	return result
}

// AnalyzeGraphContext derives durable continuity and context facts without
// categorizing the request. Tool category selection belongs to the model-side
// ephemeral router and must never enter graph state.
func AnalyzeGraphContext(message string, previousMessages []string) RequestAnalysis {
	lower := strings.ToLower(message)
	relation := RequestRelationNew
	confidence := 0.72
	if len(previousMessages) > 0 {
		if containsAny(lower, "new task", "separate task", "unrelated task", "fresh task", "clear context") {
			relation = RequestRelationNew
			confidence = 0.95
		} else if containsAny(lower, "continue", "resume", "again", "missed", "you didn't", "follow up", "also", "undo") {
			relation = RequestRelationFollowUp
			confidence = 0.9
		} else {
			relation = RequestRelationRelated
			confidence = 0.62
		}
	}
	result := RequestAnalysis{
		Category:        RequestCategoryUnknown,
		Relation:        relation,
		Confidence:      confidence,
		Intent:          strings.TrimSpace(message),
		Scope:           inferScope(lower),
		Risk:            inferRisk(lower, RequestCategoryUnknown),
		EntryPointHints: entryPointHints(lower),
		CleanupKeys:     []string{"injected_prompts", "stale_context", "duplicate_context"},
		Todos: []TodoRecommendation{
			{ID: "analyze", Title: "Understand the request and relevant context", Stage: "analysis", ContextKeys: []string{"request", "relevant_files"}},
			{ID: "execute", Title: "Perform the requested work", Stage: "execution", Dependencies: []string{"analyze"}, ContextKeys: []string{"decisions", "working_files"}},
			{ID: "verify", Title: "Verify and report the result", Stage: "verification", Dependencies: []string{"execute"}, ContextKeys: []string{"verification", "tool_results"}},
		},
	}
	if relation == RequestRelationFollowUp {
		mode := ContextShareFull
		if containsAny(lower, "summary", "briefly", "just the result") {
			mode = ContextShareSummary
		}
		result.Context = []ContextShareDecision{{Mode: mode, Reason: "follow-up request resumes the active task"}}
	} else if relation == RequestRelationRelated {
		result.Context = []ContextShareDecision{{Mode: ContextSharePartial, Keys: []string{"decisions", "relevant_files", "verification"}, Reason: "related request shares only task-relevant graph facts"}}
	} else {
		result.Context = []ContextShareDecision{{Mode: ContextShareNone, Reason: "new task starts with an isolated task context"}}
	}
	return result
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func inferScope(value string) string {
	if containsAny(value, "repository", "project", "everywhere", "end to end") {
		return "repository"
	}
	if containsAny(value, "module", "subsystem", "pipeline", "provider", "tui") {
		return "subsystem"
	}
	if strings.Contains(value, ".go") || strings.Contains(value, "file") {
		return "file_or_module"
	}
	return "unknown"
}

func inferRisk(value string, category RequestCategory) string {
	if containsAny(value, "delete", "remove", "reset", "undo", "migration", "security") {
		return "high"
	}
	if category == RequestCategoryFeature || category == RequestCategoryBugFix {
		return "medium"
	}
	return "low"
}

func entryPointHints(value string) []string {
	var hints []string
	for _, candidate := range []struct {
		words []string
		name  string
	}{
		{[]string{"tui", "terminal", "command", "keybind"}, "tui"},
		{[]string{"api", "endpoint", "http", "rest"}, "api"},
		{[]string{"cli", "flag", "argument"}, "cli"},
		{[]string{"config", "setting", "environment"}, "configuration"},
	} {
		if containsAny(value, candidate.words...) {
			hints = append(hints, candidate.name)
		}
	}
	if len(hints) == 0 {
		hints = []string{"product_entrypoint_review"}
	}
	return hints
}

func suggestTools(category RequestCategory, value string) []ToolSuggestion {
	tools := []ToolSuggestion{{Name: "read", Confidence: 0.95, Reason: "inspect existing implementation and wiring", Deferred: false}}
	if category == RequestCategoryQuestion || category == RequestCategoryPlan {
		return tools
	}
	if containsAny(value, "search", "similar", "where", "related") || category == RequestCategoryFeature {
		tools = append(tools, ToolSuggestion{Name: "search", Confidence: 0.9, Reason: "find existing features and related files", Deferred: false})
	}
	if category == RequestCategoryFeature || category == RequestCategoryBugFix {
		tools = append(tools,
			ToolSuggestion{Name: "edit", Confidence: 0.7, Reason: "modify only after analysis and approval", Deferred: true},
			ToolSuggestion{Name: "build_or_test", Confidence: 0.9, Reason: "verify behavior and detect regressions", Deferred: true},
		)
	}
	return tools
}

func recommendTodos(result RequestAnalysis) []TodoRecommendation {
	base := []TodoRecommendation{{ID: "analyze", Title: "Analyze request", Stage: "analysis", ContextKeys: []string{"request", "similar_features", "entry_points"}, Tools: []string{"read", "search"}}}
	if result.RequiresCoder {
		base = append(base,
			TodoRecommendation{ID: "implement", Title: "Implement the approved change", Stage: "execution", Dependencies: []string{"analyze"}, ContextKeys: []string{"decisions", "working_files"}, Tools: []string{"edit"}},
			TodoRecommendation{ID: "verify", Title: "Verify behavior and record evidence", Stage: "verification", Dependencies: []string{"implement"}, ContextKeys: []string{"verification", "tool_results"}, Tools: []string{"build_or_test"}},
		)
	} else if result.NeedsVerification {
		base = append(base, TodoRecommendation{ID: "verify", Title: "Verify the requested behavior", Stage: "verification", Dependencies: []string{"analyze"}, ContextKeys: []string{"verification"}, Tools: []string{"build_or_test"}})
	}
	return base
}

// ExtractTerms is shared by graph persistence and tests that need a stable
// representation of the request without relying on an LLM tokenizer.
func ExtractTerms(message string) []string {
	seen := make(map[string]struct{})
	var terms []string
	for _, term := range wordPattern.FindAllString(strings.ToLower(message), -1) {
		if len(term) < 3 {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}
	return terms
}
