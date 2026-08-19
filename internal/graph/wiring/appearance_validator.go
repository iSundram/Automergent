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

type AppearanceValidator struct {
	store       *graph.Store
	engine      *WiringEngine
	validators  map[string]FeatureValidator
	mu          sync.RWMutex
}

type FeatureValidator interface {
	Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult
	Name() string
}

func NewAppearanceValidator(store *graph.Store, engine *WiringEngine) *AppearanceValidator {
	av := &AppearanceValidator{
		store:      store,
		engine:     engine,
		validators: make(map[string]FeatureValidator),
	}
	av.registerDefaultValidators()
	return av
}

func (av *AppearanceValidator) registerDefaultValidators() {
	av.validators["entry_points"] = &EntryPointValidator{engine: av.engine}
	av.validators["config"] = &ConfigValidator{store: av.store}
	av.validators["documentation"] = &DocumentationValidator{}
	av.validators["tests"] = &TestValidator{store: av.store}
	av.validators["compatibility"] = &CompatibilityValidator{store: av.store}
}

func (av *AppearanceValidator) ValidateAppearance(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	av.mu.RLock()
	defer av.mu.RUnlock()

	result := &ValidationResult{
		Errors:   []ValidationError{},
		Warnings: []string{},
		Valid:    true,
	}

	for _, validator := range av.validators {
		vr := validator.Validate(ctx, feature)
		result.Errors = append(result.Errors, vr.Errors...)
		result.Warnings = append(result.Warnings, vr.Warnings...)
		if !vr.Valid {
			result.Valid = false
		}
	}

	feature.Validated = result.Valid
	feature.ValidationErrors = extractErrorMessages(result.Errors)
	feature.LastValidated = time.Now()

	return result
}

func (av *AppearanceValidator) GenerateUsageExamples(ctx context.Context, feature *FeatureAppearance) ([]UsageExample, error) {
	var examples []UsageExample

	for epType, entryPoints := range feature.EntryPoints {
		for _, ep := range entryPoints {
			example := av.generateExampleForEntryPoint(feature, ep, epType)
			if example != nil {
				examples = append(examples, *example)
			}
		}
	}

	return examples, nil
}

func (av *AppearanceValidator) generateExampleForEntryPoint(feature *FeatureAppearance, ep EntryPointInterface, epType EntryPointType) *UsageExample {
	switch epType {
	case EntryPointTypeTUI:
		if tuiEP, ok := ep.(*TUIEntryPoint); ok {
			return &UsageExample{
				Name:        fmt.Sprintf("TUI: %s", tuiEP.CommandName),
				Description: fmt.Sprintf("Execute %s command in TUI", tuiEP.CommandName),
				EntryPoint:  EntryPointTypeTUI,
				Code:        fmt.Sprintf("> %s", tuiEP.CommandName),
				Language:    "shell",
				Context:     map[string]interface{}{"command": tuiEP.CommandName, "aliases": tuiEP.Aliases},
				ExpectedOutput: "Command executed successfully",
			}
		}
	case EntryPointTypeAPI:
		if apiEP, ok := ep.(*APIEntryPoint); ok {
			return &UsageExample{
				Name:        fmt.Sprintf("API: %s %s", apiEP.Method, apiEP.Route),
				Description: fmt.Sprintf("Call %s endpoint", apiEP.Route),
				EntryPoint:  EntryPointTypeAPI,
				Code:        fmt.Sprintf(`curl -X %s http://localhost:8080%s`, apiEP.Method, apiEP.Route),
				Language:    "shell",
				Context:     map[string]interface{}{"method": apiEP.Method, "route": apiEP.Route},
				ExpectedOutput: `{"status": "ok"}`,
			}
		}
	case EntryPointTypeCLI:
		if cliEP, ok := ep.(*CLIEntryPoint); ok {
			args := ""
			for _, arg := range cliEP.Args {
				if arg.Required {
					args += fmt.Sprintf(" <%s>", arg.Name)
				} else {
					args += fmt.Sprintf(" [<%s>]", arg.Name)
				}
			}
			return &UsageExample{
				Name:        fmt.Sprintf("CLI: %s", cliEP.CommandName),
				Description: fmt.Sprintf("Run %s command", cliEP.CommandName),
				EntryPoint:  EntryPointTypeCLI,
				Code:        fmt.Sprintf("owecode %s%s", cliEP.CommandName, args),
				Language:    "shell",
				Context:     map[string]interface{}{"command": cliEP.CommandName, "flags": cliEP.Args},
				ExpectedOutput: "Command executed successfully",
			}
		}
	case EntryPointTypeConfig:
		if configEP, ok := ep.(*ConfigEntryPoint); ok {
			return &UsageExample{
				Name:        fmt.Sprintf("Config: %s", configEP.ConfigKey),
				Description: fmt.Sprintf("Set %s configuration", configEP.ConfigKey),
				EntryPoint:  EntryPointTypeConfig,
				Code:        fmt.Sprintf("owecode config set %s %s", configEP.ConfigKey, configEP.DefaultValue),
				Language:    "shell",
				Context:     map[string]interface{}{"key": configEP.ConfigKey, "value": configEP.DefaultValue},
				ExpectedOutput: fmt.Sprintf("Set %s = %s", configEP.ConfigKey, configEP.DefaultValue),
			}
		}
	case EntryPointTypeWebhook:
		if webhookEP, ok := ep.(*WebhookEntryPoint); ok {
			return &UsageExample{
				Name:        fmt.Sprintf("Webhook: %s", webhookEP.Name),
				Description: fmt.Sprintf("Receive %s webhook", webhookEP.Name),
				EntryPoint:  EntryPointTypeWebhook,
				Code:        fmt.Sprintf(`curl -X POST %s -H "Content-Type: application/json" -d '{"event": "test"}'`, webhookEP.URL),
				Language:    "shell",
				Context:     map[string]interface{}{"url": webhookEP.URL, "events": webhookEP.Events},
				ExpectedOutput: "Webhook processed successfully",
			}
		}
	}
	return nil
}

