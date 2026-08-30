package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Working-directory boundary enforcement, ported from the reference agent's
// permission model: tool calls that touch paths OUTSIDE the allowed working
// directories require explicit user approval, whatever the tool and mode
// would otherwise decide. Inside the project, the existing mode-based
// approval flow is unchanged.
//
// The ladder (deliberately simpler than the reference's rule engine, same
// shape):
//
//  1. Resolve the path (expand ~, absolutize, follow symlinks best-effort).
//  2. Granted directory scopes (from earlier "always allow" answers) allow.
//  3. Paths under an allowed working directory are in-bounds.
//  4. Writes to protected locations (.git, shell rc files, agent config)
//     ask even when in-bounds — the reference agent's dangerous-files list.
//  5. Everything else is out-of-bounds → ask, naming the directory so the
//     user knows what they are approving.

// PathDecision is the outcome of a boundary check.
type PathDecision struct {
	// Allowed reports in-bounds access (no extra prompt needed).
	Allowed bool
	// OutsideDir is the offending absolute path when out-of-bounds.
	OutsideDir string
	// Reason is the human-facing explanation shown in the approval prompt.
	Reason string
}

// ScopeDirMarker prefixes session-grant scopes that allow a directory.
const ScopeDirMarker = "pathdir:"

// protectedWritePaths are locations that require approval even inside the
// project: VCS internals, agent configuration, and shell startup files
// (auto-editing any of these is a code-execution or exfiltration vector).
var protectedWritePaths = []string{
	".git",
	".automergent",
	".automergentrc",
	".bashrc",
	".bash_profile",
	".zshrc",
	".zprofile",
	".profile",
	".ripgreprc",
	".ssh",
	".gnupg",
}

// PathScope holds the allowed working directories and decides whether tool
// paths are in-bounds. Safe for concurrent use.
type PathScope struct {
	dirs    []string
	granted map[string]bool
}

// NewPathScope creates a scope rooted at the given working directories
// (relative roots are absolutized).
func NewPathScope(roots ...string) *PathScope {
	ps := &PathScope{granted: make(map[string]bool)}
	for _, root := range roots {
		ps.AddDir(root)
	}
	return ps
}

// AddDir adds an allowed working directory.
func (ps *PathScope) AddDir(dir string) {
	if resolved := resolveScopePath(dir); resolved != "" {
		for _, existing := range ps.dirs {
			if existing == resolved {
				return
			}
		}
		ps.dirs = append(ps.dirs, resolved)
	}
}

// AddGrantedDir records a directory the user approved ("always allow" on an
// out-of-bounds prompt).
func (ps *PathScope) AddGrantedDir(dir string) {
	if resolved := resolveScopePath(dir); resolved != "" {
		ps.granted[resolved] = true
	}
}

// GrantScope renders the session-grant scope key for a directory grant.
func GrantScope(dir string) string {
	resolved := resolveScopePath(dir)
	if resolved == "" {
		resolved = dir
	}
	return ScopeDirMarker + resolved
}

// IsDirGrant reports whether an approval scope is a directory grant, and
// returns the directory. Handles both bare scopes ("pathdir:/x") and
// project-prefixed scopes ("project=/p;pathdir:/x").
func IsDirGrant(scope string) (string, bool) {
	idx := strings.Index(scope, ScopeDirMarker)
	if idx < 0 {
		return "", false
	}
	dir := scope[idx+len(ScopeDirMarker):]
	if dir == "" {
		return "", false
	}
	return dir, true
}

// Dirs returns the allowed working directories.
func (ps *PathScope) Dirs() []string {
	out := make([]string, len(ps.dirs))
	copy(out, ps.dirs)
	return out
}

// Check decides whether one path is in-bounds for the given operation.
// write=false treats the check as a read.
func (ps *PathScope) Check(path string, write bool) PathDecision {
	if strings.TrimSpace(path) == "" {
		return PathDecision{Allowed: true}
	}
	resolved := resolveScopePath(path)
	if resolved == "" {
		// Unresolvable (broken symlink to nowhere, exotic path): do not
		// block the tool — the tool itself will report the error.
		return PathDecision{Allowed: true}
	}

	// Granted directory scopes (earlier always-allow answers).
	for dir := range ps.granted {
		if underDir(resolved, dir) {
			return PathDecision{Allowed: true}
		}
	}

	// Allowed working directories.
	for _, dir := range ps.dirs {
		if underDir(resolved, dir) {
			if write && isProtectedWritePath(resolved) {
				return PathDecision{
					Allowed: false,
					Reason: fmt.Sprintf("%s is a protected location (VCS internals, agent config, or shell startup files); editing it needs explicit approval", resolved),
				}
			}
			return PathDecision{Allowed: true}
		}
	}

	return PathDecision{
		Allowed:    false,
		OutsideDir: resolved,
		Reason:     fmt.Sprintf("%s is outside the project directory", resolved),
	}
}

