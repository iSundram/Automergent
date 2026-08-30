package app

import (
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
	Kind      string
	Status    components.ArtifactStatus
	UpdatedAt time.Time
	SizeBytes int64
	Lines     int
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

// isArtifactWrite decides whether a completed write_file call registers an
// artifact: everything under .automergent/artifacts/, plus loose document files
// (excluding conventional project docs and dotfiles).
func isArtifactWrite(tool, path string) bool {
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
// conversation chip plus the status-bar review counter.
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
	// Re-read the preview content eagerly so /artifact is instant.
	ab := a.artifactBrowser
	ab.SetPreview(path, strings.Split(strings.TrimRight(string(data), "\n"), "\n"))
	a.artifactBrowser = ab

	for i := range a.artifacts {
		if a.artifacts[i].Path == rec.Path {
			// A rewrite after feedback returns the artifact to pending.
			wasDecided := a.artifacts[i].Status != components.ArtifactPending
			a.artifacts[i] = rec
			if wasDecided {
				a.refreshArtifactChrome()
			}
			return
		}
	}
	a.artifacts = append(a.artifacts, rec)
	a.conversation.AddMessage("system", fmt.Sprintf(
		"Artifact ready: %s — /artifact to review", filepath.Base(path)), false)
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
		if ar.Status == components.ArtifactPending {
			n++
		}
	}
	return n
}

func (a *App) artifactRows() []components.ArtifactRow {
	rows := make([]components.ArtifactRow, 0, len(a.artifacts))
	for _, ar := range a.artifacts {
		rows = append(rows, components.ArtifactRow{
			Name:      filepath.Base(ar.Path),
			Path:      ar.Path,
			Kind:      ar.Kind,
			Status:    ar.Status,
			UpdatedAt: ar.UpdatedAt,
			SizeBytes: ar.SizeBytes,
			Lines:     ar.Lines,
		})
	}
	// Newest first, pending before decided.
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := rows[i].Status == components.ArtifactPending, rows[j].Status == components.ArtifactPending
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
}

// seedArtifactsFromDisk picks up artifacts written by earlier sessions under
// the conventional directory. Statuses start pending.
func (a *App) seedArtifactsFromDisk() {
	if a.workDir == "" {
		return
	}
	dir := filepath.Join(a.workDir, artifactsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		a.registerArtifact(filepath.Join(dir, e.Name()))
	}
}

// showArtifacts opens the /artifact review browser.
func (a *App) showArtifacts() {
	a.artifactBrowser.SetArtifacts(a.artifactRows())
	a.artifactBrowser.Show()
	a.swallowNextKey = true
	a.layout()
}

// applyArtifactDecision records one approve/reject outcome.
func (a *App) applyArtifactDecision(path string, approved bool) {
	for i := range a.artifacts {
		if a.artifacts[i].Path != path {
			continue
		}
		if approved {
			a.artifacts[i].Status = components.ArtifactApproved
		} else {
			a.artifacts[i].Status = components.ArtifactRejected
		}
		a.statusBar.SetStatus("Artifact " + map[bool]string{true: "approved", false: "rejected"}[approved] + ": " + filepath.Base(path))
	}
	a.refreshArtifactChrome()
}

// applyArtifactApproveAll approves every pending artifact.
func (a *App) applyArtifactApproveAll() {
	n := 0
	for i := range a.artifacts {
		if a.artifacts[i].Status == components.ArtifactPending {
			a.artifacts[i].Status = components.ArtifactApproved
			n++
		}
	}
	a.statusBar.SetStatus(fmt.Sprintf("Approved %d artifacts", n))
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
