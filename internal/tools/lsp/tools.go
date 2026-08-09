package lsp

import (
	"context"

	"github.com/iSundram/Automergent/internal/tools"
)

// DiagnosticsTool queries LSP diagnostics for a file.
type DiagnosticsTool struct{}

func (t *DiagnosticsTool) Name() string { return "lsp_diagnostics" }
func (t *DiagnosticsTool) Description() string {
	return "Get LSP diagnostics (errors/warnings) for a file."
}
func (t *DiagnosticsTool) RequiresConfirmation(mode string) bool { return false }

func (t *DiagnosticsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{"type": "string", "description": "File path to check."},
		},
		"required": []string{"file"},
	}
}

func (t *DiagnosticsTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	file, _ := args["file"].(string)
	if file == "" {
		return tools.Result{IsError: true, Content: "file is required"}, nil
	}
	return tools.Result{Content: "LSP diagnostics not yet available for " + file}, nil
}
