package wiring

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type WiringEngine struct {
	store           *graph.Store
	templates       map[WiringPattern]*WiringTemplate
	entryPointWiring *EntryPointWiring
	eventSystem     *EventSystem
	appearanceValidator *AppearanceValidator
	mu              sync.RWMutex
}

func NewWiringEngine(store *graph.Store) *WiringEngine {
	engine := &WiringEngine{
		store:     store,
		templates: make(map[WiringPattern]*WiringTemplate),
	}
	engine.entryPointWiring = NewEntryPointWiring(store, engine)
	engine.eventSystem = NewEventSystem(store, engine)
	engine.appearanceValidator = NewAppearanceValidator(store, engine)
	engine.loadDefaultTemplates()
	return engine
}

func (e *WiringEngine) loadDefaultTemplates() {
	templates := []*WiringTemplate{
		{
			Pattern:     WiringPatternCommand,
			Name:        "TUI Command Registration",
			Description: "Register a new command in the TUI command palette",
			Template: `package {{.PackageName}}

import (
	"github.com/owecode/tui/commands"
)

func init() {
	commands.Register("{{.CommandName}}", &{{.CommandStruct}}{})
}

type {{.CommandStruct}} struct{}

func (c *{{.CommandStruct}}) Execute(ctx context.Context, args []string) error {
	{{.Implementation}}
	return nil
}

func (c *{{.CommandStruct}}) Description() string {
	return "{{.Description}}"
}

func (c *{{.CommandStruct}}) Usage() string {
	return "{{.Usage}}"
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "CommandName", Type: "string", Description: "Command name for registration", Required: true},
				{Name: "CommandStruct", Type: "string", Description: "Command struct name", Required: true},
				{Name: "Description", Type: "string", Description: "Command description", Required: true},
				{Name: "Usage", Type: "string", Description: "Command usage string", Required: true},
				{Name: "Implementation", Type: "string", Description: "Command implementation code", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeTUI},
			Dependencies: []string{"github.com/owecode/tui/commands"},
		},
		{
			Pattern:     WiringPatternEvent,
			Name:        "Event Emitter/Handler",
			Description: "Define event emission and handler registration",
			Template: `package {{.PackageName}}

import (
	"github.com/owecode/events"
)

var {{.EventName}}Event = events.DefineEvent("{{.EventType}}", {{.PayloadType}}{})

func Emit{{.EventName}}(ctx context.Context, payload {{.PayloadType}}) error {
	return events.Emit(ctx, {{.EventName}}Event, payload)
}

func Handle{{.EventName}}(handler func(ctx context.Context, payload {{.PayloadType}}) error) {
	events.RegisterHandler({{.EventName}}Event, handler)
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "EventName", Type: "string", Description: "Event name (PascalCase)", Required: true},
				{Name: "EventType", Type: "string", Description: "Event type string", Required: true},
				{Name: "PayloadType", Type: "string", Description: "Payload struct type", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeAPI, EntryPointTypeCLI, EntryPointTypeWebhook},
			Dependencies: []string{"github.com/owecode/events"},
		},
		{
			Pattern:     WiringPatternMiddleware,
			Name:        "HTTP Middleware",
			Description: "Create HTTP middleware for API entry points",
			Template: `package {{.PackageName}}

import (
	"net/http"
	"github.com/owecode/api/middleware"
)

func {{.MiddlewareName}}Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		{{.PreLogic}}
		next.ServeHTTP(w, r)
		{{.PostLogic}}
	})
}

func init() {
	middleware.Register("{{.MiddlewareName}}", {{.MiddlewareName}}Middleware)
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "MiddlewareName", Type: "string", Description: "Middleware name", Required: true},
				{Name: "PreLogic", Type: "string", Description: "Pre-request logic", Required: false},
				{Name: "PostLogic", Type: "string", Description: "Post-request logic", Required: false},
			},
			EntryPoints: []EntryPointType{EntryPointTypeAPI},
			Dependencies: []string{"github.com/owecode/api/middleware"},
		},
		{
			Pattern:     WiringPatternHook,
			Name:        "Lifecycle Hook",
			Description: "Register lifecycle hooks for feature initialization",
			Template: `package {{.PackageName}}

import (
	"github.com/owecode/hooks"
)

func init() {
	hooks.Register("{{.HookName}}", hooks.Hook{
		Name:        "{{.HookName}}",
		Priority:    {{.Priority}},
		Execute:     {{.HookFunction}},
		Description: "{{.Description}}",
	})
}

func {{.HookFunction}}(ctx context.Context) error {
	{{.Implementation}}
	return nil
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "HookName", Type: "string", Description: "Hook name", Required: true},
				{Name: "HookFunction", Type: "string", Description: "Hook function name", Required: true},
				{Name: "Priority", Type: "int", Description: "Hook priority", Required: true, Default: "100"},
				{Name: "Description", Type: "string", Description: "Hook description", Required: true},
				{Name: "Implementation", Type: "string", Description: "Hook implementation", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeTUI, EntryPointTypeAPI, EntryPointTypeCLI},
			Dependencies: []string{"github.com/owecode/hooks"},
		},
		{
			Pattern:     WiringPatternExtension,
			Name:        "Extension Point",
			Description: "Define extension points for plugin architecture",
			Template: `package {{.PackageName}}

import (
	"github.com/owecode/extensions"
)

type {{.ExtensionName}}Extension interface {
	{{.Methods}}
}

var {{.ExtensionName}}Registry = extensions.NewRegistry[{{.ExtensionName}}Extension]()

func Register{{.ExtensionName}}(ext {{.ExtensionName}}Extension) {
	{{.ExtensionName}}Registry.Register(ext.Name(), ext)
}

func Get{{.ExtensionName}}(name string) ({{.ExtensionName}}Extension, bool) {
	return {{.ExtensionName}}Registry.Get(name)
}

func List{{.ExtensionName}}() []{{.ExtensionName}}Extension {
	return {{.ExtensionName}}Registry.List()
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "ExtensionName", Type: "string", Description: "Extension name", Required: true},
				{Name: "Methods", Type: "string", Description: "Interface methods", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeAPI, EntryPointTypeCLI},
			Dependencies: []string{"github.com/owecode/extensions"},
		},
		{
			Pattern:     WiringPatternAdapter,
			Name:        "Config Adapter",
			Description: "Adapt configuration for different entry points",
			Template: `package {{.PackageName}}

import (
	"github.com/owecode/config"
)

type {{.AdapterName}}Adapter struct {
	config *config.Config
}

func New{{.AdapterName}}Adapter(cfg *config.Config) *{{.AdapterName}}Adapter {
	return &{{.AdapterName}}Adapter{config: cfg}
}

func (a *{{.AdapterName}}Adapter) Get{{.ConfigKey}}() {{.ConfigType}} {
	val := a.config.Get("{{.ConfigPath}}")
	if val == nil {
		return {{.DefaultValue}}
	}
	return val.({{.ConfigType}})
}

func (a *{{.AdapterName}}Adapter) Set{{.ConfigKey}}(value {{.ConfigType}}) error {
	return a.config.Set("{{.ConfigPath}}", value)
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "AdapterName", Type: "string", Description: "Adapter name", Required: true},
				{Name: "ConfigKey", Type: "string", Description: "Config key name", Required: true},
				{Name: "ConfigPath", Type: "string", Description: "Config path", Required: true},
				{Name: "ConfigType", Type: "string", Description: "Config value type", Required: true},
				{Name: "DefaultValue", Type: "string", Description: "Default value", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeConfig},
			Dependencies: []string{"github.com/owecode/config"},
		},
		{
			Pattern:     WiringPatternWebhook,
			Name:        "Webhook Handler",
			Description: "Handle incoming webhook requests",
			Template: `package {{.PackageName}}

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"github.com/owecode/webhooks"
)

func {{.WebhookName}}Handler(w http.ResponseWriter, r *http.Request) {
	if !verifySignature(r, "{{.Secret}}") {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	var payload {{.PayloadType}}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := process{{.WebhookName}}(ctx, payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func verifySignature(r *http.Request, secret string) bool {
	sig := r.Header.Get("{{.SignatureHeader}}")
	if sig == "" {
		return false
	}

	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

func process{{.WebhookName}}(ctx context.Context, payload {{.PayloadType}}) error {
	{{.Implementation}}
	return nil
}

func init() {
	webhooks.Register("{{.WebhookName}}", {{.WebhookName}}Handler)
}`,
			Parameters: []TemplateParameter{
				{Name: "PackageName", Type: "string", Description: "Go package name", Required: true},
				{Name: "WebhookName", Type: "string", Description: "Webhook name", Required: true},
				{Name: "Secret", Type: "string", Description: "Webhook secret", Required: true},
				{Name: "SignatureHeader", Type: "string", Description: "Signature header name", Required: true, Default: "X-Signature"},
				{Name: "PayloadType", Type: "string", Description: "Payload struct type", Required: true},
				{Name: "Implementation", Type: "string", Description: "Processing implementation", Required: true},
			},
			EntryPoints: []EntryPointType{EntryPointTypeWebhook},
			Dependencies: []string{"github.com/owecode/webhooks"},
		},
	}

	for _, tmpl := range templates {
		e.templates[tmpl.Pattern] = tmpl
	}
}

