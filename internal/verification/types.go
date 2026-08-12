// Package verification provides a multi-layer verification strategy for ensuring
// Automergent's changes are correct and safe. It implements four layers of verification:
// syntax, semantic, test, and integration checks with self-healing capabilities.
package verification

import (
	"context"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// Severity levels for verification issues
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Layer represents the verification layer type
type Layer string

const (
	LayerSyntax      Layer = "syntax"
	LayerSemantic    Layer = "semantic"
	LayerTest        Layer = "test"
	LayerIntegration Layer = "integration"
)

// Status represents the verification status
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusPassed    Status = "passed"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
	StatusHealing   Status = "healing"
	StatusCancelled Status = "cancelled"
)

// Issue represents a verification issue found during checks
type Issue struct {
	ID            string                 `json:"id"`
	Layer         Layer                  `json:"layer"`
	Severity      Severity               `json:"severity"`
	Message       string                 `json:"message"`
	Description   string                 `json:"description"`
	File          string                 `json:"file,omitempty"`
	Line          int                    `json:"line,omitempty"`
	Column        int                    `json:"column,omitempty"`
	Code          string                 `json:"code,omitempty"`
	Source        string                 `json:"source,omitempty"`
	CanAutoFix    bool                   `json:"can_auto_fix"`
	FixSuggestion string                 `json:"fix_suggestion,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// Fix represents an attempted fix for an issue
type Fix struct {
	IssueID     string                 `json:"issue_id"`
	Strategy    string                 `json:"strategy"`
	Description string                 `json:"description"`
	Applied     bool                   `json:"applied"`
	Success     bool                   `json:"success"`
	Error       string                 `json:"error,omitempty"`
	Changes     []Change               `json:"changes,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	AppliedAt   time.Time              `json:"applied_at"`
}

