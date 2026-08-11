package render

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
)

var (
	defaultRenderer *glamour.TermRenderer
	rendererMu      sync.Mutex
	currentWidth    int = 100
)

func init() {
	initRenderer(100)
}

func initRenderer(width int) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err == nil {
		defaultRenderer = r
		currentWidth = width
	}
}

// SetWidth updates the word wrap width for markdown rendering.
func SetWidth(width int) {
	if width <= 0 {
		width = 80
	}
	rendererMu.Lock()
	defer rendererMu.Unlock()
	if width != currentWidth {
		initRenderer(width)
	}
}

// Markdown renders markdown text to terminal-formatted output.
func Markdown(content string) string {
	rendererMu.Lock()
	r := defaultRenderer
	rendererMu.Unlock()

	if r == nil {
		return content
	}
	rendered, err := r.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}
