package commands

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/iSundram/Automergent/internal/config"
)

// /provider — manage AI providers: switch, setup, test, and configure
// backends, credentials and the fallback chain.

func providerCommand() Command {
	return Command{
		Name:        "provider",
		Description: "Manage AI providers (switch, setup, test, fallback)",
		Category:    "AI & Model",
		Icon:        "󰒋",
		ArgsHint:    "[use|list|setup|test|set|unset|backend|fallback] ...",
		Tier:        TierSecondary,
		SubPalette:  "provider",
		SubCommands: []SubCommand{
			{
				Name: "use", Description: "Switch active provider", ArgsHint: "<name>", Handler: handleProvider,
				ValueCompletion: providerNameCompletion,
			},
			{Name: "list", Description: "List configured providers", Handler: handleProvider},
			{Name: "setup", Description: "Setup a provider", ArgsHint: "<name>", Handler: handleProvider, ValueCompletion: providerNameCompletion},
			{Name: "login", Description: "Check or guide provider login", ArgsHint: "<name>", Handler: handleProvider, ValueCompletion: providerNameCompletion},
			{
				Name: "test", Description: "Test provider connectivity", ArgsHint: "<name>", Handler: handleProvider,
				ValueCompletion: providerNameCompletion,
			},
			{Name: "set", Description: "Set provider config", ArgsHint: "<key> <value>", Handler: handleProvider},
			{Name: "unset", Description: "Unset provider config", ArgsHint: "<key>", Handler: handleProvider},
			{Name: "backend", Description: "Manage provider backend", Handler: handleProvider},
			{Name: "fallback", Description: "Manage fallback chain", ArgsHint: "[add|remove|list]", Handler: handleProvider},
		},
		Completion: func(h Host, partial string) []string {
			return prefixFilter([]string{"use", "list", "setup", "test", "set", "unset", "backend", "fallback"}, partial)
		},
		SupportsHeadless: true,
	}
}

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
	case "login":
		return providerLogin(host, args[1:])
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
	if !config.IsKnownProvider(provider) && !containsString(host.Providers(), provider) {
		host.CommandError(fmt.Sprintf("Unknown provider %q", provider))
		return Done(nil)
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
			clean, ok := parseURL(v)
			if !ok {
				host.CommandError("base-url must be a valid http:// or https:// URL")
				return Done(nil)
			}
			pc.BaseURL = clean
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

const providerKeysHelp = "Keys: apiKey, baseUrl, backend, project, location, orgId, defaultModel, effort, thinkingLevel, temperature, maxTokens, timeout, retries, header.<Name>"

func providerSet(host Host, args []string) Result {
	if len(args) < 3 {
		host.CommandUsage("/provider set <provider> <key> <value>\n" + providerKeysHelp)
		return Done(nil)
	}
	provider := args[0]
	key := strings.ToLower(args[1])
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
		clean, ok := parseURL(value)
		if !ok {
			host.CommandError("baseUrl must be a valid http:// or https:// URL")
			return Done(nil)
		}
		pc.BaseURL = clean
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
		n, ok := parsePositiveInt(value)
		if !ok {
			host.CommandError("maxTokens must be a positive integer")
			return Done(nil)
		}
		pc.MaxTokens = n
	case "timeout":
		n, ok := parsePositiveInt(value)
		if !ok {
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
			host.CommandError(fmt.Sprintf("Unknown key %q. %s", args[1], providerKeysHelp))
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
	host.AddSystemMessage(fmt.Sprintf("%s set for %s: %s", args[1], provider, display))
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
			host.CommandError(fmt.Sprintf("Unknown key %q. %s", args[1], providerKeysHelp))
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
		from--
		to--
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

// providerNameCompletion offers every known provider for the name argument
// of /provider use|setup|test.
func providerNameCompletion(h Host, _ string) []string {
	return h.Providers()
}

// providerLogin checks the credential path for a provider and guides the
// user through it — the "how do I log in with this" entry point:
//
//   - google-aistudio: API key from config or environment; guidance points
//     at AI Studio's key page and the exact setup command.
//   - google-vertex: Application Default Credentials via gcloud; the check
//     runs `gcloud auth application-default print-access-token` and the
//     guidance names the login command, plus the project/location the
//     backend also requires.
func providerLogin(host Host, args []string) Result {
	if len(args) == 0 {
		host.CommandUsage("/provider login <google-aistudio|google-vertex>")
		return Done(nil)
	}
	provider := args[0]
	spec, ok := config.ProviderSpecFor(provider)
	if !ok {
		host.CommandError(fmt.Sprintf("Unknown provider %q — /provider list shows what is available", provider))
		return Done(nil)
	}

	var b strings.Builder
	switch config.DefaultBackend(provider) {
	case "vertex":
		b.WriteString("Google Vertex AI login\n\n")
		ok, detail := host.CheckVertexAuth()
		if ok {
			fmt.Fprintf(&b, "✓ %s\n", detail)
		} else {
			fmt.Fprintf(&b, "✗ %s\n", detail)
		}
		pc := host.ProviderConfig(provider)
		if pc.Project == "" || pc.Location == "" {
			b.WriteString("\nVertex also needs a Cloud project and location:\n")
			fmt.Fprintf(&b, "  /provider setup %s --project <gcp-project> --location us-central1\n", provider)
		} else {
			fmt.Fprintf(&b, "\nProject: %s · Location: %s\n", pc.Project, pc.Location)
		}
		b.WriteString("\nThen verify end to end: /provider test " + provider)
	default:
		b.WriteString("Google AI Studio login\n\n")
		if src := host.ProviderAuthSource(provider); src != "" {
			fmt.Fprintf(&b, "✓ API key resolves from %s\n", src)
			b.WriteString("\nVerify end to end: /provider test " + provider)
		} else {
			b.WriteString("✗ No API key configured\n\n")
			b.WriteString("Get a key at https://aistudio.google.com/apikey, then either:\n")
			if len(spec.EnvKeys) > 0 {
				fmt.Fprintf(&b, "  export %s=<key>\n", spec.EnvKeys[0])
			}
			fmt.Fprintf(&b, "  /provider setup %s --api-key <key>\n", provider)
		}
	}
	host.AddSystemMessage(strings.TrimRight(b.String(), "\n"))
	host.SetStatus("Login checked: " + provider)
	return Done(nil)
}
