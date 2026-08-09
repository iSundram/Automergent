package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// DiagnosticsTool runs Go compiler checks for a file (compile errors for the file's package).
// This is not full LSP semantic analysis, but it reports real compiler output instead of
// unrelated test results.
type DiagnosticsTool struct{}

func (t *DiagnosticsTool) Name() string { return "lsp_diagnostics" }
func (t *DiagnosticsTool) Description() string {
	return "Get compiler diagnostics for a Go file (package build errors for the file's directory)."
}
func (t *DiagnosticsTool) RequiresConfirmation(mode string) bool { return false }

func (t *DiagnosticsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{"type": "string", "description": "Path to a .go source file."},
		},
		"required": []string{"file"},
	}
}

func (t *DiagnosticsTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	file, ok := tools.StringArg(args, "file")
	if !ok || file == "" {
		return tools.Result{IsError: true, Content: "file is required"}, nil
	}
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".go":
		return goBuildDiagnostics(ctx, file), nil
	default:
		return tools.Result{
			Content: fmt.Sprintf("lsp_diagnostics: no language backend configured for %s", ext),
		}, nil
	}
}

func goBuildDiagnostics(ctx context.Context, file string) tools.Result {
	dir := filepath.Dir(file)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp("", "automergent-gobuild-*")
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("temp file: %v", err)}
	}
	outPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", outPath, ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err == nil {
		if msg != "" {
			return tools.Result{Content: msg}
		}
		return tools.Result{Content: "no compile errors (go build succeeded)"}
	}
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
	return tools.Result{Content: msg}
}
