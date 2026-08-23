package components

// Confirmation is the UI-side answer to a permission prompt. Components stay
// UI-pure: the app layer converts this to agent.ConfirmationResponse when
// replying into the agent's confirmation channel.
type Confirmation struct {
	Allow    bool
	Always   bool
	Feedback string
}
