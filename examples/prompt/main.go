package main

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/prompt"
)

func main() {
	// Create a new prompt system with mock LLM client
	mockClient := &prompt.MockLLMClient{
		Response: `{"intents":[{"type":"implement","priority":2,"confidence":0.9,"raw_text":"Add a new REST API endpoint","parameters":{"type":"api"},"dependencies":[]}],"requires_init":false}`,
	}
	ps := prompt.NewPromptSystemWithLLM(prompt.DefaultPromptConfig(), nil, "/home/user/project", mockClient, nil)

	// Example 1: Process a new feature request
	fmt.Println("=== Example 1: New Feature Request ===")
	parts, err := ps.ProcessUserMessage(
		context.Background(),
		"Add a new REST API endpoint for user authentication with JWT tokens",
		"/home/user/project",
		[]string{"internal/api/handlers.go", "internal/auth/jwt.go", "internal/config/config.go"},
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Generated %d prompt parts:\n", len(parts))
	for i, part := range parts {
		fmt.Printf("\n--- Part %d: %s ---\n", i+1, part.Stage)
		fmt.Printf("Tools: %s\n", part.Tools)
		fmt.Printf("Content preview: %s...\n", truncate(part.Content, 200))
	}

	// Example 2: Process a debug request
	fmt.Println("\n=== Example 2: Debug Request ===")
	mockClient2 := &prompt.MockLLMClient{
		Response: `{"intents":[{"type":"debug","priority":1,"confidence":0.9,"raw_text":"Debug why JWT failing","parameters":{"error_type":"token"},"dependencies":[]}],"requires_init":true,"init_goal":"Understand...","init_actions":[{"tool":"grep","target":"error","reason":"Find errors"}]}`,
	}
	ps2 := prompt.NewPromptSystemWithLLM(prompt.DefaultPromptConfig(), nil, "/home/user/project", mockClient2, nil)

	parts2, err := ps2.ProcessUserMessage(
		context.Background(),
		"Debug why the JWT token validation is failing with 'token expired' error",
		"/home/user/project",
		[]string{"internal/auth/jwt.go", "internal/middleware/auth.go"},
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Generated %d prompt parts:\n", len(parts2))
	for i, part := range parts2 {
		fmt.Printf("\n--- Part %d: %s ---\n", i+1, part.Stage)
		fmt.Printf("Tools: %s\n", part.Tools)
		fmt.Printf("Content preview: %s...\n", truncate(part.Content, 200))
	}

	// Example 3: Get next todo for execution
	fmt.Println("\n=== Example 3: Next Todo ===")
	nextPrompt := ps.GetNextAction()
	if nextPrompt != nil {
		fmt.Printf("Next action: %s\n", nextPrompt.Stage)
		fmt.Printf("Tools: %s\n", nextPrompt.Tools)
		fmt.Printf("Content preview: %s...\n", truncate(nextPrompt.Content, 200))
	}

	// Example 4: Complete a todo and inject context
	fmt.Println("\n=== Example 4: Complete Todo & Inject Context ===")
	turnCtx := ps.GetTurnContext()
	if turnCtx != nil && len(turnCtx.TodoItems) > 0 {
		todoID := turnCtx.TodoItems[0].ID
		ps.CompleteCurrentTask("") // We need a different way to complete todo
		// Actually use the manager directly for this demo
		_ = todoID
		fmt.Printf("Todo ID: %s\n", todoID)
	}

	// Example 5: Stash context
	fmt.Println("\n=== Example 5: Stash Context ===")
	stashPrompt := ps.StashCurrentContext("Switching to different feature branch")
	if stashPrompt != nil {
		fmt.Printf("Stash prompt generated: %s\n", stashPrompt.Stage)
	}

	// Example 6: Create fresh context
	fmt.Println("\n=== Example 6: Fresh Context ===")
	freshPrompt := ps.CreateFreshContext("Start working on payment integration")
	if freshPrompt != nil {
		fmt.Printf("Fresh context prompt: %s\n", freshPrompt.Stage)
	}

	// Example 7: Resume stashed context
	fmt.Println("\n=== Example 7: Resume Context ===")
	stashedContexts := ps.GetStashedContexts()
	if len(stashedContexts) > 0 {
		resumePrompt := ps.ResumeStashedContext(stashedContexts[0].ID)
		if resumePrompt != nil {
			fmt.Printf("Resume prompt: %s\n", resumePrompt.Stage)
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}