package custom

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
)

// AgentFile represents a parsed agent definition file.
type AgentFile struct {
	Path    string
	Def     *agentdef.AgentDefinition
	IsValid bool
	Errors  []string
}

// LoadAgentFiles loads agent definitions from a directory.
// Supports both project (.agents/) and user (~/.config/automergent/agents/)
// directories.
func LoadAgentFiles(dir string) ([]*AgentFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []*AgentFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		af, err := ParseAgentFile(path)
		if err != nil {
			continue
		}
		files = append(files, af)
	}
	return files, nil
}

// ParseAgentFile parses a single agent definition file.
func ParseAgentFile(path string) (*AgentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return ParseAgentContent(path, string(data))
}

// ParseAgentContent parses agent definition content.
func ParseAgentContent(path, content string) (*AgentFile, error) {
	af := &AgentFile{Path: path}

	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		af.Errors = append(af.Errors, "missing frontmatter (must start with ---)")
		return af, nil
	}

	end := strings.Index(content[3:], "\n---")
	if end < 0 {
		af.Errors = append(af.Errors, "unterminated frontmatter")
		return af, nil
	}

	frontmatter := content[4 : end+3]
	body := strings.TrimSpace(content[end+7:])

	def := &agentdef.AgentDefinition{
		SystemPrompt: body,
		Source:       agentdef.SourceProject,
	}

	// Parse frontmatter
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		val = strings.Trim(val, "\"'")

		switch key {
		case "name":
			def.Name = val
		case "description":
			def.Description = val
		case "when_to_use", "whentouse", "when":
			def.WhenToUse = val
		case "model":
			def.Model = val
		case "color":
			def.Color = val
		case "effort":
			switch strings.ToLower(val) {
			case "low":
				def.Effort = agentdef.EffortLow
			case "high":
				def.Effort = agentdef.EffortHigh
			default:
				def.Effort = agentdef.EffortMedium
			}
		case "memory", "memory_scope":
			switch strings.ToLower(val) {
			case "global":
				def.MemoryScope = agentdef.MemoryScopeGlobal
			case "none":
				def.MemoryScope = agentdef.MemoryScopeNone
			default:
				def.MemoryScope = agentdef.MemoryScopeProject
			}
		case "tools":
			def.Tools = parseToolsList(val)
		case "timeout":
			// Parse duration string, ignore errors
			def.Timeout = 0 // will use default
		}
	}

	af.Def = def
	af.IsValid = len(af.Errors) == 0
	return af, nil
}

// parseToolsList parses a tools list from various formats.
// Supports: [bash, read], "bash,read", bash read
func parseToolsList(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}

	// Handle array format: [bash, read]
	if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
		val = strings.Trim(val, "[]")
	}

	// Split by comma or space
	tools := make([]string, 0)
	for _, t := range regexp.MustCompile(`[,\s]+`).Split(val, -1) {
		t = strings.TrimSpace(t)
		if t != "" {
			tools = append(tools, t)
		}
	}
	return tools
}

// LoadAndRegister loads agent files from standard locations and registers them.
func LoadAndRegister(registry interface {
	RegisterOverride(def *agentdef.AgentDefinition)
}) (int, error) {
	loaded := 0

	// Project agents: .agents/
	projectDir := ".agents"
	if files, err := LoadAgentFiles(projectDir); err == nil {
		for _, af := range files {
			if af.IsValid && af.Def != nil {
				af.Def.Source = agentdef.SourceProject
				registry.RegisterOverride(af.Def)
				loaded++
			}
		}
	}

	// User agents: ~/.config/automergent/agents/
	if home, err := os.UserHomeDir(); err == nil {
		userDir := filepath.Join(home, ".config", "automergent", "agents")
		if files, err := LoadAgentFiles(userDir); err == nil {
			for _, af := range files {
				if af.IsValid && af.Def != nil {
					af.Def.Source = agentdef.SourceUser
					registry.RegisterOverride(af.Def)
					loaded++
				}
			}
		}
	}

	return loaded, nil
}
