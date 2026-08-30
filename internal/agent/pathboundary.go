package agent

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
	toolsshell "github.com/iSundram/Automergent/internal/tools/shell"
)

// Working-directory boundary integration (see tools/pathscope.go for the
// decision logic): the agent owns the PathScope, checks every tool call
// before the mode-based approval flow, and renders out-of-bounds prompts in
// the reference agent's voice — the user is told WHICH path is outside, not
// just asked to confirm a tool.

// pathScope returns the agent's boundary scope, creating it around the
// working directory on first use.
func (a *Agent) pathScope() *tools.PathScope {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.boundary == nil {
		a.boundary = tools.NewPathScope(a.workDir)
	}
	return a.boundary
}

// stripProjectPrefix removes the project= prefix a persisted grant scope
// carries, leaving the bare pathdir: scope content.
func stripProjectPrefix(scope, dir string) string {
	// The grant stored in the session is "project=<dir>;pathdir:<dir>"; the
	// IsDirGrant helper already extracted the pathdir part, so nothing more
	// is needed — but scopes saved before the prefix existed may be bare.
	_ = scope
	return dir
}

// checkPathBoundary applies the boundary rules to one tool call.
func (a *Agent) checkPathBoundary(tc ai.ToolCall, t tools.Tool) tools.PathDecision {
	scope := a.pathScope()
	write := !t.IsReadOnly(tc.Args)

	// Plan-phase write restriction: during the PLAN phase the only writable
	// location is the artifacts directory (plans, review documents). All
	// other writes wait for the BUILD phase — the plan itself says so, and
	// this makes it a fact about the runtime rather than a suggestion.
	if write && a.currentPhaseIsPlan() {
		if decision := a.checkPlanArtifactWrite(tc); !decision.Allowed {
			return decision
		}
	}

	// The shell tool gets command-level analysis: cd targets resolved
	// against the shell's persistent working directory, absolute path
	// arguments, and the compound cd+write guard.
	if tc.Name == "bash" {
		if command, ok := tc.Args["command"].(string); ok {
			return scope.CheckBashCommand(command, toolsshell.CurrentCwd())
		}
		return tools.PathDecision{Allowed: true}
	}

	return scope.CheckToolArgs(tc.Name, tc.Args, write)
}

// artifactsDir is the only writable location during the plan phase.
const artifactsDirName = ".automergent/artifacts"

// currentPhaseIsPlan reports whether the phase machinery is in the plan
// phase (nil-safe: no phase manager means no restriction).
func (a *Agent) currentPhaseIsPlan() bool {
	a.mu.RLock()
	pm := a.phaseManager
	a.mu.RUnlock()
	return pm != nil && pm.CurrentPhase() == shared.PhasePlan
}

// checkPlanArtifactWrite enforces the plan-phase write allowlist: writes
// must land under the artifacts directory.
func (a *Agent) checkPlanArtifactWrite(tc ai.ToolCall) tools.PathDecision {
	for _, key := range []string{"path", "file_path", "dir"} {
		path, ok := tc.Args[key].(string)
		if !ok || path == "" {
			continue
		}
		if isPlanArtifactPath(path, a.workDir) {
			continue
		}
		return tools.PathDecision{
			Allowed: false,
			Reason:  "the PLAN phase writes only plan artifacts under " + artifactsDirName + "; implementation edits happen in the BUILD phase",
		}
	}
	// Bash writes during plan (redirections) cannot be resolved statically —
	// the compound cd+write guard already asks for those.
	return tools.PathDecision{Allowed: true}
}

// isPlanArtifactPath reports whether a path targets the artifacts dir.
func isPlanArtifactPath(path, workDir string) bool {
	clean := filepath.Clean(path)
	if strings.Contains(clean, "/"+artifactsDirName+"/") || strings.HasPrefix(clean, artifactsDirName+"/") {
		return true
	}
	// Absolute paths into the project's artifacts dir also qualify.
	if workDir != "" {
		abs := filepath.Join(workDir, artifactsDirName)
		if strings.HasPrefix(clean, abs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// requestPathConfirmation asks the user about an out-of-bounds access,
// carrying the decision reason so the UI can show what is being approved.
func (a *Agent) requestPathConfirmation(tc ai.ToolCall, decision tools.PathDecision) ConfirmationResponse {
	ch := make(chan ConfirmationResponse, 1)
	a.Emit(EventConfirm, map[string]any{
		"tool_call": tc,
		"reply":     ch,
		"reason":    decision.Reason,
		"path":      decision.OutsideDir,
	})
	timeout := 10 * time.Minute
	if a.cfg != nil && a.cfg.ConfirmationTimeout != "" {
		if d, err := time.ParseDuration(a.cfg.ConfirmationTimeout); err == nil {
			timeout = d
		}
	}
	select {
	case res := <-ch:
		return res
	case <-time.After(timeout):
		return ConfirmationResponse{Allow: false}
	}
}

// pathGrantFor renders the always-allow scope for a boundary approval: the
// directory containing the offending path (for the shell tool, the
// directory the command cds into). Granting the whole tool would
// accidentally allow every future out-of-bounds access.
func (a *Agent) pathGrantFor(tc ai.ToolCall, t tools.Tool, decision tools.PathDecision) string {
	dir := decision.OutsideDir
	if dir == "" {
		return ""
	}
	// Grant the containing directory for file paths; the directory itself
	// when the call targets a directory argument.
	if tc.Name != "bash" {
		dir = parentOf(dir)
	}
	grant := tools.GrantScope(dir)
	if a.workDir == "" {
		return grant
	}
	return "project=" + a.workDir + ";" + grant
}

// parentOf returns a path's directory ("" when the path is already a root).
func parentOf(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	parent := filepath.Dir(path)
	if parent == "" || parent == "." {
		return path
	}
	return parent
}
