package wiring

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WiringPattern string

const (
	WiringPatternCommand       WiringPattern = "command"
	WiringPatternEvent         WiringPattern = "event"
	WiringPatternMiddleware    WiringPattern = "middleware"
	WiringPatternHook          WiringPattern = "hook"
	WiringPatternExtension     WiringPattern = "extension"
	WiringPatternPlugin        WiringPattern = "plugin"
	WiringPatternInterceptor   WiringPattern = "interceptor"
	WiringPatternDecorator     WiringPattern = "decorator"
	WiringPatternAdapter       WiringPattern = "adapter"
	WiringPatternFacade        WiringPattern = "facade"
	WiringPatternObserver      WiringPattern = "observer"
	WiringPatternStrategy      WiringPattern = "strategy"
	WiringPatternTemplate      WiringPattern = "template"
	WiringPatternFactory       WiringPattern = "factory"
	WiringPatternBuilder       WiringPattern = "builder"
	WiringPatternSingleton     WiringPattern = "singleton"
	WiringPatternRegistry      WiringPattern = "registry"
	WiringPatternPipeline      WiringPattern = "pipeline"
	WiringPatternChain         WiringPattern = "chain"
	WiringPatternBridge        WiringPattern = "bridge"
	WiringPatternWebhook       WiringPattern = "webhook"
)

type EntryPointType string

const (
	EntryPointTypeTUI        EntryPointType = "tui"
	EntryPointTypeAPI        EntryPointType = "api"
	EntryPointTypeCLI        EntryPointType = "cli"
	EntryPointTypeConfig     EntryPointType = "config"
	EntryPointTypeWebhook    EntryPointType = "webhook"
)

type EntryPointInterface interface {
	GetBaseEntryPoint() *EntryPoint
	GetEntryPointType() EntryPointType
	GetID() uuid.UUID
	GetName() string
	GetDescription() string
	GetPath() string
	GetMetadata() map[string]interface{}
	GetHandler() string
	GetPriority() int
	GetEnabled() bool
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}

type EntryPoint struct {
	ID          uuid.UUID              `json:"id"`
	Type        EntryPointType         `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Path        string                 `json:"path"`
	Metadata    map[string]interface{} `json:"metadata"`
	Handler     string                 `json:"handler"`
	Priority    int                    `json:"priority"`
	Enabled     bool                   `json:"enabled"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

func (e EntryPoint) GetBaseEntryPoint() *EntryPoint {
	return &e
}

func (e EntryPoint) GetEntryPointType() EntryPointType {
	return e.Type
}

func (e EntryPoint) GetID() uuid.UUID {
	return e.ID
}

func (e EntryPoint) GetName() string {
	return e.Name
}

func (e EntryPoint) GetDescription() string {
	return e.Description
}

func (e EntryPoint) GetPath() string {
	return e.Path
}

func (e EntryPoint) GetMetadata() map[string]interface{} {
	return e.Metadata
}

func (e EntryPoint) GetHandler() string {
	return e.Handler
}

func (e EntryPoint) GetPriority() int {
	return e.Priority
}

func (e EntryPoint) GetEnabled() bool {
	return e.Enabled
}

func (e EntryPoint) GetCreatedAt() time.Time {
	return e.CreatedAt
}

func (e EntryPoint) GetUpdatedAt() time.Time {
	return e.UpdatedAt
}

type TUIEntryPoint struct {
	EntryPoint
	CommandName  string   `json:"command_name"`
	MenuPath     string   `json:"menu_path"`
	KeyBinding   string   `json:"key_binding"`
	Aliases      []string `json:"aliases"`
	Category     string   `json:"category"`
	Hidden       bool     `json:"hidden"`
	RequiresAuth bool     `json:"requires_auth"`
}

type APIEntryPoint struct {
	EntryPoint
	Method       string            `json:"method"`
	Route        string            `json:"route"`
	Websocket    bool              `json:"websocket"`
	Middleware   []string          `json:"middleware"`
	RateLimit    *RateLimitConfig  `json:"rate_limit"`
	AuthRequired bool              `json:"auth_required"`
	RequestSchema json.RawMessage  `json:"request_schema"`
	ResponseSchema json.RawMessage `json:"response_schema"`
}

type CLIEntryPoint struct {
	EntryPoint
	FlagName        string   `json:"flag_name"`
	ShortFlag       string   `json:"short_flag"`
	CommandName     string   `json:"command_name"`
	SubCommands     []string `json:"sub_commands"`
	Args            []CLIArg `json:"args"`
	EnvVar          string   `json:"env_var"`
	DefaultValue    string   `json:"default_value"`
	Required        bool     `json:"required"`
	Hidden          bool     `json:"hidden"`
}

type CLIArg struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
}

