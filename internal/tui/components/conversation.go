package components

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	emptyState string
	welcome    bool
	browsing   bool
	// Builders used during streaming to avoid quadratic concatenation
	currentBuilder        *strings.Builder
	currentThoughtBuilder *strings.Builder
}

func (c *Conversation) refreshWithFollow(shouldFollow bool) {
	c.refresh()
	if shouldFollow {
		c.viewport.GotoBottom()
	}
}

func NewConversation(styles *themes.Styles) Conversation {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.MouseWheelEnabled = true
	vp.KeyMap.Up.SetKeys("up")
	vp.KeyMap.Down.SetKeys("down")
	return Conversation{viewport: vp, styles: styles}
}

func (c *Conversation) ensureViewport() {
	if c.viewport.Width() == 0 && c.viewport.Height() == 0 {
		vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
		vp.MouseWheelEnabled = true
		vp.KeyMap.Up.SetKeys("up")
		vp.KeyMap.Down.SetKeys("down")
		c.viewport = vp
	}
}

func (c *Conversation) SetSize(w, h int) {
	c.ensureViewport()
	shouldFollow := c.viewport.AtBottom()
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	c.width = w
	c.height = h
	viewportWidth := w
	if c.browsing {
		viewportWidth--
	}
	if viewportWidth < 1 {
		viewportWidth = 1
	}
	c.viewport.SetWidth(viewportWidth)
	c.viewport.SetHeight(h)
	c.refreshWithFollow(shouldFollow)
}

// SetEmptyState sets content shown only while the conversation has no messages.
func (c *Conversation) SetEmptyState(content string) {
	c.emptyState = content
	c.welcome = false
	c.refresh()
}

// SetWelcomeState shows the structured new-session welcome until the first
// conversation message arrives.
func (c *Conversation) SetWelcomeState() {
	c.emptyState = "Automergent is ready"
	c.welcome = true
	c.refresh()
}

func (c *Conversation) SetBrowsing(enabled bool) {
	c.browsing = enabled
	c.viewport.MouseWheelEnabled = enabled
	if c.width > 0 {
		width := c.width
		if enabled {
			width--
		}
		if width < 1 {
			width = 1
		}
		c.viewport.SetWidth(width)
	}
	c.refresh()
}

func (c *Conversation) AddMessage(role, content string, isError bool) {
	c.FinalizeStreaming()
	c.ensureViewport()
	shouldFollow := c.viewport.AtBottom()
	c.messages = append(c.messages, ConversationMsg{
		Role:      role,
		Content:   content,
		IsError:   isError,
		Timestamp: time.Now(),
	})
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AddToolLifecycleStart(id, name, args, context string) {
	c.FinalizeStreaming()
	shouldFollow := c.viewport.AtBottom()
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
	c.FinalizeStreaming()
	shouldFollow := c.viewport.AtBottom()
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
	c.ensureViewport()
	shouldFollow := c.viewport.AtBottom()
	if len(c.messages) == 0 || !c.streaming {
		c.messages = append(c.messages, ConversationMsg{
			Role:      "assistant",
			Content:   "",
			Timestamp: time.Now(),
		})
		c.streaming = true
		c.currentBuilder = &strings.Builder{}
		c.currentBuilder.WriteString(token)
	} else {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			if c.currentBuilder == nil {
				c.currentBuilder = &strings.Builder{}
				c.currentBuilder.WriteString(last.Content)
			}
			c.currentBuilder.WriteString(token)
		} else {
			c.messages = append(c.messages, ConversationMsg{Role: "assistant", Content: "", Timestamp: time.Now()})
			c.streaming = true
			c.currentBuilder = &strings.Builder{}
			c.currentBuilder.WriteString(token)
		}
	}
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) AppendThought(thought string) {
	c.ensureViewport()
	shouldFollow := c.viewport.AtBottom()
	if len(c.messages) == 0 || !c.streaming {
		c.messages = append(c.messages, ConversationMsg{
			Role:      "assistant",
			Thought:   "",
			Timestamp: time.Now(),
		})
		c.streaming = true
		c.currentThoughtBuilder = &strings.Builder{}
		c.currentThoughtBuilder.WriteString(thought)
	} else {
		last := &c.messages[len(c.messages)-1]
		if last.Role == "assistant" {
			if c.currentThoughtBuilder == nil {
				c.currentThoughtBuilder = &strings.Builder{}
				c.currentThoughtBuilder.WriteString(last.Thought)
			}
			c.currentThoughtBuilder.WriteString(thought)
		} else {
			c.messages = append(c.messages, ConversationMsg{Role: "assistant", Thought: "", Timestamp: time.Now()})
			c.streaming = true
			c.currentThoughtBuilder = &strings.Builder{}
			c.currentThoughtBuilder.WriteString(thought)
		}
	}
	c.refreshWithFollow(shouldFollow)
}

