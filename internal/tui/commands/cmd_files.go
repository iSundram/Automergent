package commands

import (
	"fmt"
	"strings"
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
		Immediate:        true,
		SupportsHeadless: true,
	}
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
