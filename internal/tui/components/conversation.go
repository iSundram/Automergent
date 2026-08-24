package components

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tools"
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
	// Metadata carries tools.Result.Metadata verbatim: match counts, file
	// counts, shell ids, exit codes, agent ids. Family renderers read it
	// instead of re-parsing Content.
	Metadata map[string]any
}

// msgRender caches the rendered output of one conversation row so unchanged
// messages are never re-rendered (markdown/glamour and card assembly are the
// expensive parts).
type msgRender struct {
	key      uint64
	width    int
	rendered string
}

// ExpandMode controls how much detail tool cards show.
type ExpandMode int

const (
	ExpandAuto    ExpandMode = iota // classic behavior (lightweight tools collapse)
	ExpandCompact                   // headers only
	ExpandFull                      // everything, like review mode
)

type Conversation struct {
	viewport   viewport.Model
	messages   []ConversationMsg
	cache      []msgRender
	styles     *themes.Styles
	width      int
	height     int
	streaming  bool
	reviewMode bool
	emptyState string
	browsing   bool
	expandMode ExpandMode
	// dirty marks that streamed content changed but has not been rendered
	// yet; rendering is coalesced onto a ~80ms tick by the App.
	dirty       bool
	dirtyFollow bool
	styleEpoch  uint64 // bumped on theme/style changes to invalidate cache
	// Builders used during streaming to avoid quadratic concatenation
	currentBuilder        *strings.Builder
	currentThoughtBuilder *strings.Builder
	// Incremental markdown cache for the streaming message: the completed
	// lines are expensive-rendered once and reused until they change.
	streamMDKey   uint64
	streamMDWidth int
	streamMDOut   string
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

// Invalidate drops all cached renders (call after a theme/style switch).
func (c *Conversation) Invalidate() {
	c.styleEpoch++
	c.cache = nil
	c.streamMDKey = 0
	c.streamMDOut = ""
	c.refresh()
}

// NeedsRender reports whether streamed content awaits a coalesced render.
func (c *Conversation) NeedsRender() bool { return c.dirty }

// RenderIfDirty flushes pending streamed content to the viewport at most once
// per caller-driven tick instead of once per token.
func (c *Conversation) RenderIfDirty() {
	if !c.dirty {
		return
	}
	follow := c.dirtyFollow
	c.refresh()
	if follow {
		c.viewport.GotoBottom()
	}
	c.dirty = false
}

// CycleExpand moves between auto → compact → full tool-detail modes and
// returns a human-readable label for the status bar.
func (c *Conversation) CycleExpand() string {
	switch c.expandMode {
	case ExpandAuto:
		c.expandMode = ExpandCompact
	case ExpandCompact:
		c.expandMode = ExpandFull
	default:
		c.expandMode = ExpandAuto
	}
	c.invalidateAll()
	switch c.expandMode {
	case ExpandCompact:
		return "Tool cards: collapsed"
	case ExpandFull:
		return "Tool cards: expanded"
	default:
		return "Tool cards: auto"
	}
}

// ExpandMode reports the current tool-detail mode.
func (c Conversation) ExpandMode() ExpandMode { return c.expandMode }

func (c *Conversation) invalidateAll() {
	c.cache = nil
	c.streamMDKey = 0
	c.streamMDOut = ""
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
	c.AddMessageFull(role, content, "", isError)
}

// AddMessageFull appends a message including an optional thought (thinking
// box). Used when restoring a session so resuming shows the conversation
// exactly as it did while running.
func (c *Conversation) AddMessageFull(role, content, thought string, isError bool) {
	c.FinalizeStreaming()
	c.ensureViewport()
	shouldFollow := c.viewport.AtBottom()
	c.messages = append(c.messages, ConversationMsg{
		Role:      role,
		Content:   content,
		Thought:   thought,
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
	apply := func(i int) {
		c.messages[i].Status = "done"
		if result.IsError {
			c.messages[i].Status = "error"
			c.messages[i].IsError = true
		}
		c.messages[i].Duration = duration
		c.messages[i].Content = result.Content
		c.messages[i].Metadata = result.Metadata
		if context != "" {
			c.messages[i].ToolContext = context
		}
		if summary != "" {
			c.messages[i].ToolSummary = summary
		}
		c.refreshWithFollow(shouldFollow)
	}
	if id != "" {
		for i := len(c.messages) - 1; i >= 0; i-- {
			if c.messages[i].Role == "tool_call" && c.messages[i].Status == "running" && c.messages[i].ToolID == id {
				apply(i)
				return
			}
		}
	}
	// Fallback: match latest running tool call with same name.
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "tool_call" && c.messages[i].ToolName == name && c.messages[i].Status == "running" {
			apply(i)
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
		Metadata:    result.Metadata,
		Status:      status,
		IsError:     result.IsError,
		Duration:    duration,
		Timestamp:   time.Now(),
	})
	c.refreshWithFollow(shouldFollow)
}

// AppendToken buffers streamed text. Rendering is deferred to RenderIfDirty
// so a burst of tokens costs one paint, not one paint per token.
func (c *Conversation) AppendToken(token string) {
	c.ensureViewport()
	c.dirtyFollow = c.viewport.AtBottom()
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
	c.dirty = true
}

// AppendThought buffers streamed thinking text; see AppendToken.
func (c *Conversation) AppendThought(thought string) {
	c.ensureViewport()
	c.dirtyFollow = c.viewport.AtBottom()
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
	c.dirty = true
}

func (c *Conversation) Clear() {
	c.messages = nil
	c.cache = nil
	c.streaming = false
	c.currentBuilder = nil
	c.currentThoughtBuilder = nil
	c.streamMDKey = 0
	c.streamMDOut = ""
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
		c.dirty = false
		c.streamMDKey = 0
		c.streamMDOut = ""
		c.refresh()
	}
}

// SetReviewMode toggles detailed tool output rendering.
func (c *Conversation) SetReviewMode(enabled bool) {
	c.reviewMode = enabled
	c.invalidateAll()
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

// hashMessage computes the cache key for a rendered row: any field the
// renderer reads participates, so mutations naturally invalidate.
func hashMessage(epoch uint64, expand ExpandMode, review bool, m ConversationMsg, spanExtra string) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d|%d|%t|", epoch, expand, review)
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\x00%s\x00%d\x00%d",
		m.Role, m.Content, m.Thought, m.ToolName, m.ToolArgs,
		m.ToolContext, m.ToolSummary, m.IsError, m.Status,
		m.Duration.Nanoseconds(), m.Timestamp.UnixNano())
	// Metadata drives family renderers, so it must participate — over sorted
	// keys, since map iteration order would otherwise churn the cache.
	for _, k := range metaKeys(m.Metadata) {
		fmt.Fprintf(h, "\x02%s\x00%v", k, m.Metadata[k])
	}
	if spanExtra != "" {
		h.Write([]byte(spanExtra))
	}
	sum := h.Sum(nil)
	var v uint64
	for _, b := range sum {
		v = v<<8 | uint64(b)
	}
	return v
}

