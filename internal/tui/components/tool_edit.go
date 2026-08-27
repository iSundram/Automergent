package components

// Mutation families: edit (write_file, create_file, edit_file, multi_edit) and
// fileop (delete_file, move_file, copy_file).
//
// Edits are the one place where vertical space is always worth spending: the
// diff is the whole point of the log entry. Everything else about an edit —
// which file, how many occurrences, replace-all or not — compresses into the
// call line's chips.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/render"
)

// renderEditCard renders one file-mutation call.
//
//	● Edit  tool_box.go  ·  1 line replaced                              8ms
//	  +1 −1 ──────────────────────────────────────
//	   34 │ limit := c.tailLimit()
//	   35 │ limit := c.tailLimit() + 2
//	  ↑ first 3 hidden · ctrl+e expands
func (c *Conversation) renderEditCard(m ConversationMsg, width int) string {
	head := c.callLine(m, width, subjectFor(m), c.editChips(m), durationChip(m.Duration))

	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	if !c.showDetail() {
		return head
	}
	diff := editDiffText(m)
	if diff == "" {
		// Finished results carry prose summaries or validation banners, not
		// diff text — rebuild the preview from the arguments instead so the
		// log shows what changed, not just that something changed.
		diff = syntheticEditDiff(m)
	}
	if diff != "" {
		return join(head, c.diffBox(diff, width))
	}
	// No preview possible (summary-only result): fall back to a single result
	// row so the card still says what happened.
	if s := oneLine(m.ToolSummary); s != "" {
		return join(head, c.resultRow("", s, width))
	}
	return head
}

// syntheticEditDiff rebuilds a displayable diff from a mutation's arguments
// when the recorded result carries none: create/write render their content as
// an all-additions block, edit renders its old→new replacement through the
// shared dmp engine, multi_edit joins one labeled hunk per sub-edit.
func syntheticEditDiff(m ConversationMsg) string {
	args := argsOf(m)

	if content, ok := args["content"].(string); ok && strings.TrimSpace(content) != "" {
		return render.AllAdds(content)
	}

	if edits, ok := args["edits"].([]any); ok && len(edits) > 0 {
		var parts []string
		shown := 0
		for i, e := range edits {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			oldStr, _ := em["old_str"].(string)
			newStr, _ := em["new_str"].(string)
			if oldStr == "" || oldStr == newStr {
				continue
			}
			shown++
			parts = append(parts,
				render.HunkLabel("edit %d of %d", i+1, len(edits)),
				render.SimpleDiff(oldStr, newStr))
		}
		if shown > 0 {
			return strings.Join(parts, "\n")
		}
	}

	oldStr, _ := args["old_str"].(string)
	newStr, _ := args["new_str"].(string)
	if oldStr != "" && newStr != "" && oldStr != newStr {
		return render.SimpleDiff(oldStr, newStr)
	}
	return ""
}

// editChips describes the shape of the change from the arguments: how many
// edits in a multi_edit, how many lines a replacement spans, and whether it
// applied to every occurrence.
func (c *Conversation) editChips(m ConversationMsg) []string {
	args := argsOf(m)
	var chips []string

	if edits, ok := args["edits"].([]any); ok && len(edits) > 0 {
		chips = append(chips, plural(len(edits), "edit"))
	} else if old, ok := args["old_str"].(string); ok && old != "" {
		chips = append(chips, plural(strings.Count(old, "\n")+1, "line replaced"))
		if all, _ := args["replace_all"].(bool); all {
			chips = append(chips, "all occurrences")
		}
	} else if content, ok := args["content"].(string); ok && content != "" {
		chips = append(chips, plural(strings.Count(strings.TrimRight(content, "\n"), "\n")+1, "line"))
	}

	if m.Status == "running" {
		chips = append(chips, "pending review")
	}
	return chips
}

// editDiffText extracts diff-ish preview text from a message. Running
// proposals carry the proposed diff in Content; finished writes carry an
// applied-diff summary. Plain prose summaries are rejected so they don't get
// rendered as a diff with no +/- lines.
func editDiffText(m ConversationMsg) string {
	if strings.TrimSpace(m.Content) == "" {
		return ""
	}
	for _, l := range strings.Split(m.Content, "\n") {
		t := strings.TrimLeft(l, " ")
		if strings.HasPrefix(t, "+") || strings.HasPrefix(t, "-") {
			return m.Content
		}
	}
	return ""
}

// renderFileOpCard renders delete_file / move_file / copy_file — pure path
// bookkeeping, so one line each with no body.
//
//	● Move  internal/old.go → internal/new.go                            4ms
//	● Delete  internal/tools/interaction/finish.go                        2ms
func (c *Conversation) renderFileOpCard(m ConversationMsg, width int) string {
	args := argsOf(m)
	subject := subjectFor(m)
	if dst, ok := args["destination"].(string); ok && dst != "" {
		src, _ := args["source"].(string)
		subject = fmt.Sprintf("%s %s %s", src, glyphTo, dst)
	}

	var chips []string
	if rec, _ := args["recursive"].(bool); rec {
		chips = append(chips, "recursive")
	}
	if ow, _ := args["overwrite"].(bool); ow {
		chips = append(chips, "overwrite")
	}

	head := c.callLine(m, width, subject, chips, durationChip(m.Duration))
	if m.IsError {
		return join(head, c.resultRow(c.severityMark("error"), oneLine(m.Content), width))
	}
	return head
}

// renderFileOpGroup collapses a run of file operations into one card.
func (c *Conversation) renderFileOpGroup(group []ConversationMsg, width int) string {
	rollup := group[0]
	rollup.Duration = 0
	for _, g := range group {
		rollup.Duration += g.Duration
		if g.IsError {
			rollup.IsError = true
			rollup.Status = "error"
		}
	}

	head := c.callLine(rollup, width, "", []string{plural(len(group), "file")}, durationChip(rollup.Duration))
	if c.compactOnly() {
		return head
	}

	limit := c.bodyLimit(len(group))
	rows := make([]string, 0, limit+1)
	for i, g := range group {
		if i >= limit {
			break
		}
		mark := ""
		if g.IsError {
			mark = c.severityMark("error")
		}
		rows = append(rows, c.resultRow(mark,
			specFor(g.ToolName).Display+" "+filepath.Base(subjectFor(g)), width))
	}
	if more := len(group) - limit; more > 0 {
		rows = append(rows, c.moreRow(more, "file"))
	}
	return join(head, strings.Join(rows, "\n"))
}

func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	// Pluralize the head noun so "1 line replaced" becomes "3 lines replaced".
	if i := strings.IndexByte(singular, ' '); i > 0 {
		return fmt.Sprintf("%d %ss%s", n, singular[:i], singular[i:])
	}
	if strings.HasSuffix(singular, "s") || strings.HasSuffix(singular, "h") {
		return fmt.Sprintf("%d %ses", n, singular)
	}
	if strings.HasSuffix(singular, "y") {
		return fmt.Sprintf("%d %sies", n, singular[:len(singular)-1])
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
