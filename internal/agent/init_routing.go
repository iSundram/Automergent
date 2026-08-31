package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/ai"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
	"github.com/iSundram/Automergent/internal/shared"
)

// INIT decomposer routing: the deterministic half of the vision. The
// decomposer (internal/prompt/decomposer.go) decides WHAT each part of the
// message is; this file decides what the agent DOES with each part:
//
//   direct/question(general) -> answered by INIT itself, serious style
//   rule                     -> recorded in (or removed from) the rules store
//   noise                    -> dropped
//   clarify                  -> user asked with concrete options
//   violation_suspect        -> explore-confirmation task, or immediate
//                                escalation when unambiguous
//   task                     -> the phase loop, phase chosen per task

// decomposeFirstMessage runs the INIT decomposer on a user message. Returns
// nil when decomposition is unavailable, so the caller falls back to the
// keyword router.
func (a *Agent) decomposeFirstMessage(ctx context.Context, message string) *promptpkg.Decomposition {
	if a.provider == nil {
		return nil
	}
	if a.decomposer == nil {
		a.decomposer = promptpkg.NewInitDecomposer(promptpkg.NewAIProviderAdapter(a.provider, ""))
	}
	decomposition := a.decomposer.Decompose(ctx, message, a.workDir, a.getAvailableFiles())
	if decomposition == nil {
		a.Emit(EventStatus, "init decomposer unavailable — keyword routing")
	}
	return decomposition
}

// executeDecomposition routes every part of a decomposition to its handler.
func (a *Agent) executeDecomposition(ctx context.Context, d *promptpkg.Decomposition, originalPrompt string) error {
	if d.Summary != "" {
		a.Emit(EventStatus, "init: "+d.Summary)
	}

	// Clarification: ask with concrete options before acting on the
	// ambiguous part. Other parts still run — the answer only affects the
	// ambiguous one.
	for _, part := range d.ClarifyParts() {
		question := "Which do you mean by: " + part.Text
		if part.Options != nil {
			question += "? Options: " + strings.Join(part.Options, " / ")
		}
		a.askClarification([]string{question})
	}
	if d.RequiresClarification && d.ClarificationQuestion != "" && len(d.ClarifyParts()) == 0 {
		a.askClarification([]string{d.ClarificationQuestion})
	}

	// Rules: record (or remove) standing instructions. Persisted per
	// project; they ride in <project-instructions> from the next request on.
	for _, part := range d.RuleParts() {
		if part.Rule == "" {
			part.Rule = part.Text
		}
		if strings.EqualFold(part.RuleAction, "remove") {
			if removed, ok := a.ruleStore().Remove(part.Rule); ok {
				a.Emit(EventStatus, "rule removed: "+removed)
			} else {
				a.Emit(EventStatus, "no matching rule to remove")
			}
			continue
		}
		if confirmation, err := a.ruleStore().Add(part.Rule); err == nil {
			a.Emit(EventStatus, confirmation)
		} else {
			a.Emit(EventStatus, "rule store unavailable: "+err.Error())
		}
	}

	// Violations: unambiguous ones escalate immediately through the existing
	// ladder; ambiguous ones get an explore-confirmation task prepended so
	// the codebase context decides before escalation.
	var tasks []shared.TaskSpec
	for _, part := range d.ViolationParts() {
		if part.NeedsConfirmation {
			tasks = append(tasks, shared.TaskSpec{
				ID:          part.ID,
				Type:        "explore",
				Description: "Confirm whether this request is a genuine policy violation before escalation: " + part.Text,
				Role:        "explore",
				Agent:       "explore",
				Priority:    1,
				Context:     map[string]any{"violation_confirmation": true, "violation_type": part.ViolationType},
			})
			continue
		}
		violation := shared.ViolationCheck{
			Type:        shared.ViolationType(part.ViolationType),
			Severity:    shared.ViolationSeverityHigh,
			UserMessage: part.Text,
			Count:       1,
			Action:      "warn",
		}
		a.handleViolation(&violation)
		if violation.Action == "blocked" {
			return fmt.Errorf("session blocked due to policy violation")
		}
	}

	// Routed tasks, priority order (TaskParts sorts).
	specs := d.ToTaskSpecs()
	tasks = append(tasks, specs...)

	// Violation-confirmation tasks run first.
	for i := range tasks {
		if _, ok := tasks[i].Context["violation_confirmation"]; ok {
			tasks[i].Priority = 0
		}
	}

	// Direct parts: INIT answers them itself, in one completion, serious and
	// concise. Skipped when routed tasks exist — the phase loop's final
	// answer covers the whole message.
	direct := d.DirectParts()
	if len(direct) > 0 && len(tasks) == 0 {
		return a.answerDirectParts(ctx, direct)
	}

	if len(tasks) == 0 {
		// Nothing routed and nothing direct: the message was pure rules,
		// noise, or clarification. Rules and clarification are handled
		// above; for noise-only messages let the MODEL respond naturally
		// rather than a canned acknowledgement — a greeting misrouted as
		// noise then still gets a real reply.
		if len(d.RuleParts()) == 0 && len(d.ClarifyParts()) == 0 {
			return a.answerDirectQuestion(ctx, originalPrompt)
		}
		ack := "Recorded. " + strings.Join(noiseSummary(d), " ")
		msg := ai.NewTextMessage(ai.RoleAssistant, strings.TrimSpace(ack))
		a.sess.AddMessage(msg)
		a.recordToTranscript(msg)
		a.tryPersist()
		a.Emit(EventDone, ack)
		return nil
	}

	// Execute each task in its own phase — the phase is chosen per task,
	// which is the core of the arc.
	for _, task := range tasks {
		phase := task.Phase
		if phase == "" {
			phase = shared.PhaseBuild
		}
		var result promptpkg.PhaseResult
		if a.phaseManager.CurrentPhase() == phase {
			cfg := a.phaseManager.GetPhaseConfig(phase)
			result = promptpkg.PhaseResult{
				Phase:    phase,
				TaskSpec: task,
				Config:   cfg,
				ToolSet:  cfg.ToolSet,
				MaxSteps: cfg.MaxSteps,
			}
		} else {
			result = a.phaseManager.ExecutePhase(phase, task)
		}
		if result.Error != nil {
			a.Emit(EventError, result.Error)
			return result.Error
		}
		if result.Violation != nil {
			a.handleViolation(result.Violation)
			if result.Violation.Action == "blocked" {
				return fmt.Errorf("session blocked due to policy violation")
			}
		}

		agentLabel := task.Agent
		if agentLabel == "" {
			agentLabel = "main"
		}
		a.Emit(EventStatus, fmt.Sprintf("phase %s (%s): %s", phase, agentLabel, task.Description))

		if err := a.runPhaseLoop(ctx, phase, result); err != nil {
			return err
		}
	}
	return nil
}

