package tui

import (
	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	app "github.com/iSundram/Automergent/internal/tui/app"
)

// Run launches the TUI application (public entry point used by cmd/).
func Run(cfg *config.Config, ag *agent.Agent, sess *session.Session, storage *session.Storage, persist *session.PersistenceManager, initialPrompt string, showSessionPicker bool, projectApprovalPath string) error {
	return app.Run(cfg, ag, sess, storage, persist, initialPrompt, showSessionPicker, projectApprovalPath)
}

// RunProjectApproval presents the workspace trust gate before the TUI starts.
func RunProjectApproval(cfg *config.Config, projectPath string) (approved, remember bool, err error) {
	return app.RunProjectApproval(cfg, projectPath)
}
