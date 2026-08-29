package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// This file implements the subagent algebra WITHOUT modifying tools.go:
// - continue / fork / interrupt over existing instances
// - concurrency caps
// - custom agent definitions from .agents/*.md (frontmatter)
// - tagged result envelopes ("notifications, not narration")

const maxConcurrentAgents = 8

type customAgentDef struct {
	Name        string
	Description string
	Model       string
	SystemBody  string
	Tools       []string
}

var (
	customMu       sync.RWMutex
	customAgents   = map[string]*customAgentDef{}
	cancelRegistry sync.Map // agentID -> context.CancelFunc
)

// RegisterCustomAgent adds a user-defined agent type.
func RegisterCustomAgent(def *customAgentDef) {
	if def == nil || strings.TrimSpace(def.Name) == "" {
		return
	}
	customMu.Lock()
	defer customMu.Unlock()
	customAgents[strings.ToLower(def.Name)] = def
}

// LookupCustomAgent returns a registered custom definition.
func LookupCustomAgent(name string) (*customAgentDef, bool) {
	customMu.RLock()
	defer customMu.RUnlock()
	def, ok := customAgents[strings.ToLower(name)]
	return def, ok
}

// LoadAgentDefinitions scans dir for *.md files with frontmatter:
//
//	---
//	name: db-migrator
//	description: Runs schema migrations safely
//	model: gemini-3-flash      # optional
//	tools: [bash, sql]          # optional capability hint
//	---
//	<role instructions applied as prompt preamble>
func LoadAgentDefinitions(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var loaded []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if def := parseAgentDefinition(string(data)); def != nil {
			RegisterCustomAgent(def)
			loaded = append(loaded, def.Name)
		}
	}
	return loaded, nil
}

func parseAgentDefinition(content string) *customAgentDef {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil
	}
	end := strings.Index(content[3:], "\n---")
	if end < 0 {
		return nil
	}
	frontmatter := content[4 : end+3]
	body := strings.TrimSpace(content[end+7:])
	def := &customAgentDef{SystemBody: body}
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "name":
			def.Name = val
		case "description":
			def.Description = val
		case "model":
			def.Model = val
		case "tools":
			val = strings.Trim(val, "[]")
			for _, t := range strings.Split(val, ",") {
				if t = strings.TrimSpace(t); t != "" {
					def.Tools = append(def.Tools, t)
				}
			}
		}
	}
	if def.Name == "" {
		return nil
	}
	return def
}

// --- agent_control tool ---

// ControlTool exposes continue/fork/interrupt/list over live subagents.
type ControlTool struct {
	tools.BaseTool
}

func (*ControlTool) Name() string { return "agent_control" }
func (*ControlTool) Description() string {
	return "Manage running/completed subagents: continue their thread, fork one, interrupt it, or list them."
}
func (*ControlTool) RequiresConfirmation(string) bool      { return false }
func (*ControlTool) IsConcurrencySafe(map[string]any) bool { return true }
func (*ControlTool) IsReadOnly(args map[string]any) bool {
	action, _ := tools.StringArg(args, "action")
	return action == "list"
}
func (*ControlTool) IsDestructive(args map[string]any) bool {
	action, _ := tools.StringArg(args, "action")
	return action == "interrupt"
}
func (*ControlTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 60, LatencyMs: 20, RiskLevel: "low"}
}
func (*ControlTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:    "agents",
		DisplayName: "Control agents",
		InjectOrder: 20,
		WhenToUse:   "Continue an agent's thread for follow-ups, fork a RUNNING agent to split independent todo batches, interrupt on plan change, or list statuses.",
		WhenNotTo:   "Do not continue completed agents for brand-new unrelated tasks — spawn fresh instead (better parallelism and context hygiene).",
	}
}

func (*ControlTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":   map[string]any{"type": "string", "enum": []string{"list", "continue", "fork", "interrupt"}},
			"agent_id": map[string]any{"type": "string"},
			"prompt":   map[string]any{"type": "string", "description": "Required for continue/fork"},
			"mode":     map[string]any{"type": "string", "enum": []string{"sync", "background"}, "description": "For continue/fork (default sync)"},
		},
		"required": []string{"action"},
	}
}