func (c *Conversation) Clear() {
	c.messages = nil
	c.streaming = false
	c.currentBuilder = nil
	c.currentThoughtBuilder = nil
	c.refresh()
}

// FinalizeStreaming ends streaming mode and re-renders to apply markdown.
func (c *Conversation) FinalizeStreaming() {
	c.FinalizeStreamingWithContent("")
}

// FinalizeStreamingWithContent ends streaming and uses the provider's final
// response, when supplied, as the authoritative complete text.
func (c *Conversation) FinalizeStreamingWithContent(final string) {
	if c.streaming {
		// Flush builders to last message
		if len(c.messages) > 0 && strings.TrimSpace(final) != "" {
			last := &c.messages[len(c.messages)-1]
			last.Content = final
			c.currentBuilder = nil
		} else if c.currentBuilder != nil && len(c.messages) > 0 {
			last := &c.messages[len(c.messages)-1]
			last.Content = c.currentBuilder.String()
			c.currentBuilder = nil
		}
		if c.currentThoughtBuilder != nil && len(c.messages) > 0 {
			last := &c.messages[len(c.messages)-1]
			last.Thought = c.currentThoughtBuilder.String()
			c.currentThoughtBuilder = nil
		}
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

func (c *Conversation) UpdateToolContent(id, content string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].ToolID == id {
			c.messages[i].Content = content
			c.refresh()
			return
		}
	}
}

