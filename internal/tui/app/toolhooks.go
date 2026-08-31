package app

// Tool hooks: the plan-mode, skill and context-inspection tools call into
// the TUI through package-level callbacks (the same pattern ask_user's
// questionnaire uses). Installing them here keeps the tool packages free of
// app internals and keeps the wiring in one place.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iSundram/Automergent/internal/tools/ctxinfo"
	"github.com/iSundram/Automergent/internal/tools/planmode"
	"github.com/iSundram/Automergent/internal/tools/skills"
)

// installToolHooks wires the TUI behind the host-callback tools.
func (a *App) installToolHooks() {
	a.installPlanModeHooks()
	a.installCtxInspector()
	a.installSkillDirs()
}

// installPlanModeHooks wires enter/exit_plan_mode.
func (a *App) installPlanModeHooks() {
	planmode.SetModeChanger(func(mode string) error {
		if mode != "plan" && a.cfg.Mode != "plan" {
			// Only plan-mode transitions come through here today.
			return fmt.Errorf("unsupported mode %q", mode)
		}
		if a.cfg.Mode != "plan" {
			// Remember the mode to restore when the plan is decided.
			a.planModePrev = a.cfg.Mode
		}
		a.setModeFromTool(mode)
		return nil
	})

	planmode.SetPlanReviewer(func(ctx context.Context, planPath, plan string) (bool, string, error) {
		if a.sendToProgram == nil {
			return false, "", fmt.Errorf("no UI available")
		}
		pr := &pendingPlanReview{
			planPath: planPath,
			summary:  plan,
			reply:    make(chan planDecision, 1),
			done:     make(chan struct{}),
		}
		a.sendToProgram(planReviewRequestedMsg{pr})
		select {
		case d := <-pr.reply:
			return d.approved, d.reason, nil
		case <-pr.done:
			return false, "", fmt.Errorf("user dismissed the plan review")
		case <-ctx.Done():
			return false, "", fmt.Errorf("plan review timed out")
		}
	})
}

// setModeFromTool applies a mode change triggered by a tool call. It runs on
// a tool goroutine, so the state flips are the same ones /mode makes and the
// chrome refresh travels through the next event-loop turn.
func (a *App) setModeFromTool(mode string) {
	a.SetMode(mode)
}

// installCtxInspector wires ctx_inspect's summary provider.
func (a *App) installCtxInspector() {
	ctxinfo.SetInspector(func() string {
		if a.ag == nil {
			return ""
		}
		limit := 0
		if p := a.ag.Provider(); p != nil {
			limit = p.ContextLimit()
		}
		// A snapshot keeps the estimate off the live message slice.
		var conversation int
		if a.sess != nil {
			snap := a.sess.Snapshot()
			if mgr := a.ag.ContextManager(); mgr != nil {
				if calc := mgr.AdaptiveCalculator(); calc != nil {
					conversation = calc.EstimateMessages(snap.Messages)
				}
			}
		}
		var budgetTotal, budgetUsed, systemPrompt, toolDefs, contextFiles int
		var usagePct float64
		if mgr := a.ag.ContextManager(); mgr != nil {
			b := mgr.GetBudgetSummary()
			budgetTotal, budgetUsed = b.TotalBudget, b.TotalUsed
			systemPrompt, toolDefs, contextFiles = b.SystemPrompt, b.ToolDefinitions, b.ContextFiles
			usagePct = b.UsagePercent
		}
		if limit <= 0 {
			limit = budgetTotal
		}
		used := conversation + systemPrompt + toolDefs + contextFiles
		if budgetUsed > used {
			used = budgetUsed
		}
		if usagePct == 0 && limit > 0 {
			usagePct = float64(used) / float64(limit) * 100
		}
		return ctxinfo.FormatSummary(limit, used, conversation, systemPrompt, toolDefs, contextFiles, usagePct)
	})
}

// installSkillDirs points the skill tools at the same roots the agent uses:
// user skills first, project skills last (later dirs win on conflicts).
func (a *App) installSkillDirs() {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".automergent", "skills"))
	}
	if a.workDir != "" {
		dirs = append(dirs, filepath.Join(a.workDir, ".automergent", "skills"))
	}
	skills.SetDirs(dirs...)
}

// pendingPlanReview couples one exit_plan_mode call with its decision
// channel. The tool goroutine blocks on reply/done.
type pendingPlanReview struct {
	planPath string
	summary  string
	reply    chan planDecision
	done     chan struct{}
}

// planDecision is the user's answer to a blocking plan review.
type planDecision struct {
	approved bool
	reason   string
}

// planReviewRequestedMsg carries a plan review request into the event loop.
type planReviewRequestedMsg struct{ pr *pendingPlanReview }

// beginPlanReview presents the plan artifact in the review browser and parks
// the decision channels so ArtifactDecisionMsg can answer the waiting tool.
func (a *App) beginPlanReview(pr *pendingPlanReview) {
	// The model may have written the plan with the artifact tool already;
	// if not, persist the summary so there is something to review.
	if _, err := os.Stat(pr.planPath); err != nil {
		if err := os.MkdirAll(filepath.Dir(pr.planPath), 0o755); err == nil {
			_ = os.WriteFile(pr.planPath, []byte("# Plan\n\n"+pr.summary+"\n"), 0o644)
		}
	}
	a.registerArtifact(pr.planPath)
	a.pendingPlanReview = pr
	a.showArtifacts()
	a.statusBar.SetStatus("Plan review requested — /artifact")
}

// resolvePlanReview answers a waiting exit_plan_mode call. Approval also
// restores the mode the agent had before entering plan mode; a rejection
// carries the user's reason back to the model as revision feedback.
func (a *App) resolvePlanReview(approved bool, reason string) {
	pr := a.pendingPlanReview
	if pr == nil {
		return
	}
	a.pendingPlanReview = nil
	if approved && a.cfg.Mode == "plan" {
		restore := a.planModePrev
		if restore == "" || restore == "plan" {
			restore = "manual"
		}
		a.planModePrev = ""
		a.SetMode(restore)
	}
	select {
	case pr.reply <- planDecision{approved: approved, reason: reason}:
	default:
	}
}

// cancelPlanReview dismisses a waiting review (user closed the browser
// without deciding).
func (a *App) cancelPlanReview() {
	pr := a.pendingPlanReview
	if pr == nil {
		return
	}
	a.pendingPlanReview = nil
	close(pr.done)
}