func (t *ControlTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	action, _ := tools.StringArg(args, "action")
	switch action {
	case "list":
		return t.list(), nil
	case "continue":
		return t.continueOrFork(ctx, args, true)
	case "fork":
		return t.continueOrFork(ctx, args, false)
	case "interrupt":
		id, _ := tools.StringArg(args, "agent_id")
		return t.interrupt(id), nil
	default:
		return tools.Result{IsError: true, Content: "agent_control: unknown action"}, nil
	}
}

func (t *ControlTool) list() tools.Result {
	agents := GetAgentManager().List(true)
	if len(agents) == 0 {
		return tools.Result{Content: "No subagents."}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d subagent(s):\n", len(agents))
	for _, a := range agents {
		a.mu.Lock()
		fmt.Fprintf(&sb, "- %s [%s] %s — %s\n", a.ID, a.Status, a.Type, firstNonEmpty(a.Name, "(unnamed)"))
		a.mu.Unlock()
	}
	return tools.Result{Content: sb.String()}
}

func (t *ControlTool) continueOrFork(ctx context.Context, args map[string]any, sameInstance bool) (tools.Result, error) {
	id, _ := tools.StringArg(args, "agent_id")
	prompt, _ := tools.StringArg(args, "prompt")
	if id == "" || prompt == "" {
		return tools.Result{IsError: true, Content: "continue/fork require agent_id and prompt"}, nil
	}
	src, ok := GetAgentManager().Get(id)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("agent %q not found", id)}, nil
	}
	src.mu.Lock()
	status := src.Status
	var transcript strings.Builder
	transcript.WriteString("Original task: " + src.Prompt + "\n")
	for _, turn := range src.Turns {
		transcript.WriteString("\nPrevious turn " + fmt.Sprint(turn.Index) + " output:\n" + turn.Output + "\n")
	}
	typeName := src.Type
	name := src.Name
	modelHint := ""
	src.mu.Unlock()

	if status == AgentStatusRunning && sameInstance {
		return tools.Result{IsError: true, Content: "agent still running — use fork for parallel branches or interrupt first"}, nil
	}

	composed := transcript.String() + "\nFollow-up instruction:\n" + prompt

	mode, _ := tools.StringArg(args, "mode")
	runMode := "sync"
	if mode == "background" {
		runMode = "background"
	}

	if sameInstance {
		return runSubagentTurn(ctx, src, composed, modelHint, runMode), nil
	}

	// Fork: new branched instance.
	if !concurrencySlotAvailable() {
		return tools.Result{IsError: true, Content: fmt.Sprintf("max concurrent subagents (%d) reached", maxConcurrentAgents)}, nil
	}
	branchID := GetAgentManager().NextID(firstNonEmpty(name, "fork"))
	inst := &AgentInstance{
		ID:        branchID,
		Name:      name + "-fork",
		Type:      typeName,
		Prompt:    composed,
		Status:    AgentStatusRunning,
		StartedAt: time.Now(),
		done:      make(chan struct{}),
	}
	GetAgentManager().Create(inst)
	return dispatchSubagent(ctx, inst, "", runMode)
}

func (t *ControlTool) interrupt(id string) tools.Result {
	if id == "" {
		return tools.Result{IsError: true, Content: "interrupt requires agent_id"}
	}
	inst, ok := GetAgentManager().Get(id)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("agent %q not found", id)}
	}
	inst.mu.Lock()
	alreadyTerminal := isTerminalAgentStatus(inst.Status)
	inst.mu.Unlock()
	if alreadyTerminal {
		return tools.Result{Content: fmt.Sprintf("agent %s already %s", id, inst.Status)}
	}
	if cancelAny, loaded := cancelRegistry.Load(id); loaded {
		if cancel, ok := cancelAny.(context.CancelFunc); ok {
			cancel()
		}
	}
	_ = GetAgentManager().UpdateStatus(id, AgentStatusCancelled, "interrupted by orchestrator", nil)
	inst.dismiss.Do(func() { close(inst.done) })
	return tools.Result{Content: fmt.Sprintf("interrupted %s", id), Summary: "interrupted"}
}

