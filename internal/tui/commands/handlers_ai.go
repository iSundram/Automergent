package commands

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/iSundram/Automergent/internal/agent"
	"github.com/iSundram/Automergent/internal/config"
)

// --- AI & Model Handlers ---

// handleModel is the entry point for /model. Subcommands: (no args → status),
// list, info, add, remove, refresh, reset, and <name> (switch).
func handleModel(host Host, args []string) Result {
	provider := host.Provider()

	if len(args) == 0 {
		host.AddSystemMessage(fmt.Sprintf(
			"Current model: %s (provider: %s)\nUsage: /model <name> to switch · /model list · /model add <id> — register custom model · /model refresh · /model reset",
			host.Model(), provider))
		return Done(nil)
	}

	switch args[0] {
	case "list", "ls":
		return modelList(host, provider)
	case "info":
		if len(args) < 2 {
			host.CommandUsage("/model info <name>")
			return Done(nil)
		}
		return modelInfo(host, provider, strings.Join(args[1:], " "))
	case "refresh":
		host.AddSystemMessage(fmt.Sprintf("Refreshing model list from %s…", provider))
		return Done(host.RefreshModels())
	case "add":
		return modelAdd(host, provider, args[1:])
	case "remove", "rm":
		if len(args) < 2 {
			host.CommandUsage("/model remove <id>")
			return Done(nil)
		}
		return modelRemove(host, provider, args[1])
	case "reset":
		return modelReset(host, provider)
	}

	// /model <name>: switch the model of the active provider. Unknown names
	// are accepted (models may launch faster than catalogues update) and
	// remembered as the per-provider default for /provider use.
	model := strings.Join(args, " ")
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.DefaultModel = model
	host.SetProviderConfig(provider, pc)
	if err := host.SwitchProvider(provider, model); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Model switched to %s", model))
	host.PersistProjectConfig()
	host.SetStatus("Model updated")
	return Done(nil)
}

