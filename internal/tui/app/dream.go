package app

// Memory consolidation: periodically rewrites the project memory file
// (AUTOMERGENT.md) from the session's accumulated learnings.
//
// Gates run cheapest-first — time since last consolidation, then volume of
// new conversation, then a cross-process lock — so an idle session never
// pays for a check it cannot pass. Both the automatic idle trigger and the
// manual /dream command fork a subagent with its own context, keeping the
// main conversation untouched.

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
)

// Consolidation bounds.
const (
	dreamMinInterval    = 30 * time.Minute
	dreamMinNewMessages = 20
	dreamLockMaxAge     = 30 * time.Minute
	// dreamMaxEntryLines caps the memory entrypoint so it stays cheap to
	// load into every future session's context.
	dreamMaxEntryLines = 200
)

// dreamLockPath is the cross-process consolidation lock.
func (a *App) dreamLockPath() string {
	return filepath.Join(a.workDir, ".automergent", "dream.lock")
}

// acquireDreamLock takes the consolidation lock; a stale lock (holder died)
// is broken after dreamLockMaxAge.
func (a *App) acquireDreamLock() bool {
	path := a.dreamLockPath()
	if info, err := os.Stat(path); err == nil {
		if time.Since(info.ModTime()) < dreamLockMaxAge {
			return false // someone else is mid-consolidation
		}
		_ = os.Remove(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return true
}

func (a *App) releaseDreamLock() {
	_ = os.Remove(a.dreamLockPath())
}

// maybeConsolidateMemory fires on agent idle. Gates: no run in flight, the
// interval has elapsed, and enough new conversation accumulated since the
// last pass.
func (a *App) maybeConsolidateMemory() {
	if a.dreamRunning {
		return
	}
	if !a.dreamLastAt.IsZero() && time.Since(a.dreamLastAt) < dreamMinInterval {
		return
	}
	if a.sess == nil {
		return
	}
	if len(a.sess.Messages)-a.dreamMsgBaseline < dreamMinNewMessages {
		return
	}
	a.startDream(true)
}

// ConsolidateMemory backs /dream — the manual trigger skips the interval
// gate but still respects the lock (two consolidations must not interleave).
func (a *App) ConsolidateMemory() {
	if a.dreamRunning {
		a.conversation.AddMessage("system", "A memory consolidation is already running.", false)
		return
	}
	a.startDream(false)
}

func (a *App) startDream(auto bool) {
	if !a.acquireDreamLock() {
		if !auto {
			a.conversation.AddMessage("system", "Memory consolidation is locked by another process — try again shortly.", false)
		}
		return
	}
	a.dreamRunning = true
	a.dreamLastAt = time.Now()
	a.dreamMsgBaseline = len(a.sess.Messages)

	kind := "manual"
	if auto {
		kind = "auto"
	}
	a.statusBar.SetStatus("Memory consolidation (" + kind + ")")

	go func() {
		defer func() {
			a.releaseDreamLock()
			a.dreamRunning = false
		}()

		prompt := dreamPrompt(a.workDir, len(a.sess.Messages))
		out, err := a.ag.Execute(a.ctx, toolsagent.AgentTypeGeneralPurpose, prompt, "")
		var msg string
		if err != nil {
			msg = "✗ Memory consolidation failed: " + err.Error()
		} else {
			msg = "✓ Memory consolidated (" + kind + "):\n" + out
		}
		if a.sendToProgram != nil {
			a.sendToProgram(dreamDoneMsg{text: msg})
		}
	}()
}

// dreamPrompt instructs the forked consolidator. It runs with file tools in
// the project directory; the cap is stated explicitly because a memory file
// that outgrows its budget taxes every future session.
func dreamPrompt(workDir string, sessionMessages int) string {
	return fmt.Sprintf(`Consolidate the project memory file for %s.

1. Read AUTOMERGENT.md (create it from /init conventions if absent).
2. Review the recent session history (%d messages) for durable, project-level
   knowledge: architecture decisions, conventions, gotchas, tooling quirks,
   recurring user preferences. Ignore one-off task details.
3. Rewrite AUTOMERGENT.md merging the existing content with the new durable
   knowledge: keep what is still true, drop what is stale or duplicated,
   write new findings concisely.
4. HARD LIMIT: the file must stay under %d lines. If it would exceed the
   limit, compress: prefer tables and terse bullets over prose, and drop the
   least valuable sections first. The final line count must be reported.

Report back: the final line count, what was added, and what was dropped.`,
		workDir, sessionMessages, dreamMaxEntryLines)
}

// dreamDoneMsg carries a consolidation outcome to the main loop.
type dreamDoneMsg struct{ text string }
