package tools

import (
	"context"
	"path/filepath"
	"strings"
)

// ArtifactsHome is the conventional artifact directory inside a project.
const ArtifactsHome = ".automergent/artifacts"

type toolsCtxKey string

const sessionIDKey toolsCtxKey = "session_id"

// WithSessionID returns a context carrying the session ID, so tools can
// scope their side effects (artifact paths) to the conversation that caused
// them.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// SessionIDFromContext returns the session ID carried by ctx, "" when none.
func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(sessionIDKey).(string); ok {
		return id
	}
	return ""
}

// ScopeArtifactPath moves a relative path under the project's artifacts
// directory into a per-session subdirectory, so artifacts from different
// sessions never collide: ".automergent/artifacts/plan.md" becomes
// ".automergent/artifacts/<sessionID>/plan.md".
//
// Paths outside the artifacts home, absolute paths and empty session IDs are
// returned unchanged. A path already scoped to THIS session is a no-op, so
// re-running a call with the path the previous result reported is stable.
func ScopeArtifactPath(path, sessionID string) string {
	if sessionID == "" || path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	prefix := ArtifactsHome + "/"
	if !strings.HasPrefix(clean, prefix) {
		return path
	}
	rest := strings.TrimPrefix(clean, prefix)
	if rest == "" || rest == "." {
		return path
	}
	// Already scoped to this session (a retry, or the model echoing the
	// rewritten path from an earlier result): keep it.
	if first := rest; strings.Contains(rest, "/") {
		first = rest[:strings.IndexByte(rest, '/')]
		if first == sessionID {
			return path
		}
	}
	return prefix + sessionID + "/" + rest
}

// ScopeArtifactCall rewrites a tool call's artifact path argument in place so
// artifacts land in the session-scoped directory. Called by the execution
// layer BEFORE tool events fire, which keeps the call's recorded args, the
// event context (the review UI's artifact path) and the tool's own result
// all reporting the same scoped path.
func ScopeArtifactCall(name string, args map[string]any, sessionID string) {
	if name != "artifact" || args == nil || sessionID == "" {
		return
	}
	path, ok := args["path"].(string)
	if !ok {
		return
	}
	if scoped := ScopeArtifactPath(path, sessionID); scoped != path {
		args["path"] = scoped
	}
}
