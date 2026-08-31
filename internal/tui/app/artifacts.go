package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/tui/components"
)

// artifactRecord is one agent-produced deliverable awaiting (or having
// received) user review. Files the agent writes land here automatically; the
// user approves or rejects them from /artifact.
type artifactRecord struct {
	Path      string
	Title     string
	Kind      string
	Status    components.ArtifactStatus
	UpdatedAt time.Time
	SizeBytes int64
	Lines     int
	// SessionID scopes the artifact to the session that created it.
	SessionID string
	// Comments are user remarks collected through /artifact.
	Comments []string
}

// artifactsDir is the conventional artifact home inside a project. Anything
// the agent writes there is an artifact regardless of extension; loose
// markdown files written anywhere else also register (plans, reviews, docs
// the user asked for).
const artifactsDir = ".automergent/artifacts"

// docExtensions: loose files with these extensions register as artifacts
// when the agent creates them with write_file.
var docExtensions = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".html": true, ".mermaid": true,
}

// noiseDocs are conventional project docs that must not register as
// artifacts even though they are markdown.
var noiseDocs = map[string]bool{
	"readme.md": true, "claude.md": true, "automergent.md": true, "contributing.md": true,
	"changelog.md": true, "license.md": true,
}

// artifactKind infers the artifact kind from its path. Matches are
// delimiter-bounded so "demo_artifact.md" is a document, not a "review"-art.
func artifactKind(path string) string {
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.Dir(path))
	stem := strings.TrimSuffix(base, strings.ToLower(filepath.Ext(path)))
	containsWord := func(s, word string) bool {
		return s == word || strings.HasPrefix(s, word+"-") || strings.HasPrefix(s, word+"_") ||
			strings.HasSuffix(s, "-"+word) || strings.HasSuffix(s, "_"+word) ||
			strings.Contains(s, "-"+word+"-") || strings.Contains(s, "_"+word+"_") ||
			strings.Contains(s, "-"+word+"_") || strings.Contains(s, "_"+word+"-") ||
			strings.Contains(s, "/"+word+"/")
	}
	switch {
	case containsWord(stem, "plan") || containsWord(dir, "plan"):
		return "plan"
	case containsWord(stem, "review") || containsWord(dir, "review"):
		return "review"
	case containsWord(stem, "design") || containsWord(stem, "spec"):
		return "design"
	case containsWord(stem, "summary") || containsWord(stem, "report"):
		return "summary"
	}
	return "document"
}

// isArtifactWrite decides whether a completed tool call registers an
// artifact: the artifact tool always, everything under
// .automergent/artifacts/, plus loose document files (excluding conventional
// project docs and dotfiles).
func isArtifactWrite(tool, path string) bool {
	if tool == "artifact" {
		return path != ""
	}
	if tool != "write_file" || path == "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if strings.Contains(clean, "/"+artifactsDir+"/") || strings.HasPrefix(clean, artifactsDir+"/") {
		return true
	}
	base := strings.ToLower(filepath.Base(path))
	if !docExtensions[strings.ToLower(filepath.Ext(path))] || noiseDocs[base] || strings.HasPrefix(base, ".") {
		return false
	}
	return true
}

// registerArtifact records (or refreshes) an artifact and surfaces it: a
// conversation chip plus the status-bar review counter. Artifacts are scoped
// to the session that created them; the registry rides in the session's
// metadata so resumed sessions see their own artifacts and nobody else's.
func (a *App) registerArtifact(path string) {
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	rec := artifactRecord{
		Path:      path,
		Kind:      artifactKind(path),
		UpdatedAt: time.Now(),
		SizeBytes: int64(len(data)),
		Lines:     strings.Count(string(data), "\n"),
	}
	if a.sess != nil {
		rec.SessionID = a.sess.ID
	}
	// Re-read the preview content eagerly so /artifact is instant.
	ab := a.artifactBrowser
	ab.SetPreview(path, strings.Split(strings.TrimRight(string(data), "\n"), "\n"))
	a.artifactBrowser = ab

	for i := range a.artifacts {
		if a.artifacts[i].Path == rec.Path {
			// A rewrite after feedback returns a plan to pending; its
			// comments survive so the review context is not lost.
			wasDecided := a.artifacts[i].Status != components.ArtifactPending
			rec.Title = a.artifacts[i].Title // explicit titles survive rewrites
			rec.Comments = a.artifacts[i].Comments
			a.artifacts[i] = rec
			if wasDecided {
				a.refreshArtifactChrome()
				a.persistArtifacts()
			}
			return
		}
	}
	a.artifacts = append(a.artifacts, rec)
	a.conversation.AddMessage("system", fmt.Sprintf(
		"Artifact ready: %s — /artifact to review", filepath.Base(path)), false)
	a.refreshArtifactChrome()
	a.persistArtifacts()
}

