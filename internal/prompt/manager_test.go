package prompt

import (
	"testing"
	"time"
)

func TestPromptManager_NewPromptManager(t *testing.T) {
	pm := NewPromptManager(nil)
	if pm == nil {
		t.Fatal("expected PromptManager, got nil")
	}
	if pm.config == nil {
		t.Error("expected config to be set")
	}
	if pm.categorizer == nil {
		t.Error("expected categorizer to be set")
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
	pm := NewPromptManager(nil)
	
	parts, err := pm.ProcessUserMessage(
		nil,
		"Add a new REST API endpoint for user authentication",
		"/home/user/project",
		[]string{"internal/api/handlers.go", "internal/auth/jwt.go"},
	)
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Should have initial thinking, categorization, task definition, coder init, workflow plan
	foundStages := make(map[PromptStage]bool)
	for _, part := range parts {
		foundStages[part.Stage] = true
	}
	
	expectedStages := []PromptStage{
		StageInitialThinking,
		StageCategorization,
		StageTaskDefinition,
		StageCoderInit,
		StageWorkflowPlan,
	}
	
	for _, stage := range expectedStages {
		if !foundStages[stage] {
			t.Errorf("expected stage %s not found", stage)
		}
	}
	
	// Check that current request is set
	req := pm.GetCurrentRequest()
	if req == nil {
		t.Error("expected current request to be set")
	}
	if req.Category != CategoryNewFeature {
		t.Errorf("expected category new_feature, got %s", req.Category)
	}
	if !req.RequiresCoder {
		t.Error("expected requires coder to be true for new feature")
	}
	
	// Check coder context is initialized
	coderCtx := pm.GetCoderContext()
	if coderCtx == nil {
		t.Error("expected coder context to be initialized")
	}
	if coderCtx.WorkingDir != "/home/user/project" {
		t.Errorf("expected working dir /home/user/project, got %s", coderCtx.WorkingDir)
	}
	if len(coderCtx.TodoItems) == 0 {
		t.Error("expected todo items to be generated")
	}
}

func TestPromptManager_ProcessUserMessage_Debug(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Debug why the JWT token validation is failing",
		"/home/user/project",
		[]string{"internal/auth/jwt.go"},
	)
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	req := pm.GetCurrentRequest()
	if req == nil {
		t.Error("expected current request to be set")
	}
	if req.Category != CategoryDebug {
		t.Errorf("expected category debug, got %s", req.Category)
	}
}

func TestPromptManager_ProcessUserMessage_Simple(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Show me the current config",
		"/home/user/project",
		[]string{"internal/config/config.go"},
	)
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	req := pm.GetCurrentRequest()
	if req == nil {
		t.Error("expected current request to be set")
	}
	if req.Category != CategoryDirect && req.Category != CategorySimple {
		t.Errorf("expected category direct or simple, got %s", req.Category)
	}
}

func TestPromptManager_GetNextTodoPrompt(t *testing.T) {
	pm := NewPromptManager(nil)
	
	// Process a request first
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Get next todo
	nextPrompt := pm.GetNextTodoPrompt()
	if nextPrompt == nil {
		t.Error("expected next todo prompt")
	}
	if nextPrompt.Stage != StageExecution {
		t.Errorf("expected stage execution, got %s", nextPrompt.Stage)
	}
}

func TestPromptManager_CompleteTodo(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	coderCtx := pm.GetCoderContext()
	if coderCtx == nil || len(coderCtx.TodoItems) == 0 {
		t.Fatal("expected todo items")
	}
	
	todoID := coderCtx.TodoItems[0].ID
	pm.CompleteTodo(todoID, "Done")
	
	// Check todo is marked completed
	coderCtx = pm.GetCoderContext()
	found := false
	for _, todo := range coderCtx.TodoItems {
		if todo.ID == todoID && todo.Status == TodoStatusCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected todo to be marked completed")
	}
}

