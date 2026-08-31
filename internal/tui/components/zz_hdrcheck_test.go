package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

func TestHeaderTotalTokensChipRenders(t *testing.T) {
	theme := themes.Gruvbox()
	h := NewHeader(themes.NewStyles(theme))
	h.SetWidth(160)
	h.SetPhase("build")
	h.SetTotalTokens(185582)
	v := h.View()
	if !strings.Contains(v, "Σ") {
		t.Fatalf("Σ chip missing from header view:\n%s", v)
	}
	if !strings.Contains(v, "185.6k") {
		t.Fatalf("formatted total missing from header view:\n%s", v)
	}
	// Zero tokens: chip hidden.
	h.SetTotalTokens(0)
	if strings.Contains(h.View(), "Σ") {
		t.Fatalf("Σ chip should hide when total is zero")
	}
}
