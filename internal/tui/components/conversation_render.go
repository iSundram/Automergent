package components

// Per-role message rendering (user/assistant/thought) and text helpers.
// Moved verbatim from conversation.go.

import (
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
// blocks are markdown-rendered once each and the trailing partial line is styled
// inline, so long responses stay smooth and still look like markdown.
func (c *Conversation) renderAssistant(m ConversationMsg, isLast bool, prevTool bool, msgW, w int) string {
	// The body is indented one column inside the bubble, so it must be wrapped
	// to one column less than the bubble's width. Wrapping to responseW and then
	// indenting put every full line one cell over budget, which the terminal
	// resolved by wrapping it again.
	responseW := w - 2
	if responseW < 2 {
		responseW = 2
	}
	bodyW := responseW - 1

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

	// The label is the bare brand mark. The wordmark used to sit here too, but it
	// repeated on every assistant turn and the name adds nothing the mark does
	// not already say. Errors keep a word because a blue glyph alone does not
	// tell you what went wrong; BrandMark already ends in a space, so it is
	// appended directly.
	label := " " + c.styles.BrandMark()
	if m.IsError {
		label += c.styles.Error.Render("Error")
	}

	var content string
	switch {
	case m.IsError:
		content = c.styles.Error.MaxWidth(bodyW).Render(strings.TrimSpace(m.Content))
	case liveStreaming:
		content = c.streamingBody(m.Content, bodyW)
	default:
		// MarkdownStream, not MarkdownWithWidth: a finished answer must occupy
		// exactly the lines it occupied a tick earlier, while it was still
		// streaming. See render.MarkdownStream.
		content = render.MarkdownStream(strings.TrimSpace(m.Content), bodyW)
	}
	content = ansi.Hardwrap(content, bodyW, true)

	response := c.styles.AssistantBubble.MaxWidth(responseW).Render(indentLines(content, 1))
	sb.WriteString(label + "\n" + response + "\n\n")
	return sb.String()
}

// streamingBody renders partial assistant text through the incremental
// streamer: finalized blocks come from cache, the open block costs one render,
// and the partial line is styled inline so markers never show as literals.
func (c *Conversation) streamingBody(content string, width int) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	s := c.stream()
	s.SetWidth(width)
	// The streamer is fed by AppendToken. If a caller mutated the message
	// content some other way (session restore, a provider's final text arriving
	// mid-stream), the two have diverged — rebuild from the authoritative copy.
	if s.Raw() != content {
		s.Reset()
		s.SetWidth(width)
		s.Write(content)
	}
	return s.View(true)
}

func wrapPlain(s string, width int) string {
	return ansi.Hardwrap(ansi.Wordwrap(s, width, ""), width, true)
}

// renderLiveThought shows in-flight thinking as lightweight wrapped text;
// the markdown thought box is applied once the turn completes.
func (c *Conversation) renderLiveThought(thought string, width int) string {
	header := c.styles.Dim.Copy().Bold(true).Render("● Thinking")
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

	// Render thinking header OUTSIDE the box (like the brand label)
	header := c.styles.Dim.Copy().Bold(true).Render("● Thinking")

	// Markdown-render to the box's *inner* width. Rendering unwrapped and
	// letting lipgloss re-wrap split glamour's ANSI spans mid-sequence, which
	// leaked escape codes into the visible text.
	inner := width - 4 // 2 border columns + 2 padding columns
	if inner < 8 {
		inner = 8
	}
	renderedContent := render.MarkdownWithWidth(trimmed, inner)

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

	// Return header above box (matching the brand label pattern)
	return header + "\n" + box
}
