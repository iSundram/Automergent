package components

import (
	"fmt"
	"strings"

	"github.com/iSundram/Automergent/internal/lsp"
	"github.com/iSundram/Automergent/internal/tui/themes"
)

// LSPPanel shows LSP diagnostics for a file.
type LSPPanel struct {
	styles      *themes.Styles
	diagnostics []lsp.Diagnostic
	file        string
	visible     bool
	width       int
	height      int
}

// NewLSPPanel creates a new LSPPanel.
func NewLSPPanel(styles *themes.Styles) LSPPanel {
	return LSPPanel{styles: styles}
}

// SetSize updates dimensions.
func (p *LSPPanel) SetSize(w, h int) { p.width = w; p.height = h }

// SetFile updates the current file and clears diagnostics.
func (p *LSPPanel) SetFile(file string) { p.file = file; p.diagnostics = nil }

// SetDiagnostics updates the diagnostics list.
func (p *LSPPanel) SetDiagnostics(diags []lsp.Diagnostic) { p.diagnostics = diags }

// Toggle shows/hides the panel.
func (p *LSPPanel) Toggle() { p.visible = !p.visible }

// Visible reports visibility.
func (p LSPPanel) Visible() bool { return p.visible }

// View renders the LSP panel.
func (p LSPPanel) View() string {
	if !p.visible {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(p.styles.Bold.Render(fmt.Sprintf("LSP: %s\n", p.file)))
	if len(p.diagnostics) == 0 {
		sb.WriteString(p.styles.Success.Render("No diagnostics\n"))
	} else {
		for _, d := range p.diagnostics {
			style := p.styles.Error
			if d.Severity == lsp.SeverityWarning {
				style = p.styles.Warning
			}
			sb.WriteString(style.Render(fmt.Sprintf("%s:%d %s\n",
				d.Severity.String(), d.Range.Start.Line+1, d.Message)))
		}
	}
	return p.styles.Border.Width(p.width).Render(sb.String())
}
