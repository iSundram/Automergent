package components

// Per-role message rendering (user/assistant/thought) and text helpers.
// Moved verbatim from conversation.go.

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/iSundram/Automergent/internal/tui/render"
)

func (c *Conversation) renderUser(m ConversationMsg, msgW, w int) string {
	label := c.styles.UserLabel.Copy().MarginBottom(0).Render(" You ")
	content := c.styles.UserBubble.Width(msgW).Render(m.Content)

	fullWidth := lipgloss.Width(content)
	labelWidth := lipgloss.Width(label)

	labelSpacer := strings.Repeat(" ", max(0, w-labelWidth-2))
	sb := strings.Builder{}
	sb.WriteString(labelSpacer + label + "\n")

	contentSpacer := strings.Repeat(" ", max(0, w-fullWidth-2))
	for _, line := range strings.Split(content, "\n") {
		if line != "" {
			sb.WriteString(contentSpacer + line + "\n")
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// renderAssistant builds one assistant row. During streaming the completed
// portion is markdown-cached incrementally and only the trailing partial line
// is rewrapped, so long responses stay smooth.
func (c *Conversation) renderAssistant(m ConversationMsg, isLast bool, prevTool bool, msgW, w int) string {
	responseW := w - 2
	if responseW < 1 {
		responseW = 1
	}

	liveStreaming := c.streaming && isLast

	var sb strings.Builder
	if m.Thought != "" {
		if liveStreaming {
			sb.WriteString(c.renderLiveThought(m.Thought, responseW) + "\n")
		} else {
			sb.WriteString(c.renderThoughtBox(m.Thought, msgW) + "\n")
		}
	}

	if m.Content == "" && !m.IsError {
		if m.Thought != "" {
			sb.WriteString("\n")
		}
		return sb.String()
	}

	if prevTool {
		sb.WriteString("\n")
	}

	labelStr := " ⟡ Automergent "
	if m.IsError {
		labelStr = " ⟡ Automergent (Error) "
	}
	label := c.styles.AssistantLabel.Render(labelStr)

	var content string
	switch {
	case m.IsError:
		content = c.styles.Error.MaxWidth(responseW).Render(strings.TrimSpace(m.Content))
	case liveStreaming:
		content = c.streamingBody(m.Content, responseW)
	default:
		content = render.MarkdownWithWidth(strings.TrimSpace(m.Content), responseW)
	}
	content = ansi.Hardwrap(content, responseW, true)

	response := c.styles.AssistantBubble.MaxWidth(responseW).Render(indentLines(content, 1))
	sb.WriteString(label + "\n" + response + "\n\n")
	return sb.String()
}

// streamingBody renders partial assistant text: markdown for completed lines
// (cached until they change), plain wrapping for the trailing fragment.
func (c *Conversation) streamingBody(content string, width int) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	trimmedLeading := strings.HasPrefix(content, " ") // irrelevant; kept simple
	_ = trimmedLeading

	idx := strings.LastIndexByte(content, '\n')
	if idx < 0 {
		// No completed line yet: cheap wrap only.
		return wrapPlain(text, width)
	}

	stable := content[:idx]
	tail := strings.TrimRight(content[idx+1:], " \t")

	key := hashMessage(c.styleEpoch, 0, false, ConversationMsg{Content: stable}, fmt.Sprintf("mdw:%d", width))
	if key != c.streamMDKey || c.streamMDWidth != width || c.streamMDOut == "" {
		c.streamMDOut = render.MarkdownWithWidth(strings.TrimSpace(stable), width)
		c.streamMDKey = key
		c.streamMDWidth = width
	}

	out := c.streamMDOut
	if tail != "" {
		out += "\n" + wrapPlain(tail, width)
	}
	return out
}

func wrapPlain(s string, width int) string {
	return ansi.Hardwrap(ansi.Wordwrap(s, width, ""), width, true)
}

// renderLiveThought shows in-flight thinking as lightweight wrapped text;
// the markdown thought box is applied once the turn completes.
func (c *Conversation) renderLiveThought(thought string, width int) string {
	header := c.styles.Dim.Copy().Bold(true).Render("💭 Thinking")
	body := c.styles.Dim.Copy().Italic(true).Render(truncateTail(thought, width, 6))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(c.styles.T.BorderNormal).
		Width(width).
		Padding(0, 1).
		Render(body)
	return header + "\n" + box
}

// truncateTail keeps the last maxLines lines of growing text so the live
// thought box stays a fixed height.
func truncateTail(s string, width int, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width-4, "…")
	}
	return strings.Join(lines, "\n")
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

// toolBranding returns the icon, accent color and display name for a tool.
