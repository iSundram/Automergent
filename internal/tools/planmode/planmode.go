package planmode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// Plan-mode tools: the model can switch itself into plan (read-only) mode,
// present a finished plan for user approval (blocking until the user decides
// in the review UI), and verify that an executed plan's file changes are all
// present. Mode changes and plan review are host callbacks so this package
// never touches agent or app internals.

var (
	hookMu       sync.RWMutex
	modeChanger  func(mode string) error
	planReviewer func(ctx context.Context, planPath, plan string) (approved bool, feedback string, err error)
)

// SetModeChanger installs the mode switch (the TUI wires it to its /mode
// path). nil disables mode changes.
func SetModeChanger(fn func(mode string) error) {
	hookMu.Lock()
	defer hookMu.Unlock()
	modeChanger = fn
}

// SetPlanReviewer installs the blocking plan-review UI hook. The tool
// goroutine blocks until the reviewer returns.
func SetPlanReviewer(fn func(ctx context.Context, planPath, plan string) (bool, string, error)) {
	hookMu.Lock()
	defer hookMu.Unlock()
	planReviewer = fn
}

// EnterPlanModeTool switches the agent into plan mode: read-only analysis,
// edits refused, output is a plan.
type EnterPlanModeTool struct {
	tools.BaseTool
}

func (t *EnterPlanModeTool) Name() string { return "enter_plan_mode" }
func (t *EnterPlanModeTool) Description() string {
	return `Switch to plan mode: read-only analysis and planning; edits are refused.
- Use at the start of a non-trivial task that warrants a reviewed plan.
- Produce the plan with the artifact tool (kind "plan", request_feedback=true), or exit_plan_mode to present it for approval.
- If the request does not warrant a plan, do NOT enter plan mode — continue normally.`
}
func (t *EnterPlanModeTool) RequiresConfirmation(mode string) bool { return false }
func (t *EnterPlanModeTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *EnterPlanModeTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *EnterPlanModeTool) IsDestructive(args map[string]any) bool { return false }

func (t *EnterPlanModeTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "planning",
		Usage:      "Enter once per task; the mode reverts when the plan is approved or rejected via exit_plan_mode.",
		WhenToUse:  "The task is non-trivial (multiple files, unclear approach) and the user has not already planned it.",
		WhenNotTo:  "Trivia, single-file edits with a clear instruction, or tasks the user asked to just do.",
	}
}

func (t *EnterPlanModeTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *EnterPlanModeTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	hookMu.RLock()
	fn := modeChanger
	hookMu.RUnlock()
	if fn == nil {
		return tools.Result{IsError: true, Content: "mode switching is not available in this runtime"}, nil
	}
	if err := fn("plan"); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("enter plan mode: %v", err)}, nil
	}
	return tools.Result{
		Content: "Plan mode active: read-only analysis. Explore, then write the plan as an artifact (.automergent/artifacts/plan.md) and call exit_plan_mode to present it.",
		Summary: "entered plan mode",
	}, nil
}

// ExitPlanModeTool presents the finished plan for user approval and blocks
// until the user approves or rejects it. Approval exits plan mode.
type ExitPlanModeTool struct {
	tools.BaseTool
}

func (t *ExitPlanModeTool) Name() string { return "exit_plan_mode" }
func (t *ExitPlanModeTool) Description() string {
	return `Present your plan for user approval and wait for their decision. Blocks until the user responds.
- plan: a short summary of the plan being presented (the full plan lives in the artifact file).
- plan_path: path to the plan artifact (default .automergent/artifacts/plan.md).
- On approval, plan mode ends and you may implement. On rejection, the user's feedback tells you what to change — revise the plan artifact and call this again.`
}
func (t *ExitPlanModeTool) RequiresConfirmation(mode string) bool { return false }
func (t *ExitPlanModeTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *ExitPlanModeTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *ExitPlanModeTool) IsDestructive(args map[string]any) bool { return false }