func (av *AppearanceValidator) CheckIntegrationWithExisting(ctx context.Context, feature *FeatureAppearance) (*IntegrationCheckResult, error) {
	result := &IntegrationCheckResult{
		Compatible:      true,
		Conflicts:       []Conflict{},
		Dependencies:    []Dependency{},
		Recommendations: []string{},
	}

	existingFeatures, err := av.getExistingFeatures(ctx)
	if err != nil {
		return nil, err
	}

	for _, existing := range existingFeatures {
		if existing.FeatureID == feature.FeatureID {
			continue
		}

		conflicts := av.detectConflicts(feature, existing)
		result.Conflicts = append(result.Conflicts, conflicts...)
		if len(conflicts) > 0 {
			result.Compatible = false
		}

		deps := av.detectDependencies(feature, existing)
		result.Dependencies = append(result.Dependencies, deps...)

		recs := av.generateRecommendations(feature, existing)
		result.Recommendations = append(result.Recommendations, recs...)
	}

	return result, nil
}

func (av *AppearanceValidator) getExistingFeatures(ctx context.Context) ([]*FeatureAppearance, error) {
	nodes, err := av.store.ListNodes(ctx, graph.NodeTypeContextBucket, 100, 0)
	if err != nil {
		return nil, err
	}

	var features []*FeatureAppearance
	for _, node := range nodes {
		var data map[string]interface{}
		if err := node.UnmarshalData(&data); err != nil {
			continue
		}
		if data["type"] == "feature_appearance" {
			if featureData, ok := data["data"].(string); ok {
				var feature FeatureAppearance
				if json.Unmarshal([]byte(featureData), &feature) == nil {
					features = append(features, &feature)
				}
			}
		}
	}

	return features, nil
}

