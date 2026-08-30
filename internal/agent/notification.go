package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	subagent "github.com/iSundram/Automergent/internal/tools/agent"
)

// TaskNotification represents a notification about a subagent's lifecycle.
// This is delivered to the parent agent as a user-role message.
type TaskNotification struct {
	TaskID     string `json:"task_id"`
	ToolUseID  string `json:"tool_use_id,omitempty"`
	Status     string `json:"status"` // "completed", "failed", "killed"
	Summary    string `json:"summary"`
	Result     string `json:"result,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	TotalToken int    `json:"total_tokens,omitempty"`
	ToolUses   int    `json:"tool_uses,omitempty"`
	Error      string `json:"error,omitempty"`
}

// FormatAsXML renders the notification in the XML format expected by
// the parent agent's message stream.
func (n *TaskNotification) FormatAsXML() string {
	var sb strings.Builder
	sb.WriteString("<task-notification>\n")
	sb.WriteString(fmt.Sprintf("  <task-id>%s</task-id>\n", n.TaskID))
	if n.ToolUseID != "" {
		sb.WriteString(fmt.Sprintf("  <tool-use-id>%s</tool-use-id>\n", n.ToolUseID))
	}
	if n.OutputFile != "" {
		sb.WriteString(fmt.Sprintf("  <output-file>%s</output-file>\n", n.OutputFile))
	}
	sb.WriteString(fmt.Sprintf("  <status>%s</status>\n", n.Status))
	sb.WriteString(fmt.Sprintf("  <summary>%s</summary>\n", n.Summary))
	if n.Result != "" {
		sb.WriteString(fmt.Sprintf("  <result>%s</result>\n", n.Result))
	}
	if n.Error != "" {
		sb.WriteString(fmt.Sprintf("  <error>%s</error>\n", n.Error))
	}
	sb.WriteString("  <usage>\n")
	if n.TotalToken > 0 {
		sb.WriteString(fmt.Sprintf("    <total_tokens>%d</total_tokens>\n", n.TotalToken))
	}
	if n.ToolUses > 0 {
		sb.WriteString(fmt.Sprintf("    <tool_uses>%d</tool_uses>\n", n.ToolUses))
	}
	if n.DurationMs > 0 {
		sb.WriteString(fmt.Sprintf("    <duration_ms>%d</duration_ms>\n", n.DurationMs))
	}
	sb.WriteString("  </usage>\n")
	sb.WriteString("</task-notification>")
	return sb.String()
}

// NotificationQueue manages pending notifications for delivery to the parent agent.
type NotificationQueue struct {
	mu      sync.Mutex
	pending []TaskNotification
	maxSize int
}

// NewNotificationQueue creates a new notification queue.
func NewNotificationQueue(maxSize int) *NotificationQueue {
	if maxSize <= 0 {
		maxSize = 64
	}
	return &NotificationQueue{
		pending: make([]TaskNotification, 0, maxSize),
		maxSize: maxSize,
	}
}

// Enqueue adds a notification to the queue.
// If the queue is full, the oldest notification is dropped.
func (q *NotificationQueue) Enqueue(n TaskNotification) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) >= q.maxSize {
		// Drop oldest
		q.pending = q.pending[1:]
	}
	q.pending = append(q.pending, n)
}

// Dequeue removes and returns the oldest notification.
// Returns false if the queue is empty.
func (q *NotificationQueue) Dequeue() (TaskNotification, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 {
		return TaskNotification{}, false
	}

	n := q.pending[0]
	q.pending = q.pending[1:]
	return n, true
}

// Drain returns all pending notifications and clears the queue.
func (q *NotificationQueue) Drain() []TaskNotification {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := q.pending
	q.pending = make([]TaskNotification, 0, q.maxSize)
	return result
}

// Len returns the number of pending notifications.
func (q *NotificationQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// FormatNotificationMessage formats a notification as a user-role message
// that can be injected into the parent agent's conversation.
func FormatNotificationMessage(n TaskNotification) string {
	return fmt.Sprintf("<task-notification task-id=\"%s\" status=\"%s\">\n%s\n</task-notification>",
		n.TaskID, n.Status, n.FormatAsXML())
}

// Global notification queue instance.
var globalNotificationQueue = NewNotificationQueue(128)

// EnqueueNotification adds a notification to the global queue.
func EnqueueNotification(n TaskNotification) {
	globalNotificationQueue.Enqueue(n)
}

// DrainNotifications returns all pending notifications.
func DrainNotifications() []TaskNotification {
	return globalNotificationQueue.Drain()
}

// NewCompletionNotification creates a notification for a completed agent.
func NewCompletionNotification(agentID, result string, duration time.Duration, toolCalls int) TaskNotification {
	return TaskNotification{
		TaskID:     agentID,
		Status:     "completed",
		Summary:    fmt.Sprintf("Agent %s completed successfully", agentID),
		Result:     result,
		DurationMs: duration.Milliseconds(),
		ToolUses:   toolCalls,
	}
}

// NewFailureNotification creates a notification for a failed agent.
func NewFailureNotification(agentID string, err error, duration time.Duration) TaskNotification {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return TaskNotification{
		TaskID:     agentID,
		Status:     "failed",
		Summary:    fmt.Sprintf("Agent %s failed: %s", agentID, errMsg),
		Error:      errMsg,
		DurationMs: duration.Milliseconds(),
	}
}

// NewKilledNotification creates a notification for a killed/cancelled agent.
func NewKilledNotification(agentID, reason string) TaskNotification {
	return TaskNotification{
		TaskID:  agentID,
		Status:  "killed",
		Summary: fmt.Sprintf("Agent %s was killed: %s", agentID, reason),
	}
}

// maxNotificationResultChars bounds the result text carried inside a
// notification: the model can call read_agent for the full output, so the
// injected message only needs enough to act on.
const maxNotificationResultChars = 2000

// EnableSubagentNotifications connects the subagent manager's completion
// hook to this agent's steering channel. A background subagent finishing
// then reaches the model as a <task-notification> user message at the next
// tool boundary (or the start of the next run when the agent is idle),
// instead of the model having to poll read_agent. Call once on the ROOT
// agent: children share the global manager and would double-deliver.
func (a *Agent) EnableSubagentNotifications() {
	subagent.GetAgentManager().RegisterCompletionHook(func(n subagent.AgentNotification) {
		var notification TaskNotification
		switch n.Status {
		case subagent.AgentStatusCompleted:
			notification = NewCompletionNotification(n.AgentID, clipNotificationResult(n.Result), n.Duration, 0)
		case subagent.AgentStatusFailed:
			notification = NewFailureNotification(n.AgentID, fmt.Errorf("%s", n.ErrMessage), n.Duration)
		case subagent.AgentStatusCancelled:
			notification = NewKilledNotification(n.AgentID, "cancelled by user")
		default:
			return
		}
		if n.Name != "" {
			notification.Summary = fmt.Sprintf("Agent %s (%s): %s", n.Name, n.Type, notification.Summary)
		}
		a.Steer(FormatNotificationMessage(notification))
	})
}

// clipNotificationResult bounds a notification's embedded result text.
func clipNotificationResult(result string) string {
	result = strings.TrimSpace(result)
	if len(result) <= maxNotificationResultChars {
		return result
	}
	return result[:maxNotificationResultChars] + "\n... [truncated — use read_agent for the full result]"
}
