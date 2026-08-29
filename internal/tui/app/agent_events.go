package app

// Agent event stream handling (tokens, tools, notify, todos).
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/shared"
	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/components"
	"os"
	"strings"
)

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
				a.runTokens++
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
			a.statusBar.SetStatus(fmt.Sprintf("▸ %s…", te.Name))
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
			a.statusBar.SetStatus(fmt.Sprintf("▸ %s…", tc.Name))
		}
		return a.waitForAgentEvent()
	case agent.EventToolDone:
		a.refreshGitBranch()
		a.activeTool = ""
		a.runToolCount++
		if td, ok := ev.Payload.(agent.ToolDoneEvent); ok {
			a.conversation.AddToolLifecycleDone(td.ID, td.Name, td.Context, td.Result.Summary, td.Duration, td.Result, a.conversation.ReviewMode())
		} else if r, ok := ev.Payload.(tools.Result); ok {
			if r.IsError {
				a.conversation.AddMessage("assistant", "Tool error: "+r.Content, true)
			} else if strings.TrimSpace(r.Content) != "" {
				a.conversation.AddMessage("tool_result", truncateUIContent(r.Content, a.conversation.ReviewMode()), false)
			}
		}
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		a.statusBar.SetStatus("Thinking…")
		return a.waitForAgentEvent()
	case agent.EventStatus:
		if s, ok := ev.Payload.(string); ok {
			// Ignore stale transient statuses that can arrive after completion.
			if !a.thinking && isTransientStatus(s) {
				return nil
			}
			a.statusBar.SetStatus(s)
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
	case agent.EventDone:
		a.thinking = false
		a.spin.Stop()
		a.spin.SetLabel("thinking")
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
		a.statusBar.SetStatus("Ready")
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
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		if strings.TrimSpace(text) != "" && !a.streamedReply {
			a.conversation.AddMessage("assistant", text, false)
		}
		// A message queued during this run is delivered now that the turn is
		// over. One per turn: the reply to the first may change whether the
		// rest still make sense.
		if cmd := a.drainQueue(); cmd != nil {
			return cmd
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
		a.header.SetPhase(string(agent.DetectPhase(a.sess.Messages)))
		a.conversation.AddMessage("system", "Context compacted successfully", false)
		return nil
	case agent.EventError:
		a.thinking = false
		a.spin.Stop()
		a.spin.SetLabel("thinking")
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

				// Special handling for file edits: show diff
				// Special handling for file edits: show diff with inline confirmation
				if tc.Name == "write_file" || tc.Name == "edit_file" || tc.Name == "create_file" {
					path, _ := tc.Args["path"].(string)
					newContent := ""
					if tc.Name == "write_file" || tc.Name == "create_file" {
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
					permission := permissionInfoForTool(tc, name)
					a.confirm.ShowPermission(permission)
					a.permissionTool = name
					a.statusBar.SetPermission(permission.Tool)
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