// runSubagentTurn appends a follow-up turn to an existing instance.
func runSubagentTurn(ctx context.Context, inst *AgentInstance, prompt, model, mode string) tools.Result {
	ctx, cancel := context.WithCancel(WithAgentID(ctx, inst.ID))
	cancelRegistry.Store(inst.ID, cancel)
	defer cancelRegistry.Delete(inst.ID)

	started := time.Now()
	result, err := executeAgentWithRolePreamble(ctx, inst.Type, prompt, model, "")
	duration := time.Since(started)
	if err != nil {
		_ = GetAgentManager().UpdateStatus(inst.ID, AgentStatusFailed, err.Error(), err)
		return tools.Result{IsError: true, Content: fmt.Sprintf("continue %s failed: %v", inst.ID, err)}
	}
	inst.mu.Lock()
	inst.Turns = append(inst.Turns, AgentTurn{Index: len(inst.Turns) + 1, Input: prompt, Output: result, Duration: duration})
	inst.mu.Unlock()
	_ = GetAgentManager().UpdateStatus(inst.ID, AgentStatusCompleted, result, nil)
	return tools.Result{Content: result, Metadata: map[string]any{"agent_id": inst.ID}}
}

// dispatchSubagent launches a new instance honoring sync/background semantics.
func dispatchSubagent(ctx context.Context, inst *AgentInstance, model, mode string) (tools.Result, error) {
	// A subagent spawned from inside another subagent's tool loop is that
	// agent's child: recording it here is what lets the dock indent a
	// coordinator's fan-out under the coordinator instead of listing every
	// agent in the run as an unrelated sibling.
	GetAgentManager().NoteSpawn(inst.ID, AgentIDFrom(ctx))

	ctx, cancel := context.WithCancel(WithAgentID(ctx, inst.ID))
	cancelRegistry.Store(inst.ID, cancel)

	finish := func(result string, err error) {
		defer func() {
			cancelRegistry.Delete(inst.ID)
			inst.dismiss.Do(func() { close(inst.done) })
		}()
		if err != nil {
			_ = GetAgentManager().UpdateStatus(inst.ID, AgentStatusFailed, err.Error(), err)
			return
		}
		_ = GetAgentManager().UpdateStatus(inst.ID, AgentStatusCompleted, result, nil)
	}

	if mode == "background" {
		go func() {
			result, err := executeAgentWithRolePreamble(ctx, inst.Type, inst.Prompt, model, rolePreambleFor(inst.Type))
			finish(result, err)
		}()
		return tools.Result{
			Content:  fmt.Sprintf("<subagent-started id=\"%s\" name=\"%s\"/>\nReport progress minimally; results arrive as tagged notifications.", inst.ID, inst.Name),
			Metadata: map[string]any{"agent_id": inst.ID, "mode": "background"},
		}, nil
	}

	result, err := executeAgentWithRolePreamble(ctx, inst.Type, inst.Prompt, model, rolePreambleFor(inst.Type))
	if err != nil {
		finish("", err)
		return tools.Result{IsError: true, Content: fmt.Sprintf("<subagent-failed id=\"%s\">%v</subagent-failed>", inst.ID, err)}, nil
	}
	finish(result, nil)
	return tools.Result{
		Content:  fmt.Sprintf("<subagent-result id=\"%s\" name=\"%s\">\n%s\n</subagent-result>", inst.ID, inst.Name, result),
		Metadata: map[string]any{"agent_id": inst.ID, "duration": inst.CompletedAt.Sub(inst.StartedAt).String()},
	}, nil
}

// rolePreambleFor prepends custom-agent role instructions when present.
func rolePreambleFor(t AgentType) string {
	def, ok := LookupCustomAgent(string(t))
	if !ok || def.SystemBody == "" {
		return ""
	}
	return "## Role instructions (from .agents/" + def.Name + ".md)\n" + def.SystemBody + "\n"
}

// executeAgentWithRolePreamble mirrors executeAgent but supports a preamble
// and honors per-definition model overrides for custom agents.
func executeAgentWithRolePreamble(ctx context.Context, t AgentType, prompt, model, preamble string) (string, error) {
	manager := GetAgentManager()
	manager.mu.RLock()
	exec := manager.executor
	manager.mu.RUnlock()

	if preamble != "" {
		prompt = preamble + "\n\n## Task\n" + prompt
	}
	// Priority: explicit model > custom agent model > registry model > empty
	if model == "" {
		if def, ok := LookupCustomAgent(string(t)); ok && def.Model != "" {
			model = def.Model
		} else if model == "" {
			model = lookupAgentModel(string(t))
		}
	}
	if exec == nil {
		return fmt.Sprintf("[Agent %s would execute: %s]", t, prompt), nil
	}
	return exec.Execute(ctx, t, prompt, model)
}

