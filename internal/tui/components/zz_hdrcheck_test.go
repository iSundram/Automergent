package components

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// The header no longer prints token counts: the live streamed count lives in
// the conversation spinner's parenthetical ("(12s · ↓ 1.2k tokens)"), and the
// cumulative session total is not the header's business either. What the
// header must always show — even at zero — is the session cost.
func TestHeaderOmitsTokenCountsAndShowsCost(t *testing.T) {
	theme := themes.Gruvbox()
	h := NewHeader(themes.NewStyles(theme))
	h.SetWidth(160)
	h.SetPhase("build")
	h.SetTotalTokens(185582)
	v := h.View()
	if strings.Contains(v, "Σ") || strings.Contains(v, "185.6k") {
		t.Fatalf("token counts must not render in the header:\n%s", v)
	}
	if !strings.Contains(v, "$0.00") {
		t.Fatalf("cost must always render, even at zero:\n%s", v)
	}
}
