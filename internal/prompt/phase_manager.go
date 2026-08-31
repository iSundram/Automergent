package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/shared"
)

// PhaseManager manages phase transitions and execution.
type PhaseManager struct {
	currentPhase   shared.AgentPhase
	phaseHistory   []shared.PhaseTransition
	taskQueue      *TaskQueue
	promptSystem   *PromptSystem
	agent          *agentdef.AgentDefinition
	violationCount map[shared.ViolationType]int
}

// TaskQueue holds pending tasks organized by phase.
type TaskQueue struct {
	tasks map[shared.AgentPhase][]shared.TaskSpec
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		tasks: make(map[shared.AgentPhase][]shared.TaskSpec),
	}
}

func (q *TaskQueue) Enqueue(phase shared.AgentPhase, task shared.TaskSpec) {
	q.tasks[phase] = append(q.tasks[phase], task)
}

func (q *TaskQueue) Dequeue(phase shared.AgentPhase) *shared.TaskSpec {
	if tasks, ok := q.tasks[phase]; ok && len(tasks) > 0 {
		task := tasks[0]
		q.tasks[phase] = tasks[1:]
		return &task
	}
	return nil
}

func (q *TaskQueue) HasTasks(phase shared.AgentPhase) bool {
	tasks, ok := q.tasks[phase]
	return ok && len(tasks) > 0
}

func (q *TaskQueue) AllTasks() []shared.TaskSpec {
	var all []shared.TaskSpec
	for _, tasks := range q.tasks {
		all = append(all, tasks...)
	}
	return all
}

// NewPhaseManager creates a new phase manager.
func NewPhaseManager(promptSystem *PromptSystem, agent *agentdef.AgentDefinition) *PhaseManager {
	return &PhaseManager{
		currentPhase:   shared.PhaseInit,
		phaseHistory:   []shared.PhaseTransition{},
		taskQueue:      NewTaskQueue(),
		promptSystem:   promptSystem,
		agent:          agent,
		violationCount: make(map[shared.ViolationType]int),
	}
}

// CurrentPhase returns the current phase.
func (m *PhaseManager) CurrentPhase() shared.AgentPhase {
	return m.currentPhase
}

// PhaseHistory returns the phase transition history.
func (m *PhaseManager) PhaseHistory() []shared.PhaseTransition {
	return m.phaseHistory
}

