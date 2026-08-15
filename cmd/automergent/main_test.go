package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/iSundram/Automergent/internal/agent"
	aiPkg "github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	automergentErrors "github.com/iSundram/Automergent/internal/errors"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

func TestRememberedProjectDoesNotRequireApproval(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("security:\n  allowedWritePaths:\n    - %s\n", projectDir)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	viper.Reset()
	defer viper.Reset()
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	if err := decodeConfigFromViper(cfg); err != nil {
		t.Fatal(err)
	}
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldDir) }()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	if path, required := projectApprovalRequired(cfg); required {
		t.Fatalf("remembered project unexpectedly requires approval: %s; loaded paths: %v", path, cfg.Security.AllowedWritePaths)
	}
}

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected outputFormat
	}{
		{name: "text", input: "text", expected: outputFormatText},
		{name: "json", input: "json", expected: outputFormatJSON},
		{name: "stream-json", input: "stream-json", expected: outputFormatStreamJSON},
		{name: "trim and case", input: " JSON ", expected: outputFormatJSON},
		{name: "invalid falls back", input: "xml", expected: outputFormatText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOutputFormat(tt.input)
			if got != tt.expected {
				t.Fatalf("parseOutputFormat(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRunHeadlessWithRunnersDispatch(t *testing.T) {
	sentinel := errors.New("sentinel")
	type counters struct{ text, json, stream int }
	calls := counters{}
	runners := headlessRunners{
		text: func(context.Context, *agent.Agent, *session.Session, string) error {
			calls.text++
			return sentinel
		},
		json: func(context.Context, *agent.Agent, *session.Session, string) error {
			calls.json++
			return sentinel
		},
		streamJSON: func(context.Context, *agent.Agent, *session.Session, string) error {
			calls.stream++
			return sentinel
		},
	}

	cases := []struct {
		name       string
		format     outputFormat
		wantText   int
		wantJSON   int
		wantStream int
	}{
		{name: "text route", format: outputFormatText, wantText: 1},
		{name: "json route", format: outputFormatJSON, wantJSON: 1},
		{name: "stream route", format: outputFormatStreamJSON, wantStream: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls = counters{}
			err := runHeadlessWithRunners(context.Background(), nil, nil, "prompt", tc.format, runners)
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected sentinel error, got %v", err)
			}
			if calls.text != tc.wantText || calls.json != tc.wantJSON || calls.stream != tc.wantStream {
				t.Fatalf("dispatch mismatch: got %+v want text=%d json=%d stream=%d", calls, tc.wantText, tc.wantJSON, tc.wantStream)
			}
		})
	}
}

func TestRunHeadlessWithRunnersRequiresPrompt(t *testing.T) {
	called := false
	runners := headlessRunners{
		text: func(context.Context, *agent.Agent, *session.Session, string) error {
			return nil
		},
		json: func(context.Context, *agent.Agent, *session.Session, string) error {
			called = true
			return nil
		},
		streamJSON: func(context.Context, *agent.Agent, *session.Session, string) error {
			return nil
		},
	}

	err := runHeadlessWithRunners(context.Background(), nil, nil, "", outputFormatJSON, runners)
	if err != nil {
		t.Fatalf("expected no dispatch-level error, got %v", err)
	}
	if !called {
		t.Fatal("expected json runner to be called")
	}
}

type testProvider struct {
	resp aiPkg.CompletionResponse
	err  error
}

func (p *testProvider) Name() string { return "test" }

func (p *testProvider) Complete(context.Context, aiPkg.CompletionRequest) (aiPkg.CompletionResponse, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.resp, nil
}

func (p *testProvider) Models(context.Context) ([]aiPkg.Model, error) { return nil, nil }
func (p *testProvider) TokenCount([]aiPkg.Message) (int, error)       { return 0, nil }
func (p *testProvider) ContextLimit() int                             { return 128000 }

func TestRunHeadlessJSON_ValidSuccessEnvelope(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = "plan"
	sess := session.New()
	ag := agent.New(cfg, &testProvider{
		resp: aiPkg.NewStaticResponse("ok", "", nil, aiPkg.StopReasonEnd, aiPkg.Usage{InputTokens: 3, OutputTokens: 4}),
	}, sess, tools.NewRegistry())

	var out bytes.Buffer
	if err := runHeadlessJSONToWriter(context.Background(), ag, sess, "say hi", &out); err != nil {
		t.Fatalf("runHeadlessJSONToWriter returned error: %v", err)
	}

	var payload jsonHeadlessOutput
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	if payload.Version != "v1" || !payload.Success || payload.Error != nil {
		t.Fatalf("unexpected envelope: %+v", payload)
	}
	if payload.Usage.InputTokens != 3 || payload.Usage.OutputTokens != 4 {
		t.Fatalf("unexpected usage: %+v", payload.Usage)
	}
	if payload.Session.ID != sess.ID {
		t.Fatalf("unexpected session id: %q", payload.Session.ID)
	}
}

func TestRunHeadlessJSON_ValidFailureEnvelope(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = "plan"
	sess := session.New()
	ag := agent.New(cfg, &testProvider{
		err: errors.New("authentication failed"),
	}, sess, tools.NewRegistry())

	var out bytes.Buffer
	err := runHeadlessJSONToWriter(context.Background(), ag, sess, "fail please", &out)
	if err == nil {
		t.Fatal("expected run error")
	}

	var payload jsonHeadlessOutput
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out.String())
	}
	if payload.Success {
		t.Fatalf("expected failure payload: %+v", payload)
	}
	if payload.Error == nil || payload.Error.Code != "auth_failed" {
		t.Fatalf("expected explicit auth_failed error payload, got %+v", payload.Error)
	}
	if payload.Error.Category != "auth" {
		t.Fatalf("expected auth category, got %+v", payload.Error)
	}
	if payload.Error.Details == nil {
		t.Fatalf("expected details in error payload, got %+v", payload.Error)
	}
}