func TestPromptManager_StashContext(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	stashPrompt := pm.StashContext("Testing stash")
	if stashPrompt == nil {
		t.Error("expected stash prompt")
	}
	if stashPrompt.Stage != StageContextManage {
		t.Errorf("expected stage context_manage, got %s", stashPrompt.Stage)
	}
}

func TestPromptManager_SaveAndResumeStash(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Save stash
	stash := pm.SaveStash("Test summary", []string{"test"})
	if stash == nil {
		t.Error("expected stash to be created")
	}
	if stash.Summary != "Test summary" {
		t.Errorf("expected summary 'Test summary', got %s", stash.Summary)
	}
	
	// Resume stash
	resumePrompt := pm.ResumeContext(stash.ID)
	if resumePrompt == nil {
		t.Error("expected resume prompt")
	}
	if resumePrompt.Stage != StageContextManage {
		t.Errorf("expected stage context_manage, got %s", resumePrompt.Stage)
	}
}

func TestPromptManager_CreateNewContext(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// Create new context using PromptManager's CreateNewContext
	newPrompt := pm.CreateNewContext("Start new task", true)
	if newPrompt == nil {
		t.Error("expected new context prompt")
	}
	if newPrompt.Stage != StageContextManage {
		t.Errorf("expected stage context_manage, got %s", newPrompt.Stage)
	}
	
	// Old request should be cleared
	if pm.GetCurrentRequest() != nil {
		t.Error("expected current request to be cleared")
	}
}

