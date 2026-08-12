package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyntaxChecksGoAndJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := runSyntaxChecks(context.Background(), &Context{WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusFailed {
		t.Fatalf("expected failed syntax status, got %s", res.Status)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected syntax issue")
	}
}

func TestVerifyUsesHooks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = time.Second
	cfg.SyntaxHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerSyntax, Status: StatusPassed}, nil
	}
	cfg.SemanticHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerSemantic, Status: StatusPassed}, nil
	}
	cfg.TestHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerTest, Status: StatusPassed}, nil
	}
	cfg.IntegrationHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerIntegration, Status: StatusPassed}, nil
	}

	engine := NewEngine(cfg)
	res, err := engine.Verify(&Context{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusPassed {
		t.Fatalf("expected passed, got %s", res.Status)
	}
	if !res.CanProceed {
		t.Fatal("expected can proceed")
	}
	if len(res.Layers) != 4 {
		t.Fatalf("expected 4 layers, got %d", len(res.Layers))
	}
}

func TestDefaultEngineHasHooks(t *testing.T) {
	engine := NewDefaultEngine()
	if engine == nil || engine.GetConfig() == nil {
		t.Fatal("expected engine and config")
	}
	if engine.GetConfig().SyntaxHook == nil || engine.GetConfig().TestHook == nil {
		t.Fatal("expected default hooks")
	}
}
