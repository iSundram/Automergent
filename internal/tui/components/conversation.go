package components

import (
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	ToolID      string
	ToolName    string
	ToolArgs    string
	ToolContext string
	ToolSummary string
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

func (c *Conversation) refreshWithFollow(shouldFollow bool) {
	c.refresh()
	if shouldFollow {
		c.viewport.GotoBottom()
	}
}

func NewConversation(styles *themes.Styles) Conversation {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = false // Enforce keyboard-only scrolling
	vp.KeyMap.Up.SetKeys("up")
	vp.KeyMap.Down.SetKeys("down")
	return Conversation{viewport: vp, styles: styles}
}

func (c *Conversation) SetSize(w, h int) {
	shouldFollow := c.viewport.AtBottom()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	c.width = w
	c.height = h
	c.viewport.SetWidth(w)
	c.viewport.SetHeight(h)
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AddMessage(role, content string, isError bool) {
	shouldFollow := c.viewport.AtBottom()
	c.streaming = false
	c.messages = append(c.messages, ConversationMsg{
		Role:      role,
		Content:   content,
		IsError:   isError,
		Timestamp: time.Now(),
	})
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AddToolLifecycleStart(id, name, args, context string) {
	shouldFollow := c.viewport.AtBottom()
	c.streaming = false
	if id != "" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "tool_call" && c.messages[i].Status == "running" && c.messages[i].ToolID == id {
				return
			}
		}
	} else if n := len(c.messages); n > 0 {
		last := c.messages[n-1]
		if last.Role == "tool_call" && last.Status == "running" &&
			last.ToolName == name && last.ToolArgs == args && last.ToolContext == context {
			return
		}
	}
	c.messages = append(c.messages, ConversationMsg{
		Role:        "tool_call",
		ToolID:      id,
		ToolName:    name,
		ToolArgs:    args,
		ToolContext: context,
		Status:      "running",
		Timestamp:   time.Now(),
	})
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AddToolLifecycleDone(id, name, context, summary string, duration time.Duration, result tools.Result, reviewMode bool) {
	shouldFollow := c.viewport.AtBottom()
	c.streaming = false
	if id != "" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "tool_call" && c.messages[i].Status == "running" && c.messages[i].ToolID == id {
				c.messages[i].Status = "done"
				if result.IsError {
					c.messages[i].Status = "error"
					c.messages[i].IsError = true
				}
				c.messages[i].Duration = duration
				c.messages[i].Content = result.Content
				if context != "" {
					c.messages[i].ToolContext = context
				}
				if summary != "" {
					c.messages[i].ToolSummary = summary
				}
				c.refreshWithFollow(shouldFollow)
				return
			}
		}
	}
	// Fallback: match latest running tool call with same name.
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "tool_call" && c.messages[i].ToolName == name && c.messages[i].Status == "running" {
			c.messages[i].Status = "done"
			if result.IsError {
				c.messages[i].Status = "error"
				c.messages[i].IsError = true
			}
			c.messages[i].Duration = duration
			c.messages[i].Content = result.Content
			if context != "" {
				c.messages[i].ToolContext = context
			}
			if summary != "" {
				c.messages[i].ToolSummary = summary
			}
			c.refreshWithFollow(shouldFollow)
			return
		}
	}

	// Fallback if not found
	status := "done"
	if result.IsError {
		status = "error"
	}
	c.messages = append(c.messages, ConversationMsg{
		Role:        "tool_call",
		ToolID:      id,
		ToolName:    name,
		ToolContext: context,
		ToolSummary: summary,
		Content:     result.Content,
		Status:      status,
		IsError:     result.IsError,
		Duration:    duration,
		Timestamp:   time.Now(),
	})
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AppendToken(token string) {
	shouldFollow := c.viewport.AtBottom()
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
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AppendThought(thought string) {
	shouldFollow := c.viewport.AtBottom()
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
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) Clear() {
	c.messages = nil
	c.streaming = false
	c.refresh()
}

// FinalizeStreaming ends streaming mode and re-renders to apply markdown.
func (c *Conversation) FinalizeStreaming() {
	if c.streaming {
		c.streaming = false
		c.refresh()
	}
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

	lastIdx := len(c.messages) - 1
	for i, m := range c.messages {
		isLast := i == lastIdx
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
			// Render thinking separately (if present) without the Automergent label
			if m.Thought != "" {
				thinkingBox := c.renderThoughtBox(m.Thought, msgW)
				sb.WriteString(thinkingBox + "\n")
			}

			// Render the main response bubble if there's actual content
			if m.Content != "" || m.IsError {
				labelStr := " ⟡ Automergent "
				bubbleStyle := c.styles.AssistantBubble

				if m.IsError {
					labelStr = " ⟡ Automergent (Error) "
					bubbleStyle = bubbleStyle.Copy().BorderForeground(c.styles.T.Red)
				}

				label := c.styles.AssistantLabel.Render(labelStr)

				// Skip expensive markdown rendering during streaming for performance
				var content string
				if c.streaming && isLast {
					// During streaming, just show raw text
					content = strings.TrimSpace(m.Content)
				} else {
					// Render markdown for completed messages
					content = render.Markdown(strings.TrimSpace(m.Content))
				}
				if m.IsError {
					content = c.styles.Error.Render(strings.TrimSpace(m.Content))
				}

				bubble := bubbleStyle.Width(msgW).Render(content)
				sb.WriteString(label + "\n" + bubble + "\n")
			} else if m.Thought != "" {
				// If we only have thinking (no response yet), add spacing
				sb.WriteString("\n")
			}

		case "system":
			sb.WriteString(c.styles.SystemMsg.Width(msgW).Render("  "+m.Content) + "\n\n")

		case "tool_call":
			sb.WriteString(c.renderToolCall(m, msgW) + "\n\n")
		}
	}
	c.viewport.SetContent(sb.String())
}

