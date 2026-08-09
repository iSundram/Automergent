package lsp

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

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
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".go":
		return goDiagnostics(file), nil
	default:
		return tools.Result{
			Content: fmt.Sprintf("lsp_diagnostics: no language backend configured for %s", ext),
		}, nil
	}
}

func goDiagnostics(file string) tools.Result {
	dir := filepath.Dir(file)
	if dir == "" {
		dir = "."
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return tools.Result{Content: "no diagnostics"}
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	base := filepath.Base(file)
	var focused []string
	for _, line := range strings.Split(msg, "\n") {
		if strings.Contains(line, file) || strings.Contains(line, base) {
			focused = append(focused, line)
		}
	}
	if len(focused) > 0 {
		return tools.Result{Content: strings.Join(focused, "\n")}
	}
	if msg == "" {
		msg = err.Error()
	}
	return tools.Result{Content: msg}
}