type ConfigEntryPoint struct {
	EntryPoint
	ConfigKey       string `json:"config_key"`
	EnvVar          string `json:"env_var"`
	ConfigType      string `json:"config_type"`
	DefaultValue    string `json:"default_value"`
	Description     string `json:"description"`
	ValidationRegex string `json:"validation_regex"`
	Required        bool   `json:"required"`
	Secret          bool   `json:"secret"`
	Deprecated      bool   `json:"deprecated"`
}

type WebhookEntryPoint struct {
	EntryPoint
	URL             string            `json:"url"`
	Secret          string            `json:"secret"`
	Events          []string          `json:"events"`
	Headers         map[string]string `json:"headers"`
	RetryPolicy     *RetryPolicy      `json:"retry_policy"`
	Timeout         time.Duration     `json:"timeout"`
	VerifySSL       bool              `json:"verify_ssl"`
	ContentType     string            `json:"content_type"`
	SignatureHeader string            `json:"signature_header"`
}

type RateLimitConfig struct {
	Requests int           `json:"requests"`
	Window   time.Duration `json:"window"`
	Burst    int           `json:"burst"`
}

type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Backoff     time.Duration `json:"backoff"`
	MaxBackoff  time.Duration `json:"max_backoff"`
	Multiplier  float64       `json:"multiplier"`
}