func (c *Conversation) renderThoughtBox(thought string, width int) string {
	if thought == "" {
		return ""
	}
	trimmed := strings.TrimSpace(thought)
	if trimmed == "" {
		return ""
	}

	// Render thinking header OUTSIDE the box (like ⟡ Automergent label)
	header := c.styles.Dim.Copy().Bold(true).Render("💭 Thinking")

	// Apply markdown rendering like the main response does
	renderedContent := render.Markdown(trimmed)

	// Ensure we have content to show
	if strings.TrimSpace(renderedContent) == "" {
		renderedContent = trimmed // Fallback to raw text if markdown returns empty
	}

	// Wrap ONLY the content in the box (use Width like the response bubble does)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.BorderNormal).
		Width(width).
		Padding(0, 1).
		Render(renderedContent)

	// Return header above box (matching Automergent label pattern)
	return header + "\n" + box
}

func (c *Conversation) renderThought(thought string, width int) string {
	if thought == "" {
		return ""
	}
	maxWidth := width - 2
	if maxWidth < 1 {
		maxWidth = 1
	}
	trimmed := strings.TrimSpace(thought)
	if trimmed == "" {
		return ""
	}

	// Keep thought text visually aligned with assistant bubble theme.
	body := c.styles.AssistantMsg.Copy().
		Foreground(c.styles.T.Subtext).
		Italic(true).
		MaxWidth(maxWidth).
		Render(trimmed)

	header := c.styles.Dim.Copy().Bold(true).Render("Thinking")
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

func (c *Conversation) renderToolCall(m ConversationMsg, width int) string {
	icon := " 󱓞 " // Executing icon
	statusColor := c.styles.T.Yellow
	statusText := "running"

	// Fallback icons if not using Nerd Fonts (simulated check via a config or just provide clean characters)
	// For Automergent, we will use characters that look good in all terminals but still unique.

	switch m.Status {
	case "done":
		icon = " 󰄬 "
		statusText = "done"
		statusColor = c.styles.T.Green
	case "error":
		icon = " 󱄊 "
		statusText = "failed"
		statusColor = c.styles.T.Red
	}

	// Simple check: if we are in a basic terminal environment, use standard brackets
	// In some environments, we might want to check an env var like AUTOMERGENT_NO_NERD_FONTS
	// But here we'll ensure the icon string is at least printable.

	// Icon with background
	iconStyled := lipgloss.NewStyle().
		Foreground(c.styles.T.Background).
		Background(statusColor).
		Render(icon)

	// Tool name and context
	nameStyled := c.styles.ToolName.Render(" " + m.ToolName)
	if m.ToolContext != "" {
		// If we have context, make it the primary description for better readability
		desc := m.ToolContext
		if m.Status == "done" {
			switch m.ToolName {
			case "view", "read_file":
				desc = "viewed " + desc
			case "write_file", "create_file":
				desc = "wrote " + desc
			case "edit_file":
				desc = "edited " + desc
			case "delete_file":
				desc = "deleted " + desc
			case "move_file":
				desc = "moved " + desc
			case "copy_file":
				desc = "copied " + desc
			case "list_directory":
				desc = "listed " + desc
			}
		}
		nameStyled = c.styles.ToolName.Render(" " + desc)
	}

	// Status and duration
	statusTextDisp := statusText
	if m.ToolSummary != "" {
		statusTextDisp = m.ToolSummary
	}

	// Hide redundant "done" if we have a descriptive summary
	if m.Status == "done" && m.ToolSummary != "" {
		statusTextDisp = m.ToolSummary
	} else if m.Status == "done" {
		statusTextDisp = "completed"
	}

	statusStyled := c.styles.ToolStatus.Foreground(statusColor).Render(statusTextDisp)
	duration := ""
	if m.Duration > 0 {
		duration = c.styles.ToolDuration.Render(m.Duration.Round(time.Millisecond).String())
	}

	// Build a cleaner header: [ICON] [ACTION/CONTEXT] [SUMMARY] [DURATION]
	header := iconStyled + nameStyled + " " + statusStyled
	if duration != "" {
		header += " " + duration
	}

	var body strings.Builder
	body.WriteString(header)

	// Tool Arguments (only show if reviewMode is ON or it's running)
	if c.reviewMode {
		argText := m.ToolArgs
		if argText != "" && argText != "{}" {
			body.WriteString("\n\n" + lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Bold(true).Render(" ARGS"))
			body.WriteString("\n" + render.Code(argText, "json"))
		}
	}

	// Tool Result/Content
	if m.Content != "" {
		if c.reviewMode {
			body.WriteString("\n\n" + lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(" RESULTS"))
			body.WriteString("\n" + m.Content)
		} else if m.Status == "running" && (m.ToolName == "write_file" || m.ToolName == "edit_file") {
			// Show stylized diff box only while running/previewing
			body.WriteString("\n\n")
			lines := strings.Split(strings.TrimSpace(m.Content), "\n")
			for _, line := range lines {
				style := lipgloss.NewStyle()
				if strings.HasPrefix(line, "+") {
					style = style.Foreground(c.styles.T.Green)
				} else if strings.HasPrefix(line, "-") {
					style = style.Foreground(c.styles.T.Red)
				} else {
					style = style.Foreground(c.styles.T.Muted)
				}
				body.WriteString(style.Render(line) + "\n")
			}
		}
	}

	// Apply side accent and padding
	return c.styles.ToolAccent.
		BorderForeground(statusColor).
		Width(width - 2).
		Render(body.String())
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
