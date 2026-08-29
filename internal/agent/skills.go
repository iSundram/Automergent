package agent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Skill is a reusable capability definition (SKILL.md style):
//
//	---
//	name: go-testing
//	description: How this repo structures and runs Go tests
//	globs: *_test.go, go.mod
//	---
//	instructions the model should follow when the skill applies
type Skill struct {
	Name        string
	Description string
	Globs       []string
	Body        string
	Path        string
}

// SkillName returns the skill name (implements prompt.Skill interface).
func (s Skill) SkillName() string {
	return s.Name
}

// SkillDescription returns the skill description (implements prompt.Skill interface).
func (s Skill) SkillDescription() string {
	return s.Description
}

// loadSkills scans directories for skills. Each immediate subdirectory may
// contain SKILL.md; plain *.md files are also accepted as single-file skills.
// Later sources win on name conflicts (project over user).
func loadSkills(dirs ...string) []Skill {
	byName := make(map[string]Skill)
	var order []string // deterministic output

	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			var mdPath string
			switch {
			case entry.IsDir():
				candidate := filepath.Join(dir, entry.Name(), "SKILL.md")
				if _, err := os.Stat(candidate); err == nil {
					mdPath = candidate
				}
			case strings.HasSuffix(entry.Name(), ".md"):
				mdPath = filepath.Join(dir, entry.Name())
			}
			if mdPath == "" {
				continue
			}
			data, err := os.ReadFile(mdPath)
			if err != nil {
				continue
			}
			skill := parseSkill(string(data), mdPath)
			if skill == nil || skill.Name == "" {
				continue
			}
			if _, seen := byName[skill.Name]; !seen {
				order = append(order, skill.Name)
			}
			byName[skill.Name] = *skill
		}
	}

	sort.Strings(order)
	out := make([]Skill, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func parseSkill(content, path string) *Skill {
	content = strings.TrimSpace(content)
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
	if bodyStart > 0 && bodyStart <= len(content) && bodyStart-4 < len(content) {
		skill.Body = strings.TrimSpace(content[bodyStart:])
	} else {
		skill.Body = strings.TrimSpace(content)
	}
	if skill.Name == "" && path != "" {
		base := filepath.Base(path)
		skill.Name = strings.TrimSuffix(base, ".md")
	}
	if skill.Body == "" && skill.Description == "" {
		return nil // empty skill
	}
	return skill
}

// skillGlobMatches reports whether a file path matches any of a skill's globs
// (simple suffix/pattern matching, case-insensitive).
func skillGlobMatches(globs []string, path string) bool {
	if len(globs) == 0 {
		return false
	}
	lowerPath := strings.ToLower(path)
	for _, g := range globs {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		pattern := strings.TrimPrefix(g, "*")
		if pattern != "" && strings.HasSuffix(lowerPath, strings.ToLower(pattern)) {
			return true
		}
		if ok, _ := filepath.Match(strings.ToLower(g), filepath.Base(lowerPath)); ok {
			return true
		}
	}
	return false
}

// skillsPromptBlock renders the availability block injected into the system
// prompt. Per the reference-agent pattern: skills are recommendable and
// followable, never executable by the model.
func skillsPromptBlock(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Skills\n")
	sb.WriteString("The following skills encode project-specific know-how. When a task matches a skill, follow its guidance inline. You cannot execute slash commands yourself — recommend them to the user.\n")
	for _, s := range skills {
		line := "- **" + s.Name + "**"
		if s.Description != "" {
			line += ": " + s.Description
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// skillProximityBlock returns the injected hint listing skills whose globs
// match recently touched files.
func skillProximityBlock(skills []Skill, recentPaths []string) string {
	if len(skills) == 0 || len(recentPaths) == 0 {
		return ""
	}
	matched := make(map[string]bool)
	for _, s := range skills {
		if len(s.Globs) == 0 {
			continue
		}
		for _, p := range recentPaths {
			if skillGlobMatches(s.Globs, p) {
				matched[s.Name] = true
				break
			}
		}
	}
	if len(matched) == 0 {
		return ""
	}
	names := make([]string, 0, len(matched))
	for name := range matched {
		names = append(names, name)
	}
	sort.Strings(names)
	return "[System note] The following skills may be relevant to the files you just accessed: " + strings.Join(names, ", ") + ".\n"
}

// skillTracker records recently accessed file paths for proximity hints.
type skillTracker struct {
	mu    sync.Mutex
	paths []string // bounded ring, most recent last
	limit int
}

func newSkillTracker(limit int) *skillTracker {
	return &skillTracker{limit: limit}
}

func (t *skillTracker) record(path string) {
	if path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.paths = append(t.paths, path)
	if len(t.paths) > t.limit {
		t.paths = t.paths[len(t.paths)-t.limit:]
	}
}

func (t *skillTracker) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.paths))
	copy(out, t.paths)
	return out
}

// toolAccessedPath extracts the primary file path from tool args for skills tracking.
func toolAccessedPath(name string, args map[string]any) string {
	switch name {
	case "read_file", "view", "edit_file", "write_file", "multi_edit":
		if p, ok := args["path"].(string); ok {
			return p
		}
	case "glob", "grep", "search":
		if p, ok := args["pattern"].(string); ok {
			return p
		}
	}
	return ""
}

// skillSnapshot returns recently accessed paths (nil-safe).
func (a *Agent) skillSnapshot() []string {
	if a.skillPaths == nil {
		return nil
	}
	return a.skillPaths.snapshot()
}
