package commands

import (
	"fmt"
	"strings"
)

// --- Memory Handlers: /memory ---

func handleMemory(host Host, args []string) Result {
	var b strings.Builder
	b.WriteString("Memory surfaces:\n")

	if p := host.GlobalConfigPath(); p != "" {
		fmt.Fprintf(&b, "%s Global config   %s\n", mark(fileExists(p)), p)
	}
	if p := host.ProjectConfigPath(); p != "" {
		fmt.Fprintf(&b, "%s Project config  %s\n", mark(fileExists(p)), p)
	}
	memoryPath := projectMemoryPath(host)
	fmt.Fprintf(&b, "%s Project memory  %s\n", mark(fileExists(memoryPath)), memoryPath)

	b.WriteString("\nThese files give Automergent persistent context across sessions. Use /init to create " + projectMemoryFile + ".")
	host.AddSystemMessage(b.String())
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
