package config

import "testing"

func TestProviderCatalogIsGoogleOnly(t *testing.T) {
	names := ProviderNames()
	if len(names) != 3 {
		t.Fatalf("catalog should be google-aistudio, google-vertex, custom — got %v", names)
	}
	for _, want := range []string{"custom", "google-aistudio", "google-vertex"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("catalog missing %q: %v", want, names)
		}
	}
}

func TestLegacyProvidersStillResolve(t *testing.T) {
	// Configs and sessions created before the narrowing must keep working:
	// the legacy names resolve to specs (hidden from menus), not errors.
	for _, legacy := range []string{"google", "openai", "anthropic", "deepseek", "ollama"} {
		if !IsKnownProvider(legacy) {
			t.Errorf("legacy provider %q no longer known — old configs will break", legacy)
		}
		spec, ok := ProviderSpecFor(legacy)
		if !ok || spec.Name == "" {
			t.Errorf("legacy provider %q has no spec", legacy)
		}
	}
}

func TestLegacyNamesHiddenFromMenus(t *testing.T) {
	for _, legacy := range []string{"google", "openai", "anthropic", "deepseek", "ollama"} {
		for _, n := range ProviderNames() {
			if n == legacy {
				t.Errorf("legacy provider %q leaked into the user-facing catalog", legacy)
			}
		}
	}
}

func TestGoogleBackendSpecs(t *testing.T) {
	ai, ok := ProviderSpecFor("google-aistudio")
	if !ok || ai.DefaultBackend != "aistudio" {
		t.Fatalf("aistudio spec wrong: %+v", ai)
	}
	vx, ok := ProviderSpecFor("google-vertex")
	if !ok || vx.DefaultBackend != "vertex" {
		t.Fatalf("vertex spec wrong: %+v", vx)
	}
	// Vertex is project+location auth; it must expose those setup keys.
	hasProject, hasLocation := false, false
	for _, k := range vx.SetupKeys {
		if k == "project" {
			hasProject = true
		}
		if k == "location" {
			hasLocation = true
		}
	}
	if !hasProject || !hasLocation {
		t.Errorf("vertex spec missing project/location setup keys: %v", vx.SetupKeys)
	}
}

func TestContextLimitClamp(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0},           // zero stays zero (unset)
		{-5, -5},         // invalid passes through for the caller to reject
		{128000, 128000}, // under the ceiling unchanged
		{1048576, 1048576},
		{1048577, 1048576}, // just over clamps
		{2097152, 1048576}, // the old pro-tier 2M clamps to 1M
		{8000000, 1048576},
	}
	for _, tc := range cases {
		if got := ClampContextLimit(tc.in); got != tc.want {
			t.Errorf("ClampContextLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