func TestStructuredErrorMachineFieldsInTextMode(t *testing.T) {
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pipeR.Close()

	oldStderr := os.Stderr
	os.Stderr = pipeW
	defer func() { os.Stderr = oldStderr }()

	printTextStructuredError(structuredErrorFromPayload(errors.New("authentication failed"), nil))
	_ = pipeW.Close()
	out, err := io.ReadAll(pipeR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	s := string(out)
	for _, want := range []string{"code=auth_failed", "category=auth", "message=", "details="} {
		if !strings.Contains(s, want) {
			t.Fatalf("text error missing %q: %q", want, s)
		}
	}
}

func TestMapStructuredEventErrorNeverSilent(t *testing.T) {
	ev := mapStructuredEvent(agent.Event{Type: agent.EventError, Payload: 123})
	if ev.Error == nil {
		t.Fatal("expected structured error for non-error payload")
	}
	if ev.Error.Code == "" || ev.Error.Category == "" || ev.Error.Message == "" {
		t.Fatalf("missing machine fields: %+v", ev.Error)
	}
}

func TestErrorModelConsistentAcrossModes(t *testing.T) {
	runErr := errors.New("authentication failed")
	expected := structuredErrorFromPayload(runErr, nil)

	cfg := config.Default()
	cfg.Mode = "plan"
	sess := session.New()
	ag := agent.New(cfg, &testProvider{err: runErr}, sess, tools.NewRegistry())

	var out bytes.Buffer
	_ = runHeadlessJSONToWriter(context.Background(), ag, sess, "fail please", &out)

	var payload jsonHeadlessOutput
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("expected json error payload")
	}

	streamEvent := mapStructuredEvent(agent.Event{Type: agent.EventError, Payload: runErr})
	if streamEvent.Error == nil {
		t.Fatal("expected stream event error payload")
	}

	for name, got := range map[string]*structuredError{
		"text":   ptrStructuredError(expected),
		"json":   payload.Error,
		"stream": streamEvent.Error,
	} {
		if got.Code != expected.Code || got.Category != expected.Category || got.Message != expected.Message || got.Details == nil {
			t.Fatalf("%s mismatch: got %+v expected %+v", name, got, expected)
		}
	}
}

