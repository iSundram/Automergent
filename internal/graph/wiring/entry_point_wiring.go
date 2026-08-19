package wiring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iSundram/Automergent/internal/graph"
)

type EntryPointWiring struct {
	store      *graph.Store
	engine     *WiringEngine
	registry   *EntryPointRegistry
	detectors  map[EntryPointType]EntryPointDetector
	mu         sync.RWMutex
}

type EntryPointDetector interface {
	Detect(ctx context.Context, projectPath string) ([]EntryPoint, error)
	Type() EntryPointType
}

func NewEntryPointWiring(store *graph.Store, engine *WiringEngine) *EntryPointWiring {
	epw := &EntryPointWiring{
		store:     store,
		engine:    engine,
		detectors: make(map[EntryPointType]EntryPointDetector),
		registry: &EntryPointRegistry{
			EntryPoints: make(map[EntryPointType][]EntryPointInterface),
			Metadata:    make(map[string]interface{}),
			UpdatedAt:   time.Now(),
		},
	}
	epw.registerDefaultDetectors()
	return epw
}

func (epw *EntryPointWiring) registerDefaultDetectors() {
	epw.detectors[EntryPointTypeTUI] = &TUIDetector{store: epw.store}
	epw.detectors[EntryPointTypeAPI] = &APIDetector{store: epw.store}
	epw.detectors[EntryPointTypeCLI] = &CLIDetector{store: epw.store}
	epw.detectors[EntryPointTypeConfig] = &ConfigDetector{store: epw.store}
	epw.detectors[EntryPointTypeWebhook] = &WebhookDetector{store: epw.store}
}

func (epw *EntryPointWiring) DetectEntryPoints(ctx context.Context, projectPath string) (*EntryPointRegistry, error) {
	epw.mu.Lock()
	defer epw.mu.Unlock()

	registry := &EntryPointRegistry{
		EntryPoints: make(map[EntryPointType][]EntryPointInterface),
		Metadata:    make(map[string]interface{}),
		UpdatedAt:   time.Now(),
	}

	for epType, detector := range epw.detectors {
		entryPoints, err := detector.Detect(ctx, projectPath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect %s entry points: %w", epType, err)
		}
		// Convert []EntryPoint to []EntryPointInterface
		interfaces := make([]EntryPointInterface, len(entryPoints))
		for i, ep := range entryPoints {
			interfaces[i] = ep
		}
		registry.EntryPoints[epType] = interfaces
	}

	epw.registry = registry
	return registry, nil
}

func (epw *EntryPointWiring) WireToEntryPoint(ctx context.Context, feature *FeatureAppearance, entryPoint EntryPointInterface) (*EntryPoint, error) {
	epw.mu.Lock()
	defer epw.mu.Unlock()

	baseEP := entryPoint.GetBaseEntryPoint()
	wiredEP := *baseEP
	wiredEP.ID = uuid.New()
	wiredEP.Enabled = true
	wiredEP.CreatedAt = time.Now()
	wiredEP.UpdatedAt = time.Now()

	switch ep := entryPoint.(type) {
	case *TUIEntryPoint:
		return epw.wireTUIEntryPoint(ctx, feature, ep)
	case *APIEntryPoint:
		return epw.wireAPIEntryPoint(ctx, feature, ep)
	case *CLIEntryPoint:
		return epw.wireCLIEntryPoint(ctx, feature, ep)
	case *ConfigEntryPoint:
		return epw.wireConfigEntryPoint(ctx, feature, ep)
	case *WebhookEntryPoint:
		return epw.wireWebhookEntryPoint(ctx, feature, ep)
	default:
		return &wiredEP, nil
	}
}