// Change represents a code change made during a fix
type Change struct {
	File      string `json:"file"`
	Operation string `json:"operation"` // edit, create, delete
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

// LayerResult represents the result of a verification layer
type LayerResult struct {
	Layer      Layer                  `json:"layer"`
	Status     Status                 `json:"status"`
	Duration   time.Duration          `json:"duration"`
	Issues     []Issue                `json:"issues"`
	Fixes      []Fix                  `json:"fixes,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	StartedAt  time.Time              `json:"started_at"`
	FinishedAt time.Time              `json:"finished_at"`
}

// Result represents the complete verification result
type Result struct {
	ID             string                 `json:"id"`
	Status         Status                 `json:"status"`
	Layers         []LayerResult          `json:"layers"`
	TotalIssues    int                    `json:"total_issues"`
	TotalFixes     int                    `json:"total_fixes"`
	SuccessRate    float64                `json:"success_rate"`
	SafetyScore    float64                `json:"safety_score"`
	QualityScore   float64                `json:"quality_score"`
	IsSafe         bool                   `json:"is_safe"`
	CanProceed     bool                   `json:"can_proceed"`
	Recommendation string                 `json:"recommendation"`
	Summary        string                 `json:"summary"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	StartedAt      time.Time              `json:"started_at"`
	FinishedAt     time.Time              `json:"finished_at"`
	Duration       time.Duration          `json:"duration"`
}

// Config represents the verification configuration
type Config struct {
	// Global settings
	Enabled         bool          `json:"enabled"`
	AutoFix         bool          `json:"auto_fix"`
	AutoRollback    bool          `json:"auto_rollback"`
	MaxAttempts     int           `json:"max_attempts"`
	Timeout         time.Duration `json:"timeout"`
	StopOnCritical  bool          `json:"stop_on_critical"`
	StopOnError     bool          `json:"stop_on_error"`
	MinSafetyScore  float64       `json:"min_safety_score"`
	MinQualityScore float64       `json:"min_quality_score"`

	// Layer enablement
	EnableSyntax      bool `json:"enable_syntax"`
	EnableSemantic    bool `json:"enable_semantic"`
	EnableTest        bool `json:"enable_test"`
	EnableIntegration bool `json:"enable_integration"`

	// Syntax layer config
	SyntaxCheckCompile bool          `json:"syntax_check_compile"`
	SyntaxCheckLint    bool          `json:"syntax_check_lint"`
	SyntaxCheckFormat  bool          `json:"syntax_check_format"`
	SyntaxCheckImports bool          `json:"syntax_check_imports"`
	SyntaxTimeout      time.Duration `json:"syntax_timeout"`

	// Semantic layer config
	SemanticCheckTypes bool          `json:"semantic_check_types"`
	SemanticCheckLogic bool          `json:"semantic_check_logic"`
	SemanticCheckAPI   bool          `json:"semantic_check_api"`
	SemanticCheckDeps  bool          `json:"semantic_check_deps"`
	SemanticTimeout    time.Duration `json:"semantic_timeout"`

	// Test layer config
	TestRunExisting     bool          `json:"test_run_existing"`
	TestGenerateMissing bool          `json:"test_generate_missing"`
	TestMeasureCoverage bool          `json:"test_measure_coverage"`
	TestRunIntegration  bool          `json:"test_run_integration"`
	TestMinCoverage     float64       `json:"test_min_coverage"`
	TestTimeout         time.Duration `json:"test_timeout"`

	// Integration layer config
	IntegrationCheckBuild    bool          `json:"integration_check_build"`
	IntegrationCheckPoints   bool          `json:"integration_check_points"`
	IntegrationCheckPerf     bool          `json:"integration_check_perf"`
	IntegrationCheckSecurity bool          `json:"integration_check_security"`
	IntegrationTimeout       time.Duration `json:"integration_timeout"`

	// Self-healing config
	HealingMaxAttempts       int           `json:"healing_max_attempts"`
	HealingTimeout           time.Duration `json:"healing_timeout"`
	HealingLearnFromMistakes bool          `json:"healing_learn_from_mistakes"`

	// Layer hooks let callers override the default verification execution.
	SyntaxHook      LayerHook `json:"-"`
	SemanticHook    LayerHook `json:"-"`
	TestHook        LayerHook `json:"-"`
	IntegrationHook LayerHook `json:"-"`
}

// DefaultConfig returns a default verification configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		AutoFix:         true,
		AutoRollback:    true,
		MaxAttempts:     3,
		Timeout:         5 * time.Minute,
		StopOnCritical:  true,
		StopOnError:     false,
		MinSafetyScore:  0.8,
		MinQualityScore: 0.7,

		EnableSyntax:      true,
		EnableSemantic:    true,
		EnableTest:        true,
		EnableIntegration: true,

		SyntaxCheckCompile: true,
		SyntaxCheckLint:    true,
		SyntaxCheckFormat:  true,
		SyntaxCheckImports: true,
		SyntaxTimeout:      30 * time.Second,

		SemanticCheckTypes: true,
		SemanticCheckLogic: true,
		SemanticCheckAPI:   true,
		SemanticCheckDeps:  true,
		SemanticTimeout:    1 * time.Minute,

		TestRunExisting:     true,
		TestGenerateMissing: false,
		TestMeasureCoverage: true,
		TestRunIntegration:  false,
		TestMinCoverage:     0.5,
		TestTimeout:         2 * time.Minute,

		IntegrationCheckBuild:    true,
		IntegrationCheckPoints:   true,
		IntegrationCheckPerf:     false,
		IntegrationCheckSecurity: true,
		IntegrationTimeout:       2 * time.Minute,

		HealingMaxAttempts:       3,
		HealingTimeout:           3 * time.Minute,
		HealingLearnFromMistakes: true,
	}
}

// Context provides context for verification
type Context struct {
	WorkingDir      string
	Files           []string
	ChangedFiles    []string
	Operation       string
	ToolName        string
	BeforeDiags     []types.Diagnostic
	AfterDiags      []types.Diagnostic
	ExpectedOutcome string
	Metadata        map[string]interface{}
}

// LayerHook can override a verification layer execution.
type LayerHook func(context.Context, *Context) (*LayerResult, error)

// Verifier is the interface for verification layers
type Verifier interface {
	// Name returns the verifier name
	Name() string

	// Layer returns the verification layer
	Layer() Layer

	// Verify performs verification and returns results
	Verify(ctx *Context) (*LayerResult, error)

	// CanFix returns true if this verifier can fix issues
	CanFix() bool

	// Fix attempts to fix an issue
	Fix(ctx *Context, issue *Issue) (*Fix, error)
}

// HealingStrategy represents a self-healing strategy
type HealingStrategy interface {
	// Name returns the strategy name
	Name() string

	// CanHandle returns true if this strategy can handle the issue
	CanHandle(issue *Issue) bool

	// Apply applies the healing strategy
	Apply(ctx *Context, issue *Issue) (*Fix, error)

	// Priority returns the strategy priority (higher = try first)
	Priority() int
}