type IntegrationPoint struct {
	ID              uuid.UUID              `json:"id"`
	FeatureID       uuid.UUID              `json:"feature_id"`
	EntryPointID    uuid.UUID              `json:"entry_point_id"`
	TargetFeatureID uuid.UUID              `json:"target_feature_id"`
	TargetEntryPointID uuid.UUID           `json:"target_entry_point_id"`
	Pattern         WiringPattern          `json:"pattern"`
	Configuration   map[string]interface{} `json:"configuration"`
	CodeTemplate    string                 `json:"code_template"`
	GeneratedCode   string                 `json:"generated_code"`
	Status          IntegrationStatus      `json:"status"`
	ValidationError string                 `json:"validation_error,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type IntegrationStatus string

const (
	IntegrationStatusPending    IntegrationStatus = "pending"
	IntegrationStatusWired      IntegrationStatus = "wired"
	IntegrationStatusValidated  IntegrationStatus = "validated"
	IntegrationStatusDeployed   IntegrationStatus = "deployed"
	IntegrationStatusFailed     IntegrationStatus = "failed"
	IntegrationStatusDeprecated IntegrationStatus = "deprecated"
)

type FeatureAppearance struct {
	FeatureID       uuid.UUID                     `json:"feature_id"`
	EntryPoints     map[EntryPointType][]EntryPointInterface `json:"entry_points"`
	ConfigKeys      []ConfigEntryPoint            `json:"config_keys"`
	EnvironmentVars []string                      `json:"environment_vars"`
	Documentation   FeatureDocumentation          `json:"documentation"`
	Tests           FeatureTests                  `json:"tests"`
	Examples        []UsageExample                `json:"examples"`
	Dependencies    []uuid.UUID                   `json:"dependencies"`
	Conflicts       []uuid.UUID                   `json:"conflicts"`
	Version         string                        `json:"version"`
	Validated       bool                          `json:"validated"`
	ValidationErrors []string                     `json:"validation_errors"`
	LastValidated   time.Time                     `json:"last_validated"`
}

type FeatureDocumentation struct {
	Readme          string            `json:"readme"`
	APIDocs         string            `json:"api_docs"`
	CLIDocs         string            `json:"cli_docs"`
	TUIDocs         string            `json:"tui_docs"`
	WebhookDocs     string            `json:"webhook_docs"`
	ConfigDocs      map[string]string `json:"config_docs"`
	ArchitectureDoc string            `json:"architecture_doc"`
	Changelog       string            `json:"changelog"`
}

type FeatureTests struct {
	UnitTests       []TestCase `json:"unit_tests"`
	IntegrationTests []TestCase `json:"integration_tests"`
	E2ETests        []TestCase `json:"e2e_tests"`
	Benchmarks      []TestCase `json:"benchmarks"`
	Coverage        float64     `json:"coverage"`
}

type TestCase struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
	EntryPoints []EntryPointType `json:"entry_points"`
	Passing     bool   `json:"passing"`
	LastRun     time.Time `json:"last_run"`
}

type UsageExample struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	EntryPoint  EntryPointType         `json:"entry_point"`
	Code        string                 `json:"code"`
	Language    string                 `json:"language"`
	Context     map[string]interface{} `json:"context"`
	ExpectedOutput string              `json:"expected_output"`
}

type EventDefinition struct {
	ID              uuid.UUID              `json:"id"`
	FeatureID       uuid.UUID              `json:"feature_id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	EventType       string                 `json:"event_type"`
	PayloadSchema   json.RawMessage        `json:"payload_schema"`
	EmissionPoints  []EmissionPoint        `json:"emission_points"`
	Handlers        []EventHandler         `json:"handlers"`
	PropagationRule PropagationRule        `json:"propagation_rule"`
	Priority        int                    `json:"priority"`
	Async           bool                   `json:"async"`
	RetryPolicy     *RetryPolicy           `json:"retry_policy"`
	DeadLetterQueue string                 `json:"dead_letter_queue"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type EmissionPoint struct {
	ID            uuid.UUID `json:"id"`
	EventID       uuid.UUID `json:"event_id"`
	Location      string    `json:"location"`
	Function      string    `json:"function"`
	Condition     string    `json:"condition"`
	PayloadBuilder string   `json:"payload_builder"`
	Priority      int       `json:"priority"`
}

type EventHandler struct {
	ID              uuid.UUID `json:"id"`
	EventID         uuid.UUID `json:"event_id"`
	FeatureID       uuid.UUID `json:"feature_id"`
	Function        string    `json:"function"`
	Filter          string    `json:"filter"`
	Priority        int       `json:"priority"`
	Async           bool      `json:"async"`
	Timeout         time.Duration `json:"timeout"`
	RetryPolicy     *RetryPolicy  `json:"retry_policy"`
	Order           int       `json:"order"`
}

type PropagationRule string

const (
	PropagationRuleBroadcast PropagationRule = "broadcast"
	PropagationRuleDirect    PropagationRule = "direct"
	PropagationRuleChain     PropagationRule = "chain"
	PropagationRuleFanout    PropagationRule = "fanout"
	PropagationRuleFilter    PropagationRule = "filter"
	PropagationRuleAggregate PropagationRule = "aggregate"
)

type EventFlow struct {
	ID              uuid.UUID              `json:"id"`
	FeatureID       uuid.UUID              `json:"feature_id"`
	EventID         uuid.UUID              `json:"event_id"`
	Graph           *EventFlowGraph        `json:"graph"`
	EntryPoints     []uuid.UUID            `json:"entry_points"`
	ExitPoints      []uuid.UUID            `json:"exit_points"`
	Cycles          [][]uuid.UUID          `json:"cycles"`
	CriticalPath    []uuid.UUID            `json:"critical_path"`
	Metadata        map[string]interface{} `json:"metadata"`
	Validated       bool                   `json:"validated"`
	ValidationErrors []string              `json:"validation_errors"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

type EventFlowGraph struct {
	Nodes []EventFlowNode `json:"nodes"`
	Edges []EventFlowEdge `json:"edges"`
}

type EventFlowNode struct {
	ID          uuid.UUID `json:"id"`
	EventID     uuid.UUID `json:"event_id"`
	HandlerID   uuid.UUID `json:"handler_id"`
	FeatureID   uuid.UUID `json:"feature_id"`
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Position    Position  `json:"position"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type EventFlowEdge struct {
	ID          uuid.UUID `json:"id"`
	FromNodeID  uuid.UUID `json:"from_node_id"`
	ToNodeID    uuid.UUID `json:"to_node_id"`
	Type        string    `json:"type"`
	Condition   string    `json:"condition"`
	Label       string    `json:"label"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type IntegrationResult struct {
	Success            bool                   `json:"success"`
	FeatureID          uuid.UUID              `json:"feature_id"`
	WiredEntryPoints   []EntryPoint           `json:"wired_entry_points"`
	IntegrationPoints  []IntegrationPoint     `json:"integration_points"`
	EventDefinitions   []EventDefinition      `json:"event_definitions"`
	EventFlows         []EventFlow            `json:"event_flows"`
	GeneratedFiles     []GeneratedFile        `json:"generated_files"`
	ValidationErrors   []ValidationError      `json:"validation_errors"`
	Warnings           []string               `json:"warnings"`
	Metrics            IntegrationMetrics     `json:"metrics"`
	Duration           time.Duration          `json:"duration"`
}

type GeneratedFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Operation   string `json:"operation"`
	BackupPath  string `json:"backup_path,omitempty"`
}

type ValidationError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Location    string `json:"location"`
	EntryPoint  EntryPointType `json:"entry_point"`
	FeatureID   uuid.UUID `json:"feature_id"`
	Suggestion  string `json:"suggestion"`
}