func (e *WiringEngine) WireFeature(ctx context.Context, feature *FeatureAppearance, similarFeatures []*SimilarFeature) (*IntegrationResult, error) {
	startTime := time.Now()
	result := &IntegrationResult{
		FeatureID:         feature.FeatureID,
		WiredEntryPoints:  []EntryPoint{},
		IntegrationPoints: []IntegrationPoint{},
		EventDefinitions:  []EventDefinition{},
		EventFlows:        []EventFlow{},
		GeneratedFiles:    []GeneratedFile{},
		ValidationErrors:  []ValidationError{},
		Warnings:          []string{},
	}

	for epType, entryPoints := range feature.EntryPoints {
		for _, ep := range entryPoints {
			wired, err := e.entryPointWiring.WireToEntryPoint(ctx, feature, ep)
			if err != nil {
				result.ValidationErrors = append(result.ValidationErrors, ValidationError{
					Code:       "WIRING_FAILED",
					Message:    err.Error(),
					Severity:   "error",
					Location:   ep.GetBaseEntryPoint().Path,
					EntryPoint: epType,
					FeatureID:  feature.FeatureID,
				})
				continue
			}
			result.WiredEntryPoints = append(result.WiredEntryPoints, *wired)
		}
	}

	for _, ep := range feature.ConfigKeys {
		ip, err := e.createConfigIntegration(ctx, feature, &ep)
		if err != nil {
			result.ValidationErrors = append(result.ValidationErrors, ValidationError{
				Code:       "CONFIG_INTEGRATION_FAILED",
				Message:    err.Error(),
				Severity:   "error",
				Location:   ep.ConfigKey,
				EntryPoint: EntryPointTypeConfig,
				FeatureID:  feature.FeatureID,
			})
			continue
		}
		result.IntegrationPoints = append(result.IntegrationPoints, *ip)
	}

	events, err := e.eventSystem.DefineFeatureEvents(ctx, feature)
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:       "EVENT_DEFINITION_FAILED",
			Message:    err.Error(),
			Severity:   "error",
			Location:   "event_system",
			EntryPoint: "",
			FeatureID:  feature.FeatureID,
		})
	} else {
		result.EventDefinitions = events
	}

	flows, err := e.eventSystem.GetEventFlows(ctx, feature.FeatureID)
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:       "EVENT_FLOW_FAILED",
			Message:    err.Error(),
			Severity:   "warning",
			Location:   "event_system",
			EntryPoint: "",
			FeatureID:  feature.FeatureID,
		})
	} else {
		result.EventFlows = flows
	}

	files, err := e.GenerateIntegrationCode(ctx, feature, []string{})
	if err != nil {
		result.ValidationErrors = append(result.ValidationErrors, ValidationError{
			Code:       "CODE_GENERATION_FAILED",
			Message:    err.Error(),
			Severity:   "error",
			Location:   "code_generation",
			EntryPoint: "",
			FeatureID:  feature.FeatureID,
		})
	} else {
		result.GeneratedFiles = files
	}

	validationResult := e.appearanceValidator.ValidateAppearance(ctx, feature)
	result.ValidationErrors = append(result.ValidationErrors, validationResult.Errors...)
	result.Warnings = append(result.Warnings, validationResult.Warnings...)

	result.Metrics = IntegrationMetrics{
		EntryPointsWired:       len(result.WiredEntryPoints),
		IntegrationPointsCreated: len(result.IntegrationPoints),
		EventsDefined:          len(result.EventDefinitions),
		HandlersRegistered:     e.countHandlers(result.EventDefinitions),
		FilesGenerated:         len(result.GeneratedFiles),
		LinesOfCodeGenerated:   e.countLinesOfCode(result.GeneratedFiles),
		ValidationPassed:       e.countPassed(result.ValidationErrors),
		ValidationFailed:       e.countFailed(result.ValidationErrors),
	}
	result.Duration = time.Since(startTime)
	result.Success = len(result.ValidationErrors) == 0

	return result, nil
}

