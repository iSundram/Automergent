package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// LLMIntentIdentifier uses an LLM to identify intents from user messages.
type LLMIntentIdentifier struct {
	config       *PromptConfig
	llmClient    LLMClient
	systemPrompt string
}

// LLMClient interface for LLM interactions.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// NewLLMIntentIdentifier creates a new LLM-based intent identifier.
func NewLLMIntentIdentifier(config *PromptConfig, llmClient LLMClient) *LLMIntentIdentifier {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &LLMIntentIdentifier{
		config:       config,
		llmClient:    llmClient,
		systemPrompt: buildIntentSystemPrompt(),
	}
}

func buildIntentSystemPrompt() string {
	return `You are an intent identification system. Analyze the user's message and identify ALL intents present.

INTENT TYPES:
- explore: User wants to find, examine, or understand files/code (keywords: see, show, find, explore, look at, examine, search, grep, list files, related files)
- implement: User wants to create new code/features (keywords: implement, create, add, build, new feature, new function, new component, new module, new api, new endpoint, write, develop)
- fix: User wants to fix a bug/issue (keywords: fix, resolve, repair, correct, patch, bug, issue, error, broken, not working, fail, solve)
- test: User wants to write or run tests (keywords: test, write test, add test, unit test, integration test, run test, verify with test, coverage)
- commit: User wants to commit changes (keywords: commit, git commit, push, git push, commit and push, save changes)
- review: User wants code review (keywords: review, code review, check code, inspect, audit, verify work, validate)
- document: User wants documentation (keywords: document, documentation, docs, write docs, add documentation, readme, comment, docstring)
- refactor: User wants to refactor/improve code (keywords: refactor, rewrite, restructure, reorganize, clean up, improve code, optimize, modernize)
- debug: User wants to debug/investigate (keywords: debug, debug why, why is, investigate, trace, analyze, root cause, what went wrong, figure out)
- question: User is asking a question (keywords: what is, how does, why does, explain, tell me, describe, clarify, ?, help)
- plan: User wants a plan/design (keywords: plan, design, architecture, approach, strategy, roadmap, steps to, proposal)
- direct: User wants a direct action (keywords: show, list, get, read, display, print, find, search, grep, cat, head, tail)

OUTPUT FORMAT (JSON only):
{
  "intents": [
    {
      "type": "explore",
      "priority": 1,
      "confidence": 0.9,
      "raw_text": "exact phrase from user message",
      "parameters": {"target": "context", "specific_files": []}
    }
  ],
  "requires_init": true,
  "init_goal": "Understand the codebase to address: ...",
  "init_actions": [
    {"tool": "glob", "target": "**/*context*", "reason": "Find context-related files"}
  ]
}

RULES:
1. Identify ALL intents in the message, not just one
2. Assign priorities: explore/debug/fix=1, implement/refactor/review=2, test/document=3, commit=4
3. Add dependencies: implement/fix/test/commit depend on explore; test/commit depend on implement/fix
4. Set requires_init=true if any intent is explore, fix, debug, or review
5. Extract relevant parameters for each intent
6. Return ONLY valid JSON, no extra text`
}

func (i *LLMIntentIdentifier) IdentifyIntents(ctx context.Context, prompt string, workingDir string, availableFiles []string) *IntentSet {
	userPrompt := i.buildUserPrompt(prompt, workingDir, availableFiles)

	response, err := i.llmClient.Complete(ctx, i.systemPrompt, userPrompt)
	if err != nil {
		return &IntentSet{
			Intents:        []Intent{{ID: uuid.New().String()[:8], Type: IntentQuestion, Priority: 1, Confidence: 0.1, RawText: prompt, Parameters: map[string]any{"error": err.Error()}}},
			RequiresInit:   false,
			OriginalPrompt: prompt,
		}
	}

	intentSet, err := i.parseResponse(response, prompt)
	if err != nil {
		return &IntentSet{
			Intents:        []Intent{{ID: uuid.New().String()[:8], Type: IntentQuestion, Priority: 1, Confidence: 0.1, RawText: prompt, Parameters: map[string]any{"error": err.Error()}}},
			RequiresInit:   false,
			OriginalPrompt: prompt,
		}
	}

	return intentSet
}

