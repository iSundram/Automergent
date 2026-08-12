// Package testing provides utilities for testing tools.
package testing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/pkg/sdk/builder"
	"github.com/iSundram/Automergent/pkg/sdk/schema"
)

// Harness provides a testing harness for tools.
type Harness struct {
	t       *testing.T
	tool    tools.Tool
	ctx     context.Context
	timeout time.Duration
}

// NewHarness creates a new test harness for a tool.
func NewHarness(t *testing.T, tool tools.Tool) *Harness {
	return &Harness{
		t:       t,
		tool:    tool,
		ctx:     context.Background(),
		timeout: 30 * time.Second,
	}
}

// WithContext sets the context for test execution.
func (h *Harness) WithContext(ctx context.Context) *Harness {
	h.ctx = ctx
	return h
}

// WithTimeout sets the timeout for test execution.
func (h *Harness) WithTimeout(d time.Duration) *Harness {
	h.timeout = d
	return h
}

// Execute runs the tool with the given arguments.
func (h *Harness) Execute(args map[string]any) *ExecutionResult {
	ctx, cancel := context.WithTimeout(h.ctx, h.timeout)
	defer cancel()

	start := time.Now()
	result, err := h.tool.Execute(ctx, args)
	duration := time.Since(start)

	return &ExecutionResult{
		t:        h.t,
		result:   result,
		err:      err,
		duration: duration,
	}
}

// ExecuteString runs the tool with a single string argument.
func (h *Harness) ExecuteString(key, value string) *ExecutionResult {
	return h.Execute(map[string]any{key: value})
}

