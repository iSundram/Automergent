package agent

import (
	"context"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	"github.com/iSundram/Automergent/internal/tools"
)

// SubAgent is a lightweight agent that runs a single focused task.
type SubAgent struct {
	parent   *Agent
	cfg      *config.Config
	provider ai.Provider
	sess     *session.Session
	tools    *tools.Registry
}

// NewSubAgent creates a sub-agent that shares the parent's provider and tools.
func NewSubAgent(parent *Agent, task string) *SubAgent {
	sess := session.New()
	sess.AddMessage(ai.NewTextMessage(ai.RoleUser, task))
	return &SubAgent{
		parent:   parent,
		cfg:      parent.cfg,
		provider: parent.provider,
		sess:     sess,
		tools:    parent.tools,
	}
}

// Run executes the sub-agent and returns the final text response.
func (s *SubAgent) Run(ctx context.Context) (string, error) {
	inner := New(s.cfg, s.provider, s.sess, s.tools)

	// Drain events in background
	go func() {
		for range inner.Events() {
		}
	}()

	// Run with the already-added user message (no new prompt needed)
	// by directly completing
	systemPrompt := buildSystemPrompt(s.cfg)
	req := ai.CompletionRequest{
		Messages:    s.sess.Messages,
		System:      systemPrompt,
		Temperature: 0.0,
		MaxTokens:   8192,
		Stream:      true,
	}
	resp, err := s.provider.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	// Drain the stream and collect text
	var text string
	for chunk := range resp.Stream() {
		if chunk.Done || chunk.Error != nil {
			break
		}
		text += chunk.Text
	}
	return text, nil
}