func (c *Conversation) refresh() {
	var sb strings.Builder
	c.ensureViewport()
	w := c.width
	if w <= 0 {
		w = 80
	}
	msgW := w - 10
	if msgW < 20 {
		msgW = 20
	}
	if len(c.messages) == 0 && c.emptyState != "" {
		if c.welcome {
			c.viewport.SetContent(c.renderWelcome(w))
			return
		}
		empty := lipgloss.NewStyle().
			Width(w).
			Align(lipgloss.Center).
			Foreground(c.styles.T.Subtext).
			PaddingTop(2).
			Render(c.emptyState)
		c.viewport.SetContent(empty)
		return
	}

	lastIdx := len(c.messages) - 1
	for i, m := range c.messages {
		isLast := i == lastIdx
		// If this is the active streaming assistant message, prefer builder content
		if isLast && c.streaming && m.Role == "assistant" {
			if c.currentBuilder != nil {
				m.Content = c.currentBuilder.String()
			}
			if c.currentThoughtBuilder != nil {
				m.Thought = c.currentThoughtBuilder.String()
			}
		}
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
			responseW := w - 2
			if responseW < 1 {
				responseW = 1
			}

			// Render thinking separately (if present) without the Automergent label
			if m.Thought != "" {
				thinkingBox := c.renderThoughtBox(m.Thought, msgW)
				sb.WriteString(thinkingBox + "\n")
			}

			// Render the main response bubble if there's actual content
			if m.Content != "" || m.IsError {
				if i > 0 && c.messages[i-1].Role == "tool_call" {
					sb.WriteString("\n")
				}
				labelStr := " ⟡ Automergent "

				if m.IsError {
					labelStr = " ⟡ Automergent (Error) "
				}

				label := c.styles.AssistantLabel.Render(labelStr)

				// Skip expensive markdown rendering during streaming for performance
				var content string
				if c.streaming && isLast {
					// During streaming, just show raw text
					content = ansi.Hardwrap(
						ansi.Wordwrap(strings.TrimSpace(m.Content), responseW, ""),
						responseW,
						true,
					)
				} else {
					// Render markdown for completed messages
					content = render.MarkdownWithWidth(strings.TrimSpace(m.Content), responseW)
				}
				if m.IsError {
					content = c.styles.Error.MaxWidth(responseW).Render(strings.TrimSpace(m.Content))
				}
				content = ansi.Hardwrap(content, responseW, true)

				response := c.styles.AssistantBubble.MaxWidth(responseW).Render(indentLines(content, 1))
				sb.WriteString(label + "\n" + response + "\n\n")
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

func (c *Conversation) renderWelcome(width int) string {
	contentW := min(64, width-8)
	if contentW < 28 {
		contentW = max(1, width-2)
	}

	brand := lipgloss.NewStyle().
		Foreground(c.styles.T.Accent).
		Bold(true).
		Render("⟡  AUTOMERGENT")
	title := lipgloss.NewStyle().
		Foreground(c.styles.T.Text).
		Bold(true).
		Render("Your workspace is ready")
	description := lipgloss.NewStyle().
		Foreground(c.styles.T.Subtext).
		Render("Describe what you want to build, fix, or explore.")

	shortcutKey := lipgloss.NewStyle().Foreground(c.styles.T.Accent).Bold(true)
	shortcutText := lipgloss.NewStyle().Foreground(c.styles.T.Muted)
	shortcuts := shortcutKey.Render("/ ") + shortcutText.Render("commands") +
		shortcutText.Render("   ·   ") + shortcutKey.Render("@ ") + shortcutText.Render("files") +
		shortcutText.Render("   ·   ") + shortcutKey.Render("? ") + shortcutText.Render("help")
	if contentW < 43 {
		shortcuts = shortcutKey.Render("/ ") + shortcutText.Render("commands") + "   " +
			shortcutKey.Render("@ ") + shortcutText.Render("files") + "   " +
			shortcutKey.Render("? ") + shortcutText.Render("help")
	}

	block := lipgloss.JoinVertical(lipgloss.Center,
		brand,
		"",
		title,
		description,
		"",
		shortcuts,
	)
	return lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		PaddingTop(2).
		Render(lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(block))
}

func indentLines(content string, spaces int) string {
	if content == "" || spaces <= 0 {
		return content
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
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
	// 1. Status & Color Logic
	statusColor := c.styles.T.Yellow
	statusText := "󱓞 Running"
	if m.Status == "done" {
		statusColor = c.styles.T.Green
		statusText = "󰄬 Completed"
	} else if m.Status == "error" {
		statusColor = c.styles.T.Red
		statusText = "󰅙 Failed"
	}

	// 2. Tool-Specific Branding & Pretty Naming
	icon := "󰆍"
	accentColor := c.styles.T.Accent
	prettyName := m.ToolName

	switch m.ToolName {
	case "read_file", "view":
		prettyName = "Readfile"
		icon = "󰈔"
		accentColor = c.styles.T.Blue
	case "write_file", "create_file":
		prettyName = "Write"
		icon = "󱇧"
		accentColor = c.styles.T.Green
	case "edit_file":
		prettyName = "Edit"
		icon = "󰛓"
		accentColor = c.styles.T.Yellow
	case "delete_file":
		prettyName = "Delete"
		icon = "󰆴"
		accentColor = c.styles.T.Red
	case "move_file":
		prettyName = "Move"
		icon = "󰪹"
		accentColor = c.styles.T.Blue
	case "copy_file":
		prettyName = "Copy"
		icon = "󰪹"
		accentColor = c.styles.T.Blue
	case "list_directory":
		prettyName = "List directory"
		icon = "󰉋"
		accentColor = c.styles.T.Blue
	case "search":
		prettyName = "Deep Search"
		icon = "󰍉"
		accentColor = c.styles.T.Magenta
	case "glob":
		prettyName = "Glob"
		icon = "󰈞"
		accentColor = c.styles.T.Blue
	case "grep", "grep_search":
		prettyName = "Search"
		icon = "󰍉"
		accentColor = c.styles.T.Magenta
	case "run_shell_command", "run_command", "bash":
		prettyName = "Run"
		icon = "󰆍"
		accentColor = c.styles.T.Yellow
	case "read_shell":
		prettyName = "Read shell"
		icon = "󰇯"
		accentColor = c.styles.T.Yellow
	case "write_shell":
		prettyName = "Write shell"
		icon = "󰇰"
		accentColor = c.styles.T.Yellow
	case "stop_shell":
		prettyName = "Stop shell"
		icon = "󰅙"
		accentColor = c.styles.T.Red
	case "git_commit":
		prettyName = "Git commit"
		icon = "󰊢"
		accentColor = c.styles.T.Red
	case "git_add":
		prettyName = "Git stage"
		icon = "󰊢"
		accentColor = c.styles.T.Green
	case "git_checkout":
		prettyName = "Git checkout"
		icon = "󰊢"
		accentColor = c.styles.T.Blue
	case "git_branch":
		prettyName = "Git branch"
		icon = "󰊢"
		accentColor = c.styles.T.Magenta
	case "git_stash":
		prettyName = "Git stash"
		icon = "󰊢"
		accentColor = c.styles.T.Yellow
	case "git_status":
		prettyName = "Git status"
		icon = "󰊢"
		accentColor = c.styles.T.Cyan
	case "git_diff":
		prettyName = "Git diff"
		icon = "󰊢"
		accentColor = c.styles.T.Yellow
	case "git_log":
		prettyName = "Git log"
		icon = "󰊢"
		accentColor = c.styles.T.Blue
	case "lsp_diagnostics", "lsp_symbols":
		prettyName = "LSP"
		icon = "󰘦"
		accentColor = c.styles.T.Cyan
	case "web_fetch", "web_search":
		prettyName = "Web"
		icon = "󰖟"
		accentColor = c.styles.T.Magenta
	case "sql":
		prettyName = "SQL"
		icon = "󰆼"
		accentColor = c.styles.T.Blue
	case "secrets_scan":
		prettyName = "Secrets scan"
		icon = "󰦝"
		accentColor = c.styles.T.Red
	case "dependency_audit":
		prettyName = "Audit"
		icon = "󰒺"
		accentColor = c.styles.T.Yellow
	case "task":
		prettyName = "Task"
		icon = "󰒋"
		accentColor = c.styles.T.Accent
	case "read_agent":
		prettyName = "Read agent"
		icon = "󰒋"
		accentColor = c.styles.T.Accent
	}

	// 3. Styles
	iconStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	statusStyle := lipgloss.NewStyle().Foreground(statusColor).Bold(true).Padding(0, 1)

	// 4. Header Construction
	nameStyled := c.styles.ToolName.Copy().Foreground(c.styles.T.Text).Render("  " + prettyName)
	headerLeft := lipgloss.JoinHorizontal(lipgloss.Center, iconStyle.Render(icon), nameStyled, statusStyle.Render(statusText))

	// Extract and Truncate Path for the right side
	pathText := m.ToolContext
	durationText := ""
	if m.Duration > 0 {
		durationText = " " + c.styles.ToolDuration.Render(m.Duration.Round(time.Millisecond).String())
	}

	availableWidth := width - 6
	leftWidth := lipgloss.Width(headerLeft)
	maxPathWidth := availableWidth - leftWidth - lipgloss.Width(durationText) - 2

	if maxPathWidth > 5 && pathText != "" {
		if utf8.RuneCountInString(pathText) > maxPathWidth {
			runes := []rune(pathText)
			pathText = "…" + string(runes[len(runes)-maxPathWidth+1:])
		}
		pathText = c.styles.Dim.Render(pathText)
	} else {
		pathText = ""
	}

	spacerWidth := availableWidth - leftWidth - lipgloss.Width(pathText) - lipgloss.Width(durationText)
	if spacerWidth < 1 {
		spacerWidth = 1
	}
	header := headerLeft + strings.Repeat(" ", spacerWidth) + pathText + durationText

	// 5. Detailed Body Construction
	var body strings.Builder
	hasBody := false
	appendField := func(label, value string, valueStyle lipgloss.Style) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if !hasBody {
			body.WriteString("\n\n")
		} else {
			body.WriteString("\n")
		}
		hasBody = true
		labelWidth := 10
		labelText := c.styles.Dim.Render(fmt.Sprintf("%-*s", labelWidth, label))
		lines := strings.Split(strings.TrimSpace(value), "\n")
		body.WriteString("  " + labelText + valueStyle.Render(lines[0]))
		continuation := "  " + strings.Repeat(" ", labelWidth)
		for _, line := range lines[1:] {
			body.WriteString("\n" + continuation + valueStyle.Render(line))
		}
	}

	isLightweight := m.ToolName == "read_file" || m.ToolName == "view" || m.ToolName == "list_directory"
	showDetails := c.reviewMode || !isLightweight

	// Section: Parameters
	if showDetails && m.ToolArgs != "" && m.ToolArgs != "{}" {
		var args map[string]any
		if err := json.Unmarshal([]byte(m.ToolArgs), &args); err == nil {
			for _, key := range []string{"path", "command", "pattern", "url", "query"} {
				if value, ok := args[key]; ok {
					appendField(strings.ToUpper(key[:1])+key[1:], fmt.Sprint(value), lipgloss.NewStyle().Foreground(c.styles.T.Text))
				}
			}
		}
		if c.reviewMode {
			appendField("Details", render.Code(m.ToolArgs, "json"), lipgloss.NewStyle())
		}
	}

	// Section: Output / Summary
	if showDetails && (m.Status == "done" || m.Status == "error") {
		if m.ToolSummary != "" {
			appendField("Result", m.ToolSummary, c.styles.Dim.Copy().Italic(true))
		}

		if m.Content != "" {
			resultText := m.Content
			if !c.reviewMode {
				lines := strings.Split(strings.TrimSpace(resultText), "\n")
				if len(lines) > 3 {
					resultText = strings.Join(lines[:3], "\n") + "\n  " + c.styles.Dim.Render("... (truncated, use Ctrl+R for full)")
				}
			}
			appendField("Output", resultText, lipgloss.NewStyle().Foreground(c.styles.T.Subtext).Faint(true))
		}
	} else if m.Status == "running" && (m.ToolName == "write_file" || m.ToolName == "edit_file") && m.Content != "" {
		appendField("Changes", "Proposed changes", c.styles.Dim.Copy())
		lines := strings.Split(strings.TrimSpace(m.Content), "\n")

		// Smart Fitting Logic:
		// Calculate a limit based on screen height (e.g., 20% of viewport height, min 5, max 15)
		limit := c.height / 5
		if limit < 5 {
			limit = 5
		}
		if limit > 15 {
			limit = 15
		}

		for i, line := range lines {
			if i >= limit {
				body.WriteString(fmt.Sprintf("\n  %s", c.styles.Dim.Render(fmt.Sprintf("... (%d more lines, see Diff pane)", len(lines)-i))))
				break
			}
			style := lipgloss.NewStyle().PaddingLeft(2)
			if strings.HasPrefix(line, "+") {
				style = style.Foreground(c.styles.T.Green)
			} else if strings.HasPrefix(line, "-") {
				style = style.Foreground(c.styles.T.Red)
			} else {
				style = style.Foreground(c.styles.T.Muted)
			}
			body.WriteString("\n" + style.Render(line))
		}
	}

	// 6. Assembly
	cardContent := header + body.String()

	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(accentColor).
		Padding(0, 1).
		MarginBottom(1).
		Width(width - 2).
		Render(cardContent)
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
	content := c.viewport.View()
	if !c.browsing || c.viewport.TotalLineCount() <= c.viewport.VisibleLineCount() {
		return content
	}
	trackHeight := c.viewport.Height()
	total := c.viewport.TotalLineCount()
	visible := c.viewport.VisibleLineCount()
	thumbHeight := visible * trackHeight / total
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	maxOffset := total - visible
	maxTop := trackHeight - thumbHeight
	thumbTop := 0
	if maxOffset > 0 {
		thumbTop = c.viewport.YOffset() * maxTop / maxOffset
	}
	bar := make([]string, trackHeight)
	for i := range bar {
		bar[i] = "░"
		if i >= thumbTop && i < thumbTop+thumbHeight {
			bar[i] = "█"
		}
	}
	trackStyle := lipgloss.NewStyle().Foreground(c.styles.T.Muted)
	thumbStyle := lipgloss.NewStyle().Foreground(c.styles.T.Accent)
	for i := range bar {
		if bar[i] == "█" {
			bar[i] = thumbStyle.Render(bar[i])
		} else {
			bar[i] = trackStyle.Render(bar[i])
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, content, strings.Join(bar, "\n"))
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
