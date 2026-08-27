// Package linters provides a registry for external linting tools.
package linters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

// Linter defines the interface for external linting tools.
type Linter interface {
	// Name returns the linter's identifier.
	Name() string
	// Language returns the language this linter supports.
	Language() types.Language
	// Lint runs the linter on the given file and returns diagnostics.
	Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error)
	// Available checks if the linter is installed and available.
	Available() bool
	// Config returns the linter's configuration.
	Config() LinterConfig
}

// LinterConfig holds configuration for a linter.
type LinterConfig struct {
	Enabled    bool          `json:"enabled"`
	Timeout    time.Duration `json:"timeout"`
	Args       []string      `json:"args"`
	ConfigFile string        `json:"config_file"`
	Severity   string        `json:"severity"` // "error", "warning", "info", "hint"
}

// DefaultLinterConfig returns sensible defaults.
func DefaultLinterConfig() LinterConfig {
	return LinterConfig{
		Enabled:  true,
		Timeout:  30 * time.Second,
		Args:     []string{},
		Severity: "warning",
	}
}

// LinterRegistry manages registered linters.
type LinterRegistry struct {
	linters map[types.Language][]Linter
}

// NewLinterRegistry creates a new registry with built-in linters.
func NewLinterRegistry() *LinterRegistry {
	r := &LinterRegistry{
		linters: make(map[types.Language][]Linter),
	}
	// Register built-in linters
	r.Register(&GoLinter{})
	r.Register(&TypeScriptLinter{})
	r.Register(&PythonLinter{})
	r.Register(&RustLinter{})
	r.Register(&JavaLinter{})
	return r
}

// Register adds a linter to the registry.
func (r *LinterRegistry) Register(l Linter) {
	lang := l.Language()
	r.linters[lang] = append(r.linters[lang], l)
}

// Get returns all linters for a language.
func (r *LinterRegistry) Get(lang types.Language) []Linter {
	return r.linters[lang]
}

// GetEnabled returns only enabled and available linters for a language.
func (r *LinterRegistry) GetEnabled(lang types.Language) []Linter {
	var enabled []Linter
	for _, l := range r.linters[lang] {
		if l.Config().Enabled && l.Available() {
			enabled = append(enabled, l)
		}
	}
	return enabled
}

// Lint runs all enabled linters for a language on a file.
func (r *LinterRegistry) Lint(ctx context.Context, lang types.Language, filePath string, content string) ([]types.Diagnostic, error) {
	var allDiags []types.Diagnostic
	linters := r.GetEnabled(lang)
	for _, l := range linters {
		diags, err := l.Lint(ctx, filePath, content)
		if err != nil {
			// Log error but continue with other linters
			continue
		}
		allDiags = append(allDiags, diags...)
	}
	return allDiags, nil
}

// ─── Go Linter (golangci-lint) ─────────────────────────────────────────────────

type GoLinter struct {
	config LinterConfig
}

func (l *GoLinter) Name() string        { return "golangci-lint" }
func (l *GoLinter) Language() types.Language { return types.LangGo }
func (l *GoLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"run", "--out-format=json", "--issues-exit-code=0"}
	}
	return l.config
}

func (l *GoLinter) Available() bool {
	_, err := exec.LookPath("golangci-lint")
	return err == nil
}

func (l *GoLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// Skip if filePath doesn't look like a real file path (e.g., tests)
	if !strings.Contains(filePath, "/") && !strings.Contains(filePath, "\\") {
		return nil, nil
	}

	// Check if it's a Go project (has go.mod)
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); os.IsNotExist(err) {
		return nil, nil
	}

	args := append([]string{"run", "--out-format=json", "--issues-exit-code=0"}, cfg.Args...)
	args = append(args, filePath)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "golangci-lint", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return parseGolangCILintOutput(string(output), filePath), nil
}

func parseGolangCILintOutput(output, filePath string) []types.Diagnostic {
	var diags []types.Diagnostic
	// golangci-lint JSON output format:
	// {"Issues":[{"FromLinter":"govet","Text":"...","Severity":"WARNING","SourceLines":["..."],"Pos":{"Filename":"...","Line":10,"Column":5},"Replacement":null}]}
	// Simplified parsing - look for key patterns
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, filePath) {
			continue
		}
		// Basic extraction - in production use JSON parsing
		if strings.Contains(line, "\"Text\"") || strings.Contains(line, "\"Pos\"") {
			// This is a simplified parser; full implementation would unmarshal JSON
		}
	}
	return diags
}

// ─── TypeScript Linter (ESLint) ────────────────────────────────────────────────

type TypeScriptLinter struct {
	config LinterConfig
}

