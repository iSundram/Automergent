package commands

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Custom commands: every *.md file under .automergent/commands or
// ~/.automergent/commands becomes a first-class slash command.
//
// Frontmatter (optional, between --- fences) supports:
//
//	description: shown in palette/help
//	aliases: r, st          (comma separated)
//	argument-hint: [focus]
//	when-to-use: guidance for future model-side invocation
//	sensitive: true
//	type: prompt | handler | fullpage   (default: prompt)
//	agent: general-purpose              (agent to route to)
//	effort: high                        (thinking effort)
//	full-page-title: Review Results     (title for fullpage type)
//
// The body is a prompt template with processors:
//   $ARGUMENTS / $1..$9  — argument substitution
//   !{shell command}     — shell output injection
//   @{path/to/file}      — file content injection

const (
	customCategory = "Custom"
	// CustomCategory is the exported palette/help category for user-defined
	// markdown commands.
	CustomCategory = customCategory
	customIcon     = "󰆙"
	userCommandsNm = ".automergent/commands"
	maxCommandBody = 64 * 1024
)

// ParseMarkdownCommand derives a command definition and prompt body from a
// markdown file. relPath is the path relative to the commands root; content
// is the raw file bytes.
func ParseMarkdownCommand(relPath string, content []byte) (Command, string, error) {
	base := filepath.Base(relPath)
	if base == "README.md" || strings.HasPrefix(base, ".") {
		return Command{}, "", fmt.Errorf("%s is not a command file", relPath)
	}
	if len(content) == 0 {
		return Command{}, "", fmt.Errorf("%s is empty", relPath)
	}

	name := customNameFromRelPath(relPath)
	body := string(content)

	meta := map[string]string{}
	if strings.HasPrefix(body, "---\n") || strings.HasPrefix(body, "---\r\n") {
		var ok bool
		meta, body, ok = splitFrontmatter(body)
		if !ok {
			return Command{}, "", fmt.Errorf("%s: unterminated frontmatter", relPath)
		}
	}

	description := strings.TrimSpace(meta["description"])
	if description == "" {
		description = firstContentLine(body)
	}
	if description == "" {
		return Command{}, "", fmt.Errorf("%s: missing description", relPath)
	}

	cmdType := CmdPrompt
	switch strings.ToLower(strings.TrimSpace(meta["type"])) {
	case "handler":
		cmdType = CmdHandler
	case "fullpage":
		cmdType = CmdFullPage
	}

	cmd := Command{
		Name:             name,
		Description:      truncate(description, 120),
		Category:         customCategory,
		Icon:             customIcon,
		ArgsHint:         strings.TrimSpace(meta["argument-hint"]),
		WhenToUse:        strings.TrimSpace(meta["when-to-use"]),
		Type:             cmdType,
		FullPageTitle:    strings.TrimSpace(meta["full-page-title"]),
		Immediate:        true,
		SupportsHeadless: true,
		Sensitive:        strings.EqualFold(strings.TrimSpace(meta["sensitive"]), "true"),
	}
	for _, alias := range strings.Split(meta["aliases"], ",") {
		if alias = strings.ToLower(strings.TrimSpace(alias)); alias != "" {
			cmd.Aliases = append(cmd.Aliases, alias)
		}
	}
	if name == "" {
		return Command{}, "", fmt.Errorf("%s: cannot derive command name", relPath)
	}
	if len(body) > maxCommandBody {
		return Command{}, "", fmt.Errorf("%s: body exceeds %d bytes", relPath, maxCommandBody)
	}
	return cmd, body, nil
}

// splitFrontmatter separates a leading "---\n...\n---" block from the body.
func splitFrontmatter(doc string) (map[string]string, string, bool) {
	end := strings.Index(doc[4:], "\n---")
	if end < 0 {
		return nil, "", false
	}
	block := doc[4 : 4+end]
	rest := doc[4+end+4:]
	rest = strings.TrimPrefix(rest, "\n")
	rest = strings.TrimPrefix(rest, "\r\n")

	meta := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return meta, rest, true
}

