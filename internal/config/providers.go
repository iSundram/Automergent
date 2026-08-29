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
	// ApiType identifies the API protocol: "openai", "anthropic", "gemini", "custom".
	ApiType string
	// ModelApiUrl is the endpoint path for fetching model lists (e.g. "/v1/models").
	ModelApiUrl string
	// SupportedEfforts lists the effort levels the provider supports.
	SupportedEfforts []string
	// SupportsEffort indicates whether the provider has configurable effort levels.
	SupportsEffort bool
}

// providerSpecs is the built-in provider catalog.
var providerSpecs = map[string]ProviderSpec{
	"google": {
		Name:           "google",
		DisplayName:    "Google Gemini",
		Description:    "Gemini models — AI Studio (API key) or Vertex AI (project + location)",
		Icon:           "●",
		DefaultModel:   "gemini-3.6-flash",
		EnvKeys:        []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Backends:       []string{"aistudio", "vertex"},
		DefaultBackend: "aistudio",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"backend", "project", "location", "orgId"},
		ApiType:        "gemini",
		ModelApiUrl:    "",
		SupportedEfforts: []string{"minimal", "low", "medium", "high"},
		SupportsEffort: true,
	},
	"openai": {
		Name:           "openai",
		DisplayName:    "OpenAI",
		Description:    "GPT models — OpenAI API or compatible endpoints",
		Icon:           "󰍉",
		DefaultModel:   "gpt-4o",
		EnvKeys:        []string{"OPENAI_API_KEY"},
		Backends:       []string{},
		DefaultBackend: "",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"organization", "project"},
		ApiType:        "openai",
		ModelApiUrl:    "/v1/models",
		SupportedEfforts: []string{"minimal", "low", "medium", "high"},
		SupportsEffort: true,
	},
	"anthropic": {
		Name:           "anthropic",
		DisplayName:    "Anthropic",
		Description:    "Claude models — Anthropic API",
		Icon:           "󰚥",
		DefaultModel:   "claude-3-5-sonnet-20241022",
		EnvKeys:        []string{"ANTHROPIC_API_KEY"},
		Backends:       []string{},
		DefaultBackend: "",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{},
		ApiType:        "anthropic",
		ModelApiUrl:    "/v1/models",
		SupportedEfforts: []string{},
		SupportsEffort: false,
	},
	"deepseek": {
		Name:           "deepseek",
		DisplayName:    "DeepSeek",
		Description:    "DeepSeek models — DeepSeek API or compatible endpoints",
		Icon:           "󱎑",
		DefaultModel:   "deepseek-chat",
		EnvKeys:        []string{"DEEPSEEK_API_KEY"},
		Backends:       []string{},
		DefaultBackend: "",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{},
		ApiType:        "openai",
		ModelApiUrl:    "/v1/models",
		SupportedEfforts: []string{},
		SupportsEffort: false,
	},
	"ollama": {
		Name:           "ollama",
		DisplayName:    "Ollama (Local)",
		Description:    "Local models via Ollama server",
		Icon:           "󱃖",
		DefaultModel:   "llama3.2",
		EnvKeys:        []string{},
		Backends:       []string{},
		DefaultBackend: "",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"host"},
		ApiType:        "openai",
		ModelApiUrl:    "/api/tags",
		SupportedEfforts: []string{},
		SupportsEffort: false,
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
	return "●"
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

// EffectiveBackend resolves which backend is in force for a provider:
// explicit config wins, then project/location inference (for providers with
// a "vertex" backend), then the catalog default. "default" is returned for
// providers with a single implicit backend.
func EffectiveBackend(provider string, pc ProviderConfig) string {
	if pc.Backend != "" {
		return pc.Backend
	}
	if spec, ok := providerSpecs[provider]; ok && len(spec.Backends) > 0 {
		for _, b := range spec.Backends {
			if b == "vertex" && pc.Project != "" && pc.Location != "" {
				return "vertex"
			}
		}
		if spec.DefaultBackend != "" {
			return spec.DefaultBackend
		}
	}
	return "default"
}
