package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

type mockLLMProvider struct {
	ai.Provider
}

func (m *mockLLMProvider) Name() string { return "test-provider" }
func (m *mockLLMProvider) Complete(ctx context.Context, req ai.CompletionRequest) (ai.CompletionResponse, error) {
	return ai.NewStaticResponse("ok", "", nil, ai.StopReasonEnd, ai.Usage{}), nil
}
func (m *mockLLMProvider) Models(ctx context.Context) ([]ai.Model, error) { return nil, nil }
func (m *mockLLMProvider) TokenCount(messages []ai.Message) (int, error) { return 0, nil }
func (m *mockLLMProvider) ContextLimit() int { return 128000 }

type scopeTestTool struct {
	readOnly    bool
	destructive bool
	risk        string
}

func (t scopeTestTool) Name() string           { return "scope_tool" }
func (t scopeTestTool) Description() string    { return "scope test tool" }
func (t scopeTestTool) Schema() map[string]any { return map[string]any{} }
func (t scopeTestTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (t scopeTestTool) RequiresConfirmation(string) bool      { return true }
func (t scopeTestTool) IsConcurrencySafe(map[string]any) bool { return true }
func (t scopeTestTool) IsReadOnly(map[string]any) bool        { return t.readOnly }
func (t scopeTestTool) IsDestructive(map[string]any) bool     { return t.destructive }
func (t scopeTestTool) EstimatedCost() tools.ToolCost         { return tools.ToolCost{RiskLevel: t.risk} }

func TestToolApprovalScopeDiffersByActionAndRisk(t *testing.T) {
	tc := ai.ToolCall{Name: "scope_tool", Args: map[string]any{}}
	readKey := toolApprovalScope(tc, scopeTestTool{readOnly: true, risk: "low"})
	writeKey := toolApprovalScope(tc, scopeTestTool{readOnly: false, risk: "low"})
	destructiveKey := toolApprovalScope(tc, scopeTestTool{readOnly: false, destructive: true, risk: "low"})
	unknownRiskKey := toolApprovalScope(tc, scopeTestTool{readOnly: false})

	if readKey == writeKey {
		t.Fatalf("expected different keys for read and write scopes")
	}
	if destructiveKey == writeKey {
		t.Fatalf("expected different keys for destructive and non-destructive scopes")
	}
	if unknownRiskKey == writeKey {
		t.Fatalf("expected different keys for unknown and explicit risk scopes")
	}
}

type persistenceScopeTool struct {
	mu   sync.RWMutex
	risk string
}

func (t *persistenceScopeTool) Name() string                          { return "persist_scope_tool" }
func (t *persistenceScopeTool) Description() string                   { return "persistence scope tool" }
func (t *persistenceScopeTool) Schema() map[string]any                { return map[string]any{} }
func (t *persistenceScopeTool) RequiresConfirmation(string) bool      { return true }
func (t *persistenceScopeTool) IsConcurrencySafe(map[string]any) bool { return true }
func (t *persistenceScopeTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	return tools.Result{Content: "ok"}, nil
}
func (t *persistenceScopeTool) IsReadOnly(args map[string]any) bool {
	return args["operation"] == "read"
}
func (t *persistenceScopeTool) IsDestructive(args map[string]any) bool {
	return args["operation"] == "destructive"
}
func (t *persistenceScopeTool) EstimatedCost() tools.ToolCost {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return tools.ToolCost{RiskLevel: t.risk}
}
func (t *persistenceScopeTool) setRisk(risk string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.risk = risk
}

func TestExecuteToolAlwaysApprovalScopedByOperationAndRisk(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	ag := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		tools:               reg,
		events:              make(chan Event, 8),
		sessionAllowedTools: map[string]bool{},
	}

	var confirms atomic.Int32
	go func() {
		for event := range ag.Events() {
			if event.Type != EventConfirm {
				continue
			}
			confirms.Add(1)
			payload := event.Payload.(map[string]any)
			reply := payload["reply"].(chan ConfirmationResponse)
			reply <- ConfirmationResponse{Allow: true, Always: true}
		}
	}()
	defer func() { _ = ag.Close() }()

	calls := []ai.ToolCall{
		{Name: tool.Name(), Args: map[string]any{"operation": "read"}},        // confirm + persist
		{Name: tool.Name(), Args: map[string]any{"operation": "read"}},        // reuse scoped approval
		{Name: tool.Name(), Args: map[string]any{"operation": "write"}},       // isolated from read
		{Name: tool.Name(), Args: map[string]any{"operation": "destructive"}}, // isolated from write
	}
	for _, call := range calls {
		res, err := ag.executeTool(context.Background(), call)
		if err != nil {
			t.Fatalf("executeTool failed: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected tool error: %s", res.Content)
		}
	}
	if got := confirms.Load(); got != 3 {
		t.Fatalf("expected 3 confirmations for read/write/destructive isolation, got %d", got)
	}

	tool.setRisk("high")
	res, err := ag.executeTool(context.Background(), ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}})
	if err != nil {
		t.Fatalf("executeTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if got := confirms.Load(); got != 4 {
		t.Fatalf("expected risk change to trigger a new confirmation, got %d confirmations", got)
	}
}

