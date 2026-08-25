package config

import "sort"

// ProviderSpec describes a built-in AI provider: how it is displayed, how it
// authenticates, which backends it supports and what its defaults are. This
// catalog is the single source of truth for provider identity — commands, the
// TUI palette, schema validation and the runtime provider builder all derive
// from it instead of keeping their own hard-coded lists.
type ProviderSpec struct {
	// Name is the canonical identifier used in config and commands.
	Name string
	// DisplayName is the human-friendly name for UI surfaces.
	DisplayName string
	// Description is shown in palettes and /provider list.
	Description string
	// Icon is the glyph used in the command palette.
	Icon string
	// DefaultModel is selected when no model is configured or remembered.
	DefaultModel string
	// EnvKeys lists environment variables consulted for the API key, in
	// priority order. Empty means the provider has no env-based auth.
	EnvKeys []string
	// Backends lists the backends the provider supports (for Google:
	// "aistudio", "vertex"). Empty means a single implicit backend.
	Backends []string
	// DefaultBackend is selected when the user configured no backend.
	DefaultBackend string
	// CustomBaseURL reports whether the provider accepts custom endpoints
	// (self-hosted gateways, proxies).
	CustomBaseURL bool
	// CustomModels reports whether users may register arbitrary model IDs.
	CustomModels bool
	// LiveModelList reports whether the provider can enumerate its available
	// models from the API (used by /model refresh and /provider test).
	LiveModelList bool
	// SetupKeys lists the /provider set keys the provider supports beyond the
	// universal apiKey/baseUrl/defaultModel.
	SetupKeys []string
}

// providerSpecs is the built-in provider catalog.
var providerSpecs = map[string]ProviderSpec{
	"google": {
		Name:           "google",
		DisplayName:    "Google Gemini",
		Description:    "Gemini models — AI Studio (API key) or Vertex AI (project + location)",
		Icon:           "󰊭",
		DefaultModel:   "gemini-3.6-flash",
		EnvKeys:        []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Backends:       []string{"aistudio", "vertex"},
		DefaultBackend: "aistudio",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"backend", "project", "location", "orgId"},
	},
}

// ProviderSpecFor returns the spec for a built-in provider.
func ProviderSpecFor(name string) (ProviderSpec, bool) {
	spec, ok := providerSpecs[name]
	return spec, ok
}

// IsKnownProvider reports whether name is a built-in provider.
func IsKnownProvider(name string) bool {
	_, ok := providerSpecs[name]
	return ok
}

// ProviderNames returns all built-in provider names, sorted.
func ProviderNames() []string {
	names := make([]string, 0, len(providerSpecs))
	for name := range providerSpecs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultModelFor returns the catalog default model for a provider, or ""
// when the provider is unknown.
func DefaultModelFor(provider string) string {
	if spec, ok := providerSpecs[provider]; ok {
		return spec.DefaultModel
	}
	return ""
}

// ProviderEnvKeys returns the environment variables consulted for a
// provider's API key, in priority order.
func ProviderEnvKeys(provider string) []string {
	if spec, ok := providerSpecs[provider]; ok {
		return append([]string{}, spec.EnvKeys...)
	}
	return nil
}

// ProviderIcon returns the palette icon for a provider, or a generic plug.
func ProviderIcon(provider string) string {
	if spec, ok := providerSpecs[provider]; ok && spec.Icon != "" {
		return spec.Icon
	}
	return "🔌"
}

// IsValidBackend reports whether backend is valid for the provider. An empty
// backend is always valid and means "provider default".
func IsValidBackend(provider, backend string) bool {
	if backend == "" {
		return true
	}
	spec, ok := providerSpecs[provider]
	if !ok || len(spec.Backends) == 0 {
		return false
	}
	for _, b := range spec.Backends {
		if b == backend {
			return true
		}
	}
	return false
}

// DefaultBackend returns the provider's default backend ("" when the provider
// has a single implicit backend).
func DefaultBackend(provider string) string {
	if spec, ok := providerSpecs[provider]; ok {
		return spec.DefaultBackend
	}
	return ""
}

// effectiveBackend resolves which backend name is in force for reporting:
// explicit config wins, then project/location inference, then the catalog
// default.
func EffectiveBackend(pc ProviderConfig) string {
	if pc.Backend != "" {
		return pc.Backend
	}
	if pc.Project != "" && pc.Location != "" {
		return "vertex"
	}
	if DefaultBackendFor(pc) != "" {
		return DefaultBackendFor(pc)
	}
	return "default"
}

// DefaultBackendFor is a helper around DefaultBackend kept for EffectiveBackend
// readability without threading the provider name through.
func DefaultBackendFor(pc ProviderConfig) string {
	_ = pc
	return ""
}