func (e *WiringEngine) createConfigIntegration(ctx context.Context, feature *FeatureAppearance, ep *ConfigEntryPoint) (*IntegrationPoint, error) {
	ip := &IntegrationPoint{
		ID:            uuid.New(),
		FeatureID:     feature.FeatureID,
		EntryPointID:  ep.ID,
		Pattern:       WiringPatternAdapter,
		Configuration: map[string]interface{}{"config_key": ep.ConfigKey, "env_var": ep.EnvVar},
		Status:        IntegrationStatusWired,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	template := e.GetWiringTemplate(WiringPatternAdapter)
	if template != nil {
		params := map[string]string{
			"PackageName":   "config",
			"AdapterName":   strings.Title(strings.ReplaceAll(ep.ConfigKey, ".", "")),
			"ConfigKey":     strings.Title(strings.ReplaceAll(ep.ConfigKey, ".", "")),
			"ConfigPath":    ep.ConfigKey,
			"ConfigType":    ep.ConfigType,
			"DefaultValue":  ep.DefaultValue,
		}
		ip.CodeTemplate = template.Template
		ip.GeneratedCode = e.renderTemplate(template.Template, params)
	}

	return ip, nil
}

func (e *WiringEngine) GetWiringTemplate(pattern WiringPattern) *WiringTemplate {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.templates[pattern]
}

func (e *WiringEngine) RegisterTemplate(template *WiringTemplate) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.templates[template.Pattern] = template
}