// CanTransition checks if a phase transition is valid.
func (m *PhaseManager) CanTransition(from, to shared.AgentPhase) bool {
	validTransitions := map[shared.AgentPhase][]shared.AgentPhase{
		shared.PhaseInit:    {shared.PhaseInit, shared.PhaseExplore, shared.PhasePlan, shared.PhaseBuild},
		shared.PhaseExplore: {shared.PhasePlan, shared.PhaseBuild, shared.PhaseInit},
		shared.PhasePlan:    {shared.PhaseBuild, shared.PhaseExplore, shared.PhaseInit},
		shared.PhaseBuild:   {shared.PhaseExplore, shared.PhasePlan, shared.PhaseInit},
	}
	
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// Transition changes the current phase.
func (m *PhaseManager) Transition(to shared.AgentPhase, reason, trigger string) error {
	if !m.CanTransition(m.currentPhase, to) {
		return fmt.Errorf("invalid phase transition: %s -> %s", m.currentPhase, to)
	}
	
	m.phaseHistory = append(m.phaseHistory, shared.PhaseTransition{
		From:      m.currentPhase,
		To:        to,
		Reason:    reason,
		Trigger:   trigger,
		Timestamp: time.Now(),
	})
	m.currentPhase = to
	return nil
}

// DefaultPhaseTools is the SINGLE source of truth for the tools each phase
// offers: the PhaseManager's tool masks, the composer's tool-guidance layer,
// and any other per-phase listing all read from here. Tool names MUST match
// the registry names (read_file/edit_file/write_file).
func DefaultPhaseTools(phase shared.AgentPhase) ([]string, bool) {
	tools := map[shared.AgentPhase][]string{
		shared.PhaseInit:    {"bash", "read_file", "glob", "grep", "edit_file", "task"},
		shared.PhaseExplore: {"glob", "grep", "read_file", "bash", "list_directory"},
		shared.PhasePlan:    {"read_file", "write_file", "bash", "task", "grep", "glob"},
		shared.PhaseBuild:   {"edit_file", "bash", "write_file", "read_file", "task", "glob", "grep", "multi_edit"},
	}
	t, ok := tools[phase]
	return t, ok
}

// GetPhaseConfig returns the configuration for a phase.
func (m *PhaseManager) GetPhaseConfig(phase shared.AgentPhase) shared.PhaseConfig {
	// Check agent-specific phase config first
	if m.agent != nil {
		if tools, ok := m.agent.PhaseTools[phase]; ok {
			return shared.PhaseConfig{
				Tools:       tools,
				ToolSet:     m.toolSetForPhase(phase),
				PromptStyle: m.promptStyleForPhase(phase),
				Agent:       m.agentForPhase(phase),
				MaxSteps:    m.maxStepsForPhase(phase),
			}
		}
	}

	if tools, ok := DefaultPhaseTools(phase); ok {
		return shared.PhaseConfig{
			Tools:       tools,
			ToolSet:     m.toolSetForPhase(phase),
			PromptStyle: m.promptStyleForPhase(phase),
			Agent:       m.agentForPhase(phase),
			MaxSteps:    m.maxStepsForPhase(phase),
		}
	}

	return shared.PhaseConfig{
		Tools:       []string{"bash", "read_file", "glob", "grep", "edit_file", "task"},
		ToolSet:     shared.ToolSetBasic,
		PromptStyle: m.promptStyleForPhase(shared.PhaseInit),
		Agent:       "main",
		MaxSteps:    3,
	}
}

// ExecutePhase executes a specific phase with the given task.
func (m *PhaseManager) ExecutePhase(phase shared.AgentPhase, taskSpec shared.TaskSpec) PhaseResult {
	if err := m.Transition(phase, "executing phase", "phase_execution"); err != nil {
		return PhaseResult{Error: err}
	}
	
	config := m.GetPhaseConfig(phase)
	
	result := PhaseResult{
		Phase:       phase,
		TaskSpec:    taskSpec,
		Config:      config,
		Tools:       config.Tools,
		ToolSet:     config.ToolSet,
		MaxSteps:    config.MaxSteps,
		PromptStyle: config.PromptStyle,
	}
	
	// Execute phase-specific logic
	switch phase {
	case shared.PhaseInit:
		result = m.executeInitPhase(taskSpec)
	case shared.PhaseExplore:
		result = m.executeExplorePhase(taskSpec)
	case shared.PhasePlan:
		result = m.executePlanPhase(taskSpec)
	case shared.PhaseBuild:
		result = m.executeBuildPhase(taskSpec)
	}
	
	return result
}

// PhaseResult contains the result of phase execution.
type PhaseResult struct {
	Phase          shared.AgentPhase
	TaskSpec       shared.TaskSpec
	Config         shared.PhaseConfig
	Tools          []string
	ToolSet        shared.ToolSet
	MaxSteps       int
	PromptStyle    string
	NextPhase      shared.AgentPhase
	NeedsPhaseChange bool
	Violation      *shared.ViolationCheck
	Error          error
	Output         string
}

func (m *PhaseManager) executeInitPhase(task shared.TaskSpec) PhaseResult {
	// Init phase: classify intent, detect violations, route to appropriate phase
	// This is handled by PhaseClassifier
	return PhaseResult{
		Phase:          shared.PhaseInit,
		NextPhase:      shared.PhaseExplore, // Default, will be overridden by classifier
		NeedsPhaseChange: true,
	}
}

func (m *PhaseManager) executeExplorePhase(task shared.TaskSpec) PhaseResult {
	return PhaseResult{
		Phase:          shared.PhaseExplore,
		NextPhase:      shared.PhasePlan,
		NeedsPhaseChange: true,
	}
}

func (m *PhaseManager) executePlanPhase(task shared.TaskSpec) PhaseResult {
	return PhaseResult{
		Phase:          shared.PhasePlan,
		NextPhase:      shared.PhaseBuild,
		NeedsPhaseChange: true,
	}
}

func (m *PhaseManager) executeBuildPhase(task shared.TaskSpec) PhaseResult {
	return PhaseResult{
		Phase:          shared.PhaseBuild,
		NextPhase:      shared.PhaseInit, // Build includes testing, back to init for next task
		NeedsPhaseChange: true,
	}
}

// toolSetForPhase returns the ToolSet for a phase.
func (m *PhaseManager) toolSetForPhase(phase shared.AgentPhase) shared.ToolSet {
	switch phase {
	case shared.PhaseInit:
		return shared.ToolSetBasic
	case shared.PhaseExplore:
		return shared.ToolSetReadOnly
	case shared.PhasePlan:
		return shared.ToolSetModerate
	case shared.PhaseBuild:
		return shared.ToolSetFull
	default:
		return shared.ToolSetBasic
	}
}

// promptStyleForPhase returns the prompt style for a phase.
func (m *PhaseManager) promptStyleForPhase(phase shared.AgentPhase) string {
	switch phase {
	case shared.PhaseInit:
		return "serious, concise, classifier"
	case shared.PhaseExplore:
		return "thorough, investigative"
	case shared.PhasePlan:
		return "structured, analytical"
	case shared.PhaseBuild:
		return "focused, pragmatic + testing + todo"
	default:
		return "serious, concise"
	}
}

// agentForPhase returns the agent type for a phase.
func (m *PhaseManager) agentForPhase(phase shared.AgentPhase) string {
	switch phase {
	case shared.PhaseInit:
		return "main"
	case shared.PhaseExplore:
		return "explore"
	case shared.PhasePlan, shared.PhaseBuild:
		return "general"
	default:
		return "main"
	}
}

// maxStepsForPhase returns max steps for a phase.
func (m *PhaseManager) maxStepsForPhase(phase shared.AgentPhase) int {
	if m.agent != nil && m.agent.PhaseMaxSteps != nil {
		if steps, ok := m.agent.PhaseMaxSteps[phase]; ok {
			return steps
		}
	}
	
	defaultSteps := map[shared.AgentPhase]int{
		shared.PhaseInit:     3,
		shared.PhaseExplore:  10,
		shared.PhasePlan:     5,
		shared.PhaseBuild:    20,
	}
	
	if steps, ok := defaultSteps[phase]; ok {
		return steps
	}
	return 5
}

// RecordViolation records a violation and returns the action to take.
func (m *PhaseManager) RecordViolation(vType shared.ViolationType, severity shared.ViolationSeverity, userMsg, agentResp string) shared.ViolationCheck {
	m.violationCount[vType]++
	count := m.violationCount[vType]
	
	var action string
	policy := shared.ViolationPolicy{MaxWarnings: 2, BlockOnPersist: true, AllowOverride: true}
	if m.agent != nil && m.agent.ViolationPolicy.MaxWarnings > 0 {
		policy = m.agent.ViolationPolicy
	}
	
	switch {
	case count == 1:
		action = "warn"
	case count == 2:
		action = "block_imminent"
	case count >= 3:
		if policy.BlockOnPersist {
			action = "blocked"
		} else {
			action = "block_imminent"
		}
	}
	
	return shared.ViolationCheck{
		Type:            vType,
		Severity:        severity,
		UserMessage:     userMsg,
		AgentResponse:   agentResp,
		Count:           count,
		Action:          action,
	}
}

// ResetViolationCount resets the violation count for a type.
func (m *PhaseManager) ResetViolationCount(vType shared.ViolationType) {
	m.violationCount[vType] = 0
}

// OverrideViolation marks a violation as overridden by user justification.
func (m *PhaseManager) OverrideViolation(vType shared.ViolationType) {
	m.violationCount[vType] = 0
}

// GetViolationCount returns the violation count for a type.
func (m *PhaseManager) GetViolationCount(vType shared.ViolationType) int {
	return m.violationCount[vType]
}

// ClassifyAndRoute classifies the first message and determines the initial phase.
func (m *PhaseManager) ClassifyAndRoute(userMessage string) (shared.AgentPhase, []shared.TaskSpec, []string, *shared.ViolationCheck) {
	// Check for violations first
	if violation := m.checkViolation(userMessage); violation != nil {
		return shared.PhaseInit, nil, nil, violation
	}
	
	// Check for clarification needs
	if questions := m.checkClarification(userMessage); len(questions) > 0 {
		return shared.PhaseInit, nil, questions, nil
	}
	
	// Classify intent
	phase, tasks := m.classifyIntent(userMessage)
	
	return phase, tasks, nil, nil
}

// checkViolation is deliberately RETIRED — substring matching flagged
// ordinary engineering work ("rotate the api key", "fix the exploit in our
// sanitizer") far more often than real violations. Detection belongs to the
// INIT decomposer's LLM judgment with its confirmation step; the escalation
// ladder here only fires on verdicts that reach it.
func (m *PhaseManager) checkViolation(message string) *shared.ViolationCheck {
	return nil
}

func (m *PhaseManager) checkClarification(message string) []string {
	// Check for ambiguous requests that could mean multiple things
	lower := strings.ToLower(message)
	var questions []string

	// Multiple intents detected
	intentKeywords := map[string][]string{
		"explore": {"find", "search", "look for", "where is", "how does", "explore"},
		"implement": {"implement", "create", "build", "make", "add", "write"},
		"fix": {"fix", "bug", "error", "issue", "broken", "repair"},
		"plan": {"plan", "design", "architecture", "approach", "strategy"},
		"question": {"what", "how", "why", "explain", "tell me"},
	}

	detectedIntents := []string{}
	for intent, keywords := range intentKeywords {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				detectedIntents = append(detectedIntents, intent)
				break
			}
		}
	}

	if len(detectedIntents) > 1 {
		questions = append(questions, fmt.Sprintf("Your request could mean: %s. Which do you want?", strings.Join(detectedIntents, ", ")))
	}

	return questions
}

