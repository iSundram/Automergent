package app

// Queue strip wiring: making the pending-message queue visible and retractable.
//
// The queue itself lives in msgQueue (queue.go). This file only mirrors it into
// the strip component and implements the two per-item actions the strip
// advertises: drop (ctrl+x) and pull back into the prompt for editing (ctrl+o).
// Both act on the highlighted item, so the user can see exactly which promise
// is being broken before it is.

import (
	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// refreshQueueStrip mirrors the message queue into the strip. Called after
// anything that mutates msgQueue — enqueue, boundary promotion, drop, drain,
// clear — so the strip can never show a queue the app has already forgotten.
func (a *App) refreshQueueStrip() {
	if a.queueStrip == nil {
		return
	}
	items := make([]components.QueueItem, len(a.msgQueue))
	for i, m := range a.msgQueue {
		items[i] = components.QueueItem{
			Text:     m.text,
			Boundary: m.boundary,
			IsCmd:    m.isCmd,
		}
	}
	a.queueStrip.SetItems(items)
}

// handleQueueStripKeys claims the strip's keys whenever the queue is visible:
// shift+up/shift+down move the highlight, ctrl+x drops the highlighted message,
// ctrl+o pulls it back into the prompt. Reports whether the key was consumed.
//
// These keys work while the input is focused — the whole point is that you can
// prune the queue without leaving the prompt — so they are checked before the
// textarea sees them, and only when there is a queue to act on.
func (a *App) handleQueueStripKeys(m tea.KeyMsg) (tea.Cmd, bool) {
	if a.queueStrip == nil || a.queueStrip.Len() == 0 {
		return nil, false
	}
	switch m.String() {
	case "shift+up":
		a.queueStrip.MoveCursor(-1)
		return nil, true
	case "shift+down":
		a.queueStrip.MoveCursor(1)
		return nil, true
	case "ctrl+x":
		return nil, a.dropQueuedItem()
	case "ctrl+o":
		return nil, a.pullBackQueuedItem()
	}
	return nil, false
}

// dropQueuedItem discards the highlighted queued message. Reports whether
// anything was dropped.
func (a *App) dropQueuedItem() bool {
	_, idx, ok := a.queueStrip.Selected()
	if !ok {
		return false
	}
	removed := a.msgQueue[idx]
	a.msgQueue = append(a.msgQueue[:idx], a.msgQueue[idx+1:]...)
	a.refreshQueueStrip()
	a.refreshChrome()
	a.statusBar.SetStatus("Dropped: " + firstNonEmptyDock(removed.text, "queued message"))
	return true
}

// pullBackQueuedItem removes the highlighted queued message and puts its text
// back in the prompt for editing. Reports whether anything was pulled back.
//
// Pulling back replaces the input's current contents: the two are competing
// drafts of the same intent, and silently concatenating them would produce a
// message the user never wrote.
func (a *App) pullBackQueuedItem() bool {
	_, idx, ok := a.queueStrip.Selected()
	if !ok {
		return false
	}
	pulled := a.msgQueue[idx]
	a.msgQueue = append(a.msgQueue[:idx], a.msgQueue[idx+1:]...)
	a.input.SetValue(pulled.text)
	a.updateActiveTokens()
	a.refreshQueueStrip()
	a.refreshChrome()
	a.statusBar.SetStatus("Editing queued message")
	return true
}