func (i *LLMIntentIdentifier) buildUserPrompt(prompt, workingDir string, availableFiles []string) string {
	var sb strings.Builder
	sb.WriteString("User Message: ")
	sb.WriteString(prompt)
	sb.WriteString("\n\n")
	sb.WriteString("Working Directory: ")
	sb.WriteString(workingDir)
	sb.WriteString("\n")
	if len(availableFiles) > 0 {
		sb.WriteString("Available Files: ")
		sb.WriteString(strings.Join(availableFiles, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\nIdentify all intents and return JSON.")
	return sb.String()
}

func (i *LLMIntentIdentifier) parseResponse(response, originalPrompt string) (*IntentSet, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	var parsed struct {
		Intents []struct {
			Type         string         `json:"type"`
			Priority     int            `json:"priority"`
			Confidence   float64        `json:"confidence"`
			RawText      string         `json:"raw_text"`
			Parameters   map[string]any `json:"parameters"`
			Dependencies []string       `json:"dependencies"`
		} `json:"intents"`
		RequiresInit bool   `json:"requires_init"`
		InitGoal     string `json:"init_goal"`
		InitActions  []struct {
			Tool   string `json:"tool"`
			Target string `json:"target"`
			Reason string `json:"reason"`
		} `json:"init_actions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	intentSet := &IntentSet{
		Intents:        make([]Intent, 0, len(parsed.Intents)),
		RequiresInit:   parsed.RequiresInit,
		OriginalPrompt: originalPrompt,
	}

	intentIDMap := make(map[string]string)
	for _, intent := range parsed.Intents {
		intentType := IntentType(intent.Type)
		if !isValidIntentType(intentType) {
			continue
		}
		id := uuid.New().String()[:8]
		intentIDMap[intent.Type] = id
	}

	for _, intent := range parsed.Intents {
		intentType := IntentType(intent.Type)
		if !isValidIntentType(intentType) {
			continue
		}

		id := intentIDMap[intent.Type]
		dependencies := intent.Dependencies
		if len(dependencies) == 0 {
			dependencies = i.inferDependencies(intentType, intentIDMap)
		}

		intentSet.Intents = append(intentSet.Intents, Intent{
			ID:           id,
			Type:         intentType,
			Priority:     intent.Priority,
			Dependencies: dependencies,
			Parameters:   intent.Parameters,
			RawText:      intent.RawText,
			Confidence:   intent.Confidence,
		})
	}

	if parsed.RequiresInit && len(parsed.InitActions) > 0 {
		intentSet.InitPhase = &InitPhase{
			ID:              uuid.New().String()[:8],
			Goal:            parsed.InitGoal,
			SuccessCriteria: []string{"Found relevant files", "Understood the issue/requirement", "Identified key code areas"},
			Actions:         make([]InitAction, len(parsed.InitActions)),
		}
		for j, action := range parsed.InitActions {
			intentSet.InitPhase.Actions[j] = InitAction{
				ID:     uuid.New().String()[:8],
				Tool:   action.Tool,
				Target: action.Target,
				Reason: action.Reason,
				Status: InitActionPending,
			}
		}
	}

	return intentSet, nil
}

func (i *LLMIntentIdentifier) inferDependencies(currentType IntentType, intentIDMap map[string]string) []string {
	var deps []string
	currentTypeStr := string(currentType)

	if exploreID, ok := intentIDMap[string(IntentExplore)]; ok {
		if currentTypeStr != string(IntentExplore) && currentTypeStr != string(IntentQuestion) && currentTypeStr != string(IntentPlan) && currentTypeStr != string(IntentDirect) {
			deps = append(deps, exploreID)
		}
	}

	if currentType == IntentTest || currentType == IntentCommit {
		if implementID, ok := intentIDMap[string(IntentImplement)]; ok {
			deps = append(deps, implementID)
		}
		if fixID, ok := intentIDMap[string(IntentFix)]; ok {
			deps = append(deps, fixID)
		}
	}

	return deps
}

func isValidIntentType(t IntentType) bool {
	valid := map[IntentType]bool{
		IntentExplore: true, IntentImplement: true, IntentFix: true,
		IntentTest: true, IntentCommit: true, IntentReview: true,
		IntentDocument: true, IntentRefactor: true, IntentDebug: true,
		IntentQuestion: true, IntentPlan: true, IntentDirect: true,
	}
	return valid[t]
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

// MockLLMClient for testing without actual LLM. Routes responses by system
// prompt: TaskResponse is served to the task planner, Response to everything
// else (intent identification).
type MockLLMClient struct {
	Response     string
	TaskResponse string
	Error        error
}

func (m *MockLLMClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	if strings.Contains(systemPrompt, "task planning system") && m.TaskResponse != "" {
		return m.TaskResponse, nil
	}
	return m.Response, nil
}
