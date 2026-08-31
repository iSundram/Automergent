// Package linters provides a registry for external linting tools.
package linters

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// LinterConfig holds configuration for a linter. Args is the full argument
// list for the binary (including subcommands), so a user override replaces
// the defaults rather than being appended to them.
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

// baseLinter carries the state shared by every linter implementation:
// lazily-initialized configuration and memoized availability (probing the
// PATH on every Analyze call is not free).
type baseLinter struct {
	mu        sync.Mutex
	config    *LinterConfig
	availOnce sync.Once
	avail     bool
}

// initConfig returns the effective config, installing defaults on first use.
func (b *baseLinter) initConfig(defaults LinterConfig) LinterConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.config == nil {
		cfg := defaults
		b.config = &cfg
	}
	return *b.config
}

func (b *baseLinter) setConfig(cfg LinterConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = &cfg
}

// probeAvailable memoizes an exec.LookPath check for the given binary.
func (b *baseLinter) probeAvailable(binary string) bool {
	b.availOnce.Do(func() {
		_, err := exec.LookPath(binary)
		b.avail = err == nil
	})
	return b.avail
}

// runLinter executes binary with args in dir, applies the timeout, and
// returns combined output. Non-zero exit codes are not errors — linters use
// them to report findings.
func runLinter(ctx context.Context, cfg LinterConfig, dir, binary string, args []string) (string, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return "", err
	}
	return string(output), nil
}

// looksLikePath guards against linting synthetic paths (bare filenames used
// in tests and in-memory content).
func looksLikePath(filePath string) bool {
	return strings.Contains(filePath, "/") || strings.Contains(filePath, "\\")
}

// findProjectRoot walks up from start until a marker file is found.
func findProjectRoot(start string, markers []string) string {
	dir := filepath.Clean(start)
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// LinterRegistry manages registered linters.
type LinterRegistry struct {
	mu      sync.RWMutex
	linters map[types.Language][]Linter
}

// NewLinterRegistry creates a new registry with built-in linters.
func NewLinterRegistry() *LinterRegistry {
	r := &LinterRegistry{
		linters: make(map[types.Language][]Linter),
	}
	// Register built-in linters. Only linters whose output we actually
	// parse are registered — a stub that shells out and discards the result
	// costs seconds per run for nothing.
	r.Register(&GoLinter{})
	r.Register(&RuffLinter{})
	return r
}

// Register adds a linter to the registry.
func (r *LinterRegistry) Register(l Linter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.linters[l.Language()] = append(r.linters[l.Language()], l)
}

// Get returns all linters for a language.
func (r *LinterRegistry) Get(lang types.Language) []Linter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.linters[lang]
}

// GetEnabled returns only enabled and available linters for a language.
func (r *LinterRegistry) GetEnabled(lang types.Language) []Linter {
	r.mu.RLock()
	linters := append([]Linter(nil), r.linters[lang]...)
	r.mu.RUnlock()

	var enabled []Linter
	for _, l := range linters {
		if l.Config().Enabled && l.Available() {
			enabled = append(enabled, l)
		}
	}
	return enabled
}

