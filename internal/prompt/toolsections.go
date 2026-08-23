package prompt

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// RenderToolSections builds the per-tool portion of the system prompt from
// the LIVE registry. Every registered tool gets a documentation section
// derived from its ToolMeta (explicit metadata when the tool provides it,
// inferred defaults otherwise), grouped by category. This replaces the old
// hand-written seven-line contract: new tools document themselves.
func RenderToolSections(reg *tools.Registry, model string) string {
	if reg == nil {
		return ""
	}
	all := reg.All()
	if len(all) == 0 {
		return ""
	}

	family := tools.ModelFamily(model)
	sorted := tools.SortToolsForPrompt(all)

	var sb strings.Builder
	sb.WriteString("## Tool Documentation\n")
	sb.WriteString("Schemas for these tools accompany this prompt; this documentation explains intent, sequencing, and judgment. Use a tool only if it is listed here.\n")

	currentCat := ""
	for _, t := range sorted {
		meta := tools.MetaOf(t)
		if meta.Category != currentCat {
			currentCat = meta.Category
			sb.WriteString(fmt.Sprintf("\n### %s tools\n", strings.Title(currentCat)))
		}
		writeToolSection(&sb, t, meta, family)
	}
	sb.WriteString("\n")
	return sb.String()
}

func writeToolSection(sb *strings.Builder, t tools.Tool, meta *tools.ToolMeta, family string) {
	name := t.Name()
	sb.WriteString(fmt.Sprintf("\n#### %s (`%s`)", meta.DisplayName, name))

	if alias := meta.AliasFor(family); alias != "" && alias != name {
		sb.WriteString(fmt.Sprintf(" — you may know this as `%s`", alias))
	}
	sb.WriteString("\n")

	if meta.WhenToUse != "" {
		sb.WriteString(wrapBullet("When to use: ", meta.WhenToUse))
	}
	if meta.WhenNotTo != "" {
		sb.WriteString(wrapBullet("Avoid: ", meta.WhenNotTo))
	}
	if usage := strings.TrimSpace(meta.Usage); usage != "" && usage != strings.TrimSpace(t.Description()) {
		for _, line := range strings.Split(usage, "\n") {
			sb.WriteString("  " + line + "\n")
		}
	} else if usage != "" {
		sb.WriteString(wrapBullet("Purpose: ", usage))
	}
	// Gemini-first per-family tuning: extra notes only for the active family.
	if note := meta.UsageByFamily[family]; note != "" {
		sb.WriteString(wrapBullet("Model notes ("+family+"): ", note))
	}
	for i, pair := range meta.Examples {
		good, bad := pair[0], pair[1]
		if good != "" {
			sb.WriteString(wrapBullet(fmt.Sprintf("Example %d (correct): ", i+1), good))
		}
		if bad != "" {
			sb.WriteString(wrapBullet(fmt.Sprintf("Example %d (wrong): ", i+1), bad))
		}
	}
}

// wrapBullet renders text as an indented bullet with hanging indent.
func wrapBullet(prefix, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	b.WriteString("  - " + prefix + lines[0] + "\n")
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		b.WriteString("    " + strings.TrimSpace(l) + "\n")
	}
	return b.String()
}
