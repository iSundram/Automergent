package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// Interaction history and rate limiting (optional features)
var (
	// EnableHistory turns on in-memory recording of interactions for debugging/auditing.
	EnableHistory bool = false
	historyMu     sync.RWMutex
	history       []InteractionRecord

	// Simple rate limiter for notifications: allow N notifications perDuration
	notifyMu          sync.Mutex
	notifyWindowStart time.Time
	notifyCount       int
	notifyLimit       = 10
	notifyWindow      = time.Minute

	// Notifier is the runtime hook used to deliver notifications. If nil, a default stdout notifier is used.
	notifierMu sync.RWMutex
	notifier   NotifierFunc
)

// InteractionRecord stores a single user interaction or notification.
type InteractionRecord struct {
	Time     time.Time
	Type     string // "ask" or "notify"
	Question string
	Answer   string
	Message  string
	Level    string
}

// NotifierFunc implements the actual delivery of notifications.
type NotifierFunc func(level string, title string, message string) error

// SetNotifier configures the global notifier used by NotifyTool.
func SetNotifier(n NotifierFunc) {
	notifierMu.Lock()
	defer notifierMu.Unlock()
	notifier = n
}

// SetNotifyRateLimit sets the maximum allowed notifications per duration.
func SetNotifyRateLimit(limit int, per time.Duration) {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	notifyLimit = limit
	notifyWindow = per
	// reset window
	notifyWindowStart = time.Time{}
	notifyCount = 0
}

// LogInteraction records an interaction when history is enabled.
func LogInteraction(r InteractionRecord) {
	if !EnableHistory {
		return
	}
	historyMu.Lock()
	defer historyMu.Unlock()
	history = append(history, r)
}

// GetHistory returns a copy of the interaction history.
func GetHistory() []InteractionRecord {
	historyMu.RLock()
	defer historyMu.RUnlock()
	out := make([]InteractionRecord, len(history))
	copy(out, history)
	return out
}

// allowNotify returns true if a notification is allowed by the rate limiter.
func allowNotify() bool {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	now := time.Now()
	if notifyWindowStart.IsZero() || now.Sub(notifyWindowStart) > notifyWindow {
		notifyWindowStart = now
		notifyCount = 0
	}
	if notifyCount >= notifyLimit {
		return false
	}
	notifyCount++
	return true
}

// AskUserTool prompts the user for input.
type AskUserTool struct {
	responder func(question string) (string, error)
}

func NewAskUserTool(responder func(string) (string, error)) *AskUserTool {
	return &AskUserTool{responder: responder}
}

func (t *AskUserTool) Name() string { return "ask_user" }
func (t *AskUserTool) Description() string {
	return "Ask the user one or more questions (with optional multiple-choice options) and get their answers."
}
func (t *AskUserTool) RequiresConfirmation(mode string) bool { return false }

func (t *AskUserTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string", "description": "Single free-text question. Use `questions` instead when options or multiple questions help."},
			"questions": map[string]any{
				"type":        "array",
				"description": "Structured questionnaire: one entry per question, each with optional multiple-choice options.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question":     map[string]any{"type": "string", "description": "The question text."},
						"options":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Multiple-choice options; the user may also type a custom answer."},
						"allow_custom": map[string]any{"type": "boolean", "description": "Whether a custom free-text answer is offered (default true)."},
					},
					"required": []string{"question"},
				},
			},
			"timeout_secs": map[string]any{"type": "integer", "description": "Timeout in seconds to wait for user response (optional)."},
			"allow_empty":  map[string]any{"type": "boolean", "description": "Allow empty responses (default false)."},
			"max_attempts": map[string]any{"type": "integer", "description": "How many times to prompt user for non-empty response (default 3)."},
		},
	}
}

// AskQuestion is one structured question with optional choices.
type AskQuestion struct {
	Text        string
	Options     []string
	AllowCustom bool
}

// QuestionnaireRequest is the structured payload handed to the UI hook.
type QuestionnaireRequest struct {
	Questions []AskQuestion
	Fallback  string // plain question text for legacy single-question asks
}

