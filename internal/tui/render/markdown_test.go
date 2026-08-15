package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownWithWidthUsesCleanTerminalFormatting(t *testing.T) {
	rendered := MarkdownWithWidth("### Heading\n\n---\n\nUse `sync` safely.", 40)
	plain := ansi.Strip(rendered)
	if strings.Contains(plain, "###") {
		t.Fatalf("rendered heading exposes markdown markers: %q", plain)
	}
	if strings.Contains(plain, "--------") {
		t.Fatalf("rendered rule exposes ASCII separator: %q", plain)
	}
	if strings.Contains(plain, "\u00a0") {
		t.Fatalf("inline code contains non-breaking spaces: %q", plain)
	}
}