func (av *AppearanceValidator) detectConflicts(feature, existing *FeatureAppearance) []Conflict {
	var conflicts []Conflict

	for epType, eps := range feature.EntryPoints {
		if existingEps, ok := existing.EntryPoints[epType]; ok {
			for _, ep := range eps {
				for _, existingEP := range existingEps {
					if ep.GetName() == existingEP.GetName() || ep.GetPath() == existingEP.GetPath() {
						conflicts = append(conflicts, Conflict{
							Type:        "entry_point_collision",
							FeatureID:   existing.FeatureID,
							FeatureName: existingName(existing),
							Message:     fmt.Sprintf("Entry point %s conflicts with existing feature", ep.GetName()),
							Severity:    "error",
							Location:    ep.GetPath(),
						})
					}
				}
			}
		}
	}

	for _, configKey := range feature.ConfigKeys {
		for _, existingConfig := range existing.ConfigKeys {
			if configKey.ConfigKey == existingConfig.ConfigKey {
				conflicts = append(conflicts, Conflict{
					Type:        "config_key_collision",
					FeatureID:   existing.FeatureID,
					FeatureName: existingName(existing),
					Message:     fmt.Sprintf("Config key %s conflicts with existing feature", configKey.ConfigKey),
					Severity:    "error",
					Location:    configKey.ConfigKey,
				})
			}
		}
	}

	return conflicts
}

func (av *AppearanceValidator) detectDependencies(feature, existing *FeatureAppearance) []Dependency {
	var deps []Dependency

	for _, depID := range feature.Dependencies {
		if depID == existing.FeatureID {
			deps = append(deps, Dependency{
				Name:        existingName(existing),
				Version:     existing.Version,
				Required:    true,
				Description: fmt.Sprintf("Required by feature %s", featureName(feature)),
			})
		}
	}

	return deps
}

func (av *AppearanceValidator) generateRecommendations(feature, existing *FeatureAppearance) []string {
	var recs []string

	sharedEvents := av.findSharedEvents(feature, existing)
	if len(sharedEvents) > 0 {
		recs = append(recs, fmt.Sprintf("Consider sharing event definitions with %s: %v", existingName(existing), sharedEvents))
	}

	if av.hasSimilarEntryPoints(feature, existing) {
		recs = append(recs, fmt.Sprintf("Feature %s has similar entry points, consider consolidation", existingName(existing)))
	}

	return recs
}

func (av *AppearanceValidator) findSharedEvents(feature, existing *FeatureAppearance) []string {
	var shared []string
	// Implementation would compare event definitions
	return shared
}

func (av *AppearanceValidator) hasSimilarEntryPoints(feature, existing *FeatureAppearance) bool {
	for epType := range feature.EntryPoints {
		if _, ok := existing.EntryPoints[epType]; ok {
			return true
		}
	}
	return false
}

func (av *AppearanceValidator) GenerateFeatureManifest(ctx context.Context, feature *FeatureAppearance) (*FeatureManifest, error) {
	examples, _ := av.GenerateUsageExamples(ctx, feature)
	integrationCheck, _ := av.CheckIntegrationWithExisting(ctx, feature)

	manifest := &FeatureManifest{
		FeatureID:   feature.FeatureID,
		Name:        featureName(feature),
		Version:     feature.Version,
		Description: av.generateDescription(feature),
		Author:      "Auto-generated",
		License:     "MIT",
		EntryPoints: feature.EntryPoints,
		Events:      av.engine.eventSystem.GetEventDefinitions(feature.FeatureID),
		Dependencies: integrationCheck.Dependencies,
		Compatibility: CompatibilityMatrix{
			MinVersion:       "1.0.0",
			MaxVersion:       "2.0.0",
			IncompatibleWith: []string{},
			TestedWith:       []string{"1.0.0", "1.1.0"},
		},
		Installation: InstallationGuide{
			Steps: []InstallationStep{
				{Order: 1, Command: "go get github.com/owecode/...", Description: "Install dependencies", Required: true},
				{Order: 2, Command: "owecode feature install " + featureName(feature), Description: "Install feature", Required: true},
			},
			Prerequisites: []string{"Go 1.21+", "OweCode CLI"},
			PostInstall:   []string{"Restart TUI/API server", "Run configuration wizard"},
		},
		Usage:     examples,
		Testing:   feature.Tests,
		Changelog: av.generateChangelog(feature),
		GeneratedAt: time.Now(),
	}

	configSchema := av.generateConfigSchema(feature)
	manifest.ConfigSchema = configSchema

	return manifest, nil
}

