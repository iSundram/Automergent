package context

import "testing"

func TestPricingForFuzzyModelMatch(t *testing.T) {
	pricing := DefaultPricing()

	cases := []struct {
		model string
		want  string // expected family ("" = no match)
	}{
		{"gemini-2.5-flash", "gemini-2.5-flash"},
		{"gemini-2.5-flash-lite", "gemini-2.5-flash"},   // suffixed variant
		{"gemini-2.5-flash-preview-05-2025", "gemini-2.5-flash"},
		{"gemini-3.5-flash-lite", "gemini-3.5-flash"},
		{"models/gemini-2.5-pro", "gemini-2.5-pro"},     // provider-prefixed ID
		{"claude-sonnet-4-5", "claude-sonnet-4"},
		{"totally-unknown-model", ""},
	}
	for _, tc := range cases {
		p, ok := pricingFor(pricing, tc.model)
		if tc.want == "" {
			if ok {
				t.Errorf("pricingFor(%q) matched, want none", tc.model)
			}
			continue
		}
		if !ok {
			t.Errorf("pricingFor(%q) did not match %q", tc.model, tc.want)
			continue
		}
		want := pricing[tc.want]
		if p != want {
			t.Errorf("pricingFor(%q) = %+v, want %q's %+v", tc.model, p, tc.want, want)
		}
	}
}

func TestCostTrackerPricesSuffixedModels(t *testing.T) {
	ct := NewCostTracker()
	// A suffixed model that is NOT a literal table key: before the fuzzy
	// matcher this recorded usage but stayed at $0.00 forever.
	ct.Add("gemini-2.5-flash-lite", 100000, 10000)

	s := ct.Summary()
	if s.TotalCostUSD == 0 {
		t.Fatal("suffixed model priced at $0.00 — fuzzy matching broken")
	}
	// 100k in * 0.000075 + 10k out * 0.0003 = 0.0075 + 0.003 = $0.0105
	if got := int(s.TotalCostUSD * 10000); got != 105 {
		t.Fatalf("cost = $%.4f, want $0.0105", s.TotalCostUSD)
	}
}