func (epw *EntryPointWiring) wireTUIEntryPoint(ctx context.Context, feature *FeatureAppearance, ep *TUIEntryPoint) (*EntryPoint, error) {
	wired := *ep
	wired.ID = uuid.New()
	wired.Enabled = true
	wired.CreatedAt = time.Now()
	wired.UpdatedAt = time.Now()

	registrationCode := fmt.Sprintf(`// Auto-generated TUI command registration
package commands

import (
	"github.com/owecode/tui/commands"
)

func init() {
	commands.Register("%s", &%sCommand{})
}

type %sCommand struct{}

func (c *%sCommand) Execute(ctx context.Context, args []string) error {
	// TODO: Implement %s command
	return nil
}

func (c *%sCommand) Description() string {
	return "%s"
}

func (c *%sCommand) Usage() string {
	return "%s [args]"
}
`, ep.CommandName, strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")), 
		strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")), 
		strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")), 
		ep.CommandName, 
		strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")), 
		ep.Description, 
		strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")), 
		ep.CommandName)

	wired.Metadata["registration_code"] = registrationCode
	wired.Handler = "commands." + strings.Title(strings.ReplaceAll(ep.CommandName, "-", "")) + "Command"

	return &wired.EntryPoint, nil
}

func (epw *EntryPointWiring) wireAPIEntryPoint(ctx context.Context, feature *FeatureAppearance, ep *APIEntryPoint) (*EntryPoint, error) {
	wired := *ep
	wired.ID = uuid.New()
	wired.Enabled = true
	wired.CreatedAt = time.Now()
	wired.UpdatedAt = time.Now()

	middleware := strings.Join(ep.Middleware, ", ")
	registrationCode := fmt.Sprintf(`// Auto-generated API route registration
package api

import (
	"net/http"
	"github.com/owecode/api"
)

func %sHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement %s handler
	api.RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func init() {
	api.RegisterRoute("%s", "%s", %sHandler%s)
}
`, strings.Title(strings.ReplaceAll(ep.Route, "/", "")), ep.Route, 
		ep.Method, ep.Route, strings.Title(strings.ReplaceAll(ep.Route, "/", "")),
		func() string {
			if middleware != "" {
				return ", " + middleware
			}
			return ""
		}())

	wired.Metadata["registration_code"] = registrationCode
	wired.Handler = "api." + strings.Title(strings.ReplaceAll(ep.Route, "/", "")) + "Handler"

	return &wired.EntryPoint, nil
}

func (epw *EntryPointWiring) wireCLIEntryPoint(ctx context.Context, feature *FeatureAppearance, ep *CLIEntryPoint) (*EntryPoint, error) {
	wired := *ep
	wired.ID = uuid.New()
	wired.Enabled = true
	wired.CreatedAt = time.Now()
	wired.UpdatedAt = time.Now()

	flags := ""
	for _, arg := range ep.Args {
		flags += fmt.Sprintf(`	cmd.Flags().String("%s", "%s", "%s")
`, arg.Name, arg.Default, arg.Description)
	}

	registrationCode := fmt.Sprintf(`// Auto-generated CLI command registration
package commands

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
}
`, strings.Title(ep.CommandName), ep.CommandName, ep.Description, ep.CommandName, flags, strings.Title(ep.CommandName))

	wired.Metadata["registration_code"] = registrationCode
	wired.Handler = "commands." + strings.Title(ep.CommandName) + "Cmd"

	return &wired.EntryPoint, nil
}

func (epw *EntryPointWiring) wireConfigEntryPoint(ctx context.Context, feature *FeatureAppearance, ep *ConfigEntryPoint) (*EntryPoint, error) {
	wired := *ep
	wired.ID = uuid.New()
	wired.Enabled = true
	wired.CreatedAt = time.Now()
	wired.UpdatedAt = time.Now()

	registrationCode := fmt.Sprintf(`// Auto-generated config key registration
package config

import (
	"github.com/owecode/config"
)

func init() {
	config.RegisterKey(config.Key{
		Key:         "%s",
		EnvVar:      "%s",
		Type:        "%s",
		Default:     "%s",
		Description: "%s",
		Required:    %v,
		Secret:      %v,
		Validation:  "%s",
	})
}
`, ep.ConfigKey, ep.EnvVar, ep.ConfigType, ep.DefaultValue, ep.Description, ep.Required, ep.Secret, ep.ValidationRegex)

	wired.Metadata["registration_code"] = registrationCode
	wired.Handler = "config.Get" + strings.Title(strings.ReplaceAll(ep.ConfigKey, ".", ""))

	return &wired.EntryPoint, nil
}