// customNameFromRelPath lowercases the stem and joins parent directories with
// ':' to form the namespaced command name.
func customNameFromRelPath(relPath string) string {
	stem := strings.TrimSuffix(filepath.Base(relPath), ".md")
	parts := strings.Split(filepath.ToSlash(filepath.Dir(relPath)), "/")
	names := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		if part != "" && part != "." {
			names = append(names, strings.ToLower(part))
		}
	}
	names = append(names, strings.ToLower(stem))
	return strings.Join(names, ":")
}

func firstContentLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ExpandPromptTemplate substitutes $ARGUMENTS (all args joined) and $1..$9
// (individual arguments), then processes !{shell} and @{file} injections.
func ExpandPromptTemplate(body string, args []string) string {
	joined := strings.Join(args, " ")
	out := strings.ReplaceAll(body, "$ARGUMENTS", joined)
	for i := 9; i >= 1; i-- {
		value := ""
		if i <= len(args) {
			value = args[i-1]
		}
		out = strings.ReplaceAll(out, "$"+strconv.Itoa(i), value)
	}
	out = processShellInjections(out)
	out = processFileInjections(out)
	return strings.TrimSpace(out)
}

// processShellInjections replaces !{command} with the command's stdout.
func processShellInjections(s string) string {
	for {
		start := strings.Index(s, "!{")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end < 0 {
			break
		}
		end += start
		cmdStr := s[start+2 : end]
		cmdStr = strings.TrimSpace(cmdStr)
		output := ""
		if cmd := exec.Command("sh", "-c", cmdStr); cmd != nil {
			if out, err := cmd.Output(); err == nil {
				output = strings.TrimSpace(string(out))
			}
		}
		s = s[:start] + output + s[end+1:]
	}
	return s
}

// processFileInjections replaces @{path} with the file's content.
func processFileInjections(s string) string {
	for {
		start := strings.Index(s, "@{")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end < 0 {
			break
		}
		end += start
		path := strings.TrimSpace(s[start+2 : end])
		content := ""
		if data, err := os.ReadFile(path); err == nil {
			content = string(data)
		}
		s = s[:start] + content + s[end+1:]
	}
	return s
}

// LoadMarkdownCommands registers every valid markdown command found under dir.
func LoadMarkdownCommands(reg *Registry, dir string) (int, []string) {
	if dir == "" {
		return 0, nil
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return 0, nil
	}

	count := 0
	var warnings []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") && path != dir {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, relErr))
			return nil
		}
		cmd, body, parseErr := ParseMarkdownCommand(rel, content)
		if parseErr != nil {
			warnings = append(warnings, parseErr.Error())
			return nil
		}
		if regErr := registerCustomMarkdownCommand(reg, cmd, body, path); regErr != nil {
			warnings = append(warnings, regErr.Error())
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
	}
	return count, warnings
}

// registerCustomMarkdownCommand wires a parsed command into the registry.
// path is the file it was loaded from, surfaced by /commands list.
func registerCustomMarkdownCommand(reg *Registry, cmd Command, body string, path string) error {
	cmd.Source = path
	return reg.RegisterCustom(cmd, func(host Host, args []string) Result {
		prompt := ExpandPromptTemplate(body, args)
		switch cmd.Type {
		case CmdFullPage:
			title := cmd.FullPageTitle
			if title == "" {
				title = cmd.Name
			}
			return TextResult(fmt.Sprintf("# %s\n\n%s", title, prompt))
		default:
			return Done(host.StartAgent(prompt))
		}
	})
}

// LoadProjectAndUserCommands loads custom commands from both standard roots.
func LoadProjectAndUserCommands(reg *Registry, workDir string) (int, []string) {
	total := 0
	var warnings []string

	if dir := findProjectCommandsDir(workDir); dir != "" {
		n, w := LoadMarkdownCommands(reg, dir)
		total += n
		warnings = append(warnings, w...)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		n, w := LoadMarkdownCommands(reg, filepath.Join(home, userCommandsNm))
		total += n
		warnings = append(warnings, w...)
	}
	return total, warnings
}

func findProjectCommandsDir(workDir string) string {
	dir := workDir
	for {
		if dir == "" {
			return ""
		}
		candidate := filepath.Join(dir, userCommandsNm)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
