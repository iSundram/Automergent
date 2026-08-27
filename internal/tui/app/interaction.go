package app

// Interactive panes: questionnaire (ask_user) and taskboard.
// Moved verbatim from internal/tui/app.go.

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	toolsagent "github.com/iSundram/Automergent/internal/tools/agent"
	"github.com/iSundram/Automergent/internal/tools/interaction"
	"github.com/iSundram/Automergent/internal/tui/components"
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

// refreshTaskBoard pulls the live subagent roster from the manager.
func (a *App) refreshTaskBoard() {
	rows := make([]components.AgentRow, 0)
	for _, inst := range toolsagent.GetAgentManager().List(true) {
		snap := inst.Snapshot()
		rows = append(rows, components.AgentRow{
			ID:        snap.ID,
			Name:      snap.Name,
			Type:      snap.Type,
			Status:    snap.Status,
			Turns:     snap.Turns,
			Elapsed:   snap.Elapsed,
			StartedAt: snap.StartedAt,
		})
	}
	a.taskBoard.SetAgents(rows)
}

// handleTaskBoardKeys implements the m/f/i/k grammar while the board shows.
// Reports whether the key was consumed.
func (a *App) handleTaskBoardKeys(m tea.KeyMsg) bool {
	if !a.taskBoard.Visible() || a.inputFocused() {
		return false
	}
	switch m.String() {
	case "j", "down":
		return a.taskBoard.MoveFocus(1)
	case "k", "up":
		return a.taskBoard.MoveFocus(-1)
	case "i": // interrupt focused agent
		if ag, ok := a.taskBoard.FocusedAgent(); ok {
			out := toolsagent.ControlAction("interrupt", ag.ID, "", "sync")
			a.conversation.AddMessage("system", out, false)
			a.refreshTaskBoard()
			a.layout()
			return true
		}
	}
	return false
}

// inputFocused reports whether keyboard focus belongs to the prompt.
func (a *App) inputFocused() bool { return a.focus == "input" }

// handleEditReviewKeys consumes the accept/reject grammar while a proposal
// is displayed. Reports whether the key was handled.
