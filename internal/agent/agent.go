package agent

import (
	"context"
	"fmt"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

// Agent is the core AI coding agent.
type Agent struct {
	cfg      *config.Config
	provider ai.Provider
	sess     *session.Session
	tools    *tools.Registry
	events   chan Event
}

// Event is an agent lifecycle event.
type Event struct {
	Type    string
	Payload any
}

const (
	EventToken     = "token"
	EventToolCall  = "tool_call"
	EventToolStart = "tool_start"
	EventToolDone  = "tool_done"
	EventDone      = "done"
	EventError     = "error"
	EventConfirm   = "confirm"
	EventStatus    = "status"
)

// New creates a new Agent.
func New(cfg *config.Config, provider ai.Provider, sess *session.Session, reg *tools.Registry) *Agent {
	return &Agent{
		cfg:      cfg,
		provider: provider,
		sess:     sess,
		tools:    reg,
		events:   make(chan Event, 512),
	}
}

// Events returns the channel of agent events.
func (a *Agent) Events() <-chan Event { return a.events }

// Session returns the current session.
func (a *Agent) Session() *session.Session { return a.sess }

// Run executes the agent loop for the given user prompt.
func (a *Agent) Run(ctx context.Context, prompt string) error {
	a.sess.AddMessage(ai.NewTextMessage(ai.RoleUser, prompt))

	for {
		systemPrompt := buildSystemPrompt(a.cfg)
		toolSchemas := buildToolSchemas(a.tools)

		req := ai.CompletionRequest{
			Messages:    a.sess.Messages,
			Tools:       toolSchemas,
			System:      systemPrompt,
			Temperature: 0.0,
			MaxTokens:   8192,
			Stream:      true,
		}

		a.emit(EventStatus, "thinking")
		resp, err := a.provider.Complete(ctx, req)
		if err != nil {
			a.emit(EventError, err)
			return fmt.Errorf("agent: complete: %w", err)
		}

		// Stream tokens as they arrive
		text, usage := a.drainStream(resp)
		toolCalls := resp.ToolCalls()
		stop := resp.StopReason()
		a.sess.AddUsage(usage)

		if text != "" {
			msg := ai.NewTextMessage(ai.RoleAssistant, text)
			a.sess.AddMessage(msg)
		}

		if stop != ai.StopReasonTools || len(toolCalls) == 0 {
			a.emit(EventDone, text)
			return nil
		}

		// Handle tool calls
		assistantMsg := ai.Message{Role: ai.RoleAssistant}
		for _, tc := range toolCalls {
			assistantMsg.Content = append(assistantMsg.Content, ai.ContentPart{
				Type:     ai.ContentTypeToolCall,
				ToolCall: &tc,
			})
		}
		a.sess.AddMessage(assistantMsg)

		for _, tc := range toolCalls {
			a.emit(EventToolCall, tc)
			a.emit(EventStatus, fmt.Sprintf("running %s", tc.Name))
			result, err := a.executeTool(ctx, tc)
			if err != nil {
				result = tools.Result{IsError: true, Content: err.Error()}
			}
			a.emit(EventToolDone, result)

			toolMsg := ai.Message{Role: ai.RoleTool}
			toolMsg.Content = append(toolMsg.Content, ai.ContentPart{
				Type: ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{
					ToolCallID: tc.ID,
					Content:    result.Content,
					IsError:    result.IsError,
				},
			})
			a.sess.AddMessage(toolMsg)
		}
	}
}

// drainStream reads all chunks from the response, emitting EventToken for each text chunk.
func (a *Agent) drainStream(resp ai.CompletionResponse) (string, ai.Usage) {
	var text string
	ch := resp.Stream()
	for chunk := range ch {
		if chunk.Error != nil {
			a.emit(EventError, chunk.Error)
			break
		}
		if chunk.Done {
			break
		}
		if chunk.Text != "" {
			a.emit(EventToken, chunk.Text)
			text += chunk.Text
		}
	}
	return text, resp.Usage()
}

func (a *Agent) executeTool(ctx context.Context, tc ai.ToolCall) (tools.Result, error) {
	t, ok := a.tools.Get(tc.Name)
	if !ok {
		return tools.Result{IsError: true, Content: fmt.Sprintf("unknown tool: %s", tc.Name)}, nil
	}

	if t.RequiresConfirmation(a.cfg.Mode) {
		confirmed := a.requestConfirmation(tc)
		if !confirmed {
			return tools.Result{Content: "user declined tool execution"}, nil
		}
	}

	return t.Execute(ctx, tc.Args)
}

func (a *Agent) requestConfirmation(tc ai.ToolCall) bool {
	ch := make(chan bool, 1)
	a.emit(EventConfirm, map[string]any{"tool_call": tc, "reply": ch})
	select {
	case confirmed := <-ch:
		return confirmed
	default:
		return false
	}
}

func (a *Agent) emit(eventType string, payload any) {
	select {
	case a.events <- Event{Type: eventType, Payload: payload}:
	default:
	}
}

func buildToolSchemas(reg *tools.Registry) []ai.ToolSchema {
	var schemas []ai.ToolSchema
	for _, t := range reg.All() {
		schemas = append(schemas, ai.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return schemas
}