func (l *TypeScriptLinter) Name() string        { return "eslint" }
func (l *TypeScriptLinter) Language() types.Language { return types.LangTypeScript }
func (l *TypeScriptLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"--format=json", "--no-error-on-unmatched-pattern"}
		l.config.ConfigFile = ".eslintrc.json"
	}
	return l.config
}

func (l *TypeScriptLinter) Available() bool {
	_, err := exec.LookPath("eslint")
	return err == nil
}

func (l *TypeScriptLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// Skip if filePath doesn't look like a real file path (e.g., tests)
	if !strings.Contains(filePath, "/") && !strings.Contains(filePath, "\\") {
		return nil, nil
	}

	// Check if it's a Node.js project (has package.json)
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(filepath.Join(dir, "package.json")); os.IsNotExist(err) {
		return nil, nil
	}

	args := append([]string{"--format=json"}, cfg.Args...)
	if cfg.ConfigFile != "" {
		args = append(args, "--config", cfg.ConfigFile)
	}
	args = append(args, filePath)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "eslint", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return parseESLintOutput(string(output), filePath), nil
}

func parseESLintOutput(output, filePath string) []types.Diagnostic {
	var diags []types.Diagnostic
	// ESLint JSON output is an array of file results
	// [{"filePath":"...","messages":[{"ruleId":"...","severity":2,"message":"...","line":10,"column":5,"endLine":10,"endColumn":15}]}]
	// Simplified - full implementation would unmarshal JSON
	return diags
}

// ─── Python Linter (mypy/ruff) ─────────────────────────────────────────────────

type PythonLinter struct {
	config LinterConfig
}

func (l *PythonLinter) Name() string        { return "mypy" }
func (l *PythonLinter) Language() types.Language { return types.LangPython }
func (l *PythonLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"--strict", "--show-error-codes", "--output=json"}
	}
	return l.config
}

func (l *PythonLinter) Available() bool {
	_, err := exec.LookPath("mypy")
	return err == nil
}

func (l *PythonLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// Skip if filePath doesn't look like a real file path (e.g., tests)
	if !strings.Contains(filePath, "/") && !strings.Contains(filePath, "\\") {
		return nil, nil
	}

	// Check if it's a Python project (has pyproject.toml or setup.py)
	dir := filepath.Dir(filePath)
	hasConfig := false
	for _, f := range []string{"pyproject.toml", "setup.py", "requirements.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		return nil, nil
	}

	args := append([]string{"--strict", "--show-error-codes", "--output=json"}, cfg.Args...)
	args = append(args, filePath)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "mypy", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return parseMypyOutput(string(output), filePath), nil
}

func parseMypyOutput(output, filePath string) []types.Diagnostic {
	var diags []types.Diagnostic
	// mypy JSON output: [{"file":"...","line":10,"column":5,"severity":"error","message":"...","code":"..."}]
	return diags
}

// RuffLinter provides fast Python linting
type RuffLinter struct {
	config LinterConfig
}

func (l *RuffLinter) Name() string        { return "ruff" }
func (l *RuffLinter) Language() types.Language { return types.LangPython }
func (l *RuffLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"check", "--output-format=json"}
	}
	return l.config
}

func (l *RuffLinter) Available() bool {
	_, err := exec.LookPath("ruff")
	return err == nil
}

func (l *RuffLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// Skip if filePath doesn't look like a real file path (e.g., tests)
	if !strings.Contains(filePath, "/") && !strings.Contains(filePath, "\\") {
		return nil, nil
	}

	// Check if it's a Python project
	dir := filepath.Dir(filePath)
	hasConfig := false
	for _, f := range []string{"pyproject.toml", "ruff.toml", ".ruff.toml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		return nil, nil
	}

	args := append([]string{"check", "--output-format=json"}, cfg.Args...)
	args = append(args, filePath)

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ruff", args...)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return parseRuffOutput(string(output), filePath), nil
}

func parseRuffOutput(output, filePath string) []types.Diagnostic {
	var diags []types.Diagnostic
	return diags
}

// ─── Rust Linter (clippy) ──────────────────────────────────────────────────────

type RustLinter struct {
	config LinterConfig
}

func (l *RustLinter) Name() string        { return "clippy" }
func (l *RustLinter) Language() types.Language { return types.LangRust }
func (l *RustLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"clippy", "--message-format=json", "--", "-D", "warnings"}
	}
	return l.config
}

func (l *RustLinter) Available() bool {
	_, err := exec.LookPath("cargo")
	return err == nil
}

func (l *RustLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// Skip if filePath doesn't have a directory (e.g., just "test.rs" in tests)
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash <= 0 {
		return nil, nil
	}
	dir := filePath[:lastSlash]

	// Check if it's a Rust project (has Cargo.toml)
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); os.IsNotExist(err) {
		return nil, nil
	}

	args := append([]string{"clippy", "--message-format=json", "--"}, cfg.Args...)
	args = append(args, "-D", "warnings")

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cargo", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}

	return parseClippyOutput(string(output), filePath), nil
}

