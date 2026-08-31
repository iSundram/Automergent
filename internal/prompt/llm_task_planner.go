package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// LLMTaskPlanner uses an LLM to generate concrete executable tasks from
// identified intents and init-phase exploration results.
type LLMTaskPlanner struct {
	config       *PromptConfig
	llmClient    LLMClient
	systemPrompt string
}

// NewLLMTaskPlanner creates a new LLM-based task planner.
func NewLLMTaskPlanner(config *PromptConfig, llmClient LLMClient) *LLMTaskPlanner {
	if config == nil {
		config = DefaultPromptConfig()
	}
	return &LLMTaskPlanner{
		config:       config,
		llmClient:    llmClient,
		systemPrompt: buildTaskPlanningSystemPrompt(),
	}
}

func buildTaskPlanningSystemPrompt() string {
	return `You are a task planning system. Given a user request, the identified intents, and exploration results from an init phase, generate the concrete executable tasks.

You decide EVERYTHING about the tasks: how many, what order, what each one does. Do not follow fixed templates — derive the plan from the actual request and the actual files found.

TASK TYPES: read, analyze, implement, fix, test, commit, review, document, refactor, answer, plan
ROLES: assistant, researcher, coder, tester, reviewer, documenter

OUTPUT FORMAT (JSON only):
{
  "tasks": [
    {
      "id": "task-1",
      "type": "analyze",
      "role": "researcher",
      "priority": 1,
      "dependencies": [],
      "description": "Short description",
      "prompt": "Detailed self-contained instruction for the executing agent. Reference ACTUAL file paths discovered during exploration.",
      "tools": ["read_file", "grep"]
    }
  ]
}

RULES:
1. Reference ACTUAL file paths from the exploration results in task prompts
2. Order by dependency: understand/read first, then changes, then verify/test/commit last
3. Dependencies must reference earlier task ids
4. Create only the tasks genuinely needed — a simple question may need zero or one task
5. Commit tasks only if the user asked to commit
6. Return ONLY valid JSON, no extra text
7. NEVER create an "answer" task that greets the user, restates the plan, or reports/summarizes the results of earlier tasks. The completion response of the LAST task is shown to the user directly — reporting is free. An "answer" task is only valid when the request is a pure informational question with no other work, and then it must be the ONLY task. Duplicating it after real work produces a second, repetitive reply.`
}

// PlanTasks asks the LLM to produce the task plan.
func (p *LLMTaskPlanner) PlanTasks(ctx context.Context, intentSet *IntentSet, initResults *InitResults) ([]TaskSpec, error) {
	userPrompt := p.buildUserPrompt(intentSet, initResults)

	response, err := p.llmClient.Complete(ctx, p.systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("task planning LLM call failed: %w", err)
	}

	return p.parseResponse(response, intentSet)
}

func (p *LLMTaskPlanner) buildUserPrompt(intentSet *IntentSet, initResults *InitResults) string {
	var sb strings.Builder

	sb.WriteString("User Request: ")
	sb.WriteString(intentSet.OriginalPrompt)
	sb.WriteString("\n\nIdentified Intents:\n")
	for _, intent := range intentSet.Intents {
		sb.WriteString(fmt.Sprintf("- %s (priority %d): %q\n", intent.Type, intent.Priority, intent.RawText))
	}

	if initResults != nil && len(initResults.FilesFound) > 0 {
		sb.WriteString("\nExploration Results (init phase):\n")
		sb.WriteString(fmt.Sprintf("Files found (%d):\n", len(initResults.FilesFound)))
		for i, f := range initResults.FilesFound {
			if i >= 40 {
				sb.WriteString(fmt.Sprintf("... and %d more\n", len(initResults.FilesFound)-40))
				break
			}
			sb.WriteString("- " + f + "\n")
		}
		if len(initResults.CodeSnippets) > 0 {
			sb.WriteString("\nFile contents already loaded:\n")
			for path := range initResults.CodeSnippets {
				sb.WriteString("- " + path + "\n")
			}
		}
		if len(initResults.Errors) > 0 {
			sb.WriteString("\nFailed exploration actions:\n")
			for _, e := range initResults.Errors {
				sb.WriteString("- " + e + "\n")
			}
		}
	} else {
		sb.WriteString("\nExploration Results: none (no init phase ran)\n")
	}

	sb.WriteString("\nGenerate the task plan as JSON.")
	return sb.String()
}

