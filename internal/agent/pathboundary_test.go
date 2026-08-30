package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/tools"
)

// pathBoundaryTool is a minimal tool whose read/write classification the
// boundary check consults.
type pathBoundaryTool struct {
	name  string
	write bool
}

func (t *pathBoundaryTool) Name() string                           { return t.name }
func (t *pathBoundaryTool) Description() string                    { return t.name }
func (t *pathBoundaryTool) Schema() map[string]any                 { return map[string]any{"type": "object"} }
func (t *pathBoundaryTool) Execute(context.Context, map[string]any) (tools.Result, error) {
	return tools.Result{}, nil
}
func (t *pathBoundaryTool) RequiresConfirmation(string) bool       { return false }
func (t *pathBoundaryTool) IsConcurrencySafe(map[string]any) bool  { return true }
func (t *pathBoundaryTool) IsReadOnly(map[string]any) bool         { return !t.write }
func (t *pathBoundaryTool) IsDestructive(map[string]any) bool      { return false }
func (t *pathBoundaryTool) EstimatedCost() tools.ToolCost          { return tools.ToolCost{} }

func testCfg() *config.Config { return &config.Config{} }

func TestCheckPathBoundary(t *testing.T) {
	dir := t.TempDir()
	ag := &Agent{cfg: testCfg(), workDir: dir}

	readTool := &pathBoundaryTool{name: "read_file", write: false}
	writeTool := &pathBoundaryTool{name: "edit_file", write: true}

	// In-bounds read and write pass without a prompt.
	if d := ag.checkPathBoundary(ai.ToolCall{Name: "read_file", Args: map[string]any{"path": filepath.Join(dir, "f.go")}}, readTool); !d.Allowed {
		t.Errorf("in-bounds read blocked: %+v", d)
	}
	if d := ag.checkPathBoundary(ai.ToolCall{Name: "edit_file", Args: map[string]any{"path": filepath.Join(dir, "f.go")}}, writeTool); !d.Allowed {
		t.Errorf("in-bounds write blocked: %+v", d)
	}

	// Out-of-bounds paths are flagged with the offending directory.
	outside := t.TempDir()
	decision := ag.checkPathBoundary(ai.ToolCall{Name: "read_file", Args: map[string]any{"path": filepath.Join(outside, "secret.txt")}}, readTool)
	if decision.Allowed || decision.OutsideDir != filepath.Join(outside, "secret.txt") {
		t.Fatalf("out-of-bounds read not flagged: %+v", decision)
	}

	// Protected locations ask even in-bounds.
	decision = ag.checkPathBoundary(ai.ToolCall{Name: "edit_file", Args: map[string]any{"path": filepath.Join(dir, ".git", "config")}}, writeTool)
	if decision.Allowed {
		t.Fatalf("write into .git not flagged: %+v", decision)
	}

	// Bash: cd outside the project is flagged against the shell cwd.
	decision = ag.checkPathBoundary(ai.ToolCall{Name: "bash", Args: map[string]any{"command": "cat /etc/hosts"}}, &pathBoundaryTool{name: "bash", write: false})
	if decision.Allowed {
		t.Fatalf("bash absolute path outside project not flagged: %+v", decision)
	}
	decision = ag.checkPathBoundary(ai.ToolCall{Name: "bash", Args: map[string]any{"command": "cd " + dir + " && ls"}}, &pathBoundaryTool{name: "bash", write: false})
	if !decision.Allowed {
		t.Fatalf("bash in-project cd flagged: %+v", decision)
	}
}

func TestPathGrantForDirectories(t *testing.T) {
	dir := t.TempDir()
	ag := &Agent{cfg: testCfg(), workDir: dir}

	// File tool: the grant covers the file's parent directory.
	readTool := &pathBoundaryTool{name: "read_file", write: false}
	decision := tools.PathDecision{OutsideDir: "/outside/proj/file.txt"}
	grant := ag.pathGrantFor(ai.ToolCall{Name: "read_file"}, readTool, decision)
	d, ok := tools.IsDirGrant(grant)
	if !ok || d != "/outside/proj" {
		t.Fatalf("file grant = %q (dir %q)", grant, d)
	}

	// Shell tool: the grant covers the flagged directory itself.
	bashTool := &pathBoundaryTool{name: "bash", write: false}
	decision = tools.PathDecision{OutsideDir: "/outside"}
	grant = ag.pathGrantFor(ai.ToolCall{Name: "bash"}, bashTool, decision)
	if d, ok := tools.IsDirGrant(grant); !ok || d != "/outside" {
		t.Fatalf("bash grant = %q", grant)
	}

	// Grants carry the project prefix like every other approval scope.
	if want := "project=" + dir + ";" + tools.GrantScope("/outside"); grant != want {
		t.Fatalf("grant = %q, want %q", grant, want)
	}
}

func TestBoundaryGrantsAllowSubsequentAccess(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	ag := &Agent{cfg: testCfg(), workDir: dir}

	// Simulate an always-allow answer: the grant is recorded and the scope
	// learns the directory.
	ag.pathScope().AddGrantedDir(outside)
	readTool := &pathBoundaryTool{name: "read_file", write: false}
	if d := ag.checkPathBoundary(ai.ToolCall{Name: "read_file", Args: map[string]any{"path": filepath.Join(outside, "data.json")}}, readTool); !d.Allowed {
		t.Fatalf("granted dir still flagged: %+v", d)
	}
}
