package commands

import (
	"fmt"
	"path/filepath"
	"strings"
)

// --- History & workspace handlers: /rewind /branch /files /cost /config
// /permissions /add-dir ---

func handleRewind(host Host, args []string) Result {
	checkpoints := host.Checkpoints()
	if len(args) == 0 {
		if len(checkpoints) == 0 {
			host.AddSystemMessage("No checkpoints yet — they are captured automatically before each agent turn.")
			return Done(nil)
		}
		host.OpenRewindPicker()
		return Done(nil)
	}
	if args[0] == "list" {
		for _, cp := range checkpoints {
			host.AddSystemMessage(fmt.Sprintf("%d. %s", cp.Index, cp.Label))
		}
		return Done(nil)
	}
	n := 0
	if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil || n < 1 {
		host.CommandUsage("/rewind <index>")
		return Done(nil)
	}
	known := false
	for _, cp := range checkpoints {
		if cp.Index == n {
			known = true
			break
		}
	}
	if !known {
		host.CommandError(fmt.Sprintf("no checkpoint at index %d (see /rewind list)", n))
		return Done(nil)
	}
	if err := host.RewindTo(n); err != nil {
		host.CommandError(err.Error())
	}
	return Done(nil)
}

func handleBranch(host Host, args []string) Result {
	name := strings.TrimSpace(strings.Join(args, " "))
	if name == "" {
		host.CommandUsage("/branch <name>")
		return Done(nil)
	}
	if err := host.BranchSession(name); err != nil {
		host.CommandError(err.Error())
	}
	return Done(nil)
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

func handleCost(host Host, args []string) Result {
	sessions, totalIn, totalOut := host.SessionTokenTotals()
	var b strings.Builder
	b.WriteString("Token usage:\n")
	fmt.Fprintf(&b, "Current session: in %d · out %d · $%.4f\n", host.InputTokens(), host.OutputTokens(), host.TotalCost())
	if sessions > 0 {
		fmt.Fprintf(&b, "All stored sessions (%d): in %d · out %d tokens\n", sessions, totalIn, totalOut)
	} else {
		b.WriteString("Stored-session totals unavailable (no storage attached).\n")
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	host.SetStatus("Usage listed")
	return Done(nil)
}

func handleConfig(host Host, args []string) Result {
	if len(args) == 0 {
		host.OpenSettingsPicker()
		return Done(nil)
	}
	kind, available := host.SandboxStatus()
	pc := host.ProviderConfig(host.Provider())
	keyState := "not set — run /api-key <value>"
	if pc.APIKey != "" {
		keyState = "set (hidden)"
	}
	effort := pc.Effort
	if effort == "" {
		effort = pc.ThinkingLevel
	}
	if effort == "" {
		effort = "default"
	}
	blocked, allowed := host.SecurityPaths()

	var b strings.Builder
	b.WriteString("Effective settings:\n")
	fmt.Fprintf(&b, "Provider/Model: %s / %s\n", host.Provider(), host.Model())
	fmt.Fprintf(&b, "Mode:           %s · Effort: %s\n", host.Mode(), effort)
	fmt.Fprintf(&b, "Theme:          %s · Keybindings: %s\n", host.CurrentTheme(), host.CurrentKeybindings())
	fmt.Fprintf(&b, "API key:        %s\n", keyState)
	sandboxLine := kind
	if !available && kind != "off" && kind != "none" {
		sandboxLine += " (unavailable)"
	}
	fmt.Fprintf(&b, "Sandbox:        %s\n", sandboxLine)
	fmt.Fprintf(&b, "Workdir:        %s\n", host.WorkDir())
	fmt.Fprintf(&b, "Write rules:    %d blocked · %d allowed paths\n", len(blocked), len(allowed))
	if problems := host.ValidateConfig(); len(problems) > 0 {
		fmt.Fprintf(&b, "Validation:     ✗ %d problem(s)\n", len(problems))
	} else {
		b.WriteString("Validation:     ✓ ok\n")
	}
	b.WriteString("\nFull layer breakdown: `automergent config show` from your shell.")
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	return Done(nil)
}

func handlePermissions(host Host, args []string) Result {
	if len(args) == 0 {
		// Log the current grants so they are part of the transcript, then
		// open the interactive picker on top.
		host.HandleApprovalsCommand(nil)
		host.OpenPermissionsPicker()
		return Done(nil)
	}
	// Argument path keeps the scriptable list/revoke flow.
	host.HandleApprovalsCommand(args)
	blocked, allowed := host.SecurityPaths()
	if len(blocked) == 0 && len(allowed) == 0 {
		return Done(nil)
	}
	var b strings.Builder
	b.WriteString("\nConfigured write-path rules:")
	for _, p := range blocked {
		b.WriteString("\n  ✗ blocked: " + p)
	}
	for _, p := range allowed {
		b.WriteString("\n  ✓ allowed: " + p)
	}
	host.AddSystemMessage(b.String())
	return Done(nil)
}

func handleAddDir(host Host, args []string) Result {
	path := strings.TrimSpace(strings.Join(args, " "))
	if path == "" {
		usage := "/add-dir <path>"
		if dirs := host.ExtraSearchDirs(); len(dirs) > 0 {
			host.CommandUsage(usage)
			host.AddSystemMessage("Current extra search roots:\n  " + strings.Join(dirs, "\n  "))
			return Done(nil)
		}
		host.CommandUsage(usage)
		return Done(nil)
	}
	if err := host.AddSearchDir(path); err != nil {
		host.CommandError(err.Error())
		return Done(nil)
	}
	abs, _ := filepath.Abs(path)
	host.AddSystemMessage("Added search root: " + abs + "\n/search now also walks this directory.")
	host.SetStatus("Directory added")
	return Done(nil)
}