func (p *LLMTaskPlanner) parseResponse(response string, intentSet *IntentSet) ([]TaskSpec, error) {
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in task planning response")
	}

	var parsed struct {
		Tasks []struct {
			ID           string   `json:"id"`
			Type         string   `json:"type"`
			Role         string   `json:"role"`
			Priority     int      `json:"priority"`
			Dependencies []string `json:"dependencies"`
			Description  string   `json:"description"`
			Prompt       string   `json:"prompt"`
			Tools        []string `json:"tools"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse task plan JSON: %w", err)
	}

	if len(parsed.Tasks) == 0 {
		return nil, fmt.Errorf("task plan contained no tasks")
	}

	validIDs := make(map[string]bool, len(parsed.Tasks))
	for _, t := range parsed.Tasks {
		validIDs[t.ID] = true
	}

	tasks := make([]TaskSpec, 0, len(parsed.Tasks))
	for i, t := range parsed.Tasks {
		// Assign IDs when the model omitted them; drop deps pointing at unknown ids.
		id := t.ID
		if id == "" {
			id = fmt.Sprintf("task-%d", i+1)
		}

		deps := make([]string, 0, len(t.Dependencies))
		for _, d := range t.Dependencies {
			if validIDs[d] {
				deps = append(deps, d)
			}
		}

		promptText := t.Prompt
		if promptText == "" {
			promptText = t.Description
		}

		tasks = append(tasks, TaskSpec{
			ID:           id,
			IntentID:     firstIntentID(intentSet),
			Type:         t.Type,
			Role:         t.Role,
			Priority:     t.Priority,
			Dependencies: deps,
			Prompt:       promptText,
			Context: map[string]any{
				"original_request": intentSet.OriginalPrompt,
			},
			Tools:       t.Tools,
			Description: t.Description,
		})
	}

	return dropRedundantAnswerTasks(tasks), nil
}

// dropRedundantAnswerTasks removes "answer"-type tasks that only greet the
// user or report the results of earlier tasks. The final task's completion
// response is the user-visible answer, so such a task produces a second,
// repetitive reply (observed in session 89869a5e: an [analyze] task followed
// by "[answer] Greet the user and report the findings" greeted twice). An
// answer task survives only when it is the sole task — a pure informational
// question — or when its description clearly does work of its own.
func dropRedundantAnswerTasks(tasks []TaskSpec) []TaskSpec {
	if len(tasks) <= 1 {
		return tasks
	}
	answerOnly := func(t TaskSpec) bool {
		if !strings.EqualFold(t.Type, "answer") {
			return false
		}
		d := strings.ToLower(t.Description)
		for _, marker := range []string{
			"greet", "greeting", "hello", "summar", "report", "restate",
			"present the", "tell the user", "respond to the user",
			"acknowledge", "wrap up", "final answer",
		} {
			if strings.Contains(d, marker) {
				return true
			}
		}
		return false
	}
	out := make([]TaskSpec, 0, len(tasks))
	for _, t := range tasks {
		if answerOnly(t) {
			continue
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		// Every task was an answer-report: keep the original plan rather
		// than executing nothing.
		return tasks
	}
	return out
}

func firstIntentID(intentSet *IntentSet) string {
	if intentSet != nil && len(intentSet.Intents) > 0 {
		return intentSet.Intents[0].ID
	}
	return uuid.New().String()[:8]
}
