package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ConversationMsg is an entry in the conversation view.
type ConversationMsg struct {
	Role    string
	Content string
	IsError bool
}

// Conversation is a scrollable conversation viewport.
type Conversation struct {
	viewport viewport.Model
	messages []ConversationMsg
	styles   *themes.Styles
	width    int
	height   int
}

// NewConversation creates a new Conversation component.
func NewConversation(styles *themes.Styles) Conversation {
	vp := viewport.New(80, 20)
	return Conversation{
		viewport: vp,
		styles:   styles,
	}
}

func (c *Conversation) SetSize(w, h int) {
	c.width = w
	c.height = h
	c.viewport.Width = w
	c.viewport.Height = h
	c.refresh()
}

// AddMessage appends a message to the conversation.
func (c *Conversation) AddMessage(role, content string, isError bool) {
	c.messages = append(c.messages, ConversationMsg{Role: role, Content: content, IsError: isError})
	c.refresh()
	c.viewport.GotoBottom()
}

// AppendToken appends to the last message (for streaming).
func (c *Conversation) AppendToken(token string) {
	if len(c.messages) == 0 {
		c.messages = append(c.messages, ConversationMsg{Role: "assistant", Content: token})
	} else {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			last.Content += token
		} else {
			c.messages = append(c.messages, ConversationMsg{Role: "assistant", Content: token})
		}
	}
	c.refresh()
	c.viewport.GotoBottom()
}

func (c *Conversation) refresh() {
	var sb strings.Builder
	w := c.width
	if w <= 0 {
		w = 80
	}
	for _, m := range c.messages {
		switch m.Role {
		case "user":
			prefix := c.styles.UserMsg.Render("You: ")
			sb.WriteString(prefix + m.Content + "\n\n")
		case "assistant":
			prefix := c.styles.AssistantMsg.Render("Automergent: ")
			sb.WriteString(prefix + m.Content + "\n\n")
		case "tool_call":
			sb.WriteString(c.styles.ToolCall.Render("⚙ "+m.Content) + "\n")
		case "tool_result":
			if m.IsError {
				sb.WriteString(c.styles.Error.Render("✗ "+m.Content) + "\n\n")
			} else {
				sb.WriteString(c.styles.ToolResult.Render("✓ "+m.Content) + "\n\n")
			}
		}
	}
	c.viewport.SetContent(lipgloss.NewStyle().Width(w).Render(sb.String()))
}

// Update handles viewport key events.
func (c Conversation) Update(msg tea.Msg) (Conversation, tea.Cmd) {
	vp, cmd := c.viewport.Update(msg)
	c.viewport = vp
	return c, cmd
}

// View renders the conversation.
func (c Conversation) View() string {
	return c.viewport.View()
}
