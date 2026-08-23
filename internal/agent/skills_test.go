package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillFrontmatter(t *testing.T) {
	content := "---\nname: go-testing\ndescription: Repo test conventions\nglobs: [*_test.go, go.mod]\n---\n\nRun scoped tests.\n"
	s := parseSkill(content, "/x/SKILL.md")
	if s == nil || s.Name != "go-testing" || len(s.Globs) != 2 {
		t.Fatalf("bad parse: %+v", s)
	}
	if s.Body != "Run scoped tests." {
		t.Fatalf("body = %q", s.Body)
	}
}

func TestLoadSkillsProjectOverridesUser(t *testing.T) {
	user := t.TempDir()
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(user, "lint"), 0o755)
	os.WriteFile(filepath.Join(user, "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: user version\n---\nuser body"), 0o644)
	os.MkdirAll(filepath.Join(proj, "lint"), 0o755)
	os.WriteFile(filepath.Join(proj, "lint", "SKILL.md"), []byte("---\nname: lint\ndescription: project version\n---\nproject body"), 0o644)
	os.WriteFile(filepath.Join(proj, "notes.md"), []byte("---\nname: notes\ndescription: single file skill\n---\nnb"), 0o644)

	skills := loadSkills(user, proj)
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %+v", len(skills), skills)
	}
	if skills[0].Name == "lint" && skills[0].Description != "project version" {
		t.Fatalf("project should override user: %+v", skills[0])
	}
}

func TestSkillGlobMatches(t *testing.T) {
	globs := []string{"*_test.go", "go.mod"}
	if !skillGlobMatches(globs, "internal/agent/agent_test.go") {
		t.Error("suffix glob must match")
	}
	if !skillGlobMatches(globs, "/repo/go.mod") {
		t.Error("exact name must match")
	}
	if skillGlobMatches(globs, "main.go") {
		t.Error("non-matching path must not match")
	}
}

func TestSkillProximityBlock(t *testing.T) {
	skills := []Skill{{
		Name:  "go-testing",
		Globs: []string{"*_test.go"},
	}}
	hint := skillProximityBlock(skills, []string{"internal/tools/git/git_test.go"})
	if !strings.Contains(hint, "go-testing") {
		t.Fatalf("proximity hint missing matched skill: %q", hint)
	}
	if got := skillProximityBlock(skills, []string{"README.md"}); got != "" {
		t.Fatalf("no match should yield empty hint, got %q", got)
	}
}

func TestSkillsPromptBlockEmpty(t *testing.T) {
	if got := skillsPromptBlock(nil); got != "" {
		t.Fatalf("no skills -> empty block, got %q", got)
	}
}
