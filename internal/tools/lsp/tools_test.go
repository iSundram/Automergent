package lsp

import (
	"context"
	"testing"
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
