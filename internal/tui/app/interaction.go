package app

// Interactive panes: questionnaire (ask_user) and taskboard.
// Moved verbatim from internal/tui/app.go.

import (
	"fmt"

	"github.com/iSundram/Automergent/internal/tools/interaction"
)

type pendingAsk struct {
	req   interaction.QuestionnaireRequest
	reply chan string
	done  chan struct{}
}

// installQuestionnaire registers the TUI as the ask_user UI. The tool-side
// goroutine blocks on reply/done; delivery into the event loop happens via
// sendToProgram (the tea.Program).
func (a *App) installQuestionnaire() {
	interaction.SetQuestionnaire(func(req interaction.QuestionnaireRequest) (string, error) {
		pa := &pendingAsk{
			req:   req,
			reply: make(chan string, 1),
			done:  make(chan struct{}),
		}
		if a.sendToProgram != nil {
			a.sendToProgram(askSessionMsg{pa})
		} else {
			return "", fmt.Errorf("no UI available")
		}
		select {
		case answer := <-pa.reply:
			return answer, nil
		case <-pa.done:
			return "", fmt.Errorf("user dismissed the question")
		}
	})
}

// askSessionMsg carries a new questionnaire request into the event loop.
type askSessionMsg struct{ pa *pendingAsk }

// registerSessionCommands adds the session-lifecycle commands (/rewind,
// /zen) without touching the core registry file.

// inputFocused reports whether keyboard focus belongs to the prompt.
func (a *App) inputFocused() bool { return a.focus == "input" }

// handleEditReviewKeys consumes the accept/reject grammar while a proposal
// is displayed. Reports whether the key was handled.
