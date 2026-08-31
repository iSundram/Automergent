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

// providerSpecs is the built-in provider catalog. Deliberately small: the
// two Google backends as first-class providers (the way leading terminal
// agents expose them — AI Studio for API-key auth, Vertex AI for Cloud
// project auth) plus one open "custom" provider for any OpenAI- or
// Gemini-compatible endpoint. Everything else is a custom provider away.
var providerSpecs = map[string]ProviderSpec{
	"google-aistudio": {
		Name:           "google-aistudio",
		DisplayName:    "Google AI Studio",
		Description:    "Gemini models via the Gemini API (API key)",
		Icon:           "●",
		DefaultModel:   "gemini-3.6-flash",
		EnvKeys:        []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Backends:       []string{"aistudio"},
		DefaultBackend: "aistudio",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"orgId"},
		ApiType:        "gemini",
		SupportedEfforts: []string{"minimal", "low", "medium", "high"},
		SupportsEffort: true,
	},
	"google-vertex": {
		Name:           "google-vertex",
		DisplayName:    "Google Vertex AI",
		Description:    "Gemini models via Vertex AI (Cloud project + location, Application Default Credentials)",
		Icon:           "●",
		DefaultModel:   "gemini-3.6-flash",
		EnvKeys:        []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Backends:       []string{"vertex"},
		DefaultBackend: "vertex",
		CustomBaseURL:  true,
		CustomModels:   true,
		LiveModelList:  true,
		SetupKeys:      []string{"project", "location"},
		ApiType:        "gemini",
		SupportedEfforts: []string{"minimal", "low", "medium", "high"},
		SupportsEffort: true,
	},
	"custom": {
		Name:          "custom",
		DisplayName:   "Custom provider",
		Description:   "Any OpenAI- or Gemini-compatible endpoint: set baseUrl, api key, and models",
		Icon:          "🔌",
		DefaultModel:  "",
		EnvKeys:       []string{},
		CustomBaseURL: true,
		CustomModels:  true,
		LiveModelList: false,
		SetupKeys:     []string{"apiType"},
		ApiType:       "custom",
	},
}

// hiddenProviderSpecs are legacy names kept working for configs and sessions
// created before the catalog was narrowed. "google" resolves to whichever
// backend its config implies (AI Studio by default, Vertex when project +
// location are set); the removed third-party providers resolve to "custom"
// so existing configs keep building a provider instead of erroring out.
var hiddenProviderSpecs = map[string]ProviderSpec{
	"google": {
		Name:            "google",
		DisplayName:     "Google Gemini",
		Description:     "Legacy alias — use google-aistudio or google-vertex",
		Icon:            "●",
		DefaultModel:    "gemini-3.6-flash",
		EnvKeys:         []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Backends:        []string{"aistudio", "vertex"},
		DefaultBackend:  "aistudio",
		CustomBaseURL:   true,
		CustomModels:    true,
		LiveModelList:   true,
		SetupKeys:       []string{"backend", "project", "location", "orgId"},
		ApiType:         "gemini",
		SupportsEffort:  true,
		SupportedEfforts: []string{"minimal", "low", "medium", "high"},
	},
	"openai":    legacySpec("openai", "OpenAI", "gpt-4o", "openai"),
	"anthropic": legacySpec("anthropic", "Anthropic", "claude-3-5-sonnet-20241022", "anthropic"),
	"deepseek":  legacySpec("deepseek", "DeepSeek", "deepseek-chat", "openai"),
	"ollama":    legacySpec("ollama", "Ollama (Local)", "llama3.2", "openai"),
}

func legacySpec(name, display, defaultModel, apiType string) ProviderSpec {
	return ProviderSpec{
		Name:          name,
		DisplayName:   display,
		Description:   "Legacy provider — kept for existing configs; configure as custom for new setups",
		Icon:          "🔌",
		DefaultModel:  defaultModel,
		EnvKeys:       []string{},
		CustomBaseURL: true,
		CustomModels:  true,
		ApiType:       apiType,
	}
}

// ProviderSpecFor returns the spec for a provider: the active catalog first,
// then legacy aliases.
func ProviderSpecFor(name string) (ProviderSpec, bool) {
	if spec, ok := providerSpecs[name]; ok {
		return spec, true
	}
	spec, ok := hiddenProviderSpecs[name]
	return spec, ok
}

// IsKnownProvider reports whether name is a provider (catalog or legacy).
func IsKnownProvider(name string) bool {
	_, ok := providerSpecs[name]
	if ok {
		return true
	}
	_, ok = hiddenProviderSpecs[name]
	return ok
}

// ProviderNames returns the user-facing provider names, sorted — the active
// catalog only; legacy aliases exist for compatibility, not for menus.
func ProviderNames() []string {
	names := make([]string, 0, len(providerSpecs))
	for name := range providerSpecs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MaxContextTokens is the hard ceiling on any model's context window: 1M
// tokens. Larger advertised limits (or user-registered --context values) are
// clamped — beyond this, provider drift between estimation and real counting
// makes the context ladder unreliable.
const MaxContextTokens = 1_048_576

// ClampContextLimit bounds a context limit to the platform ceiling.
func ClampContextLimit(n int) int {
	if n <= 0 {
		return n
	}
	if n > MaxContextTokens {
		return MaxContextTokens
	}
	return n
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