func (e *WiringEngine) ValidateWiring(ctx context.Context, feature *FeatureAppearance) (*ValidationResult, error) {
	result := e.appearanceValidator.ValidateAppearance(ctx, feature)
	return result, nil
}

func (e *WiringEngine) GenerateIntegrationCode(ctx context.Context, feature *FeatureAppearance, targetFiles []string) ([]GeneratedFile, error) {
	var files []GeneratedFile

	for epType, entryPoints := range feature.EntryPoints {
		for _, ep := range entryPoints {
			generated, err := e.generateEntryPointCode(ctx, feature, ep, epType)
			if err != nil {
				return nil, err
			}
			files = append(files, generated...)
		}
	}

	for _, ep := range feature.ConfigKeys {
		generated, err := e.generateConfigCode(ctx, feature, &ep)
		if err != nil {
			return nil, err
		}
		files = append(files, generated...)
	}

	for _, event := range feature.Documentation.APIDocs {
		_ = event
	}

	return files, nil
}

func (e *WiringEngine) generateEntryPointCode(ctx context.Context, feature *FeatureAppearance, ep EntryPointInterface, epType EntryPointType) ([]GeneratedFile, error) {
	var files []GeneratedFile

	switch epType {
	case EntryPointTypeTUI:
		if tuiEP, ok := ep.(*TUIEntryPoint); ok {
			template := e.GetWiringTemplate(WiringPatternCommand)
			if template != nil {
				params := map[string]string{
					"PackageName":    "commands",
					"CommandName":    tuiEP.CommandName,
					"CommandStruct":  strings.Title(strings.ReplaceAll(tuiEP.CommandName, "-", "")) + "Command",
					"Description":    tuiEP.Description,
					"Usage":          tuiEP.CommandName + " [args]",
					"Implementation": "// TODO: Implement command logic",
				}
				content := e.renderTemplate(template.Template, params)
				files = append(files, GeneratedFile{
					Path:      "internal/tui/commands/" + tuiEP.CommandName + ".go",
					Content:   content,
					Operation: "create",
				})
			}
		}
	case EntryPointTypeAPI:
		if apiEP, ok := ep.(*APIEntryPoint); ok {
			content := e.generateAPIHandler(feature, apiEP)
			files = append(files, GeneratedFile{
				Path:      "internal/api/handlers/" + strings.ReplaceAll(apiEP.Route, "/", "_") + ".go",
				Content:   content,
				Operation: "create",
			})
		}
	case EntryPointTypeCLI:
		if cliEP, ok := ep.(*CLIEntryPoint); ok {
			content := e.generateCLICommand(feature, cliEP)
			files = append(files, GeneratedFile{
				Path:      "internal/cli/commands/" + cliEP.CommandName + ".go",
				Content:   content,
				Operation: "create",
			})
		}
	case EntryPointTypeWebhook:
		if webhookEP, ok := ep.(*WebhookEntryPoint); ok {
			template := e.GetWiringTemplate(WiringPatternWebhook)
			if template != nil {
				params := map[string]string{
					"PackageName":      "webhooks",
					"WebhookName":      strings.Title(strings.ReplaceAll(webhookEP.Name, "-", "")),
					"Secret":           webhookEP.Secret,
					"SignatureHeader":  webhookEP.SignatureHeader,
					"PayloadType":      "WebhookPayload",
					"Implementation":   "// TODO: Implement webhook processing",
				}
				content := e.renderTemplate(template.Template, params)
				files = append(files, GeneratedFile{
					Path:      "internal/webhooks/" + webhookEP.Name + ".go",
					Content:   content,
					Operation: "create",
				})
			}
		}
	}

	return files, nil
}

