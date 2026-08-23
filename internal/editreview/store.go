// Package editreview implements the pending-edit state machine: agent edits
// land as PROPOSALS, render as diffs, and touch disk only when accepted.
// Rejection feeds back into the conversation so the model adapts instead of
// thrash-retrying (Cursor's acceptance flow).
package editreview

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Status is the lifecycle state of one proposal.
type Status string

const (
	StatusProposed Status = "proposed"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
)

// Proposal is one proposed file change.
type Proposal struct {
	ID         string
	Tool       string // edit_file | write_file | create_file | multi_edit
	Path       string
	Original   string // empty for creates
	Proposed   string
	Summary    string
	Status     Status
	CreatedAt  time.Time
	ResolvedAt time.Time
}

// Hunk is one contiguous diff section with accept/reject semantics.
type Hunk struct {
	Lines []string // raw +/-/space-prefixed lines
}

// Store holds proposals for the current session.
type Store struct {
	mu        sync.Mutex
	proposals []*Proposal
	seq       int
}

// NewStore creates an empty store.
func NewStore() *Store { return &Store{} }

// Add records a new proposal and returns its generated ID.
func (s *Store) Add(tool, path, original, proposed, summary string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	p := &Proposal{
		ID:        fmt.Sprintf("pe-%d", s.seq),
		Tool:      tool,
		Path:      path,
		Original:  original,
		Proposed:  proposed,
		Summary:   summary,
		Status:    StatusProposed,
		CreatedAt: time.Now(),
	}
	s.proposals = append(s.proposals, p)
	return p.ID
}

// Get returns a proposal by ID.
func (s *Store) Get(id string) (*Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.proposals {
		if p.ID == id {
			return p, true
		}
	}
	return nil, false
}

// Pending lists proposals awaiting review, oldest first.
func (s *Store) Pending() []*Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Proposal
	for _, p := range s.proposals {
		if p.Status == StatusProposed {
			out = append(out, p)
		}
	}
	return out
}

// PendingCount reports how many proposals await review.
func (s *Store) PendingCount() int { return len(s.Pending()) }

// All returns every proposal (any status).
func (s *Store) All() []*Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Proposal, len(s.proposals))
	copy(out, s.proposals)
	return out
}

// Resolve marks a proposal accepted/rejected and returns it.
func (s *Store) Resolve(id string, st Status) (*Proposal, error) {
	if st != StatusAccepted && st != StatusRejected {
		return nil, fmt.Errorf("invalid status %q", st)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.proposals {
		if p.ID == id {
			if p.Status == StatusProposed {
				p.Status = st
				p.ResolvedAt = time.Now()
			}
			return p, nil
		}
	}
	return nil, fmt.Errorf("proposal %q not found", id)
}

// AcceptAll resolves every pending proposal as accepted; returns count.
func (s *Store) AcceptAll() int {
	n := 0
	for _, p := range s.Pending() {
		if _, err := s.Resolve(p.ID, StatusAccepted); err == nil {
			n++
		}
	}
	return n
}

// RejectAll resolves every pending proposal as rejected; returns count.
func (s *Store) RejectAll() int {
	n := 0
	for _, p := range s.Pending() {
		if _, err := s.Resolve(p.ID, StatusRejected); err == nil {
			n++
		}
	}
	return n
}

// UnifiedDiff renders the proposal's diff for display.
func (p *Proposal) UnifiedDiff() string {
	if p.Tool == "create_file" || p.Original == "" {
		var b strings.Builder
		for _, line := range strings.Split(strings.TrimRight(p.Proposed, "\n"), "\n") {
			b.WriteString("+ " + line + "\n")
		}
		return b.String()
	}
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = time.Second
	diffs := dmp.DiffMain(p.Original, p.Proposed, false)
	var b strings.Builder
	for _, d := range diffs {
		text := strings.TrimRight(d.Text, "\n")
		if text == "" {
			continue
		}
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			for _, line := range strings.Split(text, "\n") {
				b.WriteString("+ " + line + "\n")
			}
		case diffmatchpatch.DiffDelete:
			for _, line := range strings.Split(text, "\n") {
				b.WriteString("- " + line + "\n")
			}
		default:
			lines := strings.Split(text, "\n")
			shown := 0
			for _, line := range lines {
				if shown >= 3 && len(lines) > 6 {
					b.WriteString(fmt.Sprintf("  … (%d unchanged lines)\n", len(lines)-shown))
					break
				}
				b.WriteString("  " + line + "\n")
				shown++
			}
		}
	}
	return b.String()
}