func (t *ExitPlanModeTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "planning",
		Usage:      "Always pair with a written plan artifact; the summary argument is a digest, not the plan itself.",
		WhenToUse:  "The plan is written and ready for the user's decision.",
		WhenNotTo:  "Never call speculatively — only with a complete plan.",
	}
}

func (t *ExitPlanModeTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{
				"type":        "string",
				"description": "Short summary of the plan being presented (the artifact holds the full plan).",
			},
			"plan_path": map[string]any{
				"type":        "string",
				"description": "Path to the plan artifact (default .automergent/artifacts/plan.md).",
			},
		},
		"required": []string{"plan"},
	}
}

func (t *ExitPlanModeTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	plan, ok := tools.StringArg(args, "plan")
	if !ok || strings.TrimSpace(plan) == "" {
		return tools.Result{IsError: true, Content: "plan is required (short summary of what is being presented)"}, nil
	}
	planPath, _ := tools.StringArg(args, "plan_path")
	if planPath == "" {
		planPath = filepath.Join(".automergent", "artifacts", "plan.md")
	}
	// Plans are artifacts: scope the path to the session so concurrent
	// sessions' plans never collide, exactly like the artifact tool.
	planPath = tools.ScopeArtifactPath(planPath, tools.SessionIDFromContext(ctx))

	hookMu.RLock()
	reviewer := planReviewer
	hookMu.RUnlock()
	if reviewer == nil {
		return tools.Result{IsError: true, Content: "plan review is not available in this runtime"}, nil
	}

	// Bound the wait like ask_user does: a plan review nobody answers must
	// not hang the agent forever.
	waitCtx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()

	type decision struct {
		approved bool
		feedback string
		err      error
	}
	done := make(chan decision, 1)
	go func() {
		approved, feedback, err := reviewer(waitCtx, planPath, plan)
		done <- decision{approved, feedback, err}
	}()

	select {
	case d := <-done:
		if d.err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("plan review: %v", d.err)}, nil
		}
		if d.approved {
			return tools.Result{
				Content:  "Plan approved by the user. Plan mode is off — implement the plan now.",
				Summary:  "plan approved",
				Metadata: map[string]any{"plan_approved": true},
			}, nil
		}
		feedback := strings.TrimSpace(d.feedback)
		if feedback == "" {
			feedback = "The user rejected the plan without further detail."
		}
		return tools.Result{
			Content:  "Plan rejected. User feedback:\n\n" + feedback + "\n\nRevise the plan artifact and call exit_plan_mode again.",
			Summary:  "plan rejected",
			Metadata: map[string]any{"plan_approved": false},
		}, nil
	case <-waitCtx.Done():
		return tools.Result{IsError: true, Content: "plan review timed out without a user decision"}, nil
	}
}

// VerifyPlanExecutionTool checks that every file the plan named actually
// changed: it extracts file paths from the plan artifact and compares them
// against git's working-tree changes.
type VerifyPlanExecutionTool struct {
	tools.BaseTool
}

func (t *VerifyPlanExecutionTool) Name() string { return "verify_plan_execution" }
func (t *VerifyPlanExecutionTool) Description() string {
	return `Verify that the executed plan's file changes are present.
- Reads the plan artifact (default .automergent/artifacts/plan.md), extracts the files it names, and checks each against git's working-tree changes (staged, unstaged and untracked).
- Reports per-file evidence and an overall verdict. A plan file with no corresponding git change is a gap: implement it or explain why it was dropped.`
}
func (t *VerifyPlanExecutionTool) RequiresConfirmation(mode string) bool { return false }
func (t *VerifyPlanExecutionTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *VerifyPlanExecutionTool) IsReadOnly(args map[string]any) bool    { return true }
func (t *VerifyPlanExecutionTool) IsDestructive(args map[string]any) bool { return false }

