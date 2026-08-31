package app

// Agent event stream handling (tokens, tools, notify, todos).
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/components"
	"os"
	"strings"
)

// updatePhaseHeader sets the header's phase indicator and the spinner verb.
// The live phase reported by EventPhase is authoritative once the arc has
// announced it; DetectPhase (a guess from message history) is only a
// fallback for restored sessions where no phase event has fired yet.
func (a *App) updatePhaseHeader() {
	phase := a.livePhase
	if phase == "" {
		phase = string(agent.DetectPhase(a.sess.Messages))
	}
	a.header.SetPhase(phase)
	a.setSpinVerb(verbForPhase(phase))
}

func (a *App) handleAgentEvent(ev agent.Event) tea.Cmd {
	// Live-update active token estimate on every agent event.
	a.updateActiveTokens()
	defer a.refreshChrome()
	switch ev.Type {
	case agent.EventRetry:
		// A retried attempt: the request has not failed, so the run continues.
		// Surfacing it is what keeps a rate-limited request from looking hung.
		if info, ok := ev.Payload.(ai.RetryInfo); ok {
			a.handleRetryEvent(info)
		}
		return a.waitForAgentEvent()
	case agent.EventSteered:
		if text, ok := ev.Payload.(string); ok && strings.TrimSpace(text) != "" {
			a.statusBar.SetStatus("Steering applied")
			_ = text // already logged when queued; avoids a duplicate line
		}
		return a.waitForAgentEvent()
	case agent.EventToken:
		if tok, ok := ev.Payload.(string); ok {
			a.conversation.AppendToken(tok)
			if strings.TrimSpace(tok) != "" {
				a.streamedReply = true
				a.runChars += len(tok)
			}
		}
		// Tokens arriving means the request went through: any retry sequence
		// that preceded them is over.
		a.clearRetryState()
		wait := a.waitForAgentEvent()
		if !a.streamTickPending {
			a.streamTickPending = true
			return tea.Batch(wait, scheduleStreamTick())
		}
		return wait
	case agent.EventThought:
		if thought, ok := ev.Payload.(string); ok {
			a.conversation.AppendThought(thought)
		}
		return a.waitForAgentEvent()
	case agent.EventToolCall:
		if te, ok := ev.Payload.(agent.ToolCallEvent); ok {
			argText := ""
			if len(te.Args) > 0 {
				if b, err := json.Marshal(te.Args); err == nil {
					argText = string(b)
				}
			}
			ctx := te.Context
			if ctx == "" {
				ctx = extractToolContext(te.Name, te.Args)
			}
			a.conversation.AddToolLifecycleStart(te.ID, te.Name, argText, ctx)
			a.stats.ToolCallCount++
			a.activeTool = te.Name
			a.setSpinVerb(verbForTool(te.Name))
			a.statusBar.SetStatus(fmt.Sprintf("▸ %s…", te.Name))
			a.snapshotFileWrite(te.ID, te.Name, te.Args)
		} else if tc, ok := ev.Payload.(ai.ToolCall); ok {
			argText := ""
			if len(tc.Args) > 0 {
				if b, err := json.Marshal(tc.Args); err == nil {
					argText = string(b)
				}
			}
			ctx := extractToolContext(tc.Name, tc.Args)
			a.conversation.AddToolLifecycleStart(tc.ID, tc.Name, argText, ctx)
			a.stats.ToolCallCount++
			a.activeTool = tc.Name
			a.setSpinVerb(verbForTool(tc.Name))
			a.statusBar.SetStatus(fmt.Sprintf("▸ %s…", tc.Name))
			a.snapshotFileWrite(tc.ID, tc.Name, tc.Args)
		}
		return a.waitForAgentEvent()
	case agent.EventToolDone:
		a.refreshGitBranch()
		a.activeTool = ""
		a.runToolCount++
		if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
			a.conversation.AddToolLifecycleDone(td.ID, td.Name, td.Context, td.Result.Summary, td.Duration, td.Result, a.conversation.ReviewMode())
			a.openDiffTabForCompletedWrite(td.ID, td.Name, td.Result.IsError)
			a.maybeRegisterArtifact(td)
			a.trackIntroducedProblems(td)
		} else if r, ok := ev.Payload.(tools.Result); ok {
			if r.IsError {
				a.conversation.AddMessage("assistant", "Tool error: "+r.Content, true)
			} else if strings.TrimSpace(r.Content) != "" {
				a.conversation.AddMessage("tool_result", truncateUIContent(r.Content, a.conversation.ReviewMode()), false)
			}
		}
		a.updatePhaseHeader()
		a.statusBar.SetStatus("Thinking…")
		return a.waitForAgentEvent()
	case agent.EventStatus:
		if s, ok := ev.Payload.(string); ok {
			// Ignore stale transient statuses that can arrive after completion.
			if !a.thinking && isTransientStatus(s) {
				return nil
			}
			a.statusBar.SetStatus(s)
			if a.thinking {
				if verb, ok := verbForStatus(s); ok {
					a.setSpinVerb(verb)
				}
			}
		}
		return a.waitForAgentEvent()
	case agent.EventNotify:
		// Payload expected to be map[string]any{"level":..., "title":..., "message":...}
		if payload, ok := ev.Payload.(map[string]any); ok {
			lvl, _ := payload["level"].(string)
			title, _ := payload["title"].(string)
			msg, _ := payload["message"].(string)
			if title != "" {
				a.statusBar.SetStatus(fmt.Sprintf("%s: %s", title, msg))
			} else {
				a.statusBar.SetStatus(msg)
			}
			// Info lines log plainly; warnings/errors surface as toasts
			// AND log for auditability.
			if msg != "" {
				switch lvl {
				case "", "info":
					a.conversation.AddMessage("system", msg, false)
				default:
					a.conversation.AddMessage("system", fmt.Sprintf("[%s] %s", lvl, msg), false)
					title, _ := payload["title"].(string)
					if a.toasts != nil {
						pushCmd := a.toasts.Push(lvl, title, msg)
						if pushCmd != nil {
							return tea.Batch(a.waitForAgentEvent(), pushCmd)
						}
					}
				}
			}
		}
		return a.waitForAgentEvent()
	case agent.EventTodoSnapshot, agent.EventTodoUpdate:
		if te, ok := ev.Payload.(agent.TodoEvent); ok {
			a.taskBoard.SetTodos(te.Items)
			if a.taskBoard.Visible() {
				a.layout()
			}
		}
		return a.waitForAgentEvent()
	case agent.EventInitAction:
		// Init-phase prep work renders exactly like model-driven tool calls —
		// part of the log, not separate chatter.
		if p, ok := ev.Payload.(shared.InitActionEvent); ok {
			args := initActionArgs(p.RawTool, p.Target)
			argText := ""
			if b, err := json.Marshal(args); err == nil {
				argText = string(b)
			}
			id := "init:" + p.RawTool + ":" + p.Target
			if p.Running {
				a.conversation.AddToolLifecycleStart(id, p.Tool, argText, p.Target)
			} else {
				result := tools.Result{Content: p.Summary, IsError: p.Failed}
				if p.Failed {
					result.Content = p.Err
				}
				a.conversation.AddToolLifecycleDone(id, p.Tool, p.Target, p.Summary, p.Duration, result, a.conversation.ReviewMode())
			}
		}
		return a.waitForAgentEvent()
	case agent.EventThinking:
		if thinkingText, ok := ev.Payload.(string); ok {
			a.statusBar.SetStatus(thinkingText)
		}
		return a.waitForAgentEvent()
	case agent.EventPhase:
		// The arc entered a new phase (explore/plan/build/…): drive the
		// header's phase indicator from the phase manager's real state
		// instead of the DetectPhase history heuristic.
		if p, ok := ev.Payload.(string); ok && strings.TrimSpace(p) != "" {
			a.livePhase = p
			a.updatePhaseHeader()
		}
		return a.waitForAgentEvent()
	case agent.EventPhaseDone:
		// One phase of a multi-phase arc finished; the run continues with
		// the next one. Settle the phase's reply into the transcript like
		// EventDone does, but keep the spinner and thinking state alive and
		// KEEP LISTENING — the turn is not over and EventDone must remain
		// the run's single terminal event.
		a.conversation.RenderIfDirty()
		text, _ := ev.Payload.(string)
		if a.streamedReply {
			a.conversation.FinalizeStreamingWithContent(text)
		} else {
			a.conversation.FinalizeStreaming()
		}
		if strings.TrimSpace(text) != "" && !a.streamedReply {
			a.conversation.AddMessage("assistant", text, false)
		}
		// The next phase streams into a fresh block.
		a.streamedReply = false
		a.statusBar.SetStatus("Phase complete — continuing…")
		return a.waitForAgentEvent()
	case agent.EventDone:
		a.thinking = false
		a.spin.Stop()
		a.setSpinVerb("")
		a.streamTickPending = false
		a.activeTool = ""
		a.clearRetryState()
		// A completed run supersedes any earlier interruption badge.
		a.lastOutcome = outcomeNone
		a.conversation.RenderIfDirty()
		text, _ := ev.Payload.(string)
		if a.streamedReply {
			a.conversation.FinalizeStreamingWithContent(text)
		} else {
			a.conversation.FinalizeStreaming()
		}
		a.layout() // Reclaim space from spinner
		// Post-run summary lives where the spinner was: the conversation's
		// run line settles to "✓ Done (2s • ↓ 20 tokens)" below the reply,
		// the way Claude Code leaves its spinner row's readout behind. The
		// status bar carries only the settled duration in its HUD.
		if !a.runStart.IsZero() {
			d := time.Since(a.runStart)
			a.runStart = time.Time{}
			summary := fmt.Sprintf("✓ Done (%s", formatDuration(d))
			if toks := estimateTokens(a.runChars); toks > 0 {
				summary += fmt.Sprintf(" • ↓ %s tokens", compactTokenCount(toks))
			}
			a.lastRunSummary = summary + ")"
			// The run's report lives in the spinner slot now; the bar just
			// returns to idle.
			a.statusBar.SetStatus("Ready")
		} else {
			a.statusBar.SetStatus("Ready")
		}
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		if tel := a.ag.Telemetry(); tel != nil {
			cost := tel.GetCostSummary().TotalCostUSD
			a.stats.TotalCost = cost
			a.header.SetCost(cost)
		}
		a.header.SetTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)
		if calc := a.ag.AdaptiveCalculator(); calc != nil {
			a.header.SetAdaptiveWeight(calc.Weight())
		}
		a.updatePhaseHeader()
		if strings.TrimSpace(text) != "" && !a.streamedReply {
			a.conversation.AddMessage("assistant", text, false)
		}
		// A message queued during this run is delivered now that the turn is
		// over. One per turn: the reply to the first may change whether the
		// rest still make sense.
		var cmds []tea.Cmd
		if cmd := a.maybeGenerateSessionTitle(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := a.drainQueue(); cmd != nil {
			cmds = append(cmds, cmd)
		} else if a.lastOutcome == outcomeNone {
			// Idle with no queued user message: an active goal drives the
			// next turn itself (goal.go); otherwise an idle consolidation
			// pass may fire (dream.go).
			if cmd := a.maybeContinueGoal(text); cmd != nil {
				cmds = append(cmds, cmd)
			} else {
				a.maybeConsolidateMemory()
			}
		}
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
		return nil
	case agent.EventCompacted:
		a.statusBar.SetStatus("Context compacted")
		a.stats.InputTokens = a.sess.TotalInputTokens
		a.stats.OutputTokens = a.sess.TotalOutputTokens
		a.header.SetTokens(a.sess.TotalInputTokens + a.sess.TotalOutputTokens)
		if calc := a.ag.AdaptiveCalculator(); calc != nil {
			a.header.SetAdaptiveWeight(calc.Weight())
		}
		a.updatePhaseHeader()
		a.conversation.AddMessage("system", "Context compacted successfully", false)
		return nil
	case agent.EventError:
		a.thinking = false
		a.spin.Stop()
		a.setSpinVerb("")
		a.spin.SetMeta("")
		// A failed run has no ✓ to settle on; the status bar's outcome badge
		// is its report, so the last successful run's summary must not linger
		// in the spinner slot.
		a.lastRunSummary = ""
		a.streamTickPending = false
		a.activeTool = ""
		a.conversation.RenderIfDirty()
		a.conversation.FinalizeStreaming() // Re-render with markdown
		a.layout()                         // Reclaim space from spinner
		if err, ok := ev.Payload.(error); ok {
			errStr := err.Error()
			msg := formatErrorMessage(errStr)
			if isCancellationError(errStr) {
				a.conversation.AddMessage("system", msg, false)
				a.statusBar.SetStatus("Cancelled")
				a.lastOutcome = outcomeCancelled
				a.clearRetryState()
				return nil
			}
			// File the failure before clearing retry state: the attempt count
			// from the retry sequence is what makes "failed after 10 attempts"
			// accurate.
			a.recordTerminalAPIError(err)
			a.clearRetryState()
			a.lastOutcome = outcomeError
			a.conversation.AddMessage("assistant", msg, true)
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "authentication_error") {
				a.conversation.AddMessage("system", "Tip: You can set the API key using: /api-key <key>", false)
			}
			if rec, ok := a.latestAPIError(); ok {
				a.conversation.AddMessage("system",
					fmt.Sprintf("Recorded as %s — /error shows the full log.", rec.displayCode()), false)
			}
		} else {
			a.lastOutcome = outcomeError
			a.clearRetryState()
		}
		a.statusBar.SetStatus("Error")
		return nil
	case agent.EventConfirm:
		if payload, ok := ev.Payload.(map[string]any); ok {
			if tc, ok := payload["tool_call"].(ai.ToolCall); ok {

				// Name the tool exactly as its conversation card does, from the
				// single spec table in components — not a second lookup table
				// that can drift out of sync with it.
				name := components.ToolDisplayName(tc.Name)

				// A subagent's ask is re-emitted by the parent with provenance;
				// surface who is asking so "always allow" is an informed call.
				subName, _ := payload["agent_name"].(string)
				subType, _ := payload["agent_type"].(string)
				provenance := func(pi components.PermissionInfo) components.PermissionInfo {
					if subName != "" || subType != "" {
						pi.AgentName, pi.AgentType = subName, subType
					}
					return pi
				}

				// Special handling for file edits: show diff
				// Special handling for file edits: show diff with inline confirmation
				if tc.Name == "write_file" || tc.Name == "edit_file" {
					path, _ := tc.Args["path"].(string)
					newContent := ""
					if tc.Name == "write_file" {
						newContent, _ = tc.Args["content"].(string)
					} else {
						// Patch: read file and apply patch
						oldStr, _ := tc.Args["old_str"].(string)
						replaceWith, _ := tc.Args["new_str"].(string)
						replaceAll, _ := tc.Args["replace_all"].(bool)
						data, _ := os.ReadFile(path)
						original := string(data)
						if replaceAll {
							newContent = strings.ReplaceAll(original, oldStr, replaceWith)
						} else {
							newContent = strings.Replace(original, oldStr, replaceWith, 1)
						}
					}

					oldData, _ := os.ReadFile(path)
					diff := computeSimpleDiff(path, string(oldData), newContent)
					a.conversation.UpdateToolContent(tc.ID, diff)

					// Open (or refresh) this file's tab: it moves to the
					// front of the recency-ordered strip and becomes the
					// selected view, with previously edited files to its right.
					a.diffPane.OpenFile(path, diff)

					// Confirmation rides on the fullscreen diff view.
					if replyCh, ok := payload["reply"].(chan agent.ConfirmationResponse); ok {
						a.diffPane.ShowWithConfirm(bridgeConfirmation(replyCh))
						a.pendingDiffHide = true
					}
					a.layout()
				} else {
					// Non-file tools use confirm component
					permission := provenance(permissionInfoForTool(tc, name))
					a.confirm.ShowPermission(permission)
					a.permissionTool = name
					if subName != "" || subType != "" {
						a.permissionTool = name + " · " + firstNonEmptyDock(subName, subType)
					}
					a.statusBar.SetPermission(a.permissionTool)
					a.layout()
					if replyCh, ok := payload["reply"].(chan agent.ConfirmationResponse); ok {
						a.confirm.SetReply(bridgeConfirmation(replyCh))
					}
				}
			}
		}
		return a.waitForAgentEvent()
	case agent.EventAskUser:
		// Structured asks render through the questionnaire component; this
		// event only updates the status hint for legacy flows.
		if payload, ok := ev.Payload.(map[string]any); ok {
			question, _ := payload["question"].(string)
			replyCh, _ := payload["reply"].(chan string)
			// A subagent's question carries provenance; name the asker.
			if subName, _ := payload["agent_name"].(string); subName != "" {
				question = subName + " asks: " + question
			}
			a.askUserReplyCh = replyCh
			if !(a.questionnaire != nil && a.questionnaire.Visible()) {
				a.statusBar.SetStatus("PROMPT: " + question)
				a.input.Focus()
			}
		}
		return a.waitForAgentEvent()
	}
	return a.waitForAgentEvent()
}

// introducedErrorsRe extracts the introduced-error count from a blocked
// edit_file/write_file validation result ("Introduced: N error(s), M warning(s)").
var introducedErrorsRe = regexp.MustCompile(`Introduced:\s*(\d+) error\(s\)`)

// trackIntroducedProblems feeds the statusbar problems counter: when an edit
// or write is blocked for introducing new errors, those errors count as
// problems this session.
func (a *App) trackIntroducedProblems(td agent.ToolDoneEvent) {
	if td.Name != "edit_file" && td.Name != "write_file" {
		return
	}
	if !td.Result.IsError {
		return
	}
	m := introducedErrorsRe.FindStringSubmatch(td.Result.Content)
	if m == nil {
		return
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return
	}
	a.sessionProblems += n
	a.statusBar.SetProblems(a.sessionProblems)
}
