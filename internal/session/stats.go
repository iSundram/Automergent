// stats.go aggregates usage across sessions into a small cache at the
// storage root (stats.json). Per-session records are kept so repeated saves
// of the same session update in place rather than double-counting; the
// record set is capped and evicted oldest-first.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	statsFileName    = "stats.json"
	maxStatSessions  = 500
)

// UsageStats is the aggregated usage snapshot.
type UsageStats struct {
	TotalSessions int               `json:"total_sessions"`
	TotalMessages int               `json:"total_messages"`
	TotalIn       int               `json:"total_in"`
	TotalOut      int               `json:"total_out"`
	FirstSession  time.Time         `json:"first_session,omitempty"`
	LastSession   time.Time         `json:"last_session,omitempty"`
	DailyMessages map[string]int    `json:"daily_messages,omitempty"` // date → count
	ModelTokens   map[string][2]int `json:"model_tokens,omitempty"`   // model → {in,out}
}

// sessionStat is the per-session record persisted in stats.json.
type sessionStat struct {
	Messages  int       `json:"messages"`
	In        int       `json:"in"`
	Out       int       `json:"out"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type statsFile struct {
	Sessions map[string]sessionStat `json:"sessions"`
}

type statsTracker struct {
	path   string
	mu     sync.Mutex
	file   statsFile
	loaded bool
}

func newStatsTracker(storageRoot string) *statsTracker {
	return &statsTracker{path: filepath.Join(storageRoot, statsFileName)}
}

func (t *statsTracker) load() {
	if t.loaded {
		return
	}
	t.loaded = true
	t.file.Sessions = map[string]sessionStat{}
	if data, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(data, &t.file)
	}
	if t.file.Sessions == nil {
		t.file.Sessions = map[string]sessionStat{}
	}
}

// recordSession folds one session's totals into the aggregate.
func (t *statsTracker) recordSession(sess *Session, messageCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.load()

	t.file.Sessions[sess.ID] = sessionStat{
		Messages:  messageCount,
		In:        sess.TotalInputTokens,
		Out:       sess.TotalOutputTokens,
		Model:     sess.Model,
		CreatedAt: sess.CreatedAt,
		UpdatedAt: sess.UpdatedAt,
	}
	t.evictLocked()
	t.flush()
}

// evictLocked caps the record set, dropping the least-recently-updated.
func (t *statsTracker) evictLocked() {
	if len(t.file.Sessions) <= maxStatSessions {
		return
	}
	type kv struct {
		id  string
		at time.Time
	}
	var all []kv
	for id, s := range t.file.Sessions {
		all = append(all, kv{id, s.UpdatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.Before(all[j].at) })
	for _, victim := range all[:len(all)-maxStatSessions] {
		delete(t.file.Sessions, victim.id)
	}
}

func (t *statsTracker) flush() {
	data, err := json.MarshalIndent(t.file, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteFile(t.path, data, 0o600)
}

// snapshot derives the aggregate from the per-session records.
func (t *statsTracker) snapshot() UsageStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.load()

	var out UsageStats
	out.DailyMessages = map[string]int{}
	out.ModelTokens = map[string][2]int{}
	for _, s := range t.file.Sessions {
		out.TotalSessions++
		out.TotalMessages += s.Messages
		out.TotalIn += s.In
		out.TotalOut += s.Out
		if out.FirstSession.IsZero() || s.CreatedAt.Before(out.FirstSession) {
			out.FirstSession = s.CreatedAt
		}
		if s.UpdatedAt.After(out.LastSession) {
			out.LastSession = s.UpdatedAt
		}
		day := s.UpdatedAt.Format("2006-01-02")
		out.DailyMessages[day] += s.Messages
		if s.Model != "" {
			mt := out.ModelTokens[s.Model]
			mt[0] += s.In
			mt[1] += s.Out
			out.ModelTokens[s.Model] = mt
		}
	}
	return out
}

// Stats returns the aggregated usage across all recorded sessions.
func (s *Storage) Stats() UsageStats {
	if s.stats == nil {
		return UsageStats{}
	}
	return s.stats.snapshot()
}
