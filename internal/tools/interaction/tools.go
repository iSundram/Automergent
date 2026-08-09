package interaction

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/tools"
)

// AskUserTool prompts the user for input.
type AskUserTool struct {
	responder func(question string) (string, error)
}

func NewAskUserTool(responder func(string) (string, error)) *AskUserTool {
	return &AskUserTool{responder: responder}
}

func (t *AskUserTool) Name() string                          { return "ask_user" }
func (t *AskUserTool) Description() string                   { return "Ask the user a question and get their response." }
func (t *AskUserTool) RequiresConfirmation(mode string) bool { return false }

func (t *AskUserTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "Question to ask the user."},
		},
		"required": []string{"question"},
	}
}

func (t *AskUserTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	question, ok := tools.StringArg(args, "question")
	if !ok || question == "" {
		return tools.Result{IsError: true, Content: "question is required"}, nil
	}
	if t.responder == nil {
		return tools.Result{IsError: true, Content: "no responder configured"}, nil
	}
	answer, err := t.responder(question)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("user interaction error: %v", err)}, nil
	}
	return tools.Result{Content: answer}, nil
}

// NotifyTool sends a notification to the user.
type NotifyTool struct{}

func (t *NotifyTool) Name() string                          { return "notify" }
func (t *NotifyTool) Description() string                   { return "Show a notification message to the user." }
func (t *NotifyTool) RequiresConfirmation(mode string) bool { return false }

func (t *NotifyTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "description": "Message to display."},
			"level":   map[string]any{"type": "string", "enum": []string{"info", "warning", "error"}},
		},
		"required": []string{"message"},
	}
}

func (t *NotifyTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	message, ok := tools.StringArg(args, "message")
	if !ok || message == "" {
		return tools.Result{IsError: true, Content: "message is required"}, nil
	}
	return tools.Result{Content: fmt.Sprintf("notification: %s", message)}, nil
}
