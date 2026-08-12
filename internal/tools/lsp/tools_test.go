package lsp

import (
	"context"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/diagnostics"
)

func TestDiagnosticsRequiresFile(t *testing.T) {
	tool := &DiagnosticsTool{}
	res, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result for missing file")
	}
}

func TestRecoverySuffixIncludesGuidance(t *testing.T) {
	msg := recoverySuffix("main.go:1:1: syntax error")
	if msg == "" {
		t.Fatal("expected recovery suffix")
	}
	if !strings.Contains(msg, "[RECOVERY]") {
		t.Fatalf("unexpected suffix: %q", msg)
	}
}

func TestDiagnosticsRecoveryAPI(t *testing.T) {
	report := diagnostics.RecoverCompilerOutput(`main.go:1:1: cannot find package "x"`, "")
	if report.UserMessage == "" {
		t.Fatal("expected recovery guidance")
	}
}