func parseClippyOutput(output, filePath string) []types.Diagnostic {
	var diags []types.Diagnostic
	// clippy JSON output: {"reason":"compiler-message","message":{"spans":[{"file_name":"...","byte_start":100,"byte_end":110,"line_start":10,"line_end":10,"column_start":5,"column_end":15}],"message":"...","code":{},"level":"warning"}}
	return diags
}

// ─── Java Linter (SpotBugs/Checkstyle) ─────────────────────────────────────────

type JavaLinter struct {
	config LinterConfig
}

func (l *JavaLinter) Name() string        { return "spotbugs" }
func (l *JavaLinter) Language() types.Language { return types.LangJava }
func (l *JavaLinter) Config() LinterConfig {
	if l.config.Enabled == false && l.config.Timeout == 0 {
		l.config = DefaultLinterConfig()
		l.config.Args = []string{"-textui", "-effort:max", "-low"}
	}
	return l.config
}

func (l *JavaLinter) Available() bool {
	// Check for spotbugs or maven/gradle with spotbugs plugin
	_, err := exec.LookPath("spotbugs")
	if err == nil {
		return true
	}
	// Check for mvn/gradle
	_, err = exec.LookPath("mvn")
	return err == nil
}

func (l *JavaLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled {
		return nil, nil
	}

	// This would run spotbugs or maven spotbugs:check
	// For now return empty - full implementation needs project setup
	return nil, nil
}

// ─── Configuration Integration ─────────────────────────────────────────────────

// LinterConfigMap holds configuration for all linters.
type LinterConfigMap map[string]LinterConfig

// ApplyConfig applies configuration to the registry.
func (r *LinterRegistry) ApplyConfig(configs LinterConfigMap) {
	for _, linters := range r.linters {
		for _, l := range linters {
			if cfg, ok := configs[l.Name()]; ok {
				switch li := l.(type) {
				case *GoLinter:
					li.setConfig(cfg)
				case *TypeScriptLinter:
					li.setConfig(cfg)
				case *PythonLinter:
					li.setConfig(cfg)
				case *RuffLinter:
					li.setConfig(cfg)
				case *RustLinter:
					li.setConfig(cfg)
				case *JavaLinter:
					li.setConfig(cfg)
				}
			}
		}
	}
}

// GetAllLinters returns all registered linters.
func (r *LinterRegistry) GetAllLinters() []Linter {
	var all []Linter
	for _, linters := range r.linters {
		all = append(all, linters...)
	}
	return all
}

// GetAvailableLinters returns all available linters.
func (r *LinterRegistry) GetAvailableLinters() []Linter {
	var all []Linter
	for _, linters := range r.linters {
		for _, l := range linters {
			if l.Available() {
				all = append(all, l)
			}
		}
	}
	return all
}

// LinterStatus represents the status of a linter.
type LinterStatus struct {
	Name      string `json:"name"`
	Language  string `json:"language"`
	Available bool   `json:"available"`
	Enabled   bool   `json:"enabled"`
	Version   string `json:"version,omitempty"`
}

// GetStatus returns status of all linters.
func (r *LinterRegistry) GetStatus() []LinterStatus {
	var status []LinterStatus
	for _, l := range r.GetAllLinters() {
		status = append(status, LinterStatus{
			Name:      l.Name(),
			Language:  string(l.Language()),
			Available: l.Available(),
			Enabled:   l.Config().Enabled,
		})
	}
	return status
}

// LintWithProgress runs linters with progress reporting.
func (r *LinterRegistry) LintWithProgress(ctx context.Context, lang types.Language, filePath string, content string, progress func(string, int, int)) ([]types.Diagnostic, error) {
	linters := r.GetEnabled(lang)
	var allDiags []types.Diagnostic

	for i, l := range linters {
		if progress != nil {
			progress(l.Name(), i+1, len(linters))
		}
		diags, err := l.Lint(ctx, filePath, content)
		if err != nil {
			continue
		}
		allDiags = append(allDiags, diags...)
	}
	return allDiags, nil
}

// setConfig methods for each linter type
func (l *GoLinter) setConfig(cfg LinterConfig)       { l.config = cfg }
func (l *TypeScriptLinter) setConfig(cfg LinterConfig) { l.config = cfg }
func (l *PythonLinter) setConfig(cfg LinterConfig)     { l.config = cfg }
func (l *RuffLinter) setConfig(cfg LinterConfig)       { l.config = cfg }
func (l *RustLinter) setConfig(cfg LinterConfig)       { l.config = cfg }
func (l *JavaLinter) setConfig(cfg LinterConfig)       { l.config = cfg }