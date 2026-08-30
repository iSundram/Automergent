package agent

// Live progress for the dock.
//
// An AgentInstance used to record only what it was asked and what it finally
// returned. Between those two moments — which is most of a subagent's life —
// there was nothing to display, which is why the UI's agent view was written
// against fields that had no source and consequently never shipped.
//
// Three facts turn a running subagent from a spinner into something readable:
// which tool it is in, how much work it has done, and whose child it is. This
// file records all three, and nothing else: a progress record that grows into a
// second transcript is a memory leak with a UI attached.

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
)

// maxRecentActions bounds the per-agent tool history. Five is what a dock row
// can show on expansion; keeping more would be storage for its own sake.
const maxRecentActions = 5

// agentIDKey carries the running agent's ID down to the executor.
//
// The AgentExecutor interface takes (type, prompt, model) and no identity, so
// the code that actually runs a subagent's turn had no way to say which
// instance it was running — which is why nothing ever populated the progress
// fields. Threading the ID through the context rather than widening the
// interface keeps every existing implementation valid, and an executor that
// ignores it simply reports no progress instead of failing to compile.
type agentIDKey struct{}

// WithAgentID tags ctx with the instance ID whose turn is being executed.
func WithAgentID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, agentIDKey{}, id)
}

// AgentIDFrom returns the instance ID tagged onto ctx, or "" when the caller is
// not running on behalf of a tracked instance.
func AgentIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(agentIDKey{}).(string)
	return id
}

// resumeAgentKey tags a request to continue an existing agent's conversation.
type resumeAgentKey struct{}

// WithResumeAgentID tags ctx so the executor resumes the stored child agent
// (same session, history, and memory) instead of spawning a fresh one.
func WithResumeAgentID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, resumeAgentKey{}, id)
}

// ResumeAgentIDFrom returns the tagged resume ID, or "".
func ResumeAgentIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(resumeAgentKey{}).(string)
	return id
}

// forkContextKey tags a request to inherit the parent's conversation.
type forkContextKey struct{}

// WithForkContext tags ctx so the executor seeds the child agent with the
// parent's conversation (fork semantics: warm start, no clean slate).
func WithForkContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, forkContextKey{}, true)
}

// ForkContextFrom reports whether ctx requested fork semantics.
func ForkContextFrom(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(forkContextKey{}).(bool)
	return v
}

// NoteSpawn records a parent/child relationship. Called when a subagent's own
// tool loop spawns another subagent, so the dock can indent the child under the
// agent that asked for it instead of showing a flat row of siblings whose
// relationship the user has to guess.
func (m *AgentManager) NoteSpawn(childID, parentID string) {
	if childID == "" || parentID == "" || childID == parentID {
		return
	}
	child, ok := m.Get(childID)
	if !ok {
		return
	}
	child.mu.Lock()
	child.ParentID = parentID
	child.mu.Unlock()
}

// maxActivityLines bounds the per-agent activity log. The viewer is a "what
// is it doing" pane, not a transcript; anything older than the last fifty
// steps is diagnosable from the result, not from here.
const maxActivityLines = 50

// NoteActivity appends one line to the agent's bounded activity log — the
// subagent's own short conversation, shown by the agent viewer.
func (m *AgentManager) NoteActivity(agentID, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	inst, ok := m.Get(agentID)
	if !ok {
		return
	}
	inst.mu.Lock()
	inst.Activity = append(inst.Activity, line)
	if len(inst.Activity) > maxActivityLines {
		inst.Activity = inst.Activity[len(inst.Activity)-maxActivityLines:]
	}
	inst.mu.Unlock()
	inst.lastActivity.Store(timePtr(time.Now()))
}

// ActivityLines returns a copy of the agent's activity log.
func (a *AgentInstance) ActivityLines() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.Activity))
	copy(out, a.Activity)
	return out
}

// ToolActivityLine renders one tool call as a single activity line:
// "grep "palette" internal/tui", "bash go test ./...". Arguments are reduced
// to the one or two that identify the call; everything else is noise at this
// zoom level.
func ToolActivityLine(name string, args map[string]any) string {
	if args == nil {
		return name
	}
	subject := ""
	for _, key := range []string{"command", "pattern", "query", "path", "file_path", "url", "agent_type", "name"} {
		if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
			subject = v
			break
		}
	}
	if subject == "" {
		return name
	}
	if len(subject) > 60 {
		subject = subject[:57] + "…"
	}
	return name + " " + subject
}

// NoteTool records that an agent entered a tool. Passing "" clears the current
// tool, which is what the gap between one tool finishing and the next starting
// actually looks like.
func (m *AgentManager) NoteTool(agentID, tool string) {
	inst, ok := m.Get(agentID)
	if !ok {
		return
	}
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.CurrentTool = tool
	if tool == "" {
		return
	}
	inst.ToolCount++
	inst.RecentTools = append(inst.RecentTools, tool)
	if len(inst.RecentTools) > maxRecentActions {
		inst.RecentTools = inst.RecentTools[len(inst.RecentTools)-maxRecentActions:]
	}
	inst.lastActivity.Store(timePtr(time.Now()))
}

// NoteOutput records the newest line of an agent's own output, for the dock's
// activity cell. Only the last non-empty line is kept.
func (m *AgentManager) NoteOutput(agentID, chunk string) {
	inst, ok := m.Get(agentID)
	if !ok {
		return
	}
	line := lastNonEmptyLine(chunk)
	if line == "" {
		return
	}
	inst.lastLine.Store(&line)
	inst.lastActivity.Store(timePtr(time.Now()))
}

// NoteTokens adds to an agent's running token totals.
func (m *AgentManager) NoteTokens(agentID string, in, out int) {
	inst, ok := m.Get(agentID)
	if !ok {
		return
	}
	inst.mu.Lock()
	inst.TokensIn += in
	inst.TokensOut += out
	inst.mu.Unlock()
}

// LastLine returns the newest output line, without taking the instance lock.
func (a *AgentInstance) LastLine() string {
	if p := a.lastLine.Load(); p != nil {
		return *p
	}
	return ""
}

// LastActivity returns when the agent last did something observable, or the
// zero time if it has not yet. Used to mark an agent that is running but has
// gone quiet — the state that most often means "stuck", and that a bare
// "running" label hides.
func (a *AgentInstance) LastActivity() time.Time {
	if p := a.lastActivity.Load(); p != nil {
		return *p
	}
	return time.Time{}
}

// Children returns the IDs of agents spawned by this one, in creation order.
func (m *AgentManager) Children(parentID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for id, a := range m.agents {
		a.mu.Lock()
		isChild := a.ParentID == parentID
		a.mu.Unlock()
		if isChild {
			out = append(out, id)
		}
	}
	return out
}

// lastNonEmptyLine returns the last line of s with non-space content.
func lastNonEmptyLine(s string) string {
	s = strings.TrimRight(s, "\r\n")
	for {
		i := strings.LastIndexByte(s, '\n')
		if line := strings.TrimSpace(s[i+1:]); line != "" {
			return line
		}
		if i < 0 {
			return ""
		}
		s = s[:i]
	}
}

func timePtr(t time.Time) *time.Time { return &t }

// atomicString and atomicTime hold the lock-free fields the dock reads once a
// second for every live agent.
type (
	atomicString = atomic.Pointer[string]
	atomicTime   = atomic.Pointer[time.Time]
)