// artifactsMetaKey stores the session-scoped artifact registry in session
// metadata so it survives resumes and never leaks across sessions.
const artifactsMetaKey = "artifacts"

// persistedArtifact is the JSON form stored in session metadata.
type persistedArtifact struct {
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Kind     string `json:"kind"`
	Status   int    `json:"status"`
	Comments []string `json:"comments,omitempty"`
}

// persistArtifacts serializes the registry into the session's metadata.
func (a *App) persistArtifacts() {
	if a.sess == nil {
		return
	}
	if a.sess.Metadata == nil {
		a.sess.Metadata = map[string]string{}
	}
	out := make([]persistedArtifact, 0, len(a.artifacts))
	for _, ar := range a.artifacts {
		out = append(out, persistedArtifact{
			Path:     ar.Path,
			Title:    ar.Title,
			Kind:     ar.Kind,
			Status:   int(ar.Status),
			Comments: ar.Comments,
		})
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	a.sess.Metadata[artifactsMetaKey] = string(data)
}

// loadArtifactsForSession restores this session's artifact registry from its
// metadata. Called on startup, resume and /new.
func (a *App) loadArtifactsForSession() {
	a.artifacts = nil
	if a.sess == nil || a.sess.Metadata == nil {
		a.refreshArtifactChrome()
		return
	}
	raw, ok := a.sess.Metadata[artifactsMetaKey]
	if !ok || raw == "" {
		a.refreshArtifactChrome()
		return
	}
	var stored []persistedArtifact
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		a.refreshArtifactChrome()
		return
	}
	for _, p := range stored {
		rec := artifactRecord{
			Path:     p.Path,
			Title:    p.Title,
			Kind:     p.Kind,
			Comments: p.Comments,
		}
		switch p.Status {
		case int(components.ArtifactApproved):
			rec.Status = components.ArtifactApproved
		case int(components.ArtifactRejected):
			rec.Status = components.ArtifactRejected
		default:
			rec.Status = components.ArtifactPending
		}
		if data, err := os.ReadFile(rec.Path); err == nil {
			rec.SizeBytes = int64(len(data))
			rec.Lines = strings.Count(string(data), "\n")
			ab := a.artifactBrowser
			ab.SetPreview(rec.Path, strings.Split(strings.TrimRight(string(data), "\n"), "\n"))
			a.artifactBrowser = ab
		}
		rec.SessionID = a.sess.ID
		a.artifacts = append(a.artifacts, rec)
	}
	a.refreshArtifactChrome()
}

// refreshArtifactChrome pushes the pending count into the status bar and the
// browser rows (when open).
func (a *App) refreshArtifactChrome() {
	a.statusBar.SetArtifacts(a.pendingArtifactCount())
	if a.artifactBrowser.Visible() {
		a.artifactBrowser.SetArtifacts(a.artifactRows())
	}
}

func (a *App) pendingArtifactCount() int {
	n := 0
	for _, ar := range a.artifacts {
		if isPlanKind(ar.Kind) && ar.Status == components.ArtifactPending {
			n++
		}
	}
	return n
}