// TestSchema validates that the schema is well-formed.
func (h *Harness) TestSchema() {
	h.t.Helper()
	schema := h.tool.Schema()

	if schema == nil {
		h.t.Error("schema is nil")
		return
	}

	if schema["type"] != "object" {
		h.t.Errorf("schema type should be 'object', got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		h.t.Error("schema properties should be a map")
		return
	}

	for name, prop := range props {
		propMap, ok := prop.(map[string]any)
		if !ok {
			h.t.Errorf("property %s should be a map", name)
			continue
		}
		if _, ok := propMap["type"]; !ok {
			h.t.Errorf("property %s should have a type", name)
		}
	}
}

// ExecutionResult holds the result of a tool execution for assertions.
type ExecutionResult struct {
	t        *testing.T
	result   tools.Result
	err      error
	duration time.Duration
}

// AssertSuccess asserts that the execution succeeded.
func (r *ExecutionResult) AssertSuccess() *ExecutionResult {
	r.t.Helper()
	if r.err != nil {
		r.t.Errorf("expected success, got error: %v", r.err)
	}
	if r.result.IsError {
		r.t.Errorf("expected success, got error result: %s", r.result.Content)
	}
	return r
}

// AssertError asserts that the execution returned an error.
func (r *ExecutionResult) AssertError() *ExecutionResult {
	r.t.Helper()
	if r.err == nil && !r.result.IsError {
		r.t.Error("expected error, got success")
	}
	return r
}

// AssertContentContains asserts the content contains the given substring.
func (r *ExecutionResult) AssertContentContains(substr string) *ExecutionResult {
	r.t.Helper()
	if !strings.Contains(r.result.Content, substr) {
		r.t.Errorf("expected content to contain %q, got: %s", substr, r.result.Content)
	}
	return r
}

// AssertContentEquals asserts the content equals the given string.
func (r *ExecutionResult) AssertContentEquals(expected string) *ExecutionResult {
	r.t.Helper()
	if r.result.Content != expected {
		r.t.Errorf("expected content %q, got: %q", expected, r.result.Content)
	}
	return r
}

// AssertDurationLessThan asserts the execution completed within the given duration.
func (r *ExecutionResult) AssertDurationLessThan(d time.Duration) *ExecutionResult {
	r.t.Helper()
	if r.duration > d {
		r.t.Errorf("expected duration < %v, got: %v", d, r.duration)
	}
	return r
}

// AssertMetadata asserts a metadata key has the expected value.
func (r *ExecutionResult) AssertMetadata(key string, expected any) *ExecutionResult {
	r.t.Helper()
	if r.result.Metadata == nil {
		r.t.Errorf("expected metadata key %q, but metadata is nil", key)
		return r
	}
	if r.result.Metadata[key] != expected {
		r.t.Errorf("expected metadata[%q] = %v, got: %v", key, expected, r.result.Metadata[key])
	}
	return r
}

// Content returns the result content.
func (r *ExecutionResult) Content() string {
	return r.result.Content
}

// Result returns the raw result.
func (r *ExecutionResult) Result() tools.Result {
	return r.result
}

// Error returns any error.
func (r *ExecutionResult) Error() error {
	return r.err
}

// Duration returns the execution duration.
func (r *ExecutionResult) Duration() time.Duration {
	return r.duration
}

// MockTool provides a configurable mock tool for testing.
type MockTool struct {
	name        string
	description string
	schema      map[string]any
	handler     func(ctx context.Context, args map[string]any) (tools.Result, error)
	confirmFn   func(mode string) bool

	mu          sync.Mutex
	calls       []MockCall
	returnQueue []tools.Result
}

// MockCall records a call to the mock tool.
type MockCall struct {
	Args map[string]any
	Time time.Time
}

// NewMockTool creates a new mock tool.
func NewMockTool(name string) *MockTool {
	return &MockTool{
		name: name,
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// WithDescription sets the mock tool description.
func (m *MockTool) WithDescription(desc string) *MockTool {
	m.description = desc
	return m
}

// WithSchema sets the mock tool schema.
func (m *MockTool) WithSchema(schema map[string]any) *MockTool {
	m.schema = schema
	return m
}

// WithHandler sets a custom handler.
func (m *MockTool) WithHandler(handler func(ctx context.Context, args map[string]any) (tools.Result, error)) *MockTool {
	m.handler = handler
	return m
}

// Returns configures the mock to return the given result.
func (m *MockTool) Returns(content string) *MockTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.returnQueue = append(m.returnQueue, tools.Result{Content: content})
	return m
}

// ReturnsError configures the mock to return an error result.
func (m *MockTool) ReturnsError(content string) *MockTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.returnQueue = append(m.returnQueue, tools.Result{Content: content, IsError: true})
	return m
}

// WithConfirmation sets the confirmation function.
func (m *MockTool) WithConfirmation(fn func(mode string) bool) *MockTool {
	m.confirmFn = fn
	return m
}

// Name returns the tool name.
func (m *MockTool) Name() string { return m.name }

// Description returns the tool description.
func (m *MockTool) Description() string { return m.description }

// Schema returns the tool schema.
func (m *MockTool) Schema() map[string]any { return m.schema }

// RequiresConfirmation checks if confirmation is needed.
func (m *MockTool) RequiresConfirmation(mode string) bool {
	if m.confirmFn != nil {
		return m.confirmFn(mode)
	}
	return false
}

// Execute runs the mock tool.
func (m *MockTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	m.mu.Lock()
	m.calls = append(m.calls, MockCall{Args: args, Time: time.Now()})

	// Check for queued return values
	if len(m.returnQueue) > 0 {
		result := m.returnQueue[0]
		m.returnQueue = m.returnQueue[1:]
		m.mu.Unlock()
		return result, nil
	}
	m.mu.Unlock()

	// Use custom handler if provided
	if m.handler != nil {
		return m.handler(ctx, args)
	}

	// Default response
	return tools.Result{Content: "mock response"}, nil
}

// Calls returns all recorded calls.
func (m *MockTool) Calls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockCall{}, m.calls...)
}

// CallCount returns the number of calls made.
func (m *MockTool) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// Reset clears call history and return queue.
func (m *MockTool) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.returnQueue = nil
}

