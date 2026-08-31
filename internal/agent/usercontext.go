package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/ai"
	promptpkg "github.com/iSundram/Automergent/internal/prompt"
)

// Per-conversation user context, ported from the reference agent's design:
//
// Project instructions (AUTOMERGENT.md / AGENTS.md / CLAUDE.md) and the git
// status snapshot are NOT part of the system prompt. They are injected as
// meta user messages ahead of the conversation:
//
//   - instructions get a dedicated <project-instructions> message, so their
//     instructional weight is not diluted by the "may or may not be
//     relevant" disclaimer that context snippets carry;
//   - the git snapshot rides in a <system-reminder> that explicitly marks it
//     as background context.
//
// Keeping this content out of the system prompt also keeps the prompt prefix
// stable for provider prompt caching, and it never enters the persisted
// session — it is rebuilt (and cached for the conversation's lifetime) at
// request time.

// userContext returns the conversation-scoped context map, building it once
// on first use (the git snapshot is a start-of-conversation snapshot by
// design; recomputing it per turn would make it drift mid-task).
func (a *Agent) userContext() map[string]string {
	a.userCtxOnce.Do(func() {
		ctx := map[string]string{}
		if a.workDir != "" && !a.omitProjectContext {
			// Read-only subagents omit the project instructions and git
			// snapshot: dead weight for them (they run git status themselves
			// when needed), and dropping it is a large fleet-wide token save.
			if instr := promptpkg.ProjectInstructions(a.workDir); instr != "" {
				ctx["projectInstructions"] = instr
			}
			if git := promptpkg.GitStatusBlock(a.workDir); git != "" {
				ctx["gitStatus"] = git
			}
			// User-stated rules captured by the INIT decomposer ride with the
			// project instructions so they carry the same weight.
			if rules := a.ruleStore().List(); len(rules) > 0 {
				ctx["userRules"] = strings.Join(rules, "\n- ")
			}
		}
		if global := promptpkg.GlobalInstructions(); global != "" {
			ctx["globalInstructions"] = global
		}
		// Today's date is conversation-scoped context, not system-prompt
		// content: a date line in the composed prompt would bust the provider
		// prompt cache on every phase transition (and every midnight).
		ctx["sessionDate"] = time.Now().Format("Mon Jan 2 2006")
		// Persistent agent memory (subagents with a MemoryScope).
		if a.agentMemory != nil {
			if mem := a.agentMemory.Prompt(); mem != "" {
				ctx["agentMemory"] = mem
			}
		}
		a.userCtx = ctx
	})
	return a.userCtx
}

// resetUserContext drops the cached snapshot (used when the working
// directory changes, e.g. session switch).
func (a *Agent) resetUserContext() {
	a.userCtxOnce = sync.Once{}
	a.userCtx = nil
}

// prependUserContext returns messages with the context injected as leading
// meta user messages. The input slice is never mutated; the returned slice is
// a fresh copy safe to hand to the provider without persisting.
func prependUserContext(messages []ai.Message, context map[string]string) []ai.Message {
	instr := context["projectInstructions"]
	global := context["globalInstructions"]
	rules := context["userRules"]
	memory := context["agentMemory"]
	git := context["gitStatus"]

	var prefix []ai.Message
	if instr != "" || global != "" || rules != "" {
		var sb strings.Builder
		sb.WriteString("<project-instructions>\n")
		if global != "" {
			sb.WriteString("## Global Instructions\n")
			sb.WriteString(global)
			sb.WriteString("\n\n")
		}
		if instr != "" {
			sb.WriteString("## Project Instructions\n")
			sb.WriteString(instr)
			sb.WriteString("\n")
		}
		if rules != "" {
			sb.WriteString("\n## User Rules (stated in conversation; binding)\n- ")
			sb.WriteString(rules)
			sb.WriteString("\n")
		}
		if memory != "" {
			sb.WriteString("\n" + memory + "\n")
		}
		sb.WriteString("</project-instructions>\n")
		prefix = append(prefix, ai.Message{
			Role: ai.RoleUser,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeText,
				Text: sb.String(),
			}},
			Metadata: map[string]any{"meta": true},
		})
	}

	if git != "" {
		// The git snapshot joins the same meta message rather than starting
		// a second user turn: strict providers (Google) reject consecutive
		// same-role messages, and the snapshot is context, not a turn.
		if len(prefix) == 0 {
			prefix = append(prefix, ai.Message{Role: ai.RoleUser, Metadata: map[string]any{"meta": true}})
		}
		prefix[0].Content = append(prefix[0].Content, ai.ContentPart{
			Type: ai.ContentTypeText,
			Text: fmt.Sprintf("<system-reminder>\nAs you answer the user's questions, you can use the following context:\n# gitStatus\n%s\nIMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.\n</system-reminder>\n", git),
		})
	}

	if date := context["sessionDate"]; date != "" {
		if len(prefix) == 0 {
			prefix = append(prefix, ai.Message{Role: ai.RoleUser, Metadata: map[string]any{"meta": true}})
		}
		prefix[0].Content = append(prefix[0].Content, ai.ContentPart{
			Type: ai.ContentTypeText,
			Text: fmt.Sprintf("<system-reminder>\nToday's date is %s.\n</system-reminder>\n", date),
		})
	}

	if len(prefix) == 0 {
		return messages
	}
	out := make([]ai.Message, 0, len(prefix)+len(messages))
	out = append(out, prefix...)
	out = append(out, messages...)
	return out
}

// mergeConsecutiveRoles coalesces adjacent non-tool messages of the same
// role into single messages by concatenating their content parts. Strict
// providers require user/model alternation; merged histories, steered
// notifications, and compacted blocks can all produce runs of same-role
// messages. Tool messages never merge — their pairing with the preceding
// assistant's tool calls is an API invariant.
func mergeConsecutiveRoles(messages []ai.Message) []ai.Message {
	out := make([]ai.Message, 0, len(messages))
	for _, msg := range messages {
		if n := len(out); n > 0 && out[n-1].Role == msg.Role && msg.Role != ai.RoleTool {
			out[n-1].Content = append(out[n-1].Content, msg.Content...)
			continue
		}
		out = append(out, msg)
	}
	return out
}