var (
	questionnaireMu   sync.RWMutex
	questionnaireHook func(QuestionnaireRequest) (string, error)
)

// SetQuestionnaire installs the UI handler that renders the interactive
// questionnaire and returns the user's formatted answer. Mirrors SetNotifier.
func SetQuestionnaire(fn func(QuestionnaireRequest) (string, error)) {
	questionnaireMu.Lock()
	defer questionnaireMu.Unlock()
	questionnaireHook = fn
}

// parseQuestions extracts the structured questions array from tool args.
func parseQuestions(args map[string]any) []AskQuestion {
	raw, ok := args["questions"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	var out []AskQuestion
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		text, _ := m["question"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		q := AskQuestion{Text: text, AllowCustom: true}
		if ac, ok := m["allow_custom"].(bool); ok {
			q.AllowCustom = ac
		}
		if opts, ok := m["options"].([]any); ok {
			for _, o := range opts {
				if s, ok := o.(string); ok && s != "" {
					q.Options = append(q.Options, s)
				}
			}
		}
		out = append(out, q)
	}
	return out
}

func (t *AskUserTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	fallback, _ := tools.StringArg(args, "question")
	questions := parseQuestions(args)
	if fallback == "" && len(questions) == 0 {
		return tools.Result{IsError: true, Content: "question or questions is required"}, nil
	}
	if len(questions) == 0 {
		questions = []AskQuestion{{Text: fallback, AllowCustom: true}}
	}

	// Structured UI path: the TUI questionnaire renders every question with
	// its options and returns a formatted Q/A transcript.
	questionnaireMu.RLock()
	hook := questionnaireHook
	questionnaireMu.RUnlock()
	if hook != nil {
		timeoutSecs, _ := tools.ArgInt(args, "timeout_secs")
		if timeoutSecs <= 0 {
			timeoutSecs = 3600
		}
		childCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
		defer cancel()

		type askResult struct {
			answer string
			err    error
		}
		done := make(chan askResult, 1)
		go func() {
			answer, err := hook(QuestionnaireRequest{Questions: questions, Fallback: fallback})
			done <- askResult{answer, err}
		}()
		select {
		case res := <-done:
			LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: questionsText(questions), Answer: res.answer})
			if res.err != nil {
				return tools.Result{IsError: true, Content: res.err.Error()}, nil
			}
			if strings.TrimSpace(res.answer) == "" {
				return tools.Result{IsError: true, Content: "user dismissed the question"}, nil
			}
			return tools.Result{Content: res.answer}, nil
		case <-childCtx.Done():
			LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: questionsText(questions), Message: "user response timeout"})
			return tools.Result{IsError: true, Content: "user response timeout"}, nil
		}
	}

	// Legacy single-question path (headless / stdin responders).
	legacyQuestion := fallback
	if legacyQuestion == "" {
		legacyQuestion = questionsText(questions)
	}

	// Read optional args
	timeoutSecs, _ := tools.ArgInt(args, "timeout_secs")
	if timeoutSecs <= 0 {
		// inherit context deadline if present, otherwise default to 3600s
		timeoutSecs = 3600
	}
	if t.responder == nil {
		return tools.Result{IsError: true, Content: "no responder configured"}, nil
	}
	question := legacyQuestion

	// Optional args (timeout already resolved for the structured path)
	if timeoutSecs <= 0 {
		timeoutSecs = 3600
	}
	allowEmpty, okb := tools.ArgBool(args, "allow_empty")
	if !okb {
		allowEmpty = false
	}
	maxAttempts, oka := tools.ArgInt(args, "max_attempts")
	if !oka || maxAttempts <= 0 {
		maxAttempts = 3
	}

	// Use a child context with timeout to avoid blocking forever
	childCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		responseCh := make(chan struct {
			ans string
			err error
		}, 1)

		// Call responder in a goroutine since it may block on stdin/TUI
		// Wrap in another goroutine to respect context cancellation
		go func() {
			doneCh := make(chan struct {
				ans string
				err error
			}, 1)

			// Actual responder call
			go func() {
				ans, err := t.responder(question)
				doneCh <- struct {
					ans string
					err error
				}{ans: ans, err: err}
			}()

			// Wait for responder or context cancellation
			select {
			case result := <-doneCh:
				responseCh <- result
			case <-childCtx.Done():
				// Context cancelled, don't send to responseCh
				// The responder goroutine will complete eventually without leaking
				return
			}
		}()

		select {
		case <-childCtx.Done():
			lastErr = errors.New("user response timeout")
			// Log and return
			LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: question, Answer: "", Message: lastErr.Error()})
			return tools.Result{IsError: true, Content: lastErr.Error()}, nil
		case res := <-responseCh:
			if res.err != nil {
				lastErr = fmt.Errorf("user interaction error: %v", res.err)
				LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: question, Answer: "", Message: lastErr.Error()})
				return tools.Result{IsError: true, Content: lastErr.Error()}, nil
			}
			answer := res.ans
			if answer == "" && !allowEmpty {
				// retry unless out of attempts
				if attempt < maxAttempts {
					// short backoff before retrying
					time.Sleep(200 * time.Millisecond)
					continue
				}
				lastErr = errors.New("empty response not allowed")
				LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: question, Answer: answer, Message: lastErr.Error()})
				return tools.Result{IsError: true, Content: lastErr.Error()}, nil
			}
			// success
			LogInteraction(InteractionRecord{Time: time.Now(), Type: "ask", Question: question, Answer: answer})
			return tools.Result{Content: answer}, nil
		}
	}

	// If we fall through, return last error
	if lastErr == nil {
		lastErr = errors.New("no response")
	}
	return tools.Result{IsError: true, Content: lastErr.Error()}, nil
}

