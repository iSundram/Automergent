package prompt

import (
	"strings"
	"context"
	"fmt"
	"testing"
)

func TestPromptManager_NewPromptManager(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"add new API","parameters":{"type":"api"},"dependencies":[]}],"requires_init":false}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)
	if pm == nil {
		t.Fatal("expected PromptManager, got nil")
	}
	if pm.config == nil {
		t.Error("expected config to be set")
	}
	if pm.intentIdentifier == nil {
		t.Error("expected intentIdentifier to be set")
	}
	if pm.initExecutor == nil {
		t.Error("expected initExecutor to be set")
	}
	if pm.taskPlanner == nil {
		t.Error("expected taskPlanner to be set")
	}
	if pm.assistantPrompts == nil {
		t.Error("expected assistantPrompts to be set")
	}
	if pm.coderPrompts == nil {
		t.Error("expected coderPrompts to be set")
	}
	if pm.contextPrompts == nil {
		t.Error("expected contextPrompts to be set")
	}
	if pm.workflowPrompts == nil {
		t.Error("expected workflowPrompts to be set")
	}
	if pm.toolPrompts == nil {
		t.Error("expected toolPrompts to be set")
	}
}

func TestPromptManager_ProcessUserMessage_NewFeature(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"Add a new REST API endpoint","parameters":{"type":"api"},"dependencies":[]}],"requires_init":false}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)

	parts, err := pm.ProcessUserMessage(
		context.Background(),
		"Add a new REST API endpoint for user authentication",
		"/home/user/project",
		[]string{"internal/api/handlers.go", "internal/auth/jwt.go"},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundStages := make(map[PromptStage]bool)
	for _, part := range parts {
		foundStages[part.Stage] = true
	}

	expectedStages := []PromptStage{
		StageInitialThinking,
		StageCategorization,
	}

	for _, stage := range expectedStages {
		if !foundStages[stage] {
			t.Errorf("expected stage %s not found", stage)
		}
	}

	intentSet := pm.GetCurrentIntentSet()
	if intentSet == nil {
		t.Error("expected current intent set to be set")
	}

	foundImplement := false
	for _, intent := range intentSet.Intents {
		if intent.Type == IntentImplement {
			foundImplement = true
			break
		}
	}
	if !foundImplement {
		t.Error("expected implement intent to be identified")
	}

	tasks := pm.GetCurrentTasks()
	if len(tasks) == 0 {
		t.Error("expected tasks to be generated")
	}

	turnCtx := pm.GetTurnContext()
	if turnCtx == nil {
		t.Fatal("expected unified turn context to be initialized")
	}
	if turnCtx.WorkingDir != "/home/user/project" {
		t.Errorf("expected working dir /home/user/project, got %s", turnCtx.WorkingDir)
	}
	if len(turnCtx.TodoItems) == 0 {
		t.Error("expected todo items to be generated on the turn context")
	}
}

func TestPromptManager_ProcessUserMessage_Debug(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"debug","priority":1,"confidence":0.9,"raw_text":"debug why JWT failing","parameters":{"error_type":"token"},"dependencies":[]}],"requires_init":true,"init_goal":"Understand the codebase to address: debug why JWT failing","init_actions":[{"tool":"grep","target":"error","reason":"Search for errors"}]}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)

	_, err := pm.ProcessUserMessage(
		context.Background(),
		"Debug why the JWT token validation is failing",
		"/home/user/project",
		[]string{"internal/auth/jwt.go"},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	intentSet := pm.GetCurrentIntentSet()
	if intentSet == nil {
		t.Error("expected current intent set to be set")
	}

	foundDebug := false
	for _, intent := range intentSet.Intents {
		if intent.Type == IntentDebug {
			foundDebug = true
			break
		}
	}
	if !foundDebug {
		t.Error("expected debug intent to be identified")
	}
}

func TestPromptManager_ProcessUserMessage_ExploreAndFix(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"explore","priority":1,"confidence":0.9,"raw_text":"see files related to context","parameters":{"target":"context"},"dependencies":[]},{"type":"fix","priority":2,"confidence":0.85,"raw_text":"fix it","parameters":{"area":"context"},"dependencies":[]},{"type":"commit","priority":4,"confidence":0.95,"raw_text":"git commit","parameters":{"message":"fix context issue"},"dependencies":[]}],"requires_init":true,"init_goal":"Understand the codebase to address: see files related to context and there in building context there is issue, fix it and git commit","init_actions":[{"tool":"glob","target":"**/*context*","reason":"Find context-related files"}]}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)

	_, err := pm.ProcessUserMessage(
		context.Background(),
		"see files related to context and there in building context there is issue, fix it and git commit",
		"/home/user/project",
		[]string{},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	intentSet := pm.GetCurrentIntentSet()
	if intentSet == nil {
		t.Error("expected current intent set to be set")
	}

	intentTypes := make(map[IntentType]bool)
	for _, intent := range intentSet.Intents {
		intentTypes[intent.Type] = true
	}

	expectedIntents := []IntentType{IntentExplore, IntentFix, IntentCommit}
	for _, expected := range expectedIntents {
		if !intentTypes[expected] {
			t.Errorf("expected intent %s to be identified", expected)
		}
	}

	if !intentSet.RequiresInit {
		t.Error("expected requires init to be true for explore and fix")
	}
	if intentSet.InitPhase == nil {
		t.Error("expected init phase to be set")
	}

	if len(intentSet.InitPhase.Actions) == 0 {
		t.Error("expected init phase to have actions")
	}
}