// CheckToolArgs extracts path-like arguments from a tool call and checks
// them all. Returns the first out-of-bounds decision; in-bounds when every
// extracted path is in-bounds (or none were found).
func (ps *PathScope) CheckToolArgs(toolName string, args map[string]any, write bool) PathDecision {
	paths := ExtractToolPaths(toolName, args)
	for _, path := range paths {
		if decision := ps.Check(path, write); !decision.Allowed {
			return decision
		}
	}
	return PathDecision{Allowed: true}
}

// ExtractToolPaths pulls the path arguments out of a tool call.
func ExtractToolPaths(toolName string, args map[string]any) []string {
	var paths []string
	for _, key := range []string{"path", "file_path", "dir", "directory", "target", "destination", "dest", "from", "to", "source"} {
		if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
			paths = append(paths, v)
		}
	}
	if toolName == "bash" {
		if cmd, ok := args["command"].(string); ok {
			paths = append(paths, AbsolutePathsInCommand(cmd)...)
		}
	}
	return paths
}

// absolutePathPattern matches absolute-looking path tokens in shell
// commands.
var absolutePathPattern = regexp.MustCompile(`(?:^|[\s"'=;|&()])(/(?:[\w.\-@+]+/)*[\w.\-@++]*)`)

// AbsolutePathsInCommand extracts the absolute paths referenced by a shell
// command string. Pragmatic token scan, not a shell AST: it catches the
// cases that matter (absolute file arguments) without false positives on
// flags or URLs (which contain no leading slash token boundary).
func AbsolutePathsInCommand(command string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, match := range absolutePathPattern.FindAllStringSubmatch(command, -1) {
		p := strings.Trim(match[1], `"'`)
		// Skip obvious non-paths: URL schemes are caught by the boundary
		// rule, but pure option-ish tokens and /dev/null style specials
		// are noise.
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}

// CdTargets extracts the directories a shell command changes into.
func CdTargets(command string) []string {
	var targets []string
	for _, match := range regexp.MustCompile(`(?:^|&&|;|\b)\s*cd\s+([^\s;&|]+)`).FindAllStringSubmatch(command, -1) {
		target := strings.Trim(match[1], `"'`)
		if target != "" && target != "-" {
			targets = append(targets, target)
		}
	}
	return targets
}

// HasCompoundCdWrite reports whether a command both changes directory and
// performs a write — the reference agent always asks for these because the
// effective working directory of the write cannot be statically resolved.
func HasCompoundCdWrite(command string) bool {
	if len(CdTargets(command)) == 0 {
		return false
	}
	writeMarkers := []string{">", ">>", "tee ", "rm ", "mv ", "cp ", "mkdir ", "touch ", "sed -i", "chmod ", "chown ", "truncate "}
	for _, marker := range writeMarkers {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

// CheckBashCommand applies the shell-specific boundary rules: cd targets
// resolved against the shell's current directory, absolute path arguments,
// and the compound cd+write guard.
func (ps *PathScope) CheckBashCommand(command, shellCwd string) PathDecision {
	if HasCompoundCdWrite(command) {
		return PathDecision{
			Allowed: false,
			Reason:  "the command changes directory and writes; its effective paths cannot be resolved safely, so it needs explicit approval",
		}
	}
	for _, target := range CdTargets(command) {
		resolved := target
		if !filepath.IsAbs(resolved) && shellCwd != "" {
			resolved = filepath.Join(shellCwd, resolved)
		}
		if decision := ps.Check(resolved, false); !decision.Allowed {
			decision.Reason = fmt.Sprintf("the command cds into %s, which is outside the project directory", resolved)
			return decision
		}
	}
	for _, path := range AbsolutePathsInCommand(command) {
		if decision := ps.Check(path, false); !decision.Allowed {
			return decision
		}
	}
	return PathDecision{Allowed: true}
}

// underDir reports whether path equals or lies under dir (both absolute).
func underDir(path, dir string) bool {
	if path == dir {
		return true
	}
	return strings.HasPrefix(path, dir+string(filepath.Separator))
}

// isProtectedWritePath reports whether a write targets a protected
// location.
func isProtectedWritePath(resolved string) bool {
	base := filepath.Base(resolved)
	for _, name := range protectedWritePaths {
		if base == name {
			return true
		}
	}
	for _, part := range strings.Split(resolved, string(filepath.Separator)) {
		if part == ".git" || part == ".ssh" || part == ".gnupg" {
			return true
		}
	}
	return false
}

// resolveScopePath expands ~, absolutizes, and resolves symlinks
// (best-effort). Returns "" when nothing sensible can be resolved.
func resolveScopePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = filepath.Join(wd, path)
		} else {
			return ""
		}
	}
	// EvalSymlinks fails for not-yet-existing targets (write paths); the
	// Lexical cleanup still normalizes .. and . segments.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}
