// transcript.go implements the append-only JSONL session format.
//
// Layout under the storage root:
//
//	projects/<sanitized-workdir>/<sessionID>.jsonl   append-only event log
//	projects/<sanitized-workdir>/<sessionID>/
//	  tool-results/<sha256>                          offloaded large tool results
//
// Each line is one Entry. Transcript messages chain via ParentUUID so rewind
// and fork are just "pick an older leaf and keep appending" — session files
// are never rewritten. A "summary" entry is re-appended near the tail so
// lite listing (head+tail window reads) always sees current title/count.
//
// Legacy single-file "<id>.json" sessions are read transparently and migrate
// to the transcript format on their next Save.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iSundram/Automergent/internal/ai"
)

// EntryType discriminates transcript lines.
type EntryType string

const (
	entryHeader   EntryType = "header"   // first line: session identity
	entryMessage  EntryType = "message"  // one conversation message
	entrySnapshot EntryType = "snapshot" // full message-array replacement (rewind/compact)
	entryTitle    EntryType = "title"    // title change (user source wins over ai)
	entrySummary  EntryType = "summary"  // lite-listing tail marker
	entrySession  EntryType = "session"  // scalar session fields (model, tokens, ...)
)

// Entry is one line of a transcript file.
type Entry struct {
	UUID       string          `json:"uuid"`
	ParentUUID string          `json:"parent_uuid,omitempty"`
	Type       EntryType       `json:"type"`
	Timestamp  time.Time       `json:"ts"`
	Message    *ai.Message     `json:"message,omitempty"`
	Messages   []ai.Message    `json:"messages,omitempty"`
	Title      string          `json:"title,omitempty"`
	TitleSource string         `json:"title_source,omitempty"` // "user" | "ai"
	Scalar     *SessionScalars `json:"scalar,omitempty"`
	Header     *HeaderInfo     `json:"header,omitempty"`
	Summary    *SummaryInfo    `json:"summary_info,omitempty"`
}

// HeaderInfo is written as the first entry of a transcript.
type HeaderInfo struct {
	SessionID string    `json:"session_id"`
	WorkDir   string    `json:"work_dir,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Version   int       `json:"version,omitempty"`
}

// SessionScalars carries the non-message Session fields, persisted as
// "session" entries whenever they change.
type SessionScalars struct {
	Title        string            `json:"title"`
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	AllowedTools []ToolApproval    `json:"allowed_tools,omitempty"`
	TotalIn      int               `json:"total_in,omitempty"`
	TotalOut     int               `json:"total_out,omitempty"`
}

// SummaryInfo is the lite-listing marker kept near the file tail.
type SummaryInfo struct {
	MessageCount int       `json:"message_count"`
	Title        string    `json:"title,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	TotalIn      int       `json:"total_in,omitempty"`
	TotalOut     int       `json:"total_out,omitempty"`
}

const (
	projectsSubdir      = "projects"
	toolResultsSubdir   = "tool-results"
	inlineToolResultMax = 32 * 1024 // larger tool results are offloaded to disk
	// summaryMinInterval bounds summary-entry growth: at most one small
	// summary line per interval unless title/count changed.
	summaryMinInterval = 5 * time.Minute
)