func (av *AppearanceValidator) generateDescription(feature *FeatureAppearance) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Feature %s provides integration across multiple entry points.", featureName(feature)))

	epCount := 0
	for _, eps := range feature.EntryPoints {
		epCount += len(eps)
	}
	parts = append(parts, fmt.Sprintf("It exposes %d entry points across %d categories.", epCount, len(feature.EntryPoints)))

	if len(feature.ConfigKeys) > 0 {
		parts = append(parts, fmt.Sprintf("Configuration includes %d keys.", len(feature.ConfigKeys)))
	}

	return strings.Join(parts, " ")
}

func (av *AppearanceValidator) generateConfigSchema(feature *FeatureAppearance) json.RawMessage {
	properties := make(map[string]interface{})
	required := []string{}

	for _, config := range feature.ConfigKeys {
		prop := map[string]interface{}{
			"type":        config.ConfigType,
			"description": config.Description,
			"default":     config.DefaultValue,
		}
		if config.ValidationRegex != "" {
			prop["pattern"] = config.ValidationRegex
		}
		properties[config.ConfigKey] = prop
		if config.Required {
			required = append(required, config.ConfigKey)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	data, _ := json.Marshal(schema)
	return data
}

func (av *AppearanceValidator) generateChangelog(feature *FeatureAppearance) string {
	return fmt.Sprintf(`# Changelog

## [%s] - %s
### Added
- Initial feature release
- Entry points: %s
- Configuration keys: %d
`, feature.Version, time.Now().Format("2006-01-02"), av.listEntryPoints(feature), len(feature.ConfigKeys))
}

func (av *AppearanceValidator) listEntryPoints(feature *FeatureAppearance) string {
	var parts []string
	for epType, eps := range feature.EntryPoints {
		var names []string
		for _, ep := range eps {
			names = append(names, ep.GetName())
		}
		parts = append(parts, fmt.Sprintf("%s: %s", epType, strings.Join(names, ", ")))
	}
	return strings.Join(parts, "; ")
}

func featureName(feature *FeatureAppearance) string {
	if feature.FeatureID != uuid.Nil {
		return fmt.Sprintf("feature-%s", feature.FeatureID.String()[:8])
	}
	return "unknown-feature"
}

func existingName(feature *FeatureAppearance) string {
	return featureName(feature)
}

func extractErrorMessages(errors []ValidationError) []string {
	var messages []string
	for _, err := range errors {
		messages = append(messages, err.Message)
	}
	return messages
}

type IntegrationCheckResult struct {
	Compatible      bool            `json:"compatible"`
	Conflicts       []Conflict      `json:"conflicts"`
	Dependencies    []Dependency    `json:"dependencies"`
	Recommendations []string        `json:"recommendations"`
}

type Conflict struct {
	Type        string `json:"type"`
	FeatureID   uuid.UUID `json:"feature_id"`
	FeatureName string `json:"feature_name"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Location    string `json:"location"`
}

type EntryPointValidator struct {
	engine *WiringEngine
}

func (v *EntryPointValidator) Name() string {
	return "entry_points"
}

func (v *EntryPointValidator) Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if len(feature.EntryPoints) == 0 {
		result.Errors = append(result.Errors, ValidationError{
			Code:       "NO_ENTRY_POINTS",
			Message:    "Feature has no entry points defined",
			Severity:   "error",
			FeatureID:  feature.FeatureID,
			Suggestion: "Add at least one entry point (TUI, API, CLI, Config, or Webhook)",
		})
		result.Valid = false
	}

	requiredTypes := []EntryPointType{EntryPointTypeTUI, EntryPointTypeAPI, EntryPointTypeCLI}
	for _, reqType := range requiredTypes {
		if eps, ok := feature.EntryPoints[reqType]; !ok || len(eps) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Feature missing %s entry point", reqType))
		}
	}

	for epType, eps := range feature.EntryPoints {
		for _, ep := range eps {
			if ep.GetHandler() == "" {
				result.Errors = append(result.Errors, ValidationError{
					Code:       "MISSING_HANDLER",
					Message:    fmt.Sprintf("Entry point %s (%s) has no handler", ep.GetName(), epType),
					Severity:   "error",
					Location:   ep.GetPath(),
					EntryPoint: epType,
					FeatureID:  feature.FeatureID,
					Suggestion: "Implement handler for this entry point",
				})
				result.Valid = false
			}
			if ep.GetPath() == "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Entry point %s (%s) has no path", ep.GetName(), epType))
			}
		}
	}

	return result
}

type ConfigValidator struct {
	store *graph.Store
}

func (v *ConfigValidator) Name() string {
	return "config"
}

func (v *ConfigValidator) Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	result := &ValidationResult{Valid: true}

	for _, config := range feature.ConfigKeys {
		if config.ConfigKey == "" {
			result.Errors = append(result.Errors, ValidationError{
				Code:       "EMPTY_CONFIG_KEY",
				Message:    "Config entry has empty key",
				Severity:   "error",
				Location:   "config",
				EntryPoint: EntryPointTypeConfig,
				FeatureID:  feature.FeatureID,
				Suggestion: "Provide a valid config key",
			})
			result.Valid = false
		}

		if config.ConfigType == "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Config key %s has no type specified", config.ConfigKey))
		}

		if config.Secret && config.DefaultValue != "" {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Secret config key %s has a default value (security concern)", config.ConfigKey))
		}
	}

	return result
}

type DocumentationValidator struct{}

func (v *DocumentationValidator) Name() string {
	return "documentation"
}

func (v *DocumentationValidator) Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	result := &ValidationResult{Valid: true}

	doc := feature.Documentation
	if doc.Readme == "" {
		result.Warnings = append(result.Warnings, "Feature missing README documentation")
	}
	if doc.APIDocs == "" && hasAPIEntryPoints(feature) {
		result.Warnings = append(result.Warnings, "Feature has API entry points but no API documentation")
	}
	if doc.CLIDocs == "" && hasCLIEntryPoints(feature) {
		result.Warnings = append(result.Warnings, "Feature has CLI entry points but no CLI documentation")
	}
	if doc.TUIDocs == "" && hasTUIEntryPoints(feature) {
		result.Warnings = append(result.Warnings, "Feature has TUI entry points but no TUI documentation")
	}
	if doc.ConfigDocs == nil || len(doc.ConfigDocs) == 0 {
		if len(feature.ConfigKeys) > 0 {
			result.Warnings = append(result.Warnings, "Feature has config keys but no configuration documentation")
		}
	}

	return result
}

type TestValidator struct {
	store *graph.Store
}

func (v *TestValidator) Name() string {
	return "tests"
}

func (v *TestValidator) Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	result := &ValidationResult{Valid: true}

	tests := feature.Tests
	totalTests := len(tests.UnitTests) + len(tests.IntegrationTests) + len(tests.E2ETests)
	if totalTests == 0 {
		result.Warnings = append(result.Warnings, "Feature has no tests defined")
	}

	for _, test := range tests.UnitTests {
		if !test.Passing {
			result.Errors = append(result.Errors, ValidationError{
				Code:       "FAILING_UNIT_TEST",
				Message:    fmt.Sprintf("Unit test %s is failing", test.Name),
				Severity:   "error",
				Location:   test.Path,
				FeatureID:  feature.FeatureID,
				Suggestion: "Fix failing unit test",
			})
			result.Valid = false
		}
	}

	if tests.Coverage < 0.8 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Test coverage %.1f%% is below recommended 80%%", tests.Coverage*100))
	}

	return result
}

type CompatibilityValidator struct {
	store *graph.Store
}

func (v *CompatibilityValidator) Name() string {
	return "compatibility"
}

func (v *CompatibilityValidator) Validate(ctx context.Context, feature *FeatureAppearance) *ValidationResult {
	result := &ValidationResult{Valid: true}

	for _, conflictID := range feature.Conflicts {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Feature declares conflict with %s", conflictID))
	}

	return result
}

func hasAPIEntryPoints(feature *FeatureAppearance) bool {
	_, ok := feature.EntryPoints[EntryPointTypeAPI]
	return ok
}

func hasCLIEntryPoints(feature *FeatureAppearance) bool {
	_, ok := feature.EntryPoints[EntryPointTypeCLI]
	return ok
}

func hasTUIEntryPoints(feature *FeatureAppearance) bool {
	_, ok := feature.EntryPoints[EntryPointTypeTUI]
	return ok
}