func (epw *EntryPointWiring) wireWebhookEntryPoint(ctx context.Context, feature *FeatureAppearance, ep *WebhookEntryPoint) (*EntryPoint, error) {
	wired := *ep
	wired.ID = uuid.New()
	wired.Enabled = true
	wired.CreatedAt = time.Now()
	wired.UpdatedAt = time.Now()

	events := strings.Join(ep.Events, ", ")
	registrationCode := fmt.Sprintf(`// Auto-generated webhook handler registration
package webhooks

import (
	"net/http"
	"github.com/owecode/webhooks"
)

func %sHandler(w http.ResponseWriter, r *http.Request) {
	// Verify signature
	// Parse payload
	// Process event
}

func init() {
	webhooks.Register("%s", webhooks.WebhookConfig{
		URL:             "%s",
		Secret:          "%s",
		Events:          []string{%s},
		Headers:         %v,
		Timeout:         %v,
		VerifySSL:       %v,
		ContentType:     "%s",
		SignatureHeader: "%s",
	}, %sHandler)
}
`, strings.Title(strings.ReplaceAll(ep.Name, "-", "")), ep.Name, ep.URL, ep.Secret, events, ep.Headers, ep.Timeout, ep.VerifySSL, ep.ContentType, ep.SignatureHeader, strings.Title(strings.ReplaceAll(ep.Name, "-", "")))

	wired.Metadata["registration_code"] = registrationCode
	wired.Handler = "webhooks." + strings.Title(strings.ReplaceAll(ep.Name, "-", "")) + "Handler"

	return &wired.EntryPoint, nil
}

func (epw *EntryPointWiring) ValidateEntryPointCoverage(ctx context.Context, feature *FeatureAppearance) (*ValidationResult, error) {
	result := &ValidationResult{
		Errors:   []ValidationError{},
		Warnings: []string{},
		Valid:    true,
	}

	requiredTypes := []EntryPointType{EntryPointTypeTUI, EntryPointTypeAPI, EntryPointTypeCLI, EntryPointTypeConfig}
	for _, reqType := range requiredTypes {
		eps, exists := feature.EntryPoints[reqType]
		if !exists || len(eps) == 0 {
			result.Errors = append(result.Errors, ValidationError{
				Code:       "MISSING_ENTRY_POINT",
				Message:    fmt.Sprintf("Feature missing required entry point type: %s", reqType),
				Severity:   "error",
				Location:   "feature_entry_points",
				EntryPoint: reqType,
				FeatureID:  feature.FeatureID,
				Suggestion: fmt.Sprintf("Add at least one %s entry point", reqType),
			})
			result.Valid = false
		}
	}

	for epType, eps := range feature.EntryPoints {
		for _, ep := range eps {
			if ep.GetHandler() == "" {
				result.Errors = append(result.Errors, ValidationError{
					Code:       "MISSING_HANDLER",
					Message:    fmt.Sprintf("Entry point %s has no handler", ep.GetName()),
					Severity:   "error",
					Location:   ep.GetPath(),
					EntryPoint: epType,
					FeatureID:  feature.FeatureID,
					Suggestion: "Implement handler for this entry point",
				})
				result.Valid = false
			}
		}
	}

	if len(feature.ConfigKeys) == 0 {
		result.Warnings = append(result.Warnings, "Feature has no configuration keys defined")
	}

	if len(feature.Documentation.Readme) == 0 {
		result.Warnings = append(result.Warnings, "Feature missing README documentation")
	}

	return result, nil
}

func (epw *EntryPointWiring) GetEntryPointRegistry() *EntryPointRegistry {
	epw.mu.RLock()
	defer epw.mu.RUnlock()
	return epw.registry
}

func (epw *EntryPointWiring) RegisterDetector(detector EntryPointDetector) {
	epw.mu.Lock()
	defer epw.mu.Unlock()
	epw.detectors[detector.Type()] = detector
}

type TUIDetector struct {
	store *graph.Store
}

func (d *TUIDetector) Type() EntryPointType {
	return EntryPointTypeTUI
}

