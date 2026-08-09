package components

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type ConversationMsg struct {
	Role      string
	Content   string
	IsError   bool
	Timestamp time.Time
}

type Conversation struct {
	viewport   viewport.Model
	messages   []ConversationMsg
	styles     *themes.Styles
	width      int
	height     int
	streaming  bool
	reviewMode bool
}

func NewConversation(styles *themes.Styles) Conversation {
	vp := viewport.New(80, 20)
	vp.KeyMap.Up.SetKeys("up")
	vp.KeyMap.Down.SetKeys("down")
	return Conversation{viewport: vp, styles: styles}
}

func (c *Conversation) SetSize(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	c.width = w
	c.height = h
	c.viewport.Width = w
	c.viewport.Height = h
	c.refresh()
}

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

func (c *Conversation) AddToolCall(name, args string) {
	c.streaming = false
	args = truncateContent(args, c.reviewMode)
	c.messages = append(c.messages, ConversationMsg{
		Role:      "tool_call",
		Content:   fmt.Sprintf("▸ ⚙  %s %s", name, args),
		Timestamp: time.Now(),
	})
	c.refresh()
	c.viewport.GotoBottom()
}

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
			c.messages = append(c.messages, ConversationMsg{Role: "assistant", Content: token, Timestamp: time.Now()})
			c.streaming = true
		}
	}
	c.refresh()
	c.viewport.GotoBottom()
}

func (c *Conversation) Clear() {
	c.messages = nil
	c.streaming = false
	c.refresh()
}

// SetReviewMode toggles detailed tool output rendering.
func (c *Conversation) SetReviewMode(enabled bool) {
	c.reviewMode = enabled
	c.refresh()
}

// ReviewMode reports whether detailed tool output is enabled.
func (c Conversation) ReviewMode() bool {
	return c.reviewMode
}

func (c *Conversation) refresh() {
	var sb strings.Builder
	w := c.width
	if w <= 0 {
		w = 80
	}
	msgW := w - 10
	if msgW < 20 {
		msgW = 20
	}

	for _, m := range c.messages {
		switch m.Role {
		case "user":
			label := c.styles.UserLabel.Render(" You ")
			content := c.styles.UserBubble.Width(msgW).Render(m.Content)

			// Right alignment logic
			fullWidth := lipgloss.Width(content)
			labelWidth := lipgloss.Width(label)

			// Spacer to push label right
			labelSpacer := strings.Repeat(" ", w-labelWidth-2)
			sb.WriteString(labelSpacer + label + "\n")

			// Spacer to push bubble right
			contentSpacer := strings.Repeat(" ", w-fullWidth-2)
			for _, line := range strings.Split(content, "\n") {
				if line != "" {
					sb.WriteString(contentSpacer + line + "\n")
				}
			}
			sb.WriteString("\n")

		case "assistant":
			labelStr := " ◈ Automergent "
			bubbleStyle := c.styles.AssistantBubble

			if m.IsError {
				labelStr = " ◈ Automergent (Error) "
				bubbleStyle = bubbleStyle.Copy().BorderForeground(c.styles.T.Red)
			}

			label := c.styles.AssistantLabel.Render(labelStr)
			rendered := render.Markdown(m.Content)
			if m.IsError {
				rendered = c.styles.Error.Render(m.Content)
			}

			bubble := bubbleStyle.Width(msgW).Render(rendered)
			sb.WriteString(label + "\n" + bubble + "\n\n")

		case "system":
			sb.WriteString(c.styles.SystemMsg.Width(msgW).Render("  "+m.Content) + "\n\n")

		case "tool_call":
			sb.WriteString(c.styles.ToolCall.Render("  "+m.Content) + "\n")

		case "tool_result":
			icon := " ✓ "
			style := c.styles.ToolResult
			if m.IsError {
				icon = " ✗ "
				style = c.styles.Error
			}
			sb.WriteString(style.Render(icon+m.Content) + "\n\n")
		}
	}
	c.viewport.SetContent(sb.String())
}

func (c Conversation) Update(msg tea.Msg) (Conversation, tea.Cmd) {
	vp, cmd := c.viewport.Update(msg)
	c.viewport = vp
	return c, cmd
}

func (c Conversation) View() string {
	return c.viewport.View()
}

// MessageCount returns the number of conversation entries.
func (c Conversation) MessageCount() int {
	return len(c.messages)
}

// LastMessage returns the most recent conversation entry.
func (c Conversation) LastMessage() (ConversationMsg, bool) {
	if len(c.messages) == 0 {
		return ConversationMsg{}, false
	}
	return c.messages[len(c.messages)-1], true
}

func truncateContent(s string, reviewMode bool) string {
	if reviewMode {
		return s
	}
	const maxRunes = 220
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + " … [truncated, press Ctrl+R for full review mode]"
}
