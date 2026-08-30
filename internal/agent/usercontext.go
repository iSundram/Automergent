package agent

import (
	"fmt"
	"strings"
	"sync"

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
		prefix = append(prefix, ai.Message{
			Role: ai.RoleUser,
			Content: []ai.ContentPart{{
				Type: ai.ContentTypeText,
				Text: fmt.Sprintf("<system-reminder>\nAs you answer the user's questions, you can use the following context:\n# gitStatus\n%s\nIMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.\n</system-reminder>\n", git),
			}},
			Metadata: map[string]any{"meta": true},
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
