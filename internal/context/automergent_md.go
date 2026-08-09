package context

import (
	"os"
	"path/filepath"
	"strings"
)

const fileName = "AUTOMERGENT.md"

// AutomergentMD represents the loaded AUTOMERGENT.md context file.
type AutomergentMD struct {
	Path    string
	Content string
}

// Load finds and reads the AUTOMERGENT.md file by walking up from dir.
func Load(dir string) (*AutomergentMD, error) {
	current := dir
	for {
		candidate := filepath.Join(current, fileName)
		data, err := os.ReadFile(candidate)
		if err == nil {
			return &AutomergentMD{Path: candidate, Content: string(data)}, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return &AutomergentMD{}, nil
}

// LoadAll reads a list of explicit context file paths.
func LoadAll(paths []string) string {
	var sb strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sb.WriteString("# " + p + "\n")
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String()
}