func TestPromptManager_ContinuationIsRecordedOnceAndSharesContext(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"Add thinking effort support","parameters":{},"dependencies":[]}],"requires_init":false}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)
	if _, err := pm.ProcessUserMessage(context.Background(), "Add thinking effort support", "/project", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.ProcessUserMessage(context.Background(), "Continue, wire it into the TUI", "/project", nil); err != nil {
		t.Fatal(err)
	}
	ctx := pm.GetTurnContext()
	if len(ctx.ConversationHistory) != 2 {
		t.Errorf("expected 2 messages in history, got %d", len(ctx.ConversationHistory))
	}
	if ctx.ConversationHistory[0].Role != "user" {
		t.Errorf("expected first message to be user, got %s", ctx.ConversationHistory[0].Role)
	}
	if ctx.ConversationHistory[1].Role != "user" {
		t.Errorf("expected second message to be user, got %s", ctx.ConversationHistory[1].Role)
	}
}

func TestLLMTaskPlanner_PlanTasks(t *testing.T) {
	mockClient := &MockLLMClient{
		TaskResponse: `{"tasks":[{"id":"task-1","type":"analyze","role":"researcher","priority":1,"dependencies":[],"description":"Analyze handler","prompt":"Read internal/api/handlers.go and plan the endpoint"},{"id":"task-2","type":"implement","role":"coder","priority":2,"dependencies":["task-1"],"description":"Implement endpoint","prompt":"Implement in internal/api/handlers.go"}]}`,
	}
	planner := NewLLMTaskPlanner(nil, mockClient)

	intentSet := &IntentSet{
		Intents:        []Intent{{ID: "i1", Type: IntentImplement, Priority: 2}},
		OriginalPrompt: "add a new API endpoint",
	}
	initResults := &InitResults{
		FilesFound:   []string{"internal/api/handlers.go"},
		CodeSnippets: map[string]string{"internal/api/handlers.go": "// handler code"},
	}

	tasks, err := planner.PlanTasks(context.Background(), intentSet, initResults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Type != "analyze" || tasks[1].Type != "implement" {
		t.Errorf("unexpected task types: %s, %s", tasks[0].Type, tasks[1].Type)
	}
	if len(tasks[1].Dependencies) != 1 || tasks[1].Dependencies[0] != "task-1" {
		t.Errorf("expected implement to depend on task-1, got %v", tasks[1].Dependencies)
	}
	if !strings.Contains(tasks[0].Prompt, "internal/api/handlers.go") {
		t.Error("expected task prompt to reference actual discovered file")
	}
}

func TestLLMTaskPlanner_DropsUnknownDependencies(t *testing.T) {
	mockClient := &MockLLMClient{
		TaskResponse: `{"tasks":[{"id":"task-1","type":"fix","role":"coder","priority":1,"dependencies":["nonexistent"],"description":"Fix","prompt":"Fix it"}]}`,
	}
	planner := NewLLMTaskPlanner(nil, mockClient)

	intentSet := &IntentSet{OriginalPrompt: "fix bug"}
	tasks, err := planner.PlanTasks(context.Background(), intentSet, &InitResults{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if len(tasks[0].Dependencies) != 0 {
		t.Errorf("expected unknown dependency to be dropped, got %v", tasks[0].Dependencies)
	}
}

func TestLLMTaskPlanner_Error(t *testing.T) {
	mockClient := &MockLLMClient{Error: fmt.Errorf("LLM down")}
	planner := NewLLMTaskPlanner(nil, mockClient)

	_, err := planner.PlanTasks(context.Background(), &IntentSet{OriginalPrompt: "x"}, &InitResults{})
	if err == nil {
		t.Fatal("expected error from planner when LLM fails")
	}
}

func TestLLMIntentIdentifier_IdentifyIntents(t *testing.T) {
	tests := []struct {
		name           string
		userMessage    string
		mockResponse   string
		expectedTypes  []IntentType
		expectedInit   bool
	}{
		{
			name:        "single implement intent",
			userMessage: "Add a new REST API endpoint for user authentication",
			mockResponse: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"Add a new REST API endpoint","parameters":{"type":"api"},"dependencies":[]}],"requires_init":false}`,
			expectedTypes: []IntentType{IntentImplement},
			expectedInit:  false,
		},
		{
			name:        "explore and fix with init",
			userMessage: "see files related to context and there in building context there is issue, fix it",
			mockResponse: `{"intents":[{"type":"explore","priority":1,"confidence":0.9,"raw_text":"see files related to context","parameters":{"target":"context"},"dependencies":[]},{"type":"fix","priority":2,"confidence":0.85,"raw_text":"fix it","parameters":{"area":"context"},"dependencies":[]}],"requires_init":true,"init_goal":"Understand the codebase to address: see files related to context and there in building context there is issue, fix it","init_actions":[{"tool":"glob","target":"**/*context*","reason":"Find context-related files"}]}`,
			expectedTypes: []IntentType{IntentExplore, IntentFix},
			expectedInit:  true,
		},
		{
			name:        "multi-intent: explore, implement, test, commit",
			userMessage: "explore the codebase, implement a new feature, write tests, and commit",
			mockResponse: `{"intents":[{"type":"explore","priority":1,"confidence":0.9,"raw_text":"explore the codebase","parameters":{},"dependencies":[]},{"type":"implement","priority":2,"confidence":0.85,"raw_text":"implement a new feature","parameters":{},"dependencies":[]},{"type":"test","priority":3,"confidence":0.8,"raw_text":"write tests","parameters":{},"dependencies":[]},{"type":"commit","priority":4,"confidence":0.95,"raw_text":"and commit","parameters":{},"dependencies":[]}],"requires_init":true,"init_goal":"Understand the codebase to address: explore the codebase, implement a new feature, write tests, and commit","init_actions":[{"tool":"glob","target":"**/*","reason":"Find relevant files"}]}`,
			expectedTypes: []IntentType{IntentExplore, IntentImplement, IntentTest, IntentCommit},
			expectedInit:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &MockLLMClient{Response: tc.mockResponse}
			identifier := NewLLMIntentIdentifier(nil, mockClient)

			intentSet := identifier.IdentifyIntents(context.Background(), tc.userMessage, "/tmp", []string{})
			if intentSet == nil {
				t.Fatal("expected intent set, got nil")
			}
			if len(intentSet.Intents) == 0 {
				t.Fatal("expected at least one intent")
			}

			found := make(map[IntentType]bool)
			for _, intent := range intentSet.Intents {
				found[intent.Type] = true
			}

			for _, expected := range tc.expectedTypes {
				if !found[expected] {
					t.Errorf("expected intent %s not found", expected)
				}
			}

			if intentSet.RequiresInit != tc.expectedInit {
				t.Errorf("expected requires_init=%v, got %v", tc.expectedInit, intentSet.RequiresInit)
			}

			if tc.expectedInit && intentSet.InitPhase == nil {
				t.Error("expected init phase to be set")
			}
		})
	}
}

func TestLLMIntentIdentifier_ErrorHandling(t *testing.T) {
	mockClient := &MockLLMClient{Error: fmt.Errorf("LLM unavailable")}
	identifier := NewLLMIntentIdentifier(nil, mockClient)

	intentSet := identifier.IdentifyIntents(context.Background(), "add a new feature", "/tmp", []string{})
	if intentSet == nil {
		t.Fatal("expected intent set from error handling")
	}
	if len(intentSet.Intents) == 0 {
		t.Error("expected at least one intent from error handling")
	}
	if intentSet.Intents[0].Type != IntentQuestion {
		t.Errorf("expected IntentQuestion on error, got %s", intentSet.Intents[0].Type)
	}
}

func TestPromptManager_WithLLMClient(t *testing.T) {
	mockClient := &MockLLMClient{
		Response: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"Add a new REST API endpoint","parameters":{"type":"api"},"dependencies":[]}],"requires_init":false}`,
		TaskResponse: `{"tasks":[{"id":"task-1","type":"implement","role":"coder","priority":1,"dependencies":[],"description":"Do the work","prompt":"Do the requested work"}]}`,
	}
	pm := NewPromptManager(nil, nil, "", mockClient, nil)

	_, err := pm.ProcessUserMessage(
		context.Background(),
		"Add a new REST API endpoint for user authentication",
		"/home/user/project",
		[]string{"internal/api/handlers.go", "internal/auth/jwt.go"},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	intentSet := pm.GetCurrentIntentSet()
	if intentSet == nil {
		t.Fatal("expected current intent set to be set")
	}

	foundImplement := false
	for _, intent := range intentSet.Intents {
		if intent.Type == IntentImplement {
			foundImplement = true
			break
		}
	}
	if !foundImplement {
		t.Error("expected implement intent from LLM")
	}
}