// AssertCalled asserts the tool was called with the given arguments.
func (m *MockTool) AssertCalled(t *testing.T, args map[string]any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, call := range m.calls {
		if matchArgs(call.Args, args) {
			return
		}
	}
	t.Errorf("expected call with args %v, but no matching call found", args)
}

// AssertCallCount asserts the tool was called exactly n times.
func (m *MockTool) AssertCallCount(t *testing.T, n int) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.calls) != n {
		t.Errorf("expected %d calls, got %d", n, len(m.calls))
	}
}

func matchArgs(actual, expected map[string]any) bool {
	for k, v := range expected {
		if actual[k] != v {
			return false
		}
	}
	return true
}

// TableTest runs a table-driven test for a tool.
type TableTest struct {
	t    *testing.T
	tool tools.Tool
}

// NewTableTest creates a new table test runner.
func NewTableTest(t *testing.T, tool tools.Tool) *TableTest {
	return &TableTest{t: t, tool: tool}
}

// TestCase represents a single test case.
type TestCase struct {
	Name           string
	Args           map[string]any
	ExpectSuccess  bool
	ExpectContains string
	ExpectEquals   string
	ExpectMetadata map[string]any
}

// Run executes all test cases.
func (tt *TableTest) Run(cases []TestCase) {
	for _, tc := range cases {
		tt.t.Run(tc.Name, func(t *testing.T) {
			harness := NewHarness(t, tt.tool)
			result := harness.Execute(tc.Args)

			if tc.ExpectSuccess {
				result.AssertSuccess()
			} else {
				result.AssertError()
			}

			if tc.ExpectContains != "" {
				result.AssertContentContains(tc.ExpectContains)
			}

			if tc.ExpectEquals != "" {
				result.AssertContentEquals(tc.ExpectEquals)
			}

			for k, v := range tc.ExpectMetadata {
				result.AssertMetadata(k, v)
			}
		})
	}
}

// SchemaValidator validates arguments against a schema.
type SchemaValidator struct {
	params map[string]*schema.ParamSchema
}

// NewSchemaValidator creates a validator from parameter schemas.
func NewSchemaValidator(params map[string]*schema.ParamSchema) *SchemaValidator {
	return &SchemaValidator{params: params}
}

// Validate checks if the arguments are valid.
func (v *SchemaValidator) Validate(args map[string]any) error {
	for name, param := range v.params {
		if err := param.Validate(args[name]); err != nil {
			return err
		}
	}
	return nil
}

// ValidateWithDefaults validates and applies defaults.
func (v *SchemaValidator) ValidateWithDefaults(args map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range args {
		result[k] = v
	}

	for name, param := range v.params {
		if result[name] == nil && param.Default != nil {
			result[name] = param.Default
		}
		if err := param.Validate(result[name]); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// FluentToolTestHelper provides additional helpers for FluentTool.
type FluentToolTestHelper struct {
	t    *testing.T
	tool *builder.FluentTool
}

// NewFluentToolHelper creates a helper for testing FluentTool.
func NewFluentToolHelper(t *testing.T, tool *builder.FluentTool) *FluentToolTestHelper {
	return &FluentToolTestHelper{t: t, tool: tool}
}

// TestAllExamples runs all examples as tests.
func (h *FluentToolTestHelper) TestAllExamples() {
	harness := NewHarness(h.t, h.tool)

	for _, ex := range h.tool.Examples() {
		h.t.Run(ex.Name, func(t *testing.T) {
			result := harness.Execute(ex.Args)
			result.AssertSuccess()

			if ex.Expected != "" {
				result.AssertContentContains(ex.Expected)
			}
		})
	}
}

// TestRequiredParams tests that required parameters are enforced.
func (h *FluentToolTestHelper) TestRequiredParams() {
	harness := NewHarness(h.t, h.tool)

	for name, param := range h.tool.ParamSchemas() {
		if param.Required {
			h.t.Run(fmt.Sprintf("missing_%s", name), func(t *testing.T) {
				args := make(map[string]any)
				// Don't include the required param
				result := harness.Execute(args)
				result.AssertError()
			})
		}
	}
}
