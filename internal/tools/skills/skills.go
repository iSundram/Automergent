package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/iSundram/Automergent/internal/tools"
)

// Skill tools: the model can discover and read project/user skills
// (.automergent/skills, SKILL.md style). Skills are re-read on every call so
// hot-reloaded files are picked up without restarts.

// Skill is one loaded skill definition.
type Skill struct {
	Name        string
	Description string
	Globs       []string
	Body        string
	Path        string
}

var (
	dirsMu sync.RWMutex
	dirs   []string
)

// SetDirs installs the skill search directories (project dir wins over user
// dir on name conflicts — later entries win).
func SetDirs(d ...string) {
	dirsMu.Lock()
	defer dirsMu.Unlock()
	dirs = append([]string{}, d...)
}

// DiscoverSkillsTool lists available skills, optionally filtered by query.
type DiscoverSkillsTool struct{}

func (t *DiscoverSkillsTool) Name() string { return "discover_skills" }
func (t *DiscoverSkillsTool) Description() string {
	return `List available skills with their descriptions.
- Skills are reusable, project-specific playbooks (SKILL.md files) covering how this repo does things.
- Pass a query to filter by name or description.
- Then call skill with the skill name to load its instructions.`
}
func (t *DiscoverSkillsTool) RequiresConfirmation(mode string) bool { return false }
func (t *DiscoverSkillsTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *DiscoverSkillsTool) IsReadOnly(args map[string]any) bool   { return true }
func (t *DiscoverSkillsTool) IsDestructive(args map[string]any) bool { return false }

func (t *DiscoverSkillsTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "memory",
		Usage:      "Call when starting a task that might have project-specific conventions you have not loaded yet.",
		WhenToUse:  "You suspect a skill exists for the task at hand, or at the start of unfamiliar work.",
		WhenNotTo:  "Do not call repeatedly — cache what you loaded; skills rarely change mid-task.",
		InjectOrder: 40,
	}
}

func (t *DiscoverSkillsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Optional substring matched against skill name and description.",
			},
		},
	}
}

func (t *DiscoverSkillsTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	query, _ := tools.StringArg(args, "query")
	query = strings.ToLower(strings.TrimSpace(query))

	all := loadSkills()
	var sb strings.Builder
	matched := 0
	for _, s := range all {
		if query != "" &&
			!strings.Contains(strings.ToLower(s.Name), query) &&
			!strings.Contains(strings.ToLower(s.Description), query) {
			continue
		}
		matched++
		sb.WriteString(fmt.Sprintf("## %s\n%s\n", s.Name, s.Description))
	}
	if matched == 0 {
		if query != "" {
			return tools.Result{Content: fmt.Sprintf("no skills match %q", query)}, nil
		}
		return tools.Result{Content: "no skills are installed in this project"}, nil
	}
	return tools.Result{
		Content: fmt.Sprintf("%d skill(s) available. Load one with skill(name):\n\n%s", matched, sb.String()),
		Summary: fmt.Sprintf("found %d skills", matched),
	}, nil
}

// SkillTool loads one skill's instructions by name.
type SkillTool struct{}

func (t *SkillTool) Name() string { return "skill" }
func (t *SkillTool) Description() string {
	return `Load a skill's instructions by name.
- Returns the full skill body; follow it inline for the rest of the task.
- Find available names with discover_skills.`
}
func (t *SkillTool) RequiresConfirmation(mode string) bool { return false }
func (t *SkillTool) IsConcurrencySafe(args map[string]any) bool {
	return true
}
func (t *SkillTool) IsReadOnly(args map[string]any) bool   { return true }
func (t *SkillTool) IsDestructive(args map[string]any) bool { return false }

func (t *SkillTool) Meta() *tools.ToolMeta {
	return &tools.ToolMeta{
		Category:   "memory",
		Usage:      "Load the skill once per task; its guidance stays in the conversation for the rest of the turn.",
		WhenToUse:  "A skill matching the current task exists (found via discover_skills or a proximity hint).",
		WhenNotTo:  "Do not load skills speculatively — only when the task matches.",
		InjectOrder: 41,
	}
}

func (t *SkillTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name (from discover_skills).",
			},
		},
		"required": []string{"name"},
	}
}

func (t *SkillTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	name, ok := tools.StringArg(args, "name")
	if !ok || name == "" {
		return tools.Result{IsError: true, Content: "name is required"}, nil
	}
	for _, s := range loadSkills() {
		if s.Name == name {
			return tools.Result{
				Content:  fmt.Sprintf("Skill %s (from %s):\n\n%s", s.Name, s.Path, s.Body),
				Summary:  fmt.Sprintf("loaded skill %s", s.Name),
				Metadata: map[string]any{"skill": s.Name},
			}, nil
		}
	}
	return tools.Result{IsError: true, Content: fmt.Sprintf("unknown skill %q — call discover_skills for the list", name)}, nil
}

// loadSkills scans the configured directories. Mirrors the agent's loader
// conventions: each immediate subdirectory may contain SKILL.md; plain *.md
// files count as single-file skills; later sources win on conflicts.
func loadSkills() []Skill {
	dirsMu.RLock()
	search := append([]string{}, dirs...)
	dirsMu.RUnlock()

	byName := make(map[string]Skill)
	var order []string
	for _, dir := range search {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		// Subdirectory skills first, then loose .md files, both sorted for
		// deterministic output.
		var subdirs, files []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if e.IsDir() {
				subdirs = append(subdirs, e.Name())
			} else if strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
				files = append(files, e.Name())
			}
		}
		sort.Strings(subdirs)
		sort.Strings(files)
		for _, sub := range subdirs {
			if s := parseSkillFile(filepath.Join(dir, sub, "SKILL.md")); s != nil {
				if _, seen := byName[s.Name]; !seen {
					order = append(order, s.Name)
				}
				byName[s.Name] = *s
			}
		}
		for _, f := range files {
			if s := parseSkillFile(filepath.Join(dir, f)); s != nil {
				if _, seen := byName[s.Name]; !seen {
					order = append(order, s.Name)
				}
				byName[s.Name] = *s
			}
		}
	}
	out := make([]Skill, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

// parseSkillFile reads one skill file, parsing the --- frontmatter.
func parseSkillFile(path string) *Skill {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := strings.TrimSpace(string(data))
	skill := &Skill{Path: path}
	bodyStart := 0
	if strings.HasPrefix(content, "---") {
		end := strings.Index(content[3:], "\n---")
		if end >= 0 {
			for _, line := range strings.Split(content[4:end+3], "\n") {
				colon := strings.IndexByte(line, ':')
				if colon <= 0 {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(line[:colon]))
				val := strings.TrimSpace(line[colon+1:])
				switch key {
				case "name":
					skill.Name = val
				case "description":
					skill.Description = val
				case "globs":
					val = strings.Trim(val, "[]")
					for _, g := range strings.Split(val, ",") {
						if g = strings.TrimSpace(g); g != "" {
							skill.Globs = append(skill.Globs, g)
						}
					}
				}
			}
			bodyStart = end + 7
		}
	}
	if bodyStart > 0 && bodyStart <= len(content) {
		skill.Body = strings.TrimSpace(content[bodyStart:])
	} else {
		skill.Body = content
	}
	if skill.Name == "" {
		skill.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if skill.Body == "" && skill.Description == "" {
		return nil
	}
	return skill
}