type IntegrationMetrics struct {
	EntryPointsWired        int `json:"entry_points_wired"`
	IntegrationPointsCreated int `json:"integration_points_created"`
	EventsDefined           int `json:"events_defined"`
	HandlersRegistered      int `json:"handlers_registered"`
	FilesGenerated          int `json:"files_generated"`
	LinesOfCodeGenerated    int `json:"lines_of_code_generated"`
	ValidationPassed        int `json:"validation_passed"`
	ValidationFailed        int `json:"validation_failed"`
}

type ValidationResult struct {
	Errors   []ValidationError `json:"errors"`
	Warnings []string          `json:"warnings"`
	Valid    bool              `json:"valid"`
}

type WiringTemplate struct {
	Pattern        WiringPattern `json:"pattern"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Template       string        `json:"template"`
	Parameters     []TemplateParameter `json:"parameters"`
	EntryPoints    []EntryPointType `json:"entry_points"`
	Dependencies   []string      `json:"dependencies"`
	ExampleUsage   string        `json:"example_usage"`
}

type TemplateParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Validation  string `json:"validation"`
}

type EntryPointRegistry struct {
	EntryPoints map[EntryPointType][]EntryPointInterface `json:"entry_points"`
	Metadata    map[string]interface{}                   `json:"metadata"`
	UpdatedAt   time.Time                                `json:"updated_at"`
}

type FeatureManifest struct {
	FeatureID       uuid.UUID                         `json:"feature_id"`
	Name            string                            `json:"name"`
	Version         string                            `json:"version"`
	Description     string                            `json:"description"`
	Author          string                            `json:"author"`
	License         string                            `json:"license"`
	Repository      string                            `json:"repository"`
	EntryPoints     map[EntryPointType][]EntryPointInterface `json:"entry_points"`
	Events          []EventDefinition                 `json:"events"`
	ConfigSchema    json.RawMessage                   `json:"config_schema"`
	Dependencies    []Dependency                      `json:"dependencies"`
	Compatibility   CompatibilityMatrix               `json:"compatibility"`
	Installation    InstallationGuide                 `json:"installation"`
	Usage           []UsageExample                    `json:"usage"`
	Testing         FeatureTests                      `json:"testing"`
	Changelog       string                            `json:"changelog"`
	GeneratedAt     time.Time              `json:"generated_at"`
}

type Dependency struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type CompatibilityMatrix struct {
	MinVersion     string   `json:"min_version"`
	MaxVersion     string   `json:"max_version"`
	IncompatibleWith []string `json:"incompatible_with"`
	TestedWith     []string `json:"tested_with"`
}

type InstallationGuide struct {
	Steps          []InstallationStep `json:"steps"`
	Prerequisites  []string           `json:"prerequisites"`
	PostInstall    []string           `json:"post_install"`
	Uninstall      []string           `json:"uninstall"`
}

type InstallationStep struct {
	Order       int    `json:"order"`
	Command     string `json:"command"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Rollback    string `json:"rollback"`
}

type SimilarFeature struct {
	FeatureID   uuid.UUID `json:"feature_id"`
	Name        string    `json:"name"`
	Similarity  float64   `json:"similarity"`
	Patterns    []WiringPattern `json:"patterns"`
	EntryPoints []EntryPointType `json:"entry_points"`
}