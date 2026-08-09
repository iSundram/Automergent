package session

import (
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/ai"
)

// Session represents a conversation session.
type Session struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Messages  []ai.Message      `json:"messages"`
	Metadata  map[string]string `json:"metadata"`

	TotalInputTokens  int `json:"total_input_tokens"`
	TotalOutputTokens int `json:"total_output_tokens"`
}

// New creates a new session with a random ID.
func New() *Session {
	now := time.Now()
	return &Session{
		ID:        uuid.New().String(),
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ai.Message{},
		Metadata:  map[string]string{},
	}
}

// AddMessage appends a message to the session.
func (s *Session) AddMessage(m ai.Message) {
	s.Messages = append(s.Messages, m)
	s.UpdatedAt = time.Now()
}

// AddUsage accumulates token usage.
func (s *Session) AddUsage(u ai.Usage) {
	s.TotalInputTokens += u.InputTokens
	s.TotalOutputTokens += u.OutputTokens
}

// LastMessage returns the last message in the session, or nil.
func (s *Session) LastMessage() *ai.Message {
	if len(s.Messages) == 0 {
		return nil
	}
	return &s.Messages[len(s.Messages)-1]
}