// Lint runs all enabled linters for a language on a file.
func (r *LinterRegistry) Lint(ctx context.Context, lang types.Language, filePath string, content string) ([]types.Diagnostic, error) {
	var allDiags []types.Diagnostic
	for _, l := range r.GetEnabled(lang) {
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
	baseLinter
}

func (l *GoLinter) Name() string            { return "golangci-lint" }
func (l *GoLinter) Language() types.Language { return types.LangGo }

func (l *GoLinter) Config() LinterConfig {
	return l.initConfig(LinterConfig{
		Enabled:  true,
		Timeout:  30 * time.Second,
		Args:     []string{"run", "--out-format=json", "--issues-exit-code=0"},
		Severity: "warning",
	})
}

func (l *GoLinter) Available() bool { return l.probeAvailable("golangci-lint") }

func (l *GoLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled || !looksLikePath(filePath) {
		return nil, nil
	}

	root := findProjectRoot(filepath.Dir(filePath), []string{"go.mod"})
	if root == "" {
		return nil, nil
	}

	args := append([]string{}, cfg.Args...)
	if cfg.ConfigFile != "" {
		args = append(args, "--config", cfg.ConfigFile)
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		rel = filePath
	}
	args = append(args, rel)

	output, err := runLinter(ctx, cfg, root, "golangci-lint", args)
	if err != nil {
		return nil, err
	}
	return parseGolangCILintOutput(output, cfg.Severity), nil
}

// golangciOutput mirrors the --out-format=json schema.
type golangciOutput struct {
	Issues []struct {
		FromLinter string `json:"FromLinter"`
		Text       string `json:"Text"`
		Severity   string `json:"Severity"`
		Pos        struct {
			Filename string `json:"Filename"`
			Line     int    `json:"Line"`
			Column   int    `json:"Column"`
		} `json:"Pos"`
	} `json:"Issues"`
}

func parseGolangCILintOutput(output, defaultSeverity string) []types.Diagnostic {
	var parsed golangciOutput
	// golangci-lint may prefix the JSON with log lines; find the first '{'.
	if idx := strings.Index(output, "{"); idx >= 0 {
		output = output[idx:]
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return nil
	}

	var diags []types.Diagnostic
	for _, issue := range parsed.Issues {
		severity := strings.ToLower(issue.Severity)
		switch severity {
		case "error", "warning", "info":
		default:
			severity = defaultSeverity
		}
		d := types.Diagnostic{
			FilePath:    issue.Pos.Filename,
			Line:        issue.Pos.Line,
			Column:      issue.Pos.Column,
			Severity:    severity,
			Code:        issue.FromLinter,
			Message:     issue.Text,
			Source:      "golangci-lint",
			Tags:        []string{"lint", "go"},
			Suggestions: []string{"Run 'golangci-lint run --fix' for auto-fixable issues"},
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── Python Linter (ruff) ─────────────────────────────────────────────────────

type RuffLinter struct {
	baseLinter
}

func (l *RuffLinter) Name() string            { return "ruff" }
func (l *RuffLinter) Language() types.Language { return types.LangPython }

func (l *RuffLinter) Config() LinterConfig {
	return l.initConfig(LinterConfig{
		Enabled:  true,
		Timeout:  15 * time.Second,
		Args:     []string{"check", "--output-format=json"},
		Severity: "warning",
	})
}

func (l *RuffLinter) Available() bool { return l.probeAvailable("ruff") }

func (l *RuffLinter) Lint(ctx context.Context, filePath string, content string) ([]types.Diagnostic, error) {
	cfg := l.Config()
	if !cfg.Enabled || !looksLikePath(filePath) {
		return nil, nil
	}

	dir := filepath.Dir(filePath)
	root := findProjectRoot(dir, []string{"pyproject.toml", "ruff.toml", ".ruff.toml", "setup.py"})
	if root == "" {
		root = dir
	}

	args := append([]string{}, cfg.Args...)
	if cfg.ConfigFile != "" {
		args = append(args, "--config", cfg.ConfigFile)
	}
	args = append(args, filePath)

	output, err := runLinter(ctx, cfg, root, "ruff", args)
	if err != nil {
		return nil, err
	}
	return parseRuffOutput(output, cfg.Severity), nil
}

// ruffPos is a 1-indexed row/column pair in ruff's JSON output.
type ruffPos struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

// ruffDiagnostic mirrors one entry of `ruff check --output-format=json`.
type ruffDiagnostic struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	Filename     string   `json:"filename"`
	Location     *ruffPos `json:"location"`
	EndLocation  *ruffPos `json:"end_location"`
	NoqaRow      int      `json:"noqa_row"`
	URL          string   `json:"url"`
}

func parseRuffOutput(output, defaultSeverity string) []types.Diagnostic {
	var parsed []ruffDiagnostic
	if idx := strings.Index(output, "["); idx >= 0 {
		output = output[idx:]
	}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return nil
	}

	var diags []types.Diagnostic
	for _, issue := range parsed {
		line, col := 1, 0
		if issue.Location != nil && issue.Location.Row > 0 {
			line, col = issue.Location.Row, issue.Location.Column-1
		} else if issue.NoqaRow > 0 {
			line = issue.NoqaRow
		}
		d := types.Diagnostic{
			FilePath: issue.Filename,
			Line:     line,
			Column:   col,
			Severity: defaultSeverity,
			Code:     issue.Code,
			Message:  issue.Message,
			Source:   "ruff",
			Tags:     []string{"lint", "python"},
		}
		if issue.EndLocation != nil && issue.EndLocation.Row > 0 {
			d.EndLine = issue.EndLocation.Row
			d.EndColumn = issue.EndLocation.Column
		}
		if issue.URL != "" {
			d.Suggestions = []string{"Details: " + issue.URL}
		}
		d.WithDefaults()
		diags = append(diags, d)
	}
	return diags
}

// ─── Configuration Integration ─────────────────────────────────────────────────

// LinterConfigMap holds configuration for all linters.
type LinterConfigMap map[string]LinterConfig

// configSetter is implemented by linters whose configuration can be replaced.
type configSetter interface{ setConfig(LinterConfig) }

// ApplyConfig applies configuration to the registry.
func (r *LinterRegistry) ApplyConfig(configs LinterConfigMap) {
	for _, l := range r.GetAllLinters() {
		if cfg, ok := configs[l.Name()]; ok {
			if cs, ok := l.(configSetter); ok {
				cs.setConfig(cfg)
			}
		}
	}
}

// GetAllLinters returns all registered linters.
func (r *LinterRegistry) GetAllLinters() []Linter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []Linter
	for _, ls := range r.linters {
		all = append(all, ls...)
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