func concurrencySlotAvailable() bool {
	running := 0
	for _, a := range GetAgentManager().List(false) {
		a.mu.Lock()
		if a.Status == AgentStatusRunning {
			running++
		}
		a.mu.Unlock()
	}
	return running < maxConcurrentAgents
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// AgentSnapshot is a safe point-in-time view of an instance for UIs.
type AgentSnapshot struct {
	ID        string
	Name      string
	Type      string
	Status    string
	Turns     int
	Elapsed   string
	StartedAt time.Time

	// ParentID is empty for a top-level agent; the dock indents children.
	ParentID string
	// CurrentTool is what the agent is doing right now, "" between tools.
	CurrentTool string
	// ToolCount is how many tools it has run so far.
	ToolCount int
	// LastLine is the newest line of its own output.
	LastLine string
	// Idle is how long since it last did anything observable. Zero when it has
	// not started working yet or has finished.
	Idle time.Duration
}

// LastOutput returns the most recent turn output, falling back to Result.
func (a *AgentInstance) LastOutput() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.Turns) > 0 {
		return a.Turns[len(a.Turns)-1].Output
	}
	return a.Result
}

// Detail returns a locked copy of the fields the inspector shows: the task
// prompt, the full turn log, and the outcome. The UI must not take the
// instance's lock itself, so the read is offered as a method next to the other
// state accessors.
func (a *AgentInstance) Detail() (prompt string, turns []AgentTurn, result string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Prompt, append([]AgentTurn(nil), a.Turns...), a.Result, a.Error
}

// Snapshot returns a locked copy of the instance state.
func (a *AgentInstance) Snapshot() AgentSnapshot {
	lastLine := a.LastLine()
	lastActivity := a.LastActivity()

	a.mu.Lock()
	defer a.mu.Unlock()
	out := AgentSnapshot{
		ID:          a.ID,
		Name:        a.Name,
		Type:        string(a.Type),
		Status:      string(a.Status),
		Turns:       len(a.Turns),
		StartedAt:   a.StartedAt,
		ParentID:    a.ParentID,
		CurrentTool: a.CurrentTool,
		ToolCount:   a.ToolCount,
		LastLine:    lastLine,
	}
	if !a.StartedAt.IsZero() {
		end := a.CompletedAt
		if end.IsZero() {
			end = time.Now()
		}
		out.Elapsed = end.Sub(a.StartedAt).Round(time.Second).String()
	}
	if a.Status == AgentStatusRunning && !lastActivity.IsZero() {
		out.Idle = time.Since(lastActivity)
	}
	return out
}

// ControlAction runs one subagent operation programmatically (UI entry
// point): action is list|continue|fork|interrupt.
func ControlAction(action, agentID, prompt, mode string) string {
	switch action {
	case "interrupt":
		if inst, ok := GetAgentManager().Get(agentID); ok {
			inst.mu.Lock()
			terminal := isTerminalAgentStatus(inst.Status)
			inst.mu.Unlock()
			if terminal {
				return fmt.Sprintf("%s already %s", agentID, inst.Status)
			}
			if cAny, loaded := cancelRegistry.Load(agentID); loaded {
				if cancel, ok := cAny.(context.CancelFunc); ok {
					cancel()
				}
			}
			_ = GetAgentManager().UpdateStatus(agentID, AgentStatusCancelled, "interrupted", nil)
			inst.dismiss.Do(func() { close(inst.done) })
			return fmt.Sprintf("interrupted %s", agentID)
		}
		return fmt.Sprintf("agent %q not found", agentID)
	case "list":
		var sb strings.Builder
		for _, inst := range GetAgentManager().List(true) {
			snap := inst.Snapshot()
			fmt.Fprintf(&sb, "- %s [%s] %s\n", snap.ID, snap.Status, snap.Type)
		}
		return strings.TrimRight(sb.String(), "\n")
	default:
		return fmt.Sprintf("action %q requires the task tool path", action)
	}
}

// RegisterControlTool wires the control tool into a registry.
func RegisterControlTool(reg *tools.Registry) {
	if reg != nil {
		reg.Register(&ControlTool{})
		reg.Register(&BatchTaskTool{})
	}
}
