package components

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// FileTreeItem represents a file or directory in the tree.
type FileTreeItem struct {
	Name  string
	Path  string
	IsDir bool
	Depth int
}

// FileTreeLoadedMsg is sent when the file tree has been loaded.
type FileTreeLoadedMsg struct {
	Items []FileTreeItem
}

// FileTree shows the directory structure.
type FileTree struct {
	styles  *themes.Styles
	items   []FileTreeItem
	cursor  int
	width   int
	height  int
	rootDir string
}

// NewFileTree creates a new FileTree component.
func NewFileTree(styles *themes.Styles) FileTree {
	return FileTree{styles: styles}
}

// SetSize updates dimensions.
func (f *FileTree) SetSize(w, h int) { f.width = w; f.height = h }

// SetItems populates the tree.
func (f *FileTree) SetItems(items []FileTreeItem) { f.items = items }

// Items returns the current tree items.
func (f *FileTree) Items() []FileTreeItem { return f.items }

// Load returns a Cmd that loads the file tree from the given directory.
func (f *FileTree) Load(dir string) tea.Cmd {
	f.rootDir = dir
	return func() tea.Msg {
		items := loadTree(dir, 0, 3)
		return FileTreeLoadedMsg{Items: items}
	}
}

// Update handles keyboard navigation.
func (f FileTree) Update(msg tea.Msg) (FileTree, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "up", "k":
			if f.cursor > 0 {
				f.cursor--
			}
		case "down", "j":
			if f.cursor < len(f.items)-1 {
				f.cursor++
			}
		}
	}
	return f, nil
}

// View renders the file tree.
func (f FileTree) View() string {
	if len(f.items) == 0 {
		return f.styles.FileTree.Width(f.width).Height(f.height).Render(
			f.styles.Dim.Render("(empty)"),
		)
	}

	var sb strings.Builder
	sb.WriteString(f.styles.Bold.Render("  Files") + "\n\n")

	visible := f.height - 3
	start := 0
	if f.cursor >= visible {
		start = f.cursor - visible + 1
	}

	for i, item := range f.items {
		if i < start {
			continue
		}
		if i-start >= visible {
			break
		}

		indent := strings.Repeat("  ", item.Depth)
		var line string
		if item.IsDir {
			icon := "▸ "
			line = indent + f.styles.FileTreeDir.Render(icon+item.Name+"/")
		} else {
			icon := "  "
			line = indent + f.styles.FileTreeFile.Render(icon+item.Name)
		}

		if i == f.cursor {
			line = f.styles.FileTreeSelect.Width(f.width - 4).Render(line)
		}
		sb.WriteString(line + "\n")
	}

	content := sb.String()
	return f.styles.FileTree.Width(f.width).Height(f.height).
		Render(lipgloss.NewStyle().Width(f.width - 2).Render(content))
}

// loadTree recursively loads files up to maxDepth.
func loadTree(dir string, depth, maxDepth int) []FileTreeItem {
	if depth > maxDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	// Sort: dirs first, then files
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	skip := map[string]bool{
		".git": true, "node_modules": true, ".venv": true,
		"__pycache__": true, "vendor": true, ".idea": true,
		"dist": true, "build": true, "bin": true,
	}

	var items []FileTreeItem
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && name != "." {
			if depth == 0 {
				continue
			}
		}
		if skip[name] {
			continue
		}

		path := filepath.Join(dir, name)
		item := FileTreeItem{
			Name:  name,
			Path:  path,
			IsDir: e.IsDir(),
			Depth: depth,
		}
		items = append(items, item)

		if e.IsDir() && depth < maxDepth {
			children := loadTree(path, depth+1, maxDepth)
			items = append(items, children...)
		}
	}
	return items
}
