package components

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/iSundram/Automergent/internal/tools"
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

type ConversationMsg struct {
	Role        string
	Content     string
	Thought     string
	IsError     bool
	Timestamp   time.Time
	ToolName    string
	ToolArgs    string
	ToolContext string
	Duration    time.Duration
	Status      string // "running", "done", "error"
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
	vp.MouseWheelEnabled = false // Enforce keyboard-only scrolling
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

func (c *Conversation) AddToolCall(name, args, context string) {
	c.streaming = false
	c.messages = append(c.messages, ConversationMsg{
		Role:        "tool_call",
		ToolName:    name,
		ToolArgs:    args,
		ToolContext: context,
		Status:      "running",
		Timestamp:   time.Now(),
	})
	c.refresh()
	c.viewport.GotoBottom()
}

func (c *Conversation) AddToolLifecycleStart(name, args, context string) {
	c.streaming = false
	c.messages = append(c.messages, ConversationMsg{
		Role:        "tool_call",
		ToolName:    name,
		ToolArgs:    args,
		ToolContext: context,
		Status:      "running",
		Timestamp:   time.Now(),
	})
	c.refresh()
	c.viewport.GotoBottom()
}

func (c *Conversation) AddToolLifecycleDone(name string, duration time.Duration, result tools.Result, reviewMode bool) {
	c.streaming = false
	// Find the last running tool call with this name and update it
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "tool_call" && c.messages[i].ToolName == name && c.messages[i].Status == "running" {
			c.messages[i].Status = "done"
			if result.IsError {
				c.messages[i].Status = "error"
				c.messages[i].IsError = true
			}
			c.messages[i].Duration = duration
			c.messages[i].Content = result.Content
			c.refresh()
			return
		}
	}

	// Fallback if not found
	status := "done"
	if result.IsError {
		status = "error"
	}
	c.messages = append(c.messages, ConversationMsg{
		Role:      "tool_call",
		ToolName:  name,
		Content:   result.Content,
		Status:    status,
		IsError:   result.IsError,
		Duration:  duration,
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

func (c *Conversation) AppendThought(thought string) {
	if len(c.messages) == 0 || !c.streaming {
		c.messages = append(c.messages, ConversationMsg{
			Role:      "assistant",
			Thought:   thought,
			Timestamp: time.Now(),
		})
		c.streaming = true
	} else {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			last.Thought += thought
		} else {
			c.messages = append(c.messages, ConversationMsg{Role: "assistant", Thought: thought, Timestamp: time.Now()})
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

			var rendered strings.Builder
			if m.Thought != "" {
				rendered.WriteString(c.renderThought(m.Thought, msgW) + "\n")
			}

			content := render.Markdown(m.Content)
			if m.IsError {
				content = c.styles.Error.Render(m.Content)
			}
			rendered.WriteString(content)

			bubble := bubbleStyle.Width(msgW).Render(rendered.String())
			sb.WriteString(label + "\n" + bubble + "\n") // Single newline after bubble

		case "system":
			sb.WriteString(c.styles.SystemMsg.Width(msgW).Render("  "+m.Content) + "\n\n")

		case "tool_call":
			sb.WriteString(c.renderToolCall(m, msgW) + "\n\n")
		}
	}
	c.viewport.SetContent(sb.String())
}

func (c *Conversation) renderThought(thought string, width int) string {
	if thought == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(c.styles.T.Muted).
		Italic(true).
		Faint(true).
		Width(width - 2).
		Render("  " + strings.TrimSpace(thought))
}

func (c *Conversation) renderToolCall(m ConversationMsg, width int) string {
	icon := " ⚙ "
	statusColor := c.styles.T.Yellow
	statusText := "running"

	switch m.Status {
	case "done":
		icon = " ✓ "
		statusColor = c.styles.T.Green
		statusText = fmt.Sprintf("done (%s)", m.Duration.Round(time.Millisecond))
	case "error":
		icon = " ✗ "
		statusColor = c.styles.T.Red
		statusText = "failed"
	}

	// Compact header style: sharing the box background
	headerStyle := lipgloss.NewStyle().
		Foreground(statusColor).
		Bold(true)

	headerText := icon + m.ToolName
	if m.ToolContext != "" {
		headerText += ": " + m.ToolContext
	}
	header := headerStyle.Render(headerText)

	// Faint status text
	status := lipgloss.NewStyle().
		Foreground(statusColor).
		Faint(true).
		MarginLeft(1).
		Render(statusText)

	// Join header and status on one line
	topLine := lipgloss.JoinHorizontal(lipgloss.Bottom, header, status)

	// Tool Arguments (only show if reviewMode is ON)
	args := ""
	if c.reviewMode {
		argText := m.ToolArgs
		if argText != "" && argText != "{}" {
			args = "\n" + lipgloss.NewStyle().
				Foreground(c.styles.T.Subtext).
				Italic(true).
				Render("  "+argText)
		}
	}

	// Tool Result/Content (only show if reviewMode is ON)
	content := ""
	if c.reviewMode && m.Content != "" {
		content = "\n\n" + lipgloss.NewStyle().
			Foreground(c.styles.T.Text).
			Render(m.Content)
	}

	// Base box style: using Overlay for darker contrast and compact sizing
	boxStyle := lipgloss.NewStyle().
		Background(c.styles.T.Overlay).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(statusColor).
		Padding(0, 1).
		MaxWidth(width)

	return boxStyle.Render(topLine + args + content)
}

func (c Conversation) Update(msg tea.Msg) (Conversation, tea.Cmd) {
	switch msg.(type) {
	case tea.MouseMsg:
		return c, nil
	}
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