// contentWidth computes the effective wrap width for message bodies.
func (c *Conversation) contentWidth() int {
	w := c.width
	if w <= 0 {
		w = 80
	}
	// Reserve one column for the scrollbar track when browsing so content is
	// wrapped to the same width the viewport actually displays.
	if c.browsing {
		w--
	}
	if w < 1 {
		w = 1
	}
	return w
}

func (c *Conversation) refresh() {
	var sb strings.Builder
	c.ensureViewport()
	w := c.contentWidth()
	msgW := w - 10
	if msgW < 20 {
		msgW = 20
	}
	if len(c.messages) == 0 {
		if c.emptyState != "" {
			empty := lipgloss.NewStyle().
				Width(w).
				Align(lipgloss.Center).
				Foreground(c.styles.T.Subtext).
				PaddingTop(2).
				Render(c.emptyState)
			c.viewport.SetContent(empty)
		} else {
			c.viewport.SetContent("")
		}
		c.cache = c.cache[:0]
		return
	}

	lastIdx := len(c.messages) - 1
	ci := 0

	writeCached := func(key uint64, renderBlock func() string) {
		for len(c.cache) <= ci {
			c.cache = append(c.cache, msgRender{})
		}
		if ent := &c.cache[ci]; ent.key == key && ent.width == w && ent.rendered != "" {
			sb.WriteString(ent.rendered)
		} else {
			out := renderBlock()
			*ent = msgRender{key: key, width: w, rendered: out}
			sb.WriteString(out)
		}
		ci++
	}

	i := 0
	prevRole := ""
	for i < len(c.messages) {
		m := c.messages[i]

		// Live streaming content overrides the stored message copy.
		if i == lastIdx && c.streaming && m.Role == "assistant" {
			if c.currentBuilder != nil {
				m.Content = c.currentBuilder.String()
			}
			if c.currentThoughtBuilder != nil {
				m.Thought = c.currentThoughtBuilder.String()
			}
		}

		// Collapse a run of consecutive finished calls that share a grouping
		// family (read_file + view merge; an edit between them breaks the run).
		if m.Role == "tool_call" && m.Status == "done" && groupsFor(m.ToolName) {
			key := groupKeyFor(m.ToolName)
			j := i + 1
			for j < len(c.messages) &&
				c.messages[j].Role == "tool_call" &&
				c.messages[j].Status == "done" &&
				groupKeyFor(c.messages[j].ToolName) == key {
				j++
			}
			if span := j - i; span > 1 {
				group := c.messages[i:j]
				var spanKey strings.Builder
				for _, g := range group {
					spanKey.WriteString(fmt.Sprintf("\x01%s\x00%s\x00%s\x00%s\x00%d",
						g.ToolName, g.ToolContext, g.ToolSummary, g.Status, g.Duration.Nanoseconds()))
					for _, k := range metaKeys(g.Metadata) {
						spanKey.WriteString(fmt.Sprintf("\x03%s\x00%v", k, g.Metadata[k]))
					}
				}
				hkey := hashMessage(c.styleEpoch, c.expandMode, c.reviewMode, m, spanKey.String())
				writeCached(hkey, func() string {
					return c.renderToolGroup(group, msgW) + "\n\n"
				})
				prevRole = "tool_call"
				i = j
				continue
			}
		}

		key := hashMessage(c.styleEpoch, c.expandMode, c.reviewMode, m, "")
		mm := m // copy for closure
		isLast := i == lastIdx
		afterTool := prevRole == "tool_call"
		writeCached(key, func() string {
			switch mm.Role {
			case "user":
				return c.renderUser(mm, msgW, w)
			case "assistant":
				return c.renderAssistant(mm, isLast, afterTool, msgW, w)
			case "system":
				return c.styles.SystemMsg.Width(msgW).Render("  "+mm.Content) + "\n\n"
			case "tool_call":
				return c.renderToolCall(mm, msgW) + "\n\n"
			default:
				return ""
			}
		})
		prevRole = m.Role
		i++
	}

	// Drop stale cache entries beyond the visible rows.
	if ci < len(c.cache) {
		c.cache = c.cache[:ci]
	}
	c.viewport.SetContent(sb.String())
}
