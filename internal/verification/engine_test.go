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
	res, err := engine.Verify(context.Background(), &Context{WorkingDir: t.TempDir()})
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

func TestVerifyReportsProgress(t *testing.T) {
	cfg := DefaultConfig()
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

	var events []string
	engine := NewEngine(cfg)
	_, err := engine.Verify(context.Background(), &Context{
		WorkingDir: t.TempDir(),
		OnProgress: func(layer Layer, status Status, _ string) {
			events = append(events, string(layer)+":"+string(status))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRunning := []string{"syntax:running", "semantic:running", "test:running", "integration:running"}
	for _, w := range wantRunning {
		found := false
		for _, e := range events {
			if e == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing progress event %q in %v", w, events)
		}
	}
	if len(events) < 8 {
		t.Errorf("expected at least 8 progress events (running+done per layer), got %d: %v", len(events), events)
	}
}

func TestVerifyRespectsContextCancellation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TestHook = func(ctx context.Context, _ *Context) (*LayerResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return &LayerResult{Layer: LayerTest, Status: StatusPassed}, nil
		}
	}
	cfg.SyntaxHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerSyntax, Status: StatusPassed}, nil
	}
	cfg.SemanticHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerSemantic, Status: StatusPassed}, nil
	}
	cfg.IntegrationHook = func(context.Context, *Context) (*LayerResult, error) {
		return &LayerResult{Layer: LayerIntegration, Status: StatusPassed}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine := NewEngine(cfg)
	start := time.Now()
	res, err := engine.Verify(ctx, &Context{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("cancelled context should return immediately, took %v", time.Since(start))
	}
	if res.Status != StatusFailed && res.Status != StatusCancelled {
		t.Fatalf("expected failed/cancelled status, got %s", res.Status)
	}
}

func TestPackageTargetsScoping(t *testing.T) {
	vctx := &Context{
		ChangedFiles: []string{"internal/tools/filesystem/glob.go"},
	}
	targets := packageTargets(vctx, ".")
	if len(targets) != 1 || targets[0] != "./internal/tools/filesystem/" {
		t.Fatalf("unexpected targets: %v", targets)
	}

	vctx = &Context{}
	if got := packageTargets(vctx, "."); len(got) != 1 || got[0] != "./..." {
		t.Fatalf("expected fallback ./..., got %v", got)
	}

	vctx = &Context{ChangedFiles: []string{"/etc/passwd"}}
	if got := packageTargets(vctx, "."); len(got) != 1 || got[0] != "./..." {
		t.Fatalf("expected fallback for out-of-module file, got %v", got)
	}

	vctx = &Context{ChangedFiles: []string{"glob.go", "internal/tools/filesystem/glob.go"}}
	targets = packageTargets(vctx, ".")
	if len(targets) != 2 {
		t.Fatalf("expected 2 deduped targets, got %v", targets)
	}
}
