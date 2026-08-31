package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/diagnostics"
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
	case ".py":
		return lintDiagnostics(ctx, file), nil
	default:
		return tools.Result{
			Content: fmt.Sprintf("lsp_diagnostics: no language backend configured for %s (supported: .go, .py)", ext),
		}, nil
	}
}

// lintDiagnostics runs external linters (ruff for Python) on the file and
// renders their findings. The linters read the file from disk themselves;
// content is only used for language detection via the path.
func lintDiagnostics(ctx context.Context, file string) tools.Result {
	diags := diagnostics.Lint(ctx, file, "")
	if len(diags) == 0 {
		return tools.Result{Content: "no lint findings"}
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(fmt.Sprintf("%s:%d:%d: %s: %s [%s]\n",
			file, d.Line, d.Column, d.Severity, d.Message, d.Code))
	}
	return tools.Result{Content: strings.TrimRight(b.String(), "\n")}
}

func goBuildDiagnostics(ctx context.Context, file string) tools.Result {
	dir := filepath.Dir(file)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".automergent-gobuild-*")
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
		content := strings.Join(focused, "\n")
		return tools.Result{Content: content + recoverySuffix(content)}
	}
	return tools.Result{Content: msg + recoverySuffix(msg)}
}

func recoverySuffix(output string) string {
	report := diagnostics.RecoverCompilerOutput(output, "")
	if report.UserMessage == "" {
		return ""
	}
	return "\n\n" + report.Render()
}

// EstimatedCost returns cost estimates for the diagnostics tool.
func (t *DiagnosticsTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 200, LatencyMs: 500, RiskLevel: "low"}
}
