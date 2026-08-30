package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/mcp"
	"github.com/iSundram/Automergent/internal/session"
)

// Run is the Bubble Tea program entry point.
func Run(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool, projectApprovalPath string, mcpOrch *mcp.Orchestrator) error {

	// Enter alternate screen immediately before TUI starts to prevent
	// any flash of existing terminal content
	fmt.Print("\x1b[?1049h") // Enter alt screen
	fmt.Print("\x1b[H")      // Move cursor to home position
	fmt.Print("\x1b[2J")     // Clear entire screen

	app := NewApp(cfg, ag, sess, storage, persist, initialPrompt, showSessionPicker, mcpOrch)
	app.registerSessionCommands()
	app.requireProjectApproval(projectApprovalPath)
	p := tea.NewProgram(app)
	app.sendToProgram = p.Send
	// Notification hooks fire on backend goroutines; p.Send is the only safe way
	// in, and it only exists now. This is also why the old init()-time hook was
	// dead: there was no program to send to at package-init time.
	app.installNotifications()
	app.installQuestionnaire()
	_, err := p.Run()

	// Exit alternate screen after TUI ends
	fmt.Print("\x1b[?1049l")
	return err
}

// pendingAsk couples one structured ask_user call with its reply channel.
