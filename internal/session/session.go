package session

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/ai"
)

// ToolApproval records a user's "always allow" decision for a tool scope.
type ToolApproval struct {
	Scope     string    `json:"scope"`
	GrantedAt time.Time `json:"granted_at"`
	Source    string    `json:"source,omitempty"` // e.g. "tui", "headless", "resume"
}

// Session represents a conversation session.
type Session struct {
	mu sync.RWMutex `json:"-"`

	ID        string            `json:"id"`
	Version   int               `json:"version,omitempty"`
	Title     string            `json:"title"`
	WorkDir   string            `json:"work_dir,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Messages  []ai.Message      `json:"messages"`
	Metadata  map[string]string `json:"metadata"`
	Provider  string            `json:"provider,omitempty"`
	Model     string            `json:"model,omitempty"`

	TotalInputTokens  int `json:"total_input_tokens"`
	TotalOutputTokens int `json:"total_output_tokens"`

	// AllowedTools records user-approved "always allow" tool scopes so they
	// survive process restarts and session resumes.
	AllowedTools []ToolApproval `json:"allowed_tools,omitempty"`
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

// Snapshot returns a deep copy of the session that is safe to marshal while
// the live session is being mutated by the agent goroutine.
func (s *Session) Snapshot() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, _ := json.Marshal(s)
	var copy Session
	_ = json.Unmarshal(data, &copy)
	return &copy
}

// AddMessage appends a message to the session.
func (s *Session) AddMessage(m ai.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, m)
	s.UpdatedAt = time.Now()
}

// AddUsage accumulates token usage.
func (s *Session) AddUsage(u ai.Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalInputTokens += u.InputTokens
	s.TotalOutputTokens += u.OutputTokens
}

// LastMessage returns the last message in the session, or nil.
func (s *Session) LastMessage() *ai.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.Messages) == 0 {
		return nil
	}
	return &s.Messages[len(s.Messages)-1]
}

// HasApproval reports whether the given tool scope was previously
// approved with "always allow".
func (s *Session) HasApproval(scope string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.AllowedTools {
		if a.Scope == scope {
			return true
		}
	}
	return false
}

// AddApproval records an "always allow" decision for a tool scope.
// Repeated grants are deduplicated (the original grant is kept).
func (s *Session) AddApproval(scope string, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.AllowedTools {
		if a.Scope == scope {
			return
		}
	}
	s.AllowedTools = append(s.AllowedTools, ToolApproval{
		Scope:     scope,
		GrantedAt: time.Now(),
		Source:    source,
	})
}

// RemoveApproval revokes an "always allow" decision, returning true if it
// was present.
func (s *Session) RemoveApproval(scope string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.AllowedTools {
		if a.Scope == scope {
			s.AllowedTools = append(s.AllowedTools[:i], s.AllowedTools[i+1:]...)
			return true
		}
	}
	return false
}

// ApprovalScopes returns the list of always-allow scopes.
func (s *Session) ApprovalScopes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	scopes := make([]string, 0, len(s.AllowedTools))
	for _, a := range s.AllowedTools {
		scopes = append(scopes, a.Scope)
	}
	return scopes
}

// SetMessages replaces the message list (used by /reset and resume).
func (s *Session) SetMessages(msgs []ai.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = msgs
	s.UpdatedAt = time.Now()
}
