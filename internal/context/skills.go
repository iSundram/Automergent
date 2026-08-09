package context

import (
	"os"
	"path/filepath"
	"strings"
)

// Skill is a reusable instruction loaded from the skills directory.
type Skill struct {
	Name    string
	Content string
}

// LoadSkills reads all .md files from the skills directory.
func LoadSkills(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var skills []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		skills = append(skills, Skill{Name: name, Content: string(data)})
	}
	return skills, nil
}
