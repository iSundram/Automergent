package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// ConversationMsg is an entry in the conversation view.
type ConversationMsg struct {
	Role      string
	Content   string
	IsError   bool
	Timestamp time.Time
}

// Conversation is a scrollable conversation viewport.
type Conversation struct {
	viewport viewport.Model
	messages []ConversationMsg
	styles   *themes.Styles
	width    int
	height   int
	// streaming accumulates the current in-progress assistant message
	streaming bool
}

// NewConversation creates a new Conversation component.
func NewConversation(styles *themes.Styles) Conversation {
	vp := viewport.New(80, 20)
	vp.KeyMap.Up.SetKeys("up")
	vp.KeyMap.Down.SetKeys("down")
	vp.KeyMap.PageUp.SetKeys("pgup")
	vp.KeyMap.PageDown.SetKeys("pgdown")
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
	c.streaming = false
	c.messages = append(c.messages, ConversationMsg{
		Role:      role,
		Content:   content,
		IsError:   isError,
		Timestamp: time.Now(),
	})
	c.refresh()
	c.viewport.GotoBottom()
}

// AddToolCall shows a tool call in the conversation.
func (c *Conversation) AddToolCall(name, args string) {
	c.streaming = false
	label := fmt.Sprintf("⚙  %s", name)
	if args != "" {
		label += " " + args
	}
	c.messages = append(c.messages, ConversationMsg{
		Role:      "tool_call",
		Content:   label,
		Timestamp: time.Now(),
	})
	c.refresh()
	c.viewport.GotoBottom()
}

// AppendToken appends to the last message (for streaming).
func (c *Conversation) AppendToken(token string) {
	if len(c.messages) == 0 || !c.streaming {
		c.messages = append(c.messages, ConversationMsg{
			Role:      "assistant",
			Content:   token,
			Timestamp: time.Now(),
		})
		c.streaming = true
	} else {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			last.Content += token
		} else {
			c.messages = append(c.messages, ConversationMsg{
				Role:      "assistant",
				Content:   token,
				Timestamp: time.Now(),
			})
			c.streaming = true
		}
	}
	c.refresh()
	c.viewport.GotoBottom()
}

// Clear removes all messages.
func (c *Conversation) Clear() {
	c.messages = nil
	c.streaming = false
	c.refresh()
}

func (c *Conversation) refresh() {
	var sb strings.Builder
	w := c.width
	if w <= 0 {
		w = 80
	}

	msgW := w - 4
	if msgW < 20 {
		msgW = 20
	}

	for i, m := range c.messages {
		_ = i
		switch m.Role {
		case "user":
			label := c.styles.UserLabel.Render("  You")
			ts := c.styles.Timestamp.Render(m.Timestamp.Format("15:04"))
			header := lipgloss.JoinHorizontal(lipgloss.Bottom, label, "  ", ts)
			content := c.styles.UserBubble.Width(msgW).Render(m.Content)
			sb.WriteString(header + "\n" + content + "\n\n")

		case "assistant":
			label := c.styles.AssistantLabel.Render("  Automergent")
			ts := c.styles.Timestamp.Render(m.Timestamp.Format("15:04"))
			header := lipgloss.JoinHorizontal(lipgloss.Bottom, label, "  ", ts)
			rendered := render.Markdown(m.Content)
			if m.IsError {
				rendered = c.styles.Error.Render(m.Content)
			}
			sb.WriteString(header + "\n" + rendered + "\n")

		case "system":
			sb.WriteString(c.styles.SystemMsg.Width(msgW).Render("  "+m.Content) + "\n\n")

		case "tool_call":
			sb.WriteString(c.styles.ToolCall.Render(m.Content) + "\n")

		case "tool_result":
			if m.IsError {
				sb.WriteString(c.styles.Error.Render("  ✗ "+m.Content) + "\n\n")
			} else {
				sb.WriteString(c.styles.ToolResult.Render("  ✓ "+m.Content) + "\n\n")
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