func (e *WiringEngine) generateConfigCode(ctx context.Context, feature *FeatureAppearance, ep *ConfigEntryPoint) ([]GeneratedFile, error) {
	template := e.GetWiringTemplate(WiringPatternAdapter)
	if template == nil {
		return nil, nil
	}

	params := map[string]string{
		"PackageName":   "config",
		"AdapterName":   strings.Title(strings.ReplaceAll(ep.ConfigKey, ".", "")),
		"ConfigKey":     strings.Title(strings.ReplaceAll(ep.ConfigKey, ".", "")),
		"ConfigPath":    ep.ConfigKey,
		"ConfigType":    ep.ConfigType,
		"DefaultValue":  ep.DefaultValue,
	}
	content := e.renderTemplate(template.Template, params)

	return []GeneratedFile{{
		Path:      "internal/config/adapters/" + strings.ReplaceAll(ep.ConfigKey, ".", "_") + ".go",
		Content:   content,
		Operation: "create",
	}}, nil
}

func (e *WiringEngine) generateAPIHandler(feature *FeatureAppearance, ep *APIEntryPoint) string {
	return fmt.Sprintf(`package handlers

import (
	"net/http"
	"github.com/owecode/api"
)

func %sHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement %s handler
	api.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func init() {
	api.RegisterRoute("%s", "%s", %sHandler)
}`, strings.Title(strings.ReplaceAll(ep.Route, "/", "")), ep.Route, ep.Method, ep.Route, strings.Title(strings.ReplaceAll(ep.Route, "/", "")))
}

func (e *WiringEngine) generateCLICommand(feature *FeatureAppearance, ep *CLIEntryPoint) string {
	flags := ""
	for _, arg := range ep.Args {
		flags += fmt.Sprintf(`    cmd.Flags().String("%s", "%s", "%s")
`, arg.Name, arg.Default, arg.Description)
	}

	return fmt.Sprintf(`package commands

import (
	"github.com/spf13/cobra"
)

var %sCmd = &cobra.Command{
	Use:   "%s",
	Short: "%s",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement %s command
		return nil
	},
}

func init() {
%s	RootCmd.AddCommand(%sCmd)
}`, strings.Title(ep.CommandName), ep.CommandName, ep.Description, ep.CommandName, flags, strings.Title(ep.CommandName))
}

func (e *WiringEngine) renderTemplate(template string, params map[string]string) string {
	result := template
	for key, value := range params {
		result = strings.ReplaceAll(result, "{{."+key+"}}", value)
	}
	return result
}

func (e *WiringEngine) countHandlers(events []EventDefinition) int {
	count := 0
	for _, event := range events {
		count += len(event.Handlers)
	}
	return count
}

func (e *WiringEngine) countLinesOfCode(files []GeneratedFile) int {
	count := 0
	for _, file := range files {
		count += len(strings.Split(file.Content, "\n"))
	}
	return count
}

func (e *WiringEngine) countPassed(errors []ValidationError) int {
	count := 0
	for _, err := range errors {
		if err.Severity != "error" {
			count++
		}
	}
	return count
}

func (e *WiringEngine) countFailed(errors []ValidationError) int {
	count := 0
	for _, err := range errors {
		if err.Severity == "error" {
			count++
		}
	}
	return count
}

func (e *WiringEngine) GetEntryPointWiring() *EntryPointWiring {
	return e.entryPointWiring
}

func (e *WiringEngine) GetEventSystem() *EventSystem {
	return e.eventSystem
}

func (e *WiringEngine) GetAppearanceValidator() *AppearanceValidator {
	return e.appearanceValidator
}