// answerDirectParts answers the direct/general-question parts INIT fulfills
// itself — no exploration, no todos, serious and concise.
func (a *Agent) answerDirectParts(ctx context.Context, parts []promptpkg.DecomposedPart) error {
	var sb strings.Builder
	sb.WriteString("Answer each of the user's requests concisely and factually, in a serious tone. Do not use tools.\n\n")
	for i, part := range parts {
		style := ""
		if part.AnswerStyle == "about-me" {
			style = " (this is a question about you: you are the model named in your environment context, running on the Automergent platform)"
		}
		fmt.Fprintf(&sb, "%d. %s%s\n", i+1, part.Text, style)
	}

	provider := a.Provider()
	systemPrompt := a.getSystemPrompt(ctx, provider)
	req := ai.CompletionRequest{
		Messages:    prependUserContext([]ai.Message{ai.NewTextMessage(ai.RoleUser, sb.String())}, a.userContext()),
		Tools:       []ai.ToolSchema{},
		System:      systemPrompt,
		Temperature: 0.0,
		MaxTokens:   1024,
		Stream:      true,
	}
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return err
	}
	text, _, _, err := a.drainStream(resp)
	if err != nil {
		return err
	}
	msg := ai.NewTextMessage(ai.RoleAssistant, text)
	a.sess.AddMessage(msg)
	a.recordToTranscript(msg)
	a.tryPersist()
	a.Emit(EventDone, text)
	return nil
}

// noiseSummary renders the dropped personal filler as a single summary line
// for the acknowledgement, so the user knows it was seen and set aside.
func noiseSummary(d *promptpkg.Decomposition) []string {
	var lines []string
	for _, p := range d.NoiseParts() {
		lines = append(lines, "(noted, set aside: \""+p.Text+"\")")
	}
	return lines
}

// buildIntentKeywords mark messages that expect implementation work, not
// just investigation. Used by the keyword fallback to chain BUILD after
// EXPLORE so the arc still completes when the decomposer is unavailable.
var buildIntentKeywords = []string{
	"implement", "create", "build", "make", "add", "write", "fix", "repair",
	"refactor", "optimize", "upgrade", "migrate", "delete", "remove",
	"rename", "install", "set up", "setup", "generate",
}

// looksLikeBuildWork reports whether a message carries implementation intent.
func looksLikeBuildWork(message string) bool {
	lower := strings.ToLower(message)
	for _, kw := range buildIntentKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