func modelList(host Host, provider string) Result {
	models := host.ModelsAvailable()
	current := host.Model()
	pc := host.ProviderConfig(provider)

	var b strings.Builder
	fmt.Fprintf(&b, "Models for %s (current: %s):\n", provider, current)
	if len(models) == 0 {
		b.WriteString("  (no models fetched yet — run /model refresh)\n")
	}
	for _, m := range models {
		marker := "  "
		if m.ID == current {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-36s %s", marker, m.ID, formatContextLimit(m.ContextLimit))
		if pc.Models != nil {
			if mc, ok := pc.Models[m.ID]; ok {
				if mc.DisplayName != "" {
					line += " " + mc.DisplayName
				}
				line += " (custom)"
				_ = mc
			}
		}
		if m.InputPrice > 0 || m.OutputPrice > 0 {
			line += fmt.Sprintf(" $%.4g/$%.4g", m.InputPrice, m.OutputPrice)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nUse /model <name> to switch · /model info <name> · /model add <id> for custom models")
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	return Done(nil)
}

func modelInfo(host Host, provider, name string) Result {
	for _, m := range host.ModelsAvailable() {
		if m.ID == name {
			var b strings.Builder
			fmt.Fprintf(&b, "Model: %s\nName: %s\nContext limit: %s", m.ID, m.Name, formatContextLimit(m.ContextLimit))
			if m.InputPrice > 0 || m.OutputPrice > 0 {
				fmt.Fprintf(&b, "\nPrice: $%.4g input / $%.4g output per 1M tokens", m.InputPrice, m.OutputPrice)
			}
			if pc := host.ProviderConfig(provider); pc.Models != nil {
				if mc, ok := pc.Models[name]; ok {
					b.WriteString("\nSource: custom (user-registered)")
					if mc.ContextLimit > 0 {
						fmt.Fprintf(&b, "\nRegistered context limit: %s", formatContextLimit(mc.ContextLimit))
					}
				}
			}
			b.WriteString(fmt.Sprintf("\nProvider: %s", provider))
			host.AddSystemMessage(b.String())
			return Done(nil)
		}
	}
	host.CommandError(fmt.Sprintf("Unknown model %q for provider %s — run /model list or /model refresh", name, provider))
	return Done(nil)
}

func modelAdd(host Host, provider string, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/model add <id> [--context <tokens>] [--name <label>] [--price-in <usd>] [--price-out <usd>]")
		return Done(nil)
	}
	spec, ok := config.ProviderSpecFor(provider)
	if ok && !spec.CustomModels {
		host.CommandError(fmt.Sprintf("Provider %q does not support custom models", provider))
		return Done(nil)
	}
	id := args[0]
	if strings.HasPrefix(id, "--") || strings.ContainsAny(id, " \t") {
		host.CommandError("Model ID must be a single token without spaces")
		return Done(nil)
	}
	mc := config.ModelConfig{}
	for i := 1; i < len(args); {
		flag := args[i]
		if !strings.HasPrefix(flag, "--") {
			host.CommandUsage("/model add <id> [--context <tokens>] [--name <label>] [--price-in <usd>] [--price-out <usd>]")
			return Done(nil)
		}
		if i+1 >= len(args) {
			host.CommandError(fmt.Sprintf("Flag %q requires a value", flag))
			return Done(nil)
		}
		val := args[i+1]
		i += 2
		switch strings.TrimPrefix(flag, "--") {
		case "context":
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 {
				host.CommandError("--context must be a positive integer")
				return Done(nil)
			}
			mc.ContextLimit = n
		case "name":
			mc.DisplayName = val
		case "price-in":
			p, err := strconv.ParseFloat(val, 64)
			if err != nil || p < 0 {
				host.CommandError("--price-in must be a non-negative number")
				return Done(nil)
			}
			mc.InputPrice = p
		case "price-out":
			p, err := strconv.ParseFloat(val, 64)
			if err != nil || p < 0 {
				host.CommandError("--price-out must be a non-negative number")
				return Done(nil)
			}
			mc.OutputPrice = p
		default:
			host.CommandError(fmt.Sprintf("Unknown flag %q", flag))
			return Done(nil)
		}
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	if pc.Models == nil {
		pc.Models = map[string]config.ModelConfig{}
	}
	if old, ok := pc.Models[id]; ok {
		if mc.APIKey == "" {
			mc.APIKey = old.APIKey
		}
		if mc.BaseURL == "" {
			mc.BaseURL = old.BaseURL
		}
	}
	pc.Models[id] = mc
	host.SetProviderConfig(provider, pc)
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Custom model %s registered for %s — /model refresh to list it", id, provider))
	host.SetStatus("Model registered")
	return Done(host.FetchModels())
}

func modelRemove(host Host, provider, id string) Result {
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	if pc.Models == nil {
		host.CommandError(fmt.Sprintf("%q is not a custom model for %s", id, provider))
		return Done(nil)
	}
	if _, ok := pc.Models[id]; !ok {
		host.CommandError(fmt.Sprintf("%q is not a custom model for %s — run /model list to see registered models", id, provider))
		return Done(nil)
	}
	delete(pc.Models, id)
	host.SetProviderConfig(provider, pc)
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Custom model %s removed from %s", id, provider))
	host.SetStatus("Model removed")
	if id == host.Model() {
		host.AddSystemMessage("Note: the removed model is still the active model until you switch to another one")
	}
	return Done(host.FetchModels())
}

func modelReset(host Host, provider string) Result {
	defaultModel := config.DefaultModelFor(provider)
	if defaultModel == "" {
		host.CommandError(fmt.Sprintf("No default model known for provider %q", provider))
		return Done(nil)
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.DefaultModel = defaultModel
	host.SetProviderConfig(provider, pc)
	if err := host.SwitchProvider(provider, defaultModel); err != nil {
		host.CommandError(fmt.Sprintf("Error resetting model: %v", err))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Model reset to default for %s: %s", provider, host.Model()))
	host.PersistProjectConfig()
	host.SetStatus("Model updated")
	return Done(nil)
}

// --- Provider Handlers ---

// handleProvider is the entry point for /provider. Subcommands: (no args →
// status), list, use, backend, setup, test, set, unset, fallback. Positional
// args without a subcommand are treated as `use` for backwards compatibility.
func handleProvider(host Host, args []string) Result {
	if len(args) == 0 {
		providerStatus(host)
		return Done(nil)
	}
	switch args[0] {
	case "status":
		providerStatus(host)
		return Done(nil)
	case "list", "ls":
		providerList(host)
		return Done(nil)
	case "use":
		return providerUse(host, args[1:])
	case "backend":
		return providerBackend(host, args[1:])
	case "setup":
		return providerSetup(host, args[1:])
	case "test":
		return providerTestCmd(host, args[1:])
	case "set":
		return providerSet(host, args[1:])
	case "unset":
		return providerUnset(host, args[1:])
	case "fallback":
		return providerFallbackCmd(host, args[1:])
	default:
		return providerUse(host, args)
	}
}

func providerStatus(host Host) {
	provider := host.Provider()
	pc := host.ProviderConfig(provider)
	spec, _ := config.ProviderSpecFor(provider)
	display := provider
	if spec.DisplayName != "" {
		display = spec.DisplayName
	}
	auth := host.ProviderAuthSource(provider)
	if auth == "" {
		auth = "not set — run /provider setup " + provider + " --api-key <key>"
	}
	baseURL := pc.BaseURL
	if baseURL == "" {
		baseURL = "default"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Provider: %s (%s)\n", provider, display)
	fmt.Fprintf(&b, "Backend: %s\n", config.EffectiveBackend(provider, pc))
	fmt.Fprintf(&b, "Model: %s\n", host.Model())
	fmt.Fprintf(&b, "API key: %s\n", auth)
	fmt.Fprintf(&b, "Base URL: %s\n", baseURL)
	if pc.Project != "" || pc.Location != "" {
		fmt.Fprintf(&b, "Project: %s  Location: %s\n", orDash(pc.Project), orDash(pc.Location))
	}
	if pc.Temperature != nil {
		fmt.Fprintf(&b, "Temperature: %.2f\n", *pc.Temperature)
	}
	if pc.MaxTokens > 0 {
		fmt.Fprintf(&b, "Max tokens: %d\n", pc.MaxTokens)
	}
	if pc.TimeoutSeconds > 0 {
		fmt.Fprintf(&b, "Timeout: %ds\n", pc.TimeoutSeconds)
	}
	if pc.MaxRetries > 0 {
		fmt.Fprintf(&b, "Retries: %d\n", pc.MaxRetries)
	}
	if len(pc.Headers) > 0 {
		fmt.Fprintf(&b, "Custom headers: %s\n", strings.Join(sortedKeys(pc.Headers), ", "))
	}
	if fb := host.ProviderFallbacks(); len(fb) > 0 {
		fmt.Fprintf(&b, "Fallbacks: %d configured (/provider fallback list)\n", len(fb))
	}
	b.WriteString("\nUsage: /provider <use|list|setup|test|set|unset|backend|fallback>")
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
}

func providerList(host Host) {
	active := host.Provider()
	var b strings.Builder
	b.WriteString("Providers:\n")
	for _, name := range host.Providers() {
		spec, _ := config.ProviderSpecFor(providerSpecName(name))
		desc := spec.Description
		if desc == "" {
			desc = "AI provider"
		}
		pc := host.ProviderConfig(name)
		marker := "  "
		if name == active {
			marker = "▸ "
		}
		auth := host.ProviderAuthSource(name)
		authShort := "no key"
		if auth != "" {
			authShort = "key: " + auth
		}
		fmt.Fprintf(&b, "%s%-12s %s\n   backend: %s · %s", marker, name, desc, config.EffectiveBackend(name, pc), authShort)
		if pc.DefaultModel != "" {
			fmt.Fprintf(&b, " · default model: %s", pc.DefaultModel)
		}
		if name == active {
			fmt.Fprintf(&b, " · active model: %s", host.Model())
		}
		b.WriteString("\n")
	}
	b.WriteString("\nUsage: /provider setup <name> · /provider test <name> · /provider use <name> [model]")
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
}

func providerUse(host Host, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/provider use <name> [model]")
		return Done(nil)
	}
	provider := args[0]
	model := ""
	if len(args) > 1 {
		model = args[1]
	}
	if err := host.SwitchProvider(provider, model); err != nil {
		host.CommandError(fmt.Sprintf("Error: %v", err))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Provider switched to %s", provider))
	if host.ProviderAuthSource(provider) == "" {
		spec, _ := config.ProviderSpecFor(provider)
		envHint := ""
		if len(spec.EnvKeys) > 0 {
			envHint = fmt.Sprintf(" or set %s", spec.EnvKeys[0])
		}
		host.AddSystemMessage(fmt.Sprintf("Warning: No API key set for %s. Use /provider setup %s --api-key <key>%s", provider, provider, envHint))
		host.SetStatus("Provider updated (no API key)")
	} else {
		host.SetStatus("Provider updated")
	}
	host.PersistProjectConfig()
	return Done(nil)
}

func providerBackend(host Host, args []string) Result {
	var provider, backend string
	switch len(args) {
	case 1:
		provider = host.Provider()
		backend = args[0]
	case 2:
		provider, backend = args[0], args[1]
	default:
		host.CommandUsage("/provider backend <aistudio|vertex> [provider]")
		return Done(nil)
	}
	if !config.IsKnownProvider(provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return Done(nil)
	}
	if !config.IsValidBackend(provider, backend) {
		spec, _ := config.ProviderSpecFor(provider)
		host.CommandError(fmt.Sprintf("Invalid backend %q for %s (valid: %s)", backend, provider, strings.Join(spec.Backends, ", ")))
		return Done(nil)
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	pc.Backend = backend
	host.SetProviderConfig(provider, pc)
	if backend == "vertex" && (pc.Project == "" || pc.Location == "") {
		host.AddSystemMessage("Note: Vertex AI requires project and location — use /provider set " + provider + " project <gcp-project> and /provider set " + provider + " location <region>")
	}
	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
	}
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("Backend for %s set to %s", provider, backend))
	host.SetStatus("Backend updated")
	return Done(nil)
}

func providerSetup(host Host, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/provider setup <name> [--backend aistudio|vertex] [--api-key <key>] [--base-url <url>] [--project <gcp-project>] [--location <region>] [--org <id>] [--model <id>] [--test]")
		return Done(nil)
	}
	provider := args[0]
	flags, runTest, errText := parseSetupFlags(args[1:])
	if errText != "" {
		host.CommandError(errText)
		return Done(nil)
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	model := pc.DefaultModel
	for k, v := range flags {
		switch k {
		case "api-key":
			pc.APIKey = v
		case "base-url":
			parsed, err := url.ParseRequestURI(v)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				host.CommandError("base-url must be a valid http:// or https:// URL")
				return Done(nil)
			}
			pc.BaseURL = strings.TrimRight(v, "/")
		case "backend":
			if !config.IsValidBackend(provider, v) {
				spec, _ := config.ProviderSpecFor(provider)
				host.CommandError(fmt.Sprintf("Invalid backend %q for %s (valid: %s)", v, provider, strings.Join(spec.Backends, ", ")))
				return Done(nil)
			}
			pc.Backend = v
		case "project":
			pc.Project = v
		case "location":
			pc.Location = v
		case "org":
			pc.OrgID = v
		case "model":
			model = v
			pc.DefaultModel = v
		}
	}
	if config.EffectiveBackend(provider, pc) == "vertex" && (pc.Project == "" || pc.Location == "") {
		host.CommandError("Vertex AI requires --project and --location (e.g. /provider setup " + provider + " --backend vertex --project my-gcp --location us-central1)")
		return Done(nil)
	}
	host.SetProviderConfig(provider, pc)
	switched := false
	if provider == host.Provider() {
		if model == "" {
			model = host.Model()
		}
		if err := host.SwitchProvider(provider, model); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
		switched = true
	}
	host.PersistProjectConfig()
	var b strings.Builder
	fmt.Fprintf(&b, "Provider %s configured (backend: %s", provider, config.EffectiveBackend(provider, pc))
	if switched {
		b.WriteString(", applied")
	}
	b.WriteString(")")
	host.AddSystemMessage(b.String())
	host.SetStatus("Provider configured")
	if runTest {
		return Done(host.TestProvider(provider))
	}
	return Done(nil)
}

func providerTestCmd(host Host, args []string) Result {
	provider := host.Provider()
	if len(args) > 0 {
		provider = args[0]
	}
	if !config.IsKnownProvider(provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return Done(nil)
	}
	host.AddSystemMessage(fmt.Sprintf("Testing provider %s…", provider))
	return Done(host.TestProvider(provider))
}

var validProviderKeys = map[string]bool{
	"apikey": true, "baseurl": true, "backend": true,
	"project": true, "location": true, "orgid": true,
	"defaultmodel": true, "effort": true, "thinkinglevel": true,
	"temperature": true, "maxtokens": true, "timeout": true,
	"retries": true,
}

func providerSet(host Host, args []string) Result {
	if len(args) < 3 {
		host.CommandUsage("/provider set <provider> <key> <value>\nKeys: apiKey, baseUrl, backend, project, location, orgId, defaultModel, effort, thinkingLevel, temperature, maxTokens, timeout, retries, header.<Name>")
		return Done(nil)
	}
	provider := args[0]
	key := strings.ToLower(args[0+1])
	value := strings.Join(args[2:], " ")
	if strings.TrimSpace(value) == "" {
		host.CommandUsage("/provider set <provider> <key> <value>")
		return Done(nil)
	}
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	display := value
	switch key {
	case "apikey":
		pc.APIKey = value
		display = "<hidden>"
	case "baseurl":
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			host.CommandError("baseUrl must be a valid http:// or https:// URL")
			return Done(nil)
		}
		pc.BaseURL = strings.TrimRight(value, "/")
	case "backend":
		if !config.IsValidBackend(provider, value) {
			spec, _ := config.ProviderSpecFor(provider)
			host.CommandError(fmt.Sprintf("Invalid backend %q for %s (valid: %s)", value, provider, strings.Join(spec.Backends, ", ")))
			return Done(nil)
		}
		pc.Backend = value
	case "project":
		pc.Project = value
	case "location":
		pc.Location = value
	case "orgid":
		pc.OrgID = value
	case "defaultmodel":
		pc.DefaultModel = value
	case "effort":
		v, ok := normalizeEffort(value)
		if !ok {
			host.CommandError("effort must be minimal|low|medium|high")
			return Done(nil)
		}
		pc.Effort = v
		pc.ThinkingLevel = v
	case "thinkinglevel":
		v, ok := normalizeEffort(value)
		if !ok {
			host.CommandError("thinkingLevel must be minimal|low|medium|high")
			return Done(nil)
		}
		pc.ThinkingLevel = v
	case "temperature":
		t, err := strconv.ParseFloat(value, 64)
		if err != nil || t < 0 || t > 2 {
			host.CommandError("temperature must be a number between 0 and 2")
			return Done(nil)
		}
		pc.Temperature = &t
	case "maxtokens":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			host.CommandError("maxTokens must be a positive integer")
			return Done(nil)
		}
		pc.MaxTokens = n
	case "timeout":
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			host.CommandError("timeout must be a positive integer (seconds)")
			return Done(nil)
		}
		pc.TimeoutSeconds = n
	case "retries":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 50 {
			host.CommandError("retries must be between 0 and 50")
			return Done(nil)
		}
		pc.MaxRetries = n
	default:
		if strings.HasPrefix(key, "header.") {
			name := key[len("header."):]
			if name == "" {
				host.CommandError("header name cannot be empty")
				return Done(nil)
			}
			if pc.Headers == nil {
				pc.Headers = map[string]string{}
			}
			pc.Headers[name] = value
			display = value
		} else {
			host.CommandError(fmt.Sprintf("Unknown key %q. Keys: apiKey, baseUrl, backend, project, location, orgId, defaultModel, effort, thinkingLevel, temperature, maxTokens, timeout, retries, header.<Name>", args[0+1]))
			return Done(nil)
		}
	}
	host.SetProviderConfig(provider, pc)
	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
	}
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("%s set for %s: %s", args[0+1], provider, display))
	host.SetStatus("Provider updated")
	return Done(nil)
}

