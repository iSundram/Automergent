package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /context-files — show files touched this session.

func filesCommand() Command {
	return Command{
		Name:             "context-files",
		Description:      "Show files touched this session",
		Category:         "Project",
		Icon:             "󰈔",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Context Files",
		Page:             filesPage,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

// filesPage builds the touched-files page, capped at 50 entries with a
// trailing count of the remainder.
func filesPage(h Host) components.Page {
	files := h.ContextFiles()
	page := components.Page{Title: "Context Files"}

	if len(files) == 0 {
		page.Subtitle = "No files touched yet this session"
		return page
	}

	page.Subtitle = fmt.Sprintf("%d touched this session", len(files))
	sec := components.PageSection{Heading: "Files"}
	for i, f := range files {
		if i >= 50 {
			sec.Lines = append(sec.Lines, fmt.Sprintf("… and %d more", len(files)-50))
			break
		}
		sec.Lines = append(sec.Lines, f)
	}
	page.Sections = append(page.Sections, sec)

	if dirs := h.ExtraSearchDirs(); len(dirs) > 0 {
		extra := components.PageSection{Heading: "Extra Search Roots"}
		for _, dir := range dirs {
			extra.Lines = append(extra.Lines, dir)
		}
		page.Sections = append(page.Sections, extra)
	}
	return page
}

func handleFiles(host Host, args []string) Result {
	files := host.ContextFiles()
	if len(files) == 0 {
		host.AddSystemMessage("No files touched yet this session.")
		return Done(nil)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Files touched this session (%d):\n", len(files))
	for i, f := range files {
		if i >= 50 {
			fmt.Fprintf(&b, "… and %d more\n", len(files)-50)
			break
		}
		b.WriteString("  " + f + "\n")
	}
	for _, dir := range host.ExtraSearchDirs() {
		b.WriteString("Extra search root: " + dir + "\n")
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	return Done(nil)
}