func TestExecuteToolAcceptsLegacyApprovalScope(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	tc := ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}}
	ag := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		tools:               reg,
		events:              make(chan Event, 4),
		sessionAllowedTools: map[string]bool{legacyToolApprovalScope(tc, tool): true},
	}

	var confirms atomic.Int32
	go func() {
		for event := range ag.Events() {
			if event.Type == EventConfirm {
				confirms.Add(1)
			}
		}
	}()
	defer func() { _ = ag.Close() }()

	res, err := ag.executeTool(context.Background(), tc)
	if err != nil {
		t.Fatalf("executeTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if got := confirms.Load(); got != 0 {
		t.Fatalf("expected legacy scope approval to avoid reconfirmation, got %d confirmations", got)
	}
}

func TestExecuteToolAlwaysPersistsApprovalToSession(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	sess := session.New()
	ag := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		sess:                sess,
		tools:               reg,
		events:              make(chan Event, 8),
		sessionAllowedTools: map[string]bool{},
		approvalSource:      "test",
	}

	go func() {
		for event := range ag.Events() {
			if event.Type != EventConfirm {
				continue
			}
			payload := event.Payload.(map[string]any)
			reply := payload["reply"].(chan ConfirmationResponse)
			reply <- ConfirmationResponse{Allow: true, Always: true}
		}
	}()
	defer func() { _ = ag.Close() }()

	tc := ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}}
	res, err := ag.executeTool(context.Background(), tc)
	if err != nil {
		t.Fatalf("executeTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}

	scope := toolApprovalScope(tc, tool)
	if !sess.HasApproval(scope) {
		t.Fatalf("expected approval %q to be recorded in session, got %+v", scope, sess.AllowedTools)
	}
	if len(sess.AllowedTools) != 1 || sess.AllowedTools[0].Source != "test" {
		t.Fatalf("unexpected approval record: %+v", sess.AllowedTools)
	}
}

func TestNewSeedsApprovalsFromSession(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	tc := ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}}

	provider := &mockLLMProvider{}
	ag := New(&config.Config{Mode: "plan"}, provider, nil, reg)
	// Seed the session with the project-scoped key that New() will look up.
	sess := session.New()
	sess.AddApproval(ag.scopedToolApprovalKey(tc, tool), "tui")
	ag.SetSession(sess)

	var confirms atomic.Int32
	go func() {
		for event := range ag.Events() {
			if event.Type == EventConfirm {
				confirms.Add(1)
			}
		}
	}()
	defer func() { _ = ag.Close() }()

	res, err := ag.executeTool(context.Background(), tc)
	if err != nil {
		t.Fatalf("executeTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if got := confirms.Load(); got != 0 {
		t.Fatalf("expected seeded approval to skip confirmation, got %d confirmations", got)
	}
}

func TestSetSessionReseedsApprovals(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	tc := ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}}
	scope := toolApprovalScope(tc, tool)

	ag := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		tools:               reg,
		events:              make(chan Event, 8),
		sessionAllowedTools: map[string]bool{},
	}

	resumed := session.New()
	resumed.AddApproval(scope, "tui")
	ag.SetSession(resumed)

	var confirms atomic.Int32
	go func() {
		for event := range ag.Events() {
			if event.Type == EventConfirm {
				confirms.Add(1)
			}
		}
	}()
	defer func() { _ = ag.Close() }()

	res, err := ag.executeTool(context.Background(), tc)
	if err != nil {
		t.Fatalf("executeTool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if got := confirms.Load(); got != 0 {
		t.Fatalf("expected resumed approvals to skip confirmation, got %d confirmations", got)
	}
}

