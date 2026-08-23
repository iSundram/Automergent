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

func (*ControlTool) Name() string        { return "agent_control" }
func (*ControlTool) Description() string { return "Manage running/completed subagents: continue their thread, fork one, interrupt it, or list them." }
func (*ControlTool) RequiresConfirmation(string) bool    { return false }
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
	ctx, cancel := context.WithCancel(ctx)
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
	ctx, cancel := context.WithCancel(ctx)
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
	if def, ok := LookupCustomAgent(string(t)); ok && model == "" && def.Model != "" {
		model = def.Model
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

// RegisterControlTool wires the control tool into a registry.
func RegisterControlTool(reg *tools.Registry) {
	if reg != nil {
		reg.Register(&ControlTool{})
	}
}
