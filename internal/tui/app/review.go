package app

// Edit-review flow (proposal accept/reject UX).
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/editreview"
	"github.com/iSundram/Automergent/internal/tui/render"
)

func (a *App) handleEditReviewKeys(m tea.KeyMsg) bool {
	switch m.String() {
	case "a":
		a.resolveCurrentProposal(true)
		return true
	case "u", "U":
		a.resolveCurrentProposal(false)
		return true
	case "n":
		a.openEditReview()
		return true
	case "A":
		if store := a.ag.EditReviewStore(); store != nil {
			for store.PendingCount() > 0 {
				a.reviewingProposal = store.Pending()[0].ID
				a.resolveCurrentProposal(true)
			}
			a.reviewingProposal = ""
		}
		return true
	case "esc", "q":
		a.reviewingProposal = ""
		if a.diffPane.Visible() {
			a.diffPane.Toggle()
		}
		a.layout()
		return true
	}
	return false
}

// openEditReview shows the oldest pending proposal; repeated invocations cycle.
func (a *App) openEditReview() {
	store := a.ag.EditReviewStore()
	if store == nil {
		a.conversation.AddMessage("system", "Edit review mode is off (set editReview: true).", false)
		return
	}
	pending := store.Pending()
	if len(pending) == 0 {
		a.conversation.AddMessage("system", "No proposed edits awaiting review.", false)
		return
	}
	p := pending[0]
	a.reviewingProposal = p.ID
	a.diffPane.SetContent(fmt.Sprintf("# %s — %s\n# %s\n\n%s\n\n[a] accept  [u] reject  [n] next  [A] accept all  [U] reject all",
		p.ID, render.FileLink(p.Path), p.Summary, p.UnifiedDiff()))
	if !a.diffPane.Visible() {
		a.diffPane.Toggle()
	}
	a.layout()
}

// resolveCurrentProposal accepts/rejects the proposal currently displayed and
// advances to the next pending one, if any.
func (a *App) resolveCurrentProposal(accept bool) {
	store := a.ag.EditReviewStore()
	if store == nil || a.reviewingProposal == "" {
		return
	}
	status := editreview.StatusRejected
	verb := "rejected"
	if accept {
		status = editreview.StatusAccepted
		verb = "accepted"
	}
	p, err := store.Resolve(a.reviewingProposal, status)
	if err != nil {
		a.conversation.AddMessage("system", "review: "+err.Error(), true)
		return
	}
	if accept {
		if err := editreview.Apply(p); err != nil {
			a.conversation.AddMessage("system", fmt.Sprintf("apply %s failed: %v", p.ID, err), true)
			return
		}
	} else if a.sess != nil {
		// Rejection feeds back into the conversation for the model.
		note := ai.NewTextMessage(ai.RoleSystem, editreview.RevertNote(p))
		a.sess.AddMessage(note)
	}
	a.conversation.AddMessage("system", fmt.Sprintf("%s %s (%s)", p.ID, verb, p.Path), false)

	if remaining := store.PendingCount(); remaining > 0 {
		a.openEditReview() // advance to next
	} else {
		a.reviewingProposal = ""
		if a.diffPane.Visible() {
			a.diffPane.Toggle()
		}
	}
	a.layout()
}

// refreshGitBranch updates the HUD branch segment (cached, 5s TTL).