func (m *PhaseManager) classifyIntent(message string) (shared.AgentPhase, []shared.TaskSpec) {
	lower := strings.ToLower(message)
	
	// First, check for coding task keywords - these take priority over direct questions
	codeKeywords := []string{
		"implement", "create", "build", "fix", "refactor", "debug", 
		"write", "code", "function", "class", "api", "feature",
		"bug", "error", "issue", "refactor", "optimize", "add",
		"modify", "change", "update", "delete", "remove", "test",
		"deploy", "build", "compile", "run", "execute",
	}
	isCodingTask := false
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			isCodingTask = true
			break
		}
	}
	
	// Direct questions - only if NOT a coding task
	if !isCodingTask {
		directPatterns := []string{
			"what does", "what is", "who are you", "hello", "hi", "hey",
			"explain", "describe", "tell me about", "how to",
		}
		for _, p := range directPatterns {
			if strings.Contains(lower, p) {
				return shared.PhaseInit, []shared.TaskSpec{{
					Type:        "direct",
					Description: "Answer user question directly",
					Role:        "assistant",
					Priority:    1,
				}}
			}
		}
	}
	
	// Explore patterns - explicit exploration requests
	explorePatterns := []string{
		"find files", "search for", "grep", "glob", "where is", "locate",
		"see files", "look at", "explore", "understand",
	}
	for _, p := range explorePatterns {
		if strings.Contains(lower, p) {
			return shared.PhaseExplore, []shared.TaskSpec{{
				Type:        "explore",
				Description: "Explore codebase for relevant files",
				Role:        "explore",
				Priority:    1,
			}}
		}
	}
	
	// Plan patterns - explicit planning requests
	planPatterns := []string{
		"plan", "design", "architecture", "approach", "strategy",
		"comprehensive plan", "upgrade plan", "refactor plan",
	}
	for _, p := range planPatterns {
		if strings.Contains(lower, p) {
			return shared.PhasePlan, []shared.TaskSpec{{
				Type:        "plan",
				Description: "Create implementation plan",
				Role:        "planner",
				Priority:    1,
			}}
		}
	}
	
	// Build patterns - explicit build/implementation requests
	buildPatterns := []string{
		"implement", "create", "build", "make", "add feature", "write code",
		"fix", "bug", "error", "issue", "refactor",
	}
	for _, p := range buildPatterns {
		if strings.Contains(lower, p) {
			return shared.PhaseExplore, []shared.TaskSpec{{
				Type:        "explore",
				Description: "Explore codebase to understand the task",
				Role:        "explore",
				Priority:    1,
			}}
		}
	}
	
	// Default to init for clarification
	return shared.PhaseInit, []shared.TaskSpec{{
		Type:        "clarify",
		Description: "Clarify user intent",
		Role:        "assistant",
		Priority:    1,
	}}
}