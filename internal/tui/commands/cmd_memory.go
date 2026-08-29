package commands

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/tui/components"
)

// /memory — show project and memory file locations.

func memoryCommand() Command {
	return Command{
		Name:             "memory",
		Description:      "Show project and memory file locations",
		Category:         "Configuration",
		Icon:             "󰋞",
		Tier:             TierTertiary,
		Type:             CmdFullPage,
		FullPageTitle:    "Memory",
		Immediate:        true,
		SupportsHeadless: true,
		Page:             memoryPage,
	}
}

func handleMemory(host Host, args []string) Result {
	host.AddSystemMessage(strings.Join(memoryPage(host).Lines(80), "\n"))
	host.SetStatus("Memory surfaces listed")
	return Done(nil)
}

func projectMemoryPath(host Host) string {
	dir := strings.TrimSpace(host.WorkDir())
	if dir == "" {
		return projectMemoryFile
	}
	return dir + "/" + projectMemoryFile
}

func mark(ok bool) string {
	if ok {
		return "✓"
	}
	return "–"
}

// memoryPage builds the structured /memory page.
func memoryPage(h Host) components.Page {
	flags := []components.PageFlag{}
	if p := h.GlobalConfigPath(); p != "" {
		flags = append(flags, components.PageFlag{Label: "Global config", Detail: p, Status: fileFlagStatus(fileExists(p))})
	}
	if p := h.ProjectConfigPath(); p != "" {
		flags = append(flags, components.PageFlag{Label: "Project config", Detail: p, Status: fileFlagStatus(fileExists(p))})
	}
	memoryPath := projectMemoryPath(h)
	flags = append(flags, components.PageFlag{Label: "Project memory", Detail: memoryPath, Status: fileFlagStatus(fileExists(memoryPath))})

	return components.Page{
		Title:    "Memory",
		Subtitle: "Persistent context surfaces",
		Sections: []components.PageSection{
			{Heading: "Surfaces", Flagged: flags},
			{
				Heading: "Notes",
				Lines: []string{
					"These files give Automergent persistent context across sessions.",
					fmt.Sprintf("Use /init to create %s.", projectMemoryFile),
				},
			},
		},
		Actions: []components.PageAction{
			{Key: "i", Label: "Init memory", Command: "init"},
			{Key: "e", Label: "Environment", Command: "env"},
		},
	}
}