// questionsText renders the question set for logging/legacy paths.
func questionsText(qs []AskQuestion) string {
	parts := make([]string, 0, len(qs))
	for _, q := range qs {
		parts = append(parts, q.Text)
	}
	return strings.Join(parts, " | ")
}

// NotifyTool sends a notification to the user.
type NotifyTool struct{}

func NewNotifyTool(n NotifierFunc) *NotifyTool {
	if n != nil {
		SetNotifier(n)
	}
	return &NotifyTool{}
}

func (t *NotifyTool) Name() string                          { return "notify" }
func (t *NotifyTool) Description() string                   { return "Show a notification message to the user." }
func (t *NotifyTool) RequiresConfirmation(mode string) bool { return false }

func (t *NotifyTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":   map[string]any{"type": "string", "description": "Short title for the notification."},
			"message": map[string]any{"type": "string", "description": "Message to display."},
			"level":   map[string]any{"type": "string", "enum": []string{"info", "warning", "error"}},
		},
		"required": []string{"message"},
	}
}

func (t *NotifyTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	message, ok := tools.StringArg(args, "message")
	if !ok || message == "" {
		return tools.Result{IsError: true, Content: "message is required"}, nil
	}
	title, _ := tools.StringArg(args, "title")
	level, _ := tools.StringArg(args, "level")
	if level == "" {
		level = "info"
	}

	// Rate limit
	if !allowNotify() {
		// Drop or summarize notification
		LogInteraction(InteractionRecord{Time: time.Now(), Type: "notify", Message: message, Level: level})
		return tools.Result{IsError: true, Content: "rate limit exceeded"}, nil
	}

	// Deliver notification
	notifierMu.RLock()
	n := notifier
	notifierMu.RUnlock()
	if n == nil {
		// default: print to stdout/stderr
		fmt.Printf("[%s] %s\n", level, message)
	} else {
		if err := n(level, title, message); err != nil {
			LogInteraction(InteractionRecord{Time: time.Now(), Type: "notify", Message: message, Level: level})
			return tools.Result{IsError: true, Content: fmt.Sprintf("notify handler error: %v", err)}, nil
		}
	}

	LogInteraction(InteractionRecord{Time: time.Now(), Type: "notify", Message: message, Level: level})
	return tools.Result{Content: fmt.Sprintf("notification: %s", message)}, nil
}
