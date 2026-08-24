package components

// Coordination families: agent (task, read_agent, list_agents, agent_control),
// plan (plan, replan), todo (todo_write, todo_list, todo_next), taskstate
// (task_list, task_get, task_update), context (the context_* bookkeeping
// tools) and interaction (ask_user, notify).
//
// Status glyphs here deliberately match taskboard.go, so a todo shown inline
// in the log and the same todo on the side board read as one object.
//
// The context_* tools get the quietest treatment in the app: they are internal
// bookkeeping the user did not ask for, so they cost exactly one dim line and
// never open a body.

import (
	"encoding/json"
	"strings"
)

// renderAgentCard renders task / read_agent / list_agents / agent_control.
//
//	● Task  "map the TUI render layer"  ·  explore · done                 42s
//	  ⎿ Found 4 renderers: tool_read, tool_edit, tool_terminal, tool_box
func (c *Conversation) renderAgentCard(m ConversationMsg, width int) string {
	if m.ToolName == "list_agents" {
		return c.renderAgentRoster(m, width)
	}

	args := argsOf(m)
	subject := subjectFor(m)
	var chips []string

	if t, ok := args["agent_type"].(string); ok && t != "" {
		chips = append(chips, t)
	}
	if id := metaString(m, "agent_id"); id != "" {
		chips = append(chips, shortID(id))
	}
	if status := metaString(m, "status"); status != "" {
		chips = append(chips, status)
	}
	if mode := metaString(m, "mode"); mode == "background" {
		chips = append(chips, "background")
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || strings.TrimSpace(m.Content) == "" {
		return head
	}

	// A subagent's return value is prose; show the leading lines as its report.
	lines, hidden := firstLines(m.Content, c.bodyLimit(0))
	rows := make([]string, 0, len(lines)+1)
	for _, l := range lines {
		rows = append(rows, c.resultRow("", l, width))
	}
	if hidden > 0 {
		rows = append(rows, c.moreRow(hidden, "line"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderAgentRoster renders list_agents as a status-marked roster.
func (c *Conversation) renderAgentRoster(m ConversationMsg, width int) string {
	lines, _ := firstLines(m.Content, 1<<20)
	var chips []string
	if len(lines) > 0 && !m.IsError {
		chips = append(chips, plural(len(lines), "agent"))
	}
	head := c.callLine(m, width, "", chips, durationChip(m.Duration))
	if !c.showDetail() || len(lines) == 0 {
		return head
	}

	limit := c.bodyLimit(len(lines))
	rows := make([]string, 0, limit+1)
	for i, l := range lines {
		if i >= limit {
			break
		}
		rows = append(rows, c.resultRow(c.todoMark(statusWord(l)), l, width))
	}
	if more := len(lines) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "agent"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderPlanCard renders plan / replan as a numbered step list.
//
//	● Plan  6 steps                                                      0.4s
//	  ✓ Inventory all tools
//	  ▸ Design the row grammar
//	  ○ Implement per-family renderers
func (c *Conversation) renderPlanCard(m ConversationMsg, width int) string {
	steps := parsePlanSteps(m.Content)
	var chips []string
	if len(steps) > 0 {
		chips = append(chips, plural(len(steps), "step"))
	}
	head := c.callLine(m, width, "", chips, durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || len(steps) == 0 {
		return head
	}

	limit := c.bodyLimit(len(steps))
	rows := make([]string, 0, limit+1)
	for i, s := range steps {
		if i >= limit {
			break
		}
		rows = append(rows, c.plainRow(c.todoMark(s.status), s.text, width))
	}
	if more := len(steps) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "step"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderTodoCard renders todo_write / todo_list / todo_next.
//
//	● Todos  3 done · 1 in progress · 2 pending
//	  ✓ Inventory all tools
//	  ▸ Design the row grammar
func (c *Conversation) renderTodoCard(m ConversationMsg, width int) string {
	items := parseTodoItems(m.Content)

	// todo_write with action=status moves exactly one item; say so in one line
	// rather than reprinting the whole list.
	if args := argsOf(m); m.ToolName == "todo_write" {
		if action, _ := args["action"].(string); action == "status" {
			status, _ := args["status"].(string)
			subject := strings.TrimSpace(scalarString(args["id"]))
			head := c.callLine(m, width, subject,
				[]string{c.todoMark(status) + " " + status}, durationChip(m.Duration))
			return head
		}
	}

	done, active, pending, blocked := 0, 0, 0, 0
	for _, it := range items {
		switch strings.ToLower(it.status) {
		case "completed", "done":
			done++
		case "in_progress", "in progress":
			active++
		case "blocked":
			blocked++
		default:
			pending++
		}
	}
	var chips []string
	if done > 0 {
		chips = append(chips, itoa(done)+" done")
	}
	if active > 0 {
		chips = append(chips, itoa(active)+" in progress")
	}
	if pending > 0 {
		chips = append(chips, itoa(pending)+" pending")
	}
	if blocked > 0 {
		chips = append(chips, itoa(blocked)+" blocked")
	}

	head := c.callLine(m, width, "", chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() || len(items) == 0 {
		return head
	}

	limit := c.bodyLimit(len(items))
	rows := make([]string, 0, limit+1)
	for i, it := range items {
		if i >= limit {
			break
		}
		rows = append(rows, c.plainRow(c.todoMark(it.status), it.text, width))
	}
	if more := len(items) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "item"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderTaskStateCard renders task_list / task_get / task_update.
//
//	● Tasks  1 in progress · 3 pending
//	  ▸ #1  Redesign tool cards
//	● Task  #1  ·  ✓ completed
func (c *Conversation) renderTaskStateCard(m ConversationMsg, width int) string {
	args := argsOf(m)

	// task_update is a state transition: one line naming the new state.
	if m.ToolName == "task_update" {
		status, _ := args["status"].(string)
		subject := "#" + strings.TrimPrefix(scalarString(args["task_id"]), "#")
		var chips []string
		if status != "" {
			chips = append(chips, c.todoMark(status)+" "+status)
		}
		head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
		if m.IsError {
			return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
		}
		return head
	}

	items := parseTodoItems(m.Content)
	var chips []string
	if len(items) > 0 {
		chips = append(chips, plural(len(items), "task"))
	}
	head := c.callLine(m, width, subjectFor(m), chips, durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() {
		return head
	}
	if len(items) == 0 {
		if s := oneLine(m.Content); s != "" {
			return join(head, c.resultRow("", s, width))
		}
		return head
	}

	limit := c.bodyLimit(len(items))
	rows := make([]string, 0, limit+1)
	for i, it := range items {
		if i >= limit {
			break
		}
		rows = append(rows, c.plainRow(c.todoMark(it.status), it.text, width))
	}
	if more := len(items) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "task"))
	}
	return join(head, strings.Join(rows, "\n"))
}

// renderContextCard renders the context_* bookkeeping tools. These are
// machinery the user never asked for, so they get one dim line and no body —
// visible for auditability, invisible to the reading eye.
func (c *Conversation) renderContextCard(m ConversationMsg, width int) string {
	args := argsOf(m)
	action := strings.TrimPrefix(m.ToolName, "context_")
	action = strings.TrimPrefix(action, "bucket_")
	action = strings.ReplaceAll(action, "_", " ")

	subject := action
	if bucket, ok := args["bucket"].(string); ok && bucket != "" {
		subject += "  " + bucket
		if key, ok := args["key"].(string); ok && key != "" {
			subject += "." + key
		}
	} else if name, ok := args["name"].(string); ok && name != "" {
		subject += "  " + name
	}

	head := c.callLine(m, width, subject, nil, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	return head
}

// renderInteractionCard renders ask_user / notify.
//
//	● Ask  "Which theme should I use?"
//	  ⎿ catppuccin
//	● Notify  Build failed — see terminal  ·  warn
func (c *Conversation) renderInteractionCard(m ConversationMsg, width int) string {
	args := argsOf(m)
	subject := subjectFor(m)
	var chips []string

	if m.ToolName == "ask_user" {
		if subject != "" {
			subject = `"` + subject + `"`
		}
		// In compact mode the answer rides on the call line rather than being
		// dropped — the whole point of an ask_user entry is what the user said.
		if !c.showDetail() && strings.TrimSpace(m.Content) != "" && !m.IsError {
			chips = append(chips, glyphTo+" "+oneLine(m.Content))
		}
	} else if level, ok := args["level"].(string); ok && level != "" {
		chips = append(chips, level)
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if m.ToolName == "ask_user" && c.showDetail() && strings.TrimSpace(m.Content) != "" {
		return join(head, c.resultRow("", oneLine(m.Content), width))
	}
	return head
}

// --- parsing helpers ------------------------------------------------------

// planStep / todoItem are the shared shape of a status-marked list row.
type planStep struct {
	status string
	text   string
}

// parsePlanSteps reads planner output. The planner emits JSON when it can and
// numbered prose otherwise, so both are accepted.
func parsePlanSteps(content string) []planStep {
	if steps, ok := parseJSONSteps(content); ok {
		return steps
	}
	lines, _ := firstLines(content, 1<<20)
	var out []planStep
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Numbered ("1. ", "2) ") or bulleted ("- ", "• ") rows are steps.
		if n := leadingNumber(trimmed); n > 0 {
			out = append(out, planStep{status: "pending", text: stripLeader(trimmed)})
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") {
			out = append(out, planStep{status: "pending", text: stripLeader(trimmed)})
		}
	}
	return out
}

// parseJSONSteps handles planner output shaped as {"steps":[...]} or a bare
// array of objects with description/status fields.
func parseJSONSteps(content string) ([]planStep, bool) {
	t := strings.TrimSpace(content)
	if !looksLikeJSON(t) {
		return nil, false
	}
	type entry struct {
		Description string `json:"description"`
		Title       string `json:"title"`
		Step        string `json:"step"`
		Status      string `json:"status"`
	}
	var wrapper struct {
		Steps []entry `json:"steps"`
		Plan  []entry `json:"plan"`
	}
	entries := []entry(nil)
	if json.Unmarshal([]byte(t), &wrapper) == nil {
		if len(wrapper.Steps) > 0 {
			entries = wrapper.Steps
		} else if len(wrapper.Plan) > 0 {
			entries = wrapper.Plan
		}
	}
	if entries == nil {
		var bare []entry
		if json.Unmarshal([]byte(t), &bare) != nil || len(bare) == 0 {
			return nil, false
		}
		entries = bare
	}
	out := make([]planStep, 0, len(entries))
	for _, e := range entries {
		text := firstNonEmptyStr(e.Description, e.Title, e.Step)
		if text == "" {
			continue
		}
		status := e.Status
		if status == "" {
			status = "pending"
		}
		out = append(out, planStep{status: status, text: text})
	}
	return out, len(out) > 0
}

// parseTodoItems reads the "- [status] description (pri=N)" rows emitted by
// todo_list, and the JSON shapes emitted by task_list / todo_next.
func parseTodoItems(content string) []planStep {
	if steps, ok := parseJSONSteps(content); ok {
		return steps
	}
	lines, _ := firstLines(content, 1<<20)
	var out []planStep
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") {
			continue
		}
		end := strings.IndexByte(trimmed, ']')
		if end < 0 {
			continue
		}
		status := trimmed[3:end]
		text := strings.TrimSpace(trimmed[end+1:])
		// Drop the "(pri=N)" tail — priority is not what the eye needs here.
		if i := strings.LastIndex(text, " (pri="); i > 0 {
			text = text[:i]
		}
		out = append(out, planStep{status: status, text: text})
	}
	return out
}

// statusWord finds a known status token in a free-text roster line.
func statusWord(line string) string {
	lower := strings.ToLower(line)
	for _, s := range []string{"completed", "running", "failed", "cancelled", "killed", "in_progress", "pending", "blocked"} {
		if strings.Contains(lower, s) {
			return s
		}
	}
	return "pending"
}

// shortID trims a long agent/shell id for display.
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// leadingNumber returns the leading "12." / "12)" number, or 0.
func leadingNumber(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(s) {
		return 0
	}
	if s[i] != '.' && s[i] != ')' {
		return 0
	}
	n := 0
	for _, r := range s[:i] {
		n = n*10 + int(r-'0')
	}
	return n
}

// stripLeader removes a numbered or bulleted row leader.
func stripLeader(s string) string {
	for _, prefix := range []string{"- ", "• ", "* "} {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}
	if i := strings.IndexAny(s, ".)"); i > 0 && i < 4 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

// itoa is a tiny int formatter kept local so chip building reads cleanly.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
