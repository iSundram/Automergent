package context

import "testing"

func TestPricingForFuzzyModelMatch(t *testing.T) {
	// No provider set: the catalog is skipped and the legacy family table
	// answers by longest substring.
	ct := NewCostTracker()

	cases := []struct {
		model string
		want  string // expected family ("" = no match)
	}{
		{"gemini-2.5-flash", "gemini-2.5-flash"},
		{"gemini-2.5-flash-lite", "gemini-2.5-flash"}, // suffixed variant
		{"gemini-2.5-flash-preview-05-2025", "gemini-2.5-flash"},
		{"models/gemini-2.5-pro", "gemini-2.5-pro"}, // provider-prefixed ID
		{"claude-sonnet-4-5", "claude-sonnet-4"},
		{"totally-unknown-model", ""},
	}
	legacy := legacyPricing()
	for _, tc := range cases {
		p, ok := pricingFor(ct, tc.model)
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
		want := legacy[tc.want]
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
	if got := int(s.TotalCostUSD*10000 + 0.5); got != 105 {
		t.Fatalf("cost = $%.4f, want $0.0105", s.TotalCostUSD)
	}
}

func TestCostTrackerUsesCatalogPricing(t *testing.T) {
	ct := NewCostTracker()
	ct.SetProvider("anthropic")

	// claude-sonnet-4-5 is a catalog model: the models.dev price wins over
	// any family-substring guess ($3/$15 per 1M, not the legacy 3.5 rate).
	ct.Add("claude-sonnet-4-5", 1000000, 1000000)
	s := ct.Summary()
	// 1M in * $3 + 1M out * $15 = $18.00
	if got := int(s.TotalCostUSD*100 + 0.5); got != 1800 {
		t.Fatalf("catalog cost = $%.2f, want $18.00", s.TotalCostUSD)
	}
}

func TestCostTrackerCachePricing(t *testing.T) {
	ct := NewCostTracker()
	ct.SetProvider("anthropic")

	// claude-sonnet-4-5: $3 in / $15 out / $0.30 cache-read / $3.75
	// cache-write per 1M. 500k cached-read + 500k uncached in + 100k out.
	ct.AddDetailed("claude-sonnet-4-5", 1000000, 100000, 500000, 0)
	s := ct.Summary()
	// 500k*3 + 500k*0.3 + 100k*15 = 1.5 + 0.15 + 1.5 = $3.15
	if got := int(s.TotalCostUSD*100 + 0.5); got != 315 {
		t.Fatalf("cache-aware cost = $%.4f, want $3.15", s.TotalCostUSD)
	}
	if s.ByModel["claude-sonnet-4-5"].CachedReadTokens != 500000 {
		t.Fatalf("cached read tokens not recorded: %+v", s.ByModel["claude-sonnet-4-5"])
	}
}