func (d *TUIDetector) Detect(ctx context.Context, projectPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	commands := []TUIEntryPoint{
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeTUI,
				Name:        "help",
				Description: "Show help information",
				Path:        "internal/tui/commands/help.go",
				Handler:     "commands.HelpCommand",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			CommandName:  "help",
			MenuPath:     "Help",
			KeyBinding:   "F1",
			Aliases:      []string{"h", "?"},
			Category:     "General",
			Hidden:       false,
			RequiresAuth: false,
		},
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeTUI,
				Name:        "settings",
				Description: "Open settings menu",
				Path:        "internal/tui/commands/settings.go",
				Handler:     "commands.SettingsCommand",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			CommandName:  "settings",
			MenuPath:     "Settings",
			KeyBinding:   "Ctrl+S",
			Aliases:      []string{"config", "prefs"},
			Category:     "Configuration",
			Hidden:       false,
			RequiresAuth: false,
		},
	}

	for _, cmd := range commands {
		entryPoints = append(entryPoints, cmd.EntryPoint)
	}

	return entryPoints, nil
}

type APIDetector struct {
	store *graph.Store
}

func (d *APIDetector) Type() EntryPointType {
	return EntryPointTypeAPI
}

func (d *APIDetector) Detect(ctx context.Context, projectPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	endpoints := []APIEntryPoint{
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeAPI,
				Name:        "health",
				Description: "Health check endpoint",
				Path:        "/health",
				Handler:     "api.HealthHandler",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			Method:        "GET",
			Route:         "/health",
			Websocket:     false,
			Middleware:    []string{"logging", "recovery"},
			AuthRequired:  false,
			RateLimit:     &RateLimitConfig{Requests: 100, Window: time.Minute, Burst: 10},
		},
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeAPI,
				Name:        "api-info",
				Description: "API information endpoint",
				Path:        "/api/v1/info",
				Handler:     "api.InfoHandler",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			Method:        "GET",
			Route:         "/api/v1/info",
			Websocket:     false,
			Middleware:    []string{"logging", "recovery", "auth"},
			AuthRequired:  true,
			RateLimit:     &RateLimitConfig{Requests: 60, Window: time.Minute, Burst: 5},
		},
	}

	for _, ep := range endpoints {
		entryPoints = append(entryPoints, ep.EntryPoint)
	}

	return entryPoints, nil
}

type CLIDetector struct {
	store *graph.Store
}

func (d *CLIDetector) Type() EntryPointType {
	return EntryPointTypeCLI
}

func (d *CLIDetector) Detect(ctx context.Context, projectPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	commands := []CLIEntryPoint{
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeCLI,
				Name:        "version",
				Description: "Show version information",
				Path:        "internal/cli/commands/version.go",
				Handler:     "commands.VersionCmd",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			FlagName:    "version",
			ShortFlag:   "v",
			CommandName: "version",
			Args:        []CLIArg{},
			EnvVar:      "OWECODE_VERSION",
			Required:    false,
			Hidden:      false,
		},
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeCLI,
				Name:        "config",
				Description: "Manage configuration",
				Path:        "internal/cli/commands/config.go",
				Handler:     "commands.ConfigCmd",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			FlagName:    "config",
			ShortFlag:   "c",
			CommandName: "config",
			SubCommands: []string{"get", "set", "list", "edit"},
			Args: []CLIArg{
				{Name: "key", Type: "string", Description: "Config key", Required: true},
				{Name: "value", Type: "string", Description: "Config value", Required: false},
			},
			EnvVar:   "OWECODE_CONFIG",
			Required: false,
			Hidden:   false,
		},
	}

	for _, cmd := range commands {
		entryPoints = append(entryPoints, cmd.EntryPoint)
	}

	return entryPoints, nil
}

type ConfigDetector struct {
	store *graph.Store
}

func (d *ConfigDetector) Type() EntryPointType {
	return EntryPointTypeConfig
}