// sanitizeProjectDir maps a working directory to a filesystem-safe subdir
// name: non-alphanumerics collapse to '-'.
func sanitizeProjectDir(workDir string) string {
	if workDir == "" {
		return "default"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range workDir {
		alnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if alnum {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		return "default"
	}
	return name
}

func (s *Storage) transcriptPath(project, id string) string {
	return filepath.Join(s.dir, projectsSubdir, project, id+".jsonl")
}

// findTranscript locates the transcript file for a session ID by scanning
// the project subdirectories. Returns "" when not found.
func (s *Storage) findTranscript(id string) string {
	root := filepath.Join(s.dir, projectsSubdir)
	projects, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		cand := filepath.Join(root, p.Name(), id+".jsonl")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

// saveState tracks what a Storage has already persisted for a session so
// Save can append only the delta.
type saveState struct {
	project          string
	count            int       // messages persisted
	lastDigest       string    // digest of the last persisted message
	lastUUID         string    // UUID of the last chain entry
	scalarKey        string    // digest of the last persisted scalar fields
	title            string
	lastSumAt        time.Time
	lastSummaryCount int
	lastSummaryTitle string
	lastSummaryIn    int
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func digestMessage(m ai.Message) string {
	b, _ := json.Marshal(m)
	return digestBytes(b)
}

func digestScalars(sc *SessionScalars) string {
	b, _ := json.Marshal(sc)
	return digestBytes(b)
}

// appendEntries writes entries as JSONL lines with mode 0600.
func (s *Storage) appendEntries(path string, entries []*Entry) error {
	if len(entries) == 0 {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("transcript: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("transcript: open: %w", err)
	}
	defer f.Close()
	var b strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("transcript: marshal: %w", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		return fmt.Errorf("transcript: append: %w", err)
	}
	return f.Sync()
}

// maybeOffloadToolResults replaces oversized tool-result content in msg with
// an on-disk reference, storing the full content next to the transcript.
// It returns a copy; the original message is untouched.
func (s *Storage) maybeOffloadToolResults(msg ai.Message, toolDir string) ai.Message {
	big := false
	for i := range msg.Content {
		if msg.Content[i].ToolResult != nil && len(msg.Content[i].ToolResult.Content) > inlineToolResultMax {
			big = true
			break
		}
	}
	if !big {
		return msg
	}
	out := msg
	out.Content = make([]ai.ContentPart, len(msg.Content))
	copy(out.Content, msg.Content)
	if err := os.MkdirAll(toolDir, 0o700); err == nil {
		for i := range out.Content {
			tr := out.Content[i].ToolResult
			if tr == nil || len(tr.Content) <= inlineToolResultMax {
				continue
			}
			sum := sha256.Sum256([]byte(tr.Content))
			name := hex.EncodeToString(sum[:]) + ".txt"
			if err := os.WriteFile(filepath.Join(toolDir, name), []byte(tr.Content), 0o600); err == nil {
				replaced := *tr
				replaced.Content = fmt.Sprintf("<persisted-output file=%q bytes=%d/>", name, len(tr.Content))
				out.Content[i].ToolResult = &replaced
			}
		}
	}
	return out
}

// restoreToolResults re-inlines tool-result content that was offloaded to
// disk. Missing files leave the marker in place (cleared, not corrupt).
func (s *Storage) restoreToolResults(msg ai.Message, toolDir string) ai.Message {
	restored := false
	for i := range msg.Content {
		tr := msg.Content[i].ToolResult
		if tr == nil || !strings.HasPrefix(tr.Content, "<persisted-output file=") {
			continue
		}
		var name string
		if _, err := fmt.Sscanf(tr.Content, "<persisted-output file=%q", &name); err != nil {
			continue
		}
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(toolDir, name))
		if err != nil {
			continue
		}
		if !restored {
			out := msg
			out.Content = make([]ai.ContentPart, len(msg.Content))
			copy(out.Content, msg.Content)
			msg = out
			restored = true
		}
		inl := *msg.Content[i].ToolResult
		inl.Content = string(data)
		msg.Content[i].ToolResult = &inl
	}
	return msg
}

func scalarsFrom(sess *Session) *SessionScalars {
	return &SessionScalars{
		Title:        sess.Title,
		Provider:     sess.Provider,
		Model:        sess.Model,
		Metadata:     sess.Metadata,
		AllowedTools: sess.AllowedTools,
		TotalIn:      sess.TotalInputTokens,
		TotalOut:     sess.TotalOutputTokens,
	}
}

// saveTranscript persists sess as transcript events, appending only what
// changed since the previous Save of the same session.
func (s *Storage) saveTranscript(sess *Session) error {
	project := sanitizeProjectDir(sess.WorkDir)
	path := s.transcriptPath(project, sess.ID)
	toolDir := filepath.Join(filepath.Dir(path), sess.ID, toolResultsSubdir)

	st := s.saveStates[sess.ID]
	firstSave := st == nil
	if firstSave {
		st = &saveState{project: project}
		s.saveStates[sess.ID] = st
	}

	msgs := sess.Messages
	// Entries carry the session's own clock so logical timestamps (and
	// age-based pruning) survive save/load round-trips.
	ts := sess.UpdatedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	var pending []*Entry
	appendChain := func(e *Entry) {
		e.ParentUUID = st.lastUUID
		pending = append(pending, e)
		st.lastUUID = e.UUID
	}
	// appendSnapshot writes a full-array replacement epoch and updates the
	// delta bookkeeping to match it.
	appendSnapshot := func(msgs []ai.Message) {
		appendChain(s.snapshotEntry(msgs, toolDir, ts))
		st.count = len(msgs)
		if len(msgs) > 0 {
			st.lastDigest = digestMessage(msgs[len(msgs)-1])
		} else {
			st.lastDigest = ""
		}
	}

	if firstSave {
		appendChain(&Entry{
			UUID:      uuid.NewString(),
			Type:      entryHeader,
			Timestamp: sess.CreatedAt,
			Header: &HeaderInfo{
				SessionID: sess.ID,
				WorkDir:   sess.WorkDir,
				CreatedAt: sess.CreatedAt,
			},
		})
		// A first save of a session that already has history (legacy import
		// or in-memory resume) starts with a snapshot so the chain is whole.
		if len(msgs) > 0 {
			appendSnapshot(msgs)
		}
	} else {
		cur := len(msgs)
		switch {
		case cur > st.count:
			// Grew — verify the prefix still ends where we left off.
			if st.count == 0 || digestMessage(msgs[st.count-1]) == st.lastDigest {
				for _, m := range msgs[st.count:] {
					appendChain(&Entry{
						UUID:      uuid.NewString(),
						Type:      entryMessage,
						Timestamp: ts,
						Message:   ptr(s.maybeOffloadToolResults(m, toolDir)),
					})
				}
				st.count = cur
				st.lastDigest = digestMessage(msgs[cur-1])
			} else {
				appendSnapshot(msgs)
			}
		case cur == st.count && (cur == 0 || digestMessage(msgs[cur-1]) == st.lastDigest):
			// No message change.
		default:
			// Shrank or the tail changed (rewind, compact, steering):
			// replace the whole array with a new snapshot epoch.
			appendSnapshot(msgs)
		}
	}

	// Title as its own event: user-set titles must survive and outrank
	// auto-generated ones regardless of append order.
	if sess.Title != st.title || (firstSave && sess.Title != "") {
		if sess.Title != "" {
			pending = append(pending, &Entry{
				UUID:        uuid.NewString(),
				Type:        entryTitle,
				Timestamp:   ts,
				Title:       sess.Title,
				TitleSource: "user",
			})
		}
		st.title = sess.Title
	}

	// Scalar fields when they changed.
	sc := scalarsFrom(sess)
	if key := digestScalars(sc); key != st.scalarKey || firstSave {
		pending = append(pending, &Entry{
			UUID:      uuid.NewString(),
			Type:      entrySession,
			Timestamp: ts,
			Scalar:    sc,
		})
		st.scalarKey = key
	}

	// Tail summary for lite listing: on change, or periodically so the
	// updated-at stamp stays fresh.
	summaryChanged := st.count != st.lastSummaryCount || sess.Title != st.lastSummaryTitle ||
		sess.TotalInputTokens != st.lastSummaryIn
	if summaryChanged || firstSave || ts.Sub(st.lastSumAt) >= summaryMinInterval {
		pending = append(pending, &Entry{
			UUID:      uuid.NewString(),
			Type:      entrySummary,
			Timestamp: ts,
			Summary: &SummaryInfo{
				MessageCount: st.count,
				Title:        sess.Title,
				UpdatedAt:    ts,
				TotalIn:      sess.TotalInputTokens,
				TotalOut:     sess.TotalOutputTokens,
			},
		})
		st.lastSumAt = ts
		st.lastSummaryCount = st.count
		st.lastSummaryTitle = sess.Title
		st.lastSummaryIn = sess.TotalInputTokens
	}

	if len(pending) == 0 {
		return nil
	}
	if err := s.appendEntries(path, pending); err != nil {
		return err
	}

	// Enforce the on-disk size budget in event-log form: once the file
	// grows past twice the budget, append a compacted snapshot epoch so the
	// load path (and the next delta diff) sees the compacted history.
	if budget := s.effectiveMaxBytes(); budget > 0 {
		if info, err := os.Stat(path); err == nil && info.Size() > 2*budget {
			comp := sess.Snapshot()
			if CompactForSize(comp, budget) && len(comp.Messages) < len(sess.Messages) {
				compactEntries := []*Entry{s.snapshotEntry(comp.Messages, toolDir, ts)}
				if err := s.appendEntries(path, compactEntries); err == nil {
					st.count = len(comp.Messages)
					st.lastDigest = digestMessage(comp.Messages[len(comp.Messages)-1])
				}
			}
		}
	}

	// Legacy migration: once the transcript exists the old whole-file JSON
	// is redundant (and would double-list).
	legacy := filepath.Join(s.dir, sess.ID+".json")
	if _, err := os.Stat(legacy); err == nil {
		_ = os.Remove(legacy)
	}
	return nil
}

func (s *Storage) effectiveMaxBytes() int64 {
	if s.maxSessionBytes > 0 {
		return s.maxSessionBytes
	}
	return defaultMaxSessionBytes
}

// snapshotEntry builds a full-array replacement entry. Bookkeeping is the
// caller's job (see appendSnapshot).
func (s *Storage) snapshotEntry(msgs []ai.Message, toolDir string, ts time.Time) *Entry {
	snap := make([]ai.Message, len(msgs))
	for i, m := range msgs {
		snap[i] = s.maybeOffloadToolResults(m, toolDir)
	}
	e := &Entry{
		UUID:      uuid.NewString(),
		Type:      entrySnapshot,
		Timestamp: ts,
		Messages:  snap,
	}
	return e
}

func ptr(m ai.Message) *ai.Message { return &m }

// loadTranscript rebuilds a Session from a transcript file.
func (s *Storage) loadTranscript(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	toolDir := filepath.Join(filepath.Dir(path), id, toolResultsSubdir)

	sess := &Session{ID: id, Messages: []ai.Message{}, Metadata: map[string]string{}}
	var msgs []ai.Message
	userTitle := ""
	aiTitle := ""
	var updated time.Time

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // torn tail line from a crash: keep what parsed
		}
		if !e.Timestamp.IsZero() {
			updated = e.Timestamp
		}
		switch e.Type {
		case entryHeader:
			if e.Header != nil {
				sess.CreatedAt = e.Header.CreatedAt
				sess.WorkDir = e.Header.WorkDir
			}
		case entryMessage:
			if e.Message != nil {
				msgs = append(msgs, s.restoreToolResults(*e.Message, toolDir))
			}
		case entrySnapshot:
			msgs = nil
			for _, m := range e.Messages {
				msgs = append(msgs, s.restoreToolResults(m, toolDir))
			}
		case entryTitle:
			if e.TitleSource == "user" {
				userTitle = e.Title
			} else if e.Title != "" {
				aiTitle = e.Title
			}
		case entrySession:
			if e.Scalar != nil {
				sess.Title = e.Scalar.Title
				sess.Provider = e.Scalar.Provider
				sess.Model = e.Scalar.Model
				sess.Metadata = e.Scalar.Metadata
				sess.AllowedTools = e.Scalar.AllowedTools
				sess.TotalInputTokens = e.Scalar.TotalIn
				sess.TotalOutputTokens = e.Scalar.TotalOut
			}
		case entrySummary:
			if e.Summary != nil && e.Summary.UpdatedAt.After(updated) {
				updated = e.Summary.UpdatedAt
			}
		}
	}
	// User title wins over AI/scalar title.
	if userTitle != "" {
		sess.Title = userTitle
	} else if sess.Title == "" && aiTitle != "" {
		sess.Title = aiTitle
	}
	sess.Messages = msgs
	if sess.Metadata == nil {
		sess.Metadata = map[string]string{}
	}
	sess.UpdatedAt = updated
	sess.SizeBytes = int64(len(data))
	return MigrateSession(sess)
}

// liteWindow sizes for listing without full parses.
const (
	liteHeadBytes = 16 * 1024
	liteTailBytes = 64 * 1024
)

// readLiteSession reads just the head and tail windows of a transcript and
// returns a Session with empty Messages — enough for listing, picking and
// token totals. Storage.liteSession is a thin wrapper; the free function is
// shared with AuditStorage, which has no Storage instance.
func readLiteSession(path string) *Session {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	size := info.Size()

	readWindow := func(n int64, offset int64) []byte {
		if n <= 0 {
			return nil
		}
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, offset); err != nil {
			return nil
		}
		return buf
	}
	headN := int64(liteHeadBytes)
	if size < headN {
		headN = size
	}
	head := readWindow(headN, 0)
	tailN := int64(liteTailBytes)
	if size < tailN {
		tailN = size
	}
	tail := readWindow(tailN, size-tailN)

	sess := &Session{Messages: []ai.Message{}, Metadata: map[string]string{}, SizeBytes: size}
	firstPrompt := ""

	// Head pass: identity + first user prompt.
	for _, line := range strings.Split(string(head), "\n") {
		var e Entry
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &e) != nil {
			continue
		}
		switch e.Type {
		case entryHeader:
			if e.Header != nil {
				sess.ID = e.Header.SessionID
				sess.WorkDir = e.Header.WorkDir
				sess.CreatedAt = e.Header.CreatedAt
			}
		case entryMessage:
			if e.Message != nil && firstPrompt == "" && e.Message.Role == ai.RoleUser {
				firstPrompt = firstTextOf(e.Message)
			}
		}
	}

	// Tail pass (last wins): latest summary/title/session state.
	var updated time.Time
	for _, line := range strings.Split(string(tail), "\n") {
		var e Entry
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &e) != nil {
			continue
		}
		if !e.Timestamp.IsZero() {
			updated = e.Timestamp
		}
		switch e.Type {
		case entrySummary:
			if e.Summary != nil {
				sess.TotalInputTokens = e.Summary.TotalIn
				sess.TotalOutputTokens = e.Summary.TotalOut
				if e.Summary.Title != "" {
					sess.Title = e.Summary.Title
				}
				// The lite listing carries no messages; the count rides in
				// metadata so display surfaces can show "N msgs" without a
				// full parse. Keyed without a dot to stay distinct from
				// user-set metadata.
				sess.Metadata["lite_message_count"] = strconv.Itoa(e.Summary.MessageCount)
				if e.Summary.UpdatedAt.After(updated) {
					updated = e.Summary.UpdatedAt
				}
			}
		case entryTitle:
			if e.Title != "" {
				sess.Title = e.Title
			}
		case entrySession:
			if e.Scalar != nil {
				sess.Title = e.Scalar.Title
				sess.Model = e.Scalar.Model
				sess.Provider = e.Scalar.Provider
			}
		}
	}

	if updated.IsZero() {
		updated = info.ModTime()
	}
	sess.UpdatedAt = updated
	if sess.ID == "" {
		sess.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if firstPrompt != "" && sess.Title == "" {
		sess.Title = firstPrompt
	}
	return sess
}

func firstTextOf(m *ai.Message) string {
	for _, p := range m.Content {
		if p.Type == ai.ContentTypeText && strings.TrimSpace(p.Text) != "" {
			r := strings.ReplaceAll(p.Text, "\n", " ")
			r = strings.TrimSpace(r)
			if len(r) > 200 {
				r = r[:200] + "…"
			}
			return r
		}
	}
	return ""
}