func providerUnset(host Host, args []string) Result {
	if len(args) != 2 {
		host.CommandUsage("/provider unset <provider> <key>")
		return Done(nil)
	}
	provider := args[0]
	key := strings.ToLower(args[1])
	host.EnsureProviderConfig(provider)
	pc := host.ProviderConfig(provider)
	switch key {
	case "apikey":
		pc.APIKey = ""
	case "baseurl":
		pc.BaseURL = ""
	case "backend":
		pc.Backend = ""
	case "project":
		pc.Project = ""
	case "location":
		pc.Location = ""
	case "orgid":
		pc.OrgID = ""
	case "defaultmodel":
		pc.DefaultModel = ""
	case "effort":
		pc.Effort = ""
		pc.ThinkingLevel = ""
	case "thinkinglevel":
		pc.ThinkingLevel = ""
	case "temperature":
		pc.Temperature = nil
	case "maxtokens":
		pc.MaxTokens = 0
	case "timeout":
		pc.TimeoutSeconds = 0
	case "retries":
		pc.MaxRetries = 0
	default:
		if strings.HasPrefix(key, "header.") {
			name := key[len("header."):]
			if pc.Headers != nil {
				delete(pc.Headers, name)
				if len(pc.Headers) == 0 {
					pc.Headers = nil
				}
			}
		} else {
			host.CommandError(fmt.Sprintf("Unknown key %q. Keys: apiKey, baseUrl, backend, project, location, orgId, defaultModel, effort, thinkingLevel, temperature, maxTokens, timeout, retries, header.<Name>", args[1]))
			return Done(nil)
		}
	}
	host.SetProviderConfig(provider, pc)
	if provider == host.Provider() {
		if err := host.SwitchProvider(provider, host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
	}
	host.PersistProjectConfig()
	host.AddSystemMessage(fmt.Sprintf("%s cleared for %s", args[1], provider))
	host.SetStatus("Provider updated")
	return Done(nil)
}

func providerFallbackCmd(host Host, args []string) Result {
	const maxFallbacks = 8
	if len(args) == 0 || args[0] == "list" {
		listFallbacks(host)
		return Done(nil)
	}
	switch args[0] {
	case "add":
		if len(args) != 3 {
			host.CommandUsage("/provider fallback add <provider> <model>")
			return Done(nil)
		}
provider, model := args[1], args[2]
	fps := host.ProviderFallbacks()
		if len(fps) >= maxFallbacks {
			host.CommandError(fmt.Sprintf("Fallback chain is full (max %d)", maxFallbacks))
			return Done(nil)
		}
		for _, f := range fps {
			if f.Provider == provider && f.Model == model {
				host.CommandError(fmt.Sprintf("%s/%s is already in the fallback chain", provider, model))
				return Done(nil)
			}
		}
		fps = append(fps, config.FallbackProvider{Provider: provider, Model: model})
		host.SetProviderFallbacks(fps)
		if err := host.SwitchProvider(host.Provider(), host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
		host.PersistProjectConfig()
		host.AddSystemMessage(fmt.Sprintf("Fallback %d: %s/%s", len(fps), provider, model))
		host.SetStatus("Fallback added")
	case "remove":
		if len(args) != 2 {
			host.CommandUsage("/provider fallback remove <position>")
			return Done(nil)
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			host.CommandError("position must be a number")
			return Done(nil)
		}
		fps := host.ProviderFallbacks()
		if n < 1 || n > len(fps) {
			host.CommandError(fmt.Sprintf("no fallback at position %d (chain has %d entries)", n, len(fps)))
			return Done(nil)
		}
		removed := fps[n-1]
		fps = append(fps[:n-1], fps[n:]...)
		host.SetProviderFallbacks(fps)
		if err := host.SwitchProvider(host.Provider(), host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
		host.PersistProjectConfig()
		host.AddSystemMessage(fmt.Sprintf("Removed fallback: %s/%s", removed.Provider, removed.Model))
		host.SetStatus("Fallback removed")
	case "move":
		if len(args) != 3 {
			host.CommandUsage("/provider fallback move <from> <to>")
			return Done(nil)
		}
		from, err := strconv.Atoi(args[1])
		if err != nil {
			host.CommandError("from must be a number")
			return Done(nil)
		}
		to, err := strconv.Atoi(args[2])
		if err != nil {
			host.CommandError("to must be a number")
			return Done(nil)
		}
		fps := host.ProviderFallbacks()
		if from < 1 || from > len(fps) || to < 1 || to > len(fps) {
			host.CommandError(fmt.Sprintf("positions must be between 1 and %d", len(fps)))
			return Done(nil)
		}
		from-- ; to--
		entry := fps[from]
		fps = append(fps[:from], fps[from+1:]...)
		fps = append(fps[:to], append([]config.FallbackProvider{entry}, fps[to:]...)...)
		host.SetProviderFallbacks(fps)
		if err := host.SwitchProvider(host.Provider(), host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
		host.PersistProjectConfig()
		host.AddSystemMessage(fmt.Sprintf("Moved %s/%s from position %d to %d", entry.Provider, entry.Model, from+1, to+1))
		host.SetStatus("Fallback reordered")
	case "clear":
		host.SetProviderFallbacks(nil)
		if err := host.SwitchProvider(host.Provider(), host.Model()); err != nil {
			host.CommandError(err.Error())
			return Done(nil)
		}
		host.PersistProjectConfig()
		host.AddSystemMessage("Fallback chain cleared")
		host.SetStatus("Fallback cleared")
	default:
		host.CommandUsage("/provider fallback <list|add|remove|move|clear>")
	}
	return Done(nil)
}

func listFallbacks(host Host) {
	fps := host.ProviderFallbacks()
	var b strings.Builder
	fmt.Fprintf(&b, "Fallback chain:\n0. %s / %s (primary)", host.Provider(), host.Model())
	for i, f := range fps {
		fmt.Fprintf(&b, "\n%d. %s / %s", i+1, f.Provider, f.Model)
	}
	if len(fps) == 0 {
		b.WriteString("\n\n(no fallbacks — add one with: /provider fallback add <provider> <model>)")
	}
	host.AddSystemMessage(b.String())
}

// --- Mode Handler ---

func handleMode(host Host, args []string) Result {
	if len(args) == 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "Current mode: %s — %s\n\nAvailable modes:\n",
			host.Mode(), agent.ModeDescription(host.Mode()))
		for _, mode := range agent.AllModes() {
			marker := "  "
			if agent.CanonicalMode(host.Mode()) == mode {
				marker = "▸ "
			}
			fmt.Fprintf(&b, "%s%-13s %s\n", marker, mode, agent.ModeDescription(mode))
		}
		b.WriteString("\nUsage: /mode <manual|accept-edits|auto|plan> · shift+tab cycles")
		host.AddSystemMessage(b.String())
		return Done(nil)
	}

	mode := args[0]
	if !agent.IsValid(mode) {
		host.CommandError("Error: usage /mode <manual|accept-edits|auto|plan>")
		return Done(nil)
	}

	mode = agent.CanonicalMode(mode)
	host.SetMode(mode)
	host.AddSystemMessage(fmt.Sprintf("Mode switched to %s — %s", mode, agent.ModeDescription(mode)))
	host.PersistProjectConfig()
	host.SetStatus("Mode updated")
	return Done(nil)
}

// --- Context Handler ---

func handleContext(host Host, args []string) Result {
	if len(args) > 0 && args[0] == "detail" {
		host.ShowContextDetail()
		return Done(nil)
	}

	host.AddSystemMessage(fmt.Sprintf("Provider: %s\nModel: %s\nInput tokens: %d\nOutput tokens: %d\nTotal cost: $%.4f\nActive tokens: %d\n\nUse '/context detail' for telemetry breakdown.",
		host.Provider(), host.Model(), host.InputTokens(), host.OutputTokens(), host.TotalCost(), host.ActiveTokens()))
	return Done(nil)
}

// --- Helpers ---

func formatContextLimit(n int) string {
	switch {
	case n <= 0:
		return "—"
	case n >= 1024*1024:
		return fmt.Sprintf("%gM", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%gK", float64(n)/1024)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func normalizeEffort(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "minimal", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(v)), true
	}
	return "", false
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func providerSpecName(name string) string { return name }

func parseSetupFlags(args []string) (map[string]string, bool, string) {
	flags := map[string]string{}
	runTest := false
	known := map[string]bool{
		"api-key": true, "base-url": true, "backend": true,
		"project": true, "location": true, "org": true, "model": true,
	}
	for i := 0; i < len(args); {
		tok := args[i]
		if tok == "--test" {
			runTest = true
			i++
			continue
		}
		if !strings.HasPrefix(tok, "--") {
			return nil, false, fmt.Sprintf("Unexpected argument %q — use --key value flags", tok)
		}
		name := strings.TrimPrefix(tok, "--")
		if !known[name] {
			return nil, false, fmt.Sprintf("Unknown flag %q — valid: --backend --api-key --base-url --project --location --org --model --test", tok)
		}
		if i+1 >= len(args) {
			return nil, false, fmt.Sprintf("Flag %q needs a value", tok)
		}
		flags[name] = args[i+1]
		i += 2
	}
	return flags, runTest, ""
}