func (a *App) artifactRows() []components.ArtifactRow {
	rows := make([]components.ArtifactRow, 0, len(a.artifacts))
	for _, ar := range a.artifacts {
		name := ar.Title
		if name == "" {
			name = filepath.Base(ar.Path)
		}
		rows = append(rows, components.ArtifactRow{
			Name:      name,
			Path:      ar.Path,
			Kind:      ar.Kind,
			Status:    ar.Status,
			UpdatedAt: ar.UpdatedAt,
			SizeBytes: ar.SizeBytes,
			Lines:     ar.Lines,
			Comments:  len(ar.Comments),
		})
	}
	// Newest first, pending plans before everything else.
	sort.SliceStable(rows, func(i, j int) bool {
		pi := rows[i].NeedsDecision() && rows[i].Status == components.ArtifactPending
		pj := rows[j].NeedsDecision() && rows[j].Status == components.ArtifactPending
		if pi != pj {
			return pi
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	return rows
}

// maybeRegisterArtifact inspects a completed tool call for an artifact write.
// Called from the EventToolDone handler.
func (a *App) maybeRegisterArtifact(td agent.ToolDoneEvent) {
	if td.Result.IsError || !isArtifactWrite(td.Name, td.Context) {
		return
	}
	a.registerArtifact(td.Context)
	// The artifact tool carries explicit metadata; prefer it over inferred
	// title/kind and surface the review request.
	if td.Name == "artifact" && td.Result.Metadata != nil {
		path := filepath.Clean(td.Context)
		if title, _ := td.Result.Metadata["artifact_title"].(string); title != "" {
			for i := range a.artifacts {
				if a.artifacts[i].Path == path {
					a.artifacts[i].Title = title
					a.artifacts[i].Kind = artifactKindOverride(a.artifacts[i].Kind, td.Result.Metadata)
					break
				}
			}
		}
		if rf, _ := td.Result.Metadata["request_feedback"].(bool); rf {
			a.statusBar.SetStatus("Artifact review requested — /artifact")
		}
	}
}

// artifactKindOverride replaces the inferred kind with the tool-supplied one
// when valid.
func artifactKindOverride(current string, meta map[string]any) string {
	kind, _ := meta["artifact_kind"].(string)
	switch kind {
	case "plan", "review", "design", "summary", "document":
		return kind
	}
	return current
}

// showArtifacts opens the /artifact review browser.
func (a *App) showArtifacts() {
	a.artifactBrowser.SetArtifacts(a.artifactRows())
	a.artifactBrowser.Show()
	a.swallowNextKey = true
	a.layout()
}

// applyArtifactDecision records one approve/reject outcome and returns the
// tea.Cmd of the follow-up run it launches (approved/rejected plan
// implementation), so the caller keeps the agent event listener armed.
func (a *App) applyArtifactDecision(path string, approved bool, reason string) tea.Cmd {
	rec, ok := a.findArtifact(path)
	if !ok {
		return nil
	}
	if !isPlanKind(rec.Kind) {
		return nil // informational artifacts carry no approve/reject semantics
	}
	if approved {
		rec.Status = components.ArtifactApproved
		a.statusBar.SetStatus("Plan approved: " + filepath.Base(path))
	} else {
		if strings.TrimSpace(reason) == "" {
			return nil // rejects without a reason never land
		}
		rec.Status = components.ArtifactRejected
		rec.Comments = append(rec.Comments, "rejected: "+reason)
		a.statusBar.SetStatus("Plan rejected: " + filepath.Base(path))
	}
	a.persistArtifacts()
	a.refreshArtifactChrome()

	// A blocked exit_plan_mode call gets the decision (with the rejection
	// reason) directly; the model acts on it. Otherwise, an idle agent is
	// put to work on the plan.
	if a.pendingPlanReview != nil && a.pendingPlanReview.planPath == path {
		a.resolvePlanReview(approved, reason)
		return nil
	}
	if a.thinking {
		return nil
	}
	if approved {
		return a.startAgent(fmt.Sprintf(
			"The plan at %s was approved by the user. Implement it now: read the plan file, work through it step by step, and verify each change.", path))
	}
	return a.startAgent(fmt.Sprintf(
		"The plan at %s was rejected by the user. Revise it: %s. Rewrite the plan artifact addressing the feedback, then present it again for approval.",
		path, reason))
}

// applyArtifactComment records a user comment on an artifact and, while the
// agent is working, steers the remark into the running turn.
func (a *App) applyArtifactComment(path, text string) {
	rec, ok := a.findArtifact(path)
	if !ok || strings.TrimSpace(text) == "" {
		return
	}
	rec.Comments = append(rec.Comments, text)
	a.persistArtifacts()
	a.refreshArtifactChrome()
	a.statusBar.SetStatus("Comment added: " + filepath.Base(path))
	if a.ag != nil && a.thinking {
		a.ag.Steer(fmt.Sprintf("User comment on artifact %s: %s", filepath.Base(path), text))
	}
}

// findArtifact locates a record by path.
func (a *App) findArtifact(path string) (*artifactRecord, bool) {
	path = filepath.Clean(path)
	for i := range a.artifacts {
		if a.artifacts[i].Path == path {
			return &a.artifacts[i], true
		}
	}
	return nil, false
}

// isPlanKind reports whether a kind participates in approve/reject review.
func isPlanKind(kind string) bool { return kind == "plan" }

// applyArtifactApproveAll approves every pending plan.
func (a *App) applyArtifactApproveAll() {
	n := 0
	for i := range a.artifacts {
		if isPlanKind(a.artifacts[i].Kind) && a.artifacts[i].Status == components.ArtifactPending {
			a.artifacts[i].Status = components.ArtifactApproved
			n++
		}
	}
	a.statusBar.SetStatus(fmt.Sprintf("Approved %d plan%s", n, map[bool]string{true: "", false: "s"}[n == 1]))
	a.persistArtifacts()
	a.refreshArtifactChrome()
}

// openArtifactEditor launches the user's editor on the artifact; the TUI
// suspends and resumes around it.
func (a *App) openArtifactEditor(path string) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, path)
	c.Dir = a.workDir
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return artifactEditorDoneMsg{err: err.Error()}
		}
		// The file may have changed while edited — refresh it.
		return artifactEditorDoneMsg{path: path}
	})
}

type artifactEditorDoneMsg struct {
	path string
	err  string
}

func (a *App) handleArtifactEditorDone(m artifactEditorDoneMsg) {
	if m.err != "" {
		a.statusBar.SetStatus("Editor failed: " + m.err)
		return
	}
	a.registerArtifact(m.path)
	a.statusBar.SetStatus("Editor closed")
}