func (t *VerifyPlanExecutionTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "verify",
		Usage:      "Run after implementing a plan, before reporting done. Evidence beats memory: the report is derived from git, not from your recollection.",
		WhenToUse:  "The build phase of a planned task is complete.",
		WhenNotTo:  "Unplanned work or plans without file paths.",
	}
}

func (t *VerifyPlanExecutionTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_path": map[string]any{
				"type":        "string",
				"description": "Path to the plan artifact (default .automergent/artifacts/plan.md).",
			},
		},
	}
}

// planFilePattern matches file paths as standalone tokens in plan text:
// something with an extension, delimited by whitespace, backticks, quotes,
// list markers or line boundaries.
var planFilePattern = regexp.MustCompile("(?:^|[\\s`'\"(|\\[:：])((?:[\\w.-]+/)*[\\w.-]+\\.[A-Za-z][A-Za-z0-9]{0,5})")

func (t *VerifyPlanExecutionTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	planPath, _ := tools.StringArg(args, "plan_path")
	if planPath == "" {
		planPath = filepath.Join(".automergent", "artifacts", "plan.md")
	}
	// Plans live in session-scoped artifact paths; resolve the same way the
	// plan was written.
	planPath = tools.ScopeArtifactPath(planPath, tools.SessionIDFromContext(ctx))
	planDir := filepath.Dir(planPath)
	planBytes, err := readFile(planPath)
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("read plan: %v (write the plan artifact first, or pass plan_path)", err)}, nil
	}

	// Extract candidate file paths from the plan.
	seen := map[string]bool{}
	var files []string
	for _, m := range planFilePattern.FindAllStringSubmatch(string(planBytes), -1) {
		p := filepath.ToSlash(filepath.Clean(m[1]))
		if seen[p] || isNoisePath(p) {
			continue
		}
		seen[p] = true
		files = append(files, p)
	}
	if len(files) == 0 {
		return tools.Result{IsError: true, Content: "no file paths found in the plan — a plan without file paths cannot be verified"}, nil
	}
	sort.Strings(files)

	changed, err := gitChangedFiles(ctx, filepath.Dir(planPath))
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("git status: %v", err)}, nil
	}

	var sb strings.Builder
	okCount := 0
	for _, f := range files {
		switch {
		case changed[f]:
			fmt.Fprintf(&sb, "✓ %s — changed in working tree\n", f)
			okCount++
		case fileExists(filepath.Join(planDir, f)):
			fmt.Fprintf(&sb, "! %s — exists but shows no git change (verify the change was needed)\n", f)
		default:
			fmt.Fprintf(&sb, "✗ %s — no change found\n", f)
		}
	}
	verdict := fmt.Sprintf("%d/%d plan files verified", okCount, len(files))
	sb.WriteString("\n" + verdict)
	if okCount < len(files) {
		sb.WriteString(" — address the gaps above before reporting done")
	}
	return tools.Result{
		Content:  sb.String(),
		Summary:  verdict,
		Metadata: map[string]any{"verified": okCount, "total": len(files)},
	}, nil
}

// isNoisePath filters path-like tokens that are not source files.
func isNoisePath(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	switch {
	case strings.HasPrefix(base, "example."),
		strings.HasPrefix(base, "sample."),
		base == "go.sum",
		base == "package-lock.json",
		base == "bun.lock",
		base == "pnpm-lock.yaml":
		return true
	}
	return false
}

// gitChangedFiles unions staged, unstaged and untracked files from git
// status (run in dir, the plan's directory), keyed by slash-normalized
// relative path.
func gitChangedFiles(ctx context.Context, dir string) (map[string]bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	changed := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		// Rename entries look like "old -> new"; count the new path.
		if idx := strings.Index(p, " -> "); idx >= 0 {
			p = p[idx+4:]
		}
		p = strings.Trim(p, "\"")
		if p != "" {
			changed[filepath.ToSlash(filepath.Clean(p))] = true
		}
	}
	return changed, nil
}

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
