package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/ai"
	contextmgr "github.com/iSundram/Automergent/internal/context"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
)

// Subagent parity with the reference agent, collected in one place:
//
//   - fast-model routing: read-only agents (explore/review) run on the
//     configured FastModel, the Explore→haiku pattern — the biggest cost
//     lever, since read-only spawns dominate fleet volume.
//   - context slimming: read-only children do not receive the project
//     instructions or the git snapshot (dead weight for them; they run
//     `git status` themselves when needed). Mirrors the reference agent's
//     slim-context behavior.
//   - fork: a child can inherit the parent's conversation (repaired), so
//     multi-step delegation starts warm.
//   - resume: completed/stopped children are kept around with their session
//     so a follow-up prompt continues the same conversation.
//   - sidechain transcripts: every child records its messages to its own
//     transcript file, feeding the agent viewer and resume.
//   - agent memory: definitions with a MemoryScope get persistent memory
//     files loaded into their context and an agent_memory tool to write.

// readOnlyToolSet lists tools a read-only agent may use. A definition whose
// tool list is a subset of these (or whose name is a known read-only type)
// is treated as read-only for model routing and context slimming.
var readOnlyToolSet = map[string]bool{
	"read_file": true, "grep": true, "glob": true, "list_directory": true,
	"bash": true, "read_shell": true, "list_shells": true,
	"web_search": true, "web_fetch": true, "lsp_diagnostics": true,
	"list_agents": true, "read_agent": true, "todo_list": true,
	"agent_memory": true,
}

// isReadOnlyDefinition reports whether an agent definition is read-only:
// every tool it declares is in the read-only set (an empty list means "all
// tools", i.e. NOT read-only).
func isReadOnlyDefinition(def *agentdef.AgentDefinition) bool {
	if def == nil {
		return false
	}
	switch def.Name {
	case "explore", "review":
		return true
	}
	if len(def.Tools) == 0 {
		return false
	}
	for _, t := range def.Tools {
		if !readOnlyToolSet[t] {
			return false
		}
	}
	return true
}

// resolveChildModel picks the model a child agent runs on:
// explicit override > definition's own model > FastModel for read-only
// agents > the parent's model (empty string).
func (a *Agent) resolveChildModel(explicit string, def *agentdef.AgentDefinition) string {
	if explicit != "" {
		return explicit
	}
	if def != nil && def.Model != "" {
		return def.Model
	}
	if def != nil && isReadOnlyDefinition(def) && a.cfg != nil && a.cfg.FastModel != "" {
		return a.cfg.FastModel
	}
	return ""
}

// prepareChild applies the parity behaviors to a freshly built child agent:
// context slimming for read-only definitions, a sidechain transcript, agent
// memory, and registration in the resume table.
func (a *Agent) prepareChild(childAgent *Agent, def *agentdef.AgentDefinition, trackedID string) {
	if def != nil && isReadOnlyDefinition(def) {
		childAgent.omitProjectContext = true
	}

	// Sidechain transcript + resume handle. Both keyed on the tracked
	// instance ID; direct Execute callers (untracked) skip them.
	if trackedID != "" {
		childAgent.setSidechainTranscript(trackedID)
		a.trackChild(trackedID, childAgent)
	}

	// Agent memory: definitions with a memory scope get a persistent memory
	// file and the tool to write it.
	if def != nil && def.MemoryScope != agentdef.MemoryScopeNone {
		childAgent.installAgentMemory(def)
	}
}

// setSidechainTranscript points this child's transcript at its own file
// under .automergent/subagents/, separate from the main session transcript.
func (c *Agent) setSidechainTranscript(agentID string) {
	dir := filepath.Join(c.workDir, ".automergent", "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, sanitizeFileComponent(agentID)+".jsonl")
	mgr := contextmgr.NewTranscriptManager(contextmgr.NewTranscript(path))
	c.ContextManager().SetTranscriptManager(mgr)
}

// trackChild registers a spawned child for later resume.
func (a *Agent) trackChild(agentID string, child *Agent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.childHandles == nil {
		a.childHandles = make(map[string]*Agent)
	}
	// Bounded: keep the most recent 32 children for resume.
	if len(a.childHandles) >= 32 {
		for id := range a.childHandles {
			delete(a.childHandles, id)
			break
		}
	}
	a.childHandles[agentID] = child
}

// resumeChild returns the stored child agent for an instance ID.
func (a *Agent) resumeChild(agentID string) (*Agent, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	child, ok := a.childHandles[agentID]
	return child, ok
}

// sanitizeFileComponent makes an agent ID safe as a filename.
func sanitizeFileComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return b.String()
}

// forkContextMessages returns the parent's messages, repaired so no tool
// call dangles (interrupted calls get synthetic error results), for a fork
// child to inherit.
func (a *Agent) forkContextMessages() []ai.Message {
	return ai.RepairMissingToolResults(a.sess.Messages)
}

// Context-key readers for the tags set by the task tool (see
// tools/agent/live.go). Kept as thin wrappers so the agent code reads
// naturally.

func resumeAgentIDFrom(ctx context.Context) string {
	return subagent.ResumeAgentIDFrom(ctx)
}

func forkContextFrom(ctx context.Context) bool {
	return subagent.ForkContextFrom(ctx)
}