func TestProjectScopedApprovalDoesNotLeakAcrossProjects(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &persistenceScopeTool{risk: "low"}
	reg.Register(tool)

	tc := ai.ToolCall{Name: tool.Name(), Args: map[string]any{"operation": "write"}}

	ag := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		tools:               reg,
		events:              make(chan Event, 8),
		sessionAllowedTools: map[string]bool{},
		approvalSource:      "test",
		workDir:             "/projects/alpha",
	}

	var confirms atomic.Int32
	go func() {
		for event := range ag.Events() {
			if event.Type != EventConfirm {
				continue
			}
			confirms.Add(1)
			payload := event.Payload.(map[string]any)
			reply := payload["reply"].(chan ConfirmationResponse)
			reply <- ConfirmationResponse{Allow: true, Always: true}
		}
	}()
	defer func() { _ = ag.Close() }()

	res, err := ag.executeTool(context.Background(), tc)
	if err != nil || res.IsError {
		t.Fatalf("executeTool failed: %v %s", err, res.Content)
	}
	if got := confirms.Load(); got != 1 {
		t.Fatalf("expected 1 confirmation for first approval, got %d", got)
	}

	// Same scope key but a different project must re-confirm.
	scoped := ag.scopedToolApprovalKey(tc, tool)
	if !strings.HasPrefix(scoped, "project=/projects/alpha;") {
		t.Fatalf("expected project-prefixed scope key, got %q", scoped)
	}

	other := &Agent{
		cfg:                 &config.Config{Mode: "plan"},
		tools:               reg,
		events:              make(chan Event, 8),
		sessionAllowedTools: map[string]bool{},
		approvalSource:      "test",
		workDir:             "/projects/beta",
	}
	go func() {
		for event := range other.Events() {
			if event.Type != EventConfirm {
				continue
			}
			confirms.Add(1)
			payload := event.Payload.(map[string]any)
			reply := payload["reply"].(chan ConfirmationResponse)
			reply <- ConfirmationResponse{Allow: true, Always: true}
		}
	}()
	defer func() { _ = other.Close() }()

	res, err = other.executeTool(context.Background(), tc)
	if err != nil || res.IsError {
		t.Fatalf("other project executeTool failed: %v %s", err, res.Content)
	}
	if got := confirms.Load(); got != 2 {
		t.Fatalf("expected a second confirmation in another project, got %d", got)
	}
}

func TestPruneFirstMessageTriageUsesStoredPrompt(t *testing.T) {
	sess := session.New()
	sess.AddMessage(ai.Message{
		Role: ai.RoleUser,
		Content: []ai.ContentPart{
			{Type: ai.ContentTypeText, Text: TriageInstruction + "\n\nUser Request: hello User Request: world"},
		},
		Metadata: map[string]any{
			"triage_injected":      true,
			"original_user_prompt": "hello User Request: world",
		},
	})

	ag := &Agent{sess: sess}
	ag.pruneFirstMessageTriage()

	got := sess.Messages[0].TextContent()
	if got != "hello User Request: world" {
		t.Fatalf("expected original prompt to be restored, got %q", got)
	}
	if _, ok := sess.Messages[0].Metadata["triage_injected"]; ok {
		t.Fatalf("expected triage metadata to be removed")
	}
	if _, ok := sess.Messages[0].Metadata["original_user_prompt"]; ok {
		t.Fatalf("expected original prompt metadata to be removed")
	}
}

func TestBuildToolResultMessageIncludesEveryRequestedCall(t *testing.T) {
	calls := []ai.ToolCall{
		{ID: "call_613309", Name: "read_file"},
		{ID: "call_missing", Name: "search"},
	}
	executed := []executedToolCall{{
		call:   calls[0],
		result: tools.Result{Content: "file contents"},
	}}

	message := buildToolResultMessage(calls, executed)
	sequence := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "inspect"),
		{Role: ai.RoleAssistant, Content: []ai.ContentPart{
			{Type: ai.ContentTypeToolCall, ToolCall: &calls[0]},
			{Type: ai.ContentTypeToolCall, ToolCall: &calls[1]},
		}},
		message,
	}
	if err := ai.ValidateMessageSequence(sequence); err != nil {
		t.Fatalf("tool result message does not complete sequence: %v", err)
	}
	if len(message.Content) != 2 {
		t.Fatalf("result count = %d, want 2", len(message.Content))
	}
	if !message.Content[1].ToolResult.IsError {
		t.Fatal("missing execution did not receive an interrupted error result")
	}
}