func (d *ConfigDetector) Detect(ctx context.Context, projectPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	configs := []ConfigEntryPoint{
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeConfig,
				Name:        "log-level",
				Description: "Logging level",
				Path:        "config/logging.yaml",
				Handler:     "config.GetLogLevel",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			ConfigKey:       "logging.level",
			EnvVar:          "OWECODE_LOG_LEVEL",
			ConfigType:      "string",
			DefaultValue:    "info",
			Description:     "Set the logging level (debug, info, warn, error)",
			ValidationRegex: "^(debug|info|warn|error)$",
			Required:        false,
			Secret:          false,
			Deprecated:      false,
		},
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeConfig,
				Name:        "api-port",
				Description: "API server port",
				Path:        "config/server.yaml",
				Handler:     "config.GetAPIPort",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			ConfigKey:       "server.port",
			EnvVar:          "OWECODE_API_PORT",
			ConfigType:      "int",
			DefaultValue:    "8080",
			Description:     "Port for the API server",
			ValidationRegex: "^[1-9][0-9]{0,4}$",
			Required:        false,
			Secret:          false,
			Deprecated:      false,
		},
	}

	for _, cfg := range configs {
		entryPoints = append(entryPoints, cfg.EntryPoint)
	}

	return entryPoints, nil
}

type WebhookDetector struct {
	store *graph.Store
}

func (d *WebhookDetector) Type() EntryPointType {
	return EntryPointTypeWebhook
}

func (d *WebhookDetector) Detect(ctx context.Context, projectPath string) ([]EntryPoint, error) {
	var entryPoints []EntryPoint

	webhooks := []WebhookEntryPoint{
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeWebhook,
				Name:        "github",
				Description: "GitHub webhook handler",
				Path:        "/webhooks/github",
				Handler:     "webhooks.GitHubHandler",
				Priority:    100,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			URL:             "https://api.github.com/webhooks",
			Secret:          "${GITHUB_WEBHOOK_SECRET}",
			Events:          []string{"push", "pull_request", "issues"},
			Headers:         map[string]string{"X-GitHub-Event": "event-type"},
			RetryPolicy:     &RetryPolicy{MaxAttempts: 3, Backoff: time.Second, MaxBackoff: time.Minute, Multiplier: 2},
			Timeout:         30 * time.Second,
			VerifySSL:       true,
			ContentType:     "application/json",
			SignatureHeader: "X-Hub-Signature-256",
		},
		{
			EntryPoint: EntryPoint{
				ID:          uuid.New(),
				Type:        EntryPointTypeWebhook,
				Name:        "slack",
				Description: "Slack webhook handler",
				Path:        "/webhooks/slack",
				Handler:     "webhooks.SlackHandler",
				Priority:    90,
				Enabled:     true,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			},
			URL:             "https://hooks.slack.com/services/xxx",
			Secret:          "${SLACK_WEBHOOK_SECRET}",
			Events:          []string{"message", "reaction", "channel"},
			Headers:         map[string]string{"X-Slack-Signature": "signature"},
			RetryPolicy:     &RetryPolicy{MaxAttempts: 3, Backoff: time.Second, MaxBackoff: time.Minute, Multiplier: 2},
			Timeout:         10 * time.Second,
			VerifySSL:       true,
			ContentType:     "application/json",
			SignatureHeader: "X-Slack-Signature",
		},
	}

	for _, wh := range webhooks {
		entryPoints = append(entryPoints, wh.EntryPoint)
	}

	return entryPoints, nil
}

func (epw *EntryPointWiring) PersistRegistry(ctx context.Context) error {
	data, err := json.Marshal(epw.registry)
	if err != nil {
		return err
	}

	node, err := graph.NewNode(graph.NodeTypeContextBucket, map[string]interface{}{
		"type": "entry_point_registry",
		"data": string(data),
	})
	if err != nil {
		return err
	}

	return epw.store.CreateNode(ctx, node)
}

func (epw *EntryPointWiring) LoadRegistry(ctx context.Context) error {
	nodes, err := epw.store.ListNodes(ctx, graph.NodeTypeContextBucket, 100, 0)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}
		if data["type"] == "entry_point_registry" {
			if registryData, ok := data["data"].(string); ok {
				return json.Unmarshal([]byte(registryData), epw.registry)
			}
		}
	}

	return nil
}