func TestPromptManager_ShareContextWithCoder(t *testing.T) {
	pm := NewPromptManager(nil)
	
	_, err := pm.ProcessUserMessage(
		nil,
		"Add a new feature",
		"/home/user/project",
		[]string{"internal/feature.go"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	sharePrompt := pm.ShareContextWithCoder("Share all context")
	if sharePrompt == nil {
		t.Error("expected share prompt")
	}
	if sharePrompt.Stage != StageContextManage {
		t.Errorf("expected stage context_manage, got %s", sharePrompt.Stage)
	}
}

func TestCategorizer_Categorize(t *testing.T) {
	categorizer := NewCategorizer(nil)
	
	testCases := []struct {
		prompt      string
		expectedCat RequestCategory
	}{
		{"Add a new feature for user login", CategoryNewFeature},
		{"Debug the login error", CategoryDebug},
		{"I suspect the issue is in the auth module", CategoryIssueSuspect},
		{"How do I use the API?", CategoryUserAsking},
		{"Plan the architecture for the new service", CategoryPlan},
		{"Review the pull request", CategoryVerifyWork},
		{"Show me the config file", CategoryDirect},
		{"Quick fix typo", CategorySimple},
	}
	
	for _, tc := range testCases {
		result := categorizer.Categorize(tc.prompt, "/tmp", []string{})
		if result.Category != tc.expectedCat {
			t.Errorf("for prompt %q: expected category %s, got %s", tc.prompt, tc.expectedCat, result.Category)
		}
	}
}

func TestCategorizer_ComplexityDetection(t *testing.T) {
	categorizer := NewCategorizer(nil)
	
	// Simple
	result := categorizer.Categorize("Fix typo", "/tmp", []string{})
	if result.Complexity != ComplexitySimple {
		t.Errorf("expected simple complexity, got %s", result.Complexity)
	}
	
	// Moderate (multiple files)
	result = categorizer.Categorize("Add new API endpoint", "/tmp", []string{"a.go", "b.go", "c.go", "d.go"})
	if result.Complexity != ComplexityModerate {
		t.Errorf("expected moderate complexity, got %s", result.Complexity)
	}
	
	// Complex (many files + complex keywords)
	result = categorizer.Categorize("Refactor the entire authentication system", "/tmp", []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go", "i.go", "j.go", "k.go"})
	if result.Complexity != ComplexityComplex {
		t.Errorf("expected complex complexity, got %s", result.Complexity)
	}
}

func TestAssistantPrompts_BuildInitialThinkingPrompt(t *testing.T) {
	ap := NewAssistantPrompts(nil)
	prompt := ap.BuildInitialThinkingPrompt("Test prompt")
	
	if prompt == nil {
		t.Fatal("expected prompt")
	}
	if prompt.Stage != StageInitialThinking {
		t.Errorf("expected stage initial_thinking, got %s", prompt.Stage)
	}
	if prompt.Tools != ToolSetContextOnly {
		t.Errorf("expected tools context_only, got %s", prompt.Tools)
	}
}

func TestCoderPrompts_BuildCoderInitPrompt(t *testing.T) {
	cp := NewCoderPrompts(nil)
	coderCtx := &CoderContext{
		WorkingDir:   "/tmp",
		Files:        []string{"test.go"},
		CodeSnippets: map[string]string{"test.go": "package main"},
		TodoItems: []TodoItem{
			{ID: "1", Description: "Test todo", Status: TodoStatusPending, Priority: 1},
		},
	}
	categorized := &CategorizedRequest{
		OriginalPrompt: "Test task",
		Category:       CategoryNewFeature,
		Complexity:     ComplexityModerate,
		Strategy:       StrategyCoderAgent,
		AllowedTools:   ToolSetModerate,
	}
	
	prompt := cp.BuildCoderInitPrompt(coderCtx, categorized)
	if prompt == nil {
		t.Fatal("expected prompt")
	}
	if prompt.Stage != StageCoderInit {
		t.Errorf("expected stage coder_init, got %s", prompt.Stage)
	}
	if prompt.Tools != ToolSetModerate {
		t.Errorf("expected tools moderate, got %s", prompt.Tools)
	}
}

func TestContextPrompts_BuildStashPrompt(t *testing.T) {
	cp := NewContextPrompts(nil)
	assistantCtx := &AssistantContext{
		ConversationHistory: []Message{
			{Role: "user", Content: "Hello", Timestamp: time.Now()},
		},
		CurrentTask: &CategorizedRequest{
			Category:       CategoryNewFeature,
			OriginalPrompt: "Test task",
		},
	}
	
	prompt := cp.BuildStashPrompt(assistantCtx, "Test reason")
	if prompt == nil {
		t.Fatal("expected prompt")
	}
	if prompt.Stage != StageContextManage {
		t.Errorf("expected stage context_manage, got %s", prompt.Stage)
	}
}

func TestWorkflowPrompts_BuildTodoWalkthroughPrompt(t *testing.T) {
	wp := NewWorkflowPrompts(nil)
	todos := []TodoItem{
		{ID: "1", Description: "Todo 1", Status: TodoStatusCompleted, Priority: 1},
		{ID: "2", Description: "Todo 2", Status: TodoStatusInProgress, Priority: 2},
		{ID: "3", Description: "Todo 3", Status: TodoStatusPending, Priority: 3},
	}
	
	prompt := wp.BuildTodoWalkthroughPrompt(todos, 1)
	if prompt == nil {
		t.Fatal("expected prompt")
	}
	if prompt.Stage != StageWorkflowPlan {
		t.Errorf("expected stage workflow_plan, got %s", prompt.Stage)
	}
}

func TestToolPrompts_BuildReadFilePrompt(t *testing.T) {
	tp := NewToolPrompts(nil)
	prompt := tp.BuildReadFilePrompt("/tmp/test.go", 1, 10)
	
	if prompt == nil {
		t.Fatal("expected prompt")
	}
	if prompt.Stage != StageExecution {
		t.Errorf("expected stage execution, got %s", prompt.Stage)
	}
	if prompt.Tools != ToolSetReadOnly {
		t.Errorf("expected tools read_only, got %s", prompt.Tools)
	}
}

func TestPromptSystem_Integration(t *testing.T) {
	ps := NewPromptSystem()
	
	// Process a request
	parts, err := ps.ProcessUserMessage(nil, "Add new feature", "/tmp", []string{"main.go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parts) == 0 {
		t.Error("expected prompt parts")
	}
	
	// Get next action
	next := ps.GetNextAction()
	if next == nil {
		t.Error("expected next action")
	}
	
	// Complete task
	complete := ps.CompleteCurrentTask("Done")
	if complete == nil {
		t.Error("expected completion prompt")
	}
}