func TestMapStructuredEvent_StatusLongTaskMap(t *testing.T) {
	ev := mapStructuredEvent(agent.Event{
		Type: agent.EventStatus,
		Payload: map[string]any{
			"task_id":      "task-1",
			"phase":        "execution",
			"progress_pct": 42.5,
			"eta_sec":      int64(18),
			"log":          "parsed 3/7 files",
			"message":      "working",
		},
	})
	if ev.TaskID != "task-1" || ev.Phase != "execution" {
		t.Fatalf("unexpected task metadata: %+v", ev)
	}
	if ev.Progress == nil || *ev.Progress != 42.5 {
		t.Fatalf("unexpected progress: %+v", ev.Progress)
	}
	if ev.ETASec == nil || *ev.ETASec != 18 {
		t.Fatalf("unexpected eta: %+v", ev.ETASec)
	}
	if ev.Log != "parsed 3/7 files" || ev.Content != "working" {
		t.Fatalf("unexpected long-task content mapping: %+v", ev)
	}
}

func TestMapStructuredEvent_StatusLongTaskStruct(t *testing.T) {
	ev := mapStructuredEvent(agent.Event{
		Type: agent.EventStatus,
		Payload: agent.LongTaskStatus{
			TaskID:      "task-2",
			Phase:       "verification",
			ProgressPct: 100,
			ETASec:      0,
			Log:         "done",
		},
	})
	if ev.TaskID != "task-2" || ev.Phase != "verification" {
		t.Fatalf("unexpected task metadata: %+v", ev)
	}
	if ev.Progress == nil || *ev.Progress != 100 {
		t.Fatalf("unexpected progress mapping: %+v", ev.Progress)
	}
	if ev.Log != "done" {
		t.Fatalf("unexpected log mapping: %+v", ev)
	}
}

func TestMapStructuredEvent_UsesCanonicalContractTypes(t *testing.T) {
	tokens := mapStructuredEvent(agent.Event{Type: agent.EventToken, Payload: "hi"})
	if tokens.Type != "token" || tokens.Content != "hi" {
		t.Fatalf("unexpected token mapping: %+v", tokens)
	}

	toolCall := mapStructuredEvent(agent.Event{
		Type: agent.EventToolCall,
		Payload: agent.ToolCallEvent{
			ID:      "tc-1",
			Name:    "bash",
			Context: "run",
			Args:    map[string]any{"command": "echo hi"},
		},
	})
	if toolCall.Type != "tool_call" || toolCall.ToolCall == nil || toolCall.ToolCall.ID != "tc-1" {
		t.Fatalf("unexpected tool_call mapping: %+v", toolCall)
	}

	toolResult := mapStructuredEvent(agent.Event{
		Type: agent.EventToolDone,
		Payload: agent.ToolDoneEvent{
			ID:       "tc-1",
			Name:     "bash",
			Context:  "run",
			Duration: 25 * time.Millisecond,
			Result: tools.Result{
				Content: "ok",
			},
		},
	})
	if toolResult.Type != "tool_result" || toolResult.ToolResult == nil || toolResult.ToolResult.Status != "success" || toolResult.ToolResult.Output != "ok" {
		t.Fatalf("unexpected tool_result mapping: %+v", toolResult)
	}

	done := mapStructuredEvent(agent.Event{Type: agent.EventDone, Payload: "final"})
	if done.Type != "done" || done.Content != "final" {
		t.Fatalf("unexpected done mapping: %+v", done)
	}
}

func TestRunHeadlessStreamJSON_EmitsInitAndDone(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = "plan"
	sess := session.New()
	ag := agent.New(cfg, &testProvider{
		resp: aiPkg.NewStaticResponse("ok", "", nil, aiPkg.StopReasonEnd, aiPkg.Usage{}),
	}, sess, tools.NewRegistry())

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pipeR.Close()

	oldStdout := os.Stdout
	os.Stdout = pipeW
	defer func() { os.Stdout = oldStdout }()

	if err := runHeadlessStreamJSON(context.Background(), ag, sess, "say hi"); err != nil {
		t.Fatalf("runHeadlessStreamJSON returned error: %v", err)
	}

	_ = pipeW.Close()
	raw, err := io.ReadAll(pipeR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least init and done events, got %q", string(raw))
	}

	var first, last structuredEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("invalid first NDJSON line: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("invalid last NDJSON line: %v", err)
	}
	if first.Type != "init" {
		t.Fatalf("expected init first event, got %q", first.Type)
	}
	if last.Type != "done" {
		t.Fatalf("expected done terminal event, got %q", last.Type)
	}
}

func TestRunHeadlessStreamJSON_EachLineStandaloneJSON(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = "plan"
	sess := session.New()
	ag := agent.New(cfg, &testProvider{
		resp: aiPkg.NewStaticResponse("ok", "", nil, aiPkg.StopReasonEnd, aiPkg.Usage{}),
	}, sess, tools.NewRegistry())

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pipeR.Close()

	oldStdout := os.Stdout
	os.Stdout = pipeW
	defer func() { os.Stdout = oldStdout }()

	if err := runHeadlessStreamJSON(context.Background(), ag, sess, "say hi"); err != nil {
		t.Fatalf("runHeadlessStreamJSON returned error: %v", err)
	}

	_ = pipeW.Close()
	raw, err := io.ReadAll(pipeR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected NDJSON output")
	}
	for i, line := range lines {
		var ev structuredEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d is not standalone valid JSON: %v (%q)", i+1, err, line)
		}
		if ev.Type == "" {
			t.Fatalf("line %d missing type: %q", i+1, line)
		}
	}
}

func TestRenderLongTaskStatusText(t *testing.T) {
	progress := 33.3
	eta := int64(9)
	buf := &bytes.Buffer{}
	renderLongTaskStatusText(buf, longTaskStatus{
		TaskID:   "task-3",
		Phase:    "execution",
		Progress: &progress,
		ETASec:   &eta,
		Log:      "step 1 complete",
		Message:  "running",
	})
	out := buf.String()
	for _, want := range []string{
		"[log] [execution] step 1 complete",
		"[progress] task=task-3 phase=execution progress=33.3% eta=9s running",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output %q", want, out)
		}
	}
}

func TestTopLevelErrorExitCodesForFailingPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want automergentErrors.ExitCode
	}{
		{
			name: "invalid output format",
			err:  fmt.Errorf("invalid output format %q (allowed: text, json, stream-json)", "xml"),
			want: automergentErrors.ExitInvalidArgs,
		},
		{
			name: "unknown provider",
			err:  fmt.Errorf("provider: unknown provider %q", "bad-provider"),
			want: automergentErrors.ExitInvalidArgs,
		},
		{
			name: "missing prompt in headless mode",
			err:  fmt.Errorf("prompt required in no-tui mode"),
			want: automergentErrors.ExitInvalidArgs,
		},
		{
			name: "wrapped provider authentication failure",
			err:  fmt.Errorf("agent: complete: %w", automergentErrors.New(automergentErrors.CodeUnauthorized, "unauthorized")),
			want: automergentErrors.ExitAuthFailed,
		},
		{
			name: "wrapped tool execution failure",
			err:  fmt.Errorf("agent: stream: %w", automergentErrors.New(automergentErrors.CodeToolExecFailed, "tool failed")),
			want: automergentErrors.ExitToolExecutionError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := automergentErrors.ExitCodeForError(tt.err)
			if got != tt.want {
				t.Fatalf("ExitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
