package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// Engine coordinates multi-layer verification.
type Engine struct {
	config     *Config
	verifiers  map[Layer][]Verifier
	strategies []HealingStrategy
	mu         sync.RWMutex
	history    []Result
	maxHistory int
}

// NewEngine creates a new verification engine.
func NewEngine(config *Config) *Engine {
	if config == nil {
		config = DefaultConfig()
	}
	return &Engine{
		config:     config,
		verifiers:  make(map[Layer][]Verifier),
		history:    make([]Result, 0),
		maxHistory: 100,
	}
}

// RegisterVerifier registers a verifier for a layer.
func (e *Engine) RegisterVerifier(verifier Verifier) {
	if verifier == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.verifiers[verifier.Layer()] = append(e.verifiers[verifier.Layer()], verifier)
}

// RegisterHealingStrategy registers a self-healing strategy.
func (e *Engine) RegisterHealingStrategy(strategy HealingStrategy) {
	if strategy == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies = append(e.strategies, strategy)
	sort.Slice(e.strategies, func(i, j int) bool {
		return e.strategies[i].Priority() > e.strategies[j].Priority()
	})
}

// NewDefaultEngine builds an engine with production-safe hooks for common checks.
func NewDefaultEngine() *Engine {
	e := NewEngine(DefaultConfig())
	e.config.SyntaxHook = SyntaxHook()
	e.config.SemanticHook = SemanticHook()
	e.config.TestHook = TestHook()
	e.config.IntegrationHook = IntegrationHook()
	return e
}

// Verify performs multi-layer verification.
func (e *Engine) Verify(ctx context.Context, vctx *Context) (*Result, error) {
	if vctx == nil {
		vctx = &Context{}
	}
	if !e.config.Enabled {
		now := time.Now()
		return &Result{
			ID:             uuid.New().String(),
			Status:         StatusSkipped,
			CanProceed:     true,
			Recommendation: "Verification disabled",
			StartedAt:      now,
			FinishedAt:     now,
		}, nil
	}

	startedAt := time.Now()
	result := &Result{
		ID:        uuid.New().String(),
		Status:    StatusRunning,
		Layers:    make([]LayerResult, 0, 4),
		Metadata:  map[string]interface{}{},
		StartedAt: startedAt,
	}

	if ctx == nil {
		ctx = context.Background()
	}

	for _, layer := range []Layer{LayerSyntax, LayerSemantic, LayerTest, LayerIntegration} {
		if !e.isLayerEnabled(layer) {
			continue
		}

		layerResult, err := e.verifyLayer(ctx, layer, vctx)
		if err != nil {
			return nil, fmt.Errorf("layer %s failed: %w", layer, err)
		}
		result.Layers = append(result.Layers, *layerResult)
		e.reportProgress(vctx, layer, layerResult.Status, "")

		if e.shouldStop(layerResult) {
			break
		}
	}

	e.calculateScores(result)
	result.Status = e.determineFinalStatus(result)
	result.FinishedAt = time.Now()
	result.Duration = result.FinishedAt.Sub(result.StartedAt)
	result.Recommendation = e.generateRecommendation(result)
	result.Summary = e.generateSummary(result)
	e.addToHistory(result)
	return result, nil
}

func (e *Engine) verifyLayer(ctx context.Context, layer Layer, vctx *Context) (*LayerResult, error) {
	now := time.Now()
	res := &LayerResult{
		Layer:     layer,
		Status:    StatusRunning,
		Issues:    make([]Issue, 0),
		Fixes:     make([]Fix, 0),
		Metadata:  map[string]interface{}{},
		StartedAt: now,
	}

	// Give each layer its own timeout (falls back to the global timeout).
	layerTimeout := e.config.Timeout
	switch layer {
	case LayerSyntax:
		if e.config.SyntaxTimeout > 0 {
			layerTimeout = e.config.SyntaxTimeout
		}
	case LayerSemantic:
		if e.config.SemanticTimeout > 0 {
			layerTimeout = e.config.SemanticTimeout
		}
	case LayerTest:
		if e.config.TestTimeout > 0 {
			layerTimeout = e.config.TestTimeout
		}
	case LayerIntegration:
		if e.config.IntegrationTimeout > 0 {
			layerTimeout = e.config.IntegrationTimeout
		}
	}
	if layerTimeout <= 0 {
		layerTimeout = 5 * time.Minute
	}
	layerCtx, cancel := context.WithTimeout(ctx, layerTimeout)
	defer cancel()

	e.reportProgress(vctx, layer, StatusRunning, "")

	var hook LayerHook
	switch layer {
	case LayerSyntax:
		hook = e.config.SyntaxHook
	case LayerSemantic:
		hook = e.config.SemanticHook
	case LayerTest:
		hook = e.config.TestHook
	case LayerIntegration:
		hook = e.config.IntegrationHook
	}

	if hook != nil {
		hookResult, err := hook(layerCtx, vctx)
		if err != nil {
			res.Issues = append(res.Issues, newIssue(layer, SeverityError, err.Error(), "", 0, "", "", false))
			res.Status = StatusFailed
			res.FinishedAt = time.Now()
			res.Duration = res.FinishedAt.Sub(res.StartedAt)
			return res, nil
		}
		if hookResult != nil {
			mergeLayerResult(res, hookResult)
		}
	} else {
		builtIn, err := e.runBuiltInLayer(layerCtx, layer, vctx)
		if err != nil {
			res.Issues = append(res.Issues, newIssue(layer, SeverityError, err.Error(), "", 0, "", "", false))
			res.Status = StatusFailed
			res.FinishedAt = time.Now()
			res.Duration = res.FinishedAt.Sub(res.StartedAt)
			return res, nil
		}
		if builtIn != nil {
			mergeLayerResult(res, builtIn)
		}
	}

	e.mu.RLock()
	verifiers := append([]Verifier(nil), e.verifiers[layer]...)
	e.mu.RUnlock()
	for _, verifier := range verifiers {
		select {
		case <-ctx.Done():
			res.Status = StatusCancelled
			res.FinishedAt = time.Now()
			res.Duration = res.FinishedAt.Sub(res.StartedAt)
			return res, ctx.Err()
		default:
		}

		vr, err := verifier.Verify(vctx)
		if err != nil {
			res.Issues = append(res.Issues, newIssue(layer, SeverityError, fmt.Sprintf("verifier %s failed: %v", verifier.Name(), err), "", 0, "", verifier.Name(), false))
			continue
		}
		if vr != nil {
			mergeLayerResult(res, vr)
		}
	}

	res.Status = StatusPassed
	for _, issue := range res.Issues {
		if issue.Severity == SeverityCritical || issue.Severity == SeverityError {
			res.Status = StatusFailed
			break
		}
	}
	if len(res.Issues) == 0 && res.Metadata["skipped"] == true {
		res.Status = StatusSkipped
	}

	res.FinishedAt = time.Now()
	res.Duration = res.FinishedAt.Sub(res.StartedAt)
	return res, nil
}

func (e *Engine) reportProgress(vctx *Context, layer Layer, status Status, message string) {
	if vctx == nil || vctx.OnProgress == nil {
		return
	}
	vctx.OnProgress(layer, status, message)
}

func (e *Engine) runBuiltInLayer(ctx context.Context, layer Layer, vctx *Context) (*LayerResult, error) {
	switch layer {
	case LayerSyntax:
		return runSyntaxChecks(ctx, vctx)
	case LayerSemantic:
		return runSemanticChecks(ctx, vctx)
	case LayerTest:
		return runTestChecks(ctx, vctx)
	case LayerIntegration:
		return runIntegrationChecks(ctx, vctx)
	default:
		return &LayerResult{Layer: layer, Status: StatusSkipped, Metadata: map[string]interface{}{"skipped": true}}, nil
	}
}

func runSyntaxChecks(ctx context.Context, vctx *Context) (*LayerResult, error) {
	targets := resolveTargets(vctx)
	res := &LayerResult{Layer: LayerSyntax, Status: StatusPassed, Issues: []Issue{}, Metadata: map[string]interface{}{"checks": []string{}}}

	if len(targets) == 0 {
		res.Status = StatusSkipped
		res.Metadata["skipped"] = true
		return res, nil
	}

	for _, target := range targets {
		switch strings.ToLower(filepath.Ext(target)) {
		case ".go":
			if err := parseGoFile(target); err != nil {
				res.Issues = append(res.Issues, newIssue(LayerSyntax, SeverityError, err.Error(), target, 0, "", "go-parser", false))
			}
		case ".json":
			if err := parseJSONFile(target); err != nil {
				res.Issues = append(res.Issues, newIssue(LayerSyntax, SeverityError, err.Error(), target, 0, "", "json-parser", false))
			}
		case ".yaml", ".yml":
			if err := parseYAMLFile(target); err != nil {
				res.Issues = append(res.Issues, newIssue(LayerSyntax, SeverityError, err.Error(), target, 0, "", "yaml-parser", false))
			}
		case ".toml":
			if err := parseTOMLFile(target); err != nil {
				res.Issues = append(res.Issues, newIssue(LayerSyntax, SeverityError, err.Error(), target, 0, "", "toml-parser", false))
			}
		}
	}

	if len(res.Issues) > 0 {
		res.Status = StatusFailed
	}
	return res, nil
}

func runSemanticChecks(ctx context.Context, vctx *Context) (*LayerResult, error) {
	dir := resolveWorkingDir(vctx)
	if dir == "" {
		return &LayerResult{Layer: LayerSemantic, Status: StatusSkipped, Metadata: map[string]interface{}{"skipped": true}}, nil
	}
	res := &LayerResult{Layer: LayerSemantic, Status: StatusPassed, Metadata: map[string]interface{}{}}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		res.Status = StatusSkipped
		res.Metadata["skipped"] = true
		return res, nil
	}

	args := append([]string{"vet"}, packageTargets(vctx, dir)...)
	output, err := runCommand(ctx, dir, "go", args...)
	res.Metadata["command"] = append([]string{"go"}, args...)
	res.Metadata["output"] = output
	if err != nil {
		res.Status = StatusFailed
		res.Issues = append(res.Issues, newIssue(LayerSemantic, SeverityError, compactOutput(output, err), "", 0, "", "go-vet", false))
	}
	return res, nil
}

func runTestChecks(ctx context.Context, vctx *Context) (*LayerResult, error) {
	dir := resolveWorkingDir(vctx)
	if dir == "" {
		return &LayerResult{Layer: LayerTest, Status: StatusSkipped, Metadata: map[string]interface{}{"skipped": true}}, nil
	}
	res := &LayerResult{Layer: LayerTest, Status: StatusPassed, Metadata: map[string]interface{}{}}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		res.Status = StatusSkipped
		res.Metadata["skipped"] = true
		return res, nil
	}

	args := append([]string{"test"}, packageTargets(vctx, dir)...)
	output, err := runCommand(ctx, dir, "go", args...)
	res.Metadata["command"] = append([]string{"go"}, args...)
	res.Metadata["output"] = output
	if err != nil {
		res.Status = StatusFailed
		res.Issues = append(res.Issues, newIssue(LayerTest, SeverityError, compactOutput(output, err), "", 0, "", "go-test", false))
	}
	return res, nil
}

func runIntegrationChecks(ctx context.Context, vctx *Context) (*LayerResult, error) {
	dir := resolveWorkingDir(vctx)
	if dir == "" {
		return &LayerResult{Layer: LayerIntegration, Status: StatusSkipped, Metadata: map[string]interface{}{"skipped": true}}, nil
	}
	res := &LayerResult{Layer: LayerIntegration, Status: StatusPassed, Metadata: map[string]interface{}{}}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		res.Status = StatusSkipped
		res.Metadata["skipped"] = true
		return res, nil
	}

	args := append([]string{"build"}, packageTargets(vctx, dir)...)
	output, err := runCommand(ctx, dir, "go", args...)
	res.Metadata["command"] = append([]string{"go"}, args...)
	res.Metadata["output"] = output
	if err != nil {
		res.Status = StatusFailed
		res.Issues = append(res.Issues, newIssue(LayerIntegration, SeverityError, compactOutput(output, err), "", 0, "", "go-build", false))
	}
	return res, nil
}

// packageTargets derives go package targets from changed files so verification
// is scoped to affected packages instead of always running ./... across the repo.
func packageTargets(vctx *Context, moduleRoot string) []string {
	fallback := []string{"./..."}
	if vctx == nil || len(vctx.ChangedFiles) == 0 {
		return fallback
	}

	seen := make(map[string]struct{})
	var targets []string
	for _, f := range vctx.ChangedFiles {
		pkg := packageDirForFile(f, moduleRoot)
		if pkg == "" {
			continue
		}
		if _, ok := seen[pkg]; ok {
			continue
		}
		seen[pkg] = struct{}{}
		targets = append(targets, pkg)
	}
	if len(targets) == 0 {
		return fallback
	}
	return targets
}

// packageDirForFile returns the module-relative package directory of a changed
// file (e.g. "./internal/foo/"), or "" if the file is outside the module root.
func packageDirForFile(file, moduleRoot string) string {
	if file == "" {
		return ""
	}
	rootAbs, err := filepath.Abs(moduleRoot)
	if err != nil {
		return ""
	}
	filePath := file
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(moduleRoot, filePath)
	}
	fileAbs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	rel = filepath.ToSlash(rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		return "./"
	}
	return "./" + dir + "/"
}

func resolveTargets(vctx *Context) []string {
	var targets []string
	appendUnique := func(paths []string) {
		for _, p := range paths {
			if p == "" {
				continue
			}
			abs := p
			if !filepath.IsAbs(abs) && vctx != nil && vctx.WorkingDir != "" {
				abs = filepath.Join(vctx.WorkingDir, p)
			}
			targets = append(targets, abs)
		}
	}
	if vctx != nil {
		appendUnique(vctx.ChangedFiles)
		appendUnique(vctx.Files)
		if len(targets) > 0 {
			return dedupPaths(targets)
		}
		if vctx.WorkingDir != "" {
			return collectSupportedFiles(vctx.WorkingDir)
		}
	}
	return nil
}

func resolveWorkingDir(vctx *Context) string {
	if vctx != nil && vctx.WorkingDir != "" {
		return vctx.WorkingDir
	}
	return "."
}

func collectSupportedFiles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "build", ".cache":
				return filepath.SkipDir
			}
			return nil
		}
		switch strings.ToLower(filepath.Ext(d.Name())) {
		case ".go", ".json", ".yaml", ".yml", ".toml":
			out = append(out, path)
		}
		return nil
	})
	return out
}

func parseGoFile(path string) error {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("go syntax error in %s: %w", path, err)
	}
	return nil
}

func parseJSONFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("json syntax error in %s: %w", path, err)
	}
	return nil
}

func parseYAMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("yaml syntax error in %s: %w", path, err)
	}
	return nil
}

func parseTOMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v map[string]any
	if err := toml.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("toml syntax error in %s: %w", path, err)
	}
	return nil
}

func runCommand(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func compactOutput(output string, err error) string {
	output = strings.TrimSpace(output)
	if output != "" {
		return output
	}
	if err != nil {
		return err.Error()
	}
	return "verification failed"
}

func newIssue(layer Layer, severity Severity, message, file string, line int, suggestion, source string, canFix bool) Issue {
	return Issue{
		ID:            uuid.New().String(),
		Layer:         layer,
		Severity:      severity,
		Message:       message,
		Description:   message,
		File:          file,
		Line:          line,
		Source:        source,
		CanAutoFix:    canFix,
		FixSuggestion: suggestion,
		CreatedAt:     time.Now(),
	}
}

func mergeLayerResult(dst, src *LayerResult) {
	if dst == nil || src == nil {
		return
	}
	dst.Issues = append(dst.Issues, src.Issues...)
	dst.Fixes = append(dst.Fixes, src.Fixes...)
	if src.Metadata != nil {
		if dst.Metadata == nil {
			dst.Metadata = map[string]interface{}{}
		}
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}
}

func dedupPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (e *Engine) isLayerEnabled(layer Layer) bool {
	switch layer {
	case LayerSyntax:
		return e.config.EnableSyntax
	case LayerSemantic:
		return e.config.EnableSemantic
	case LayerTest:
		return e.config.EnableTest
	case LayerIntegration:
		return e.config.EnableIntegration
	default:
		return false
	}
}

func (e *Engine) shouldStop(layerResult *LayerResult) bool {
	if layerResult == nil || layerResult.Status != StatusFailed {
		return false
	}
	for _, issue := range layerResult.Issues {
		if issue.Severity == SeverityCritical && e.config.StopOnCritical {
			return true
		}
		if issue.Severity == SeverityError && e.config.StopOnError {
			return true
		}
	}
	return false
}

func (e *Engine) calculateScores(result *Result) {
	totalIssues := 0
	criticalIssues := 0
	errorIssues := 0
	warningIssues := 0
	totalFixes := 0
	successfulFixes := 0

	for _, layer := range result.Layers {
		totalIssues += len(layer.Issues)
		totalFixes += len(layer.Fixes)
		for _, issue := range layer.Issues {
			switch issue.Severity {
			case SeverityCritical:
				criticalIssues++
			case SeverityError:
				errorIssues++
			case SeverityWarning:
				warningIssues++
			}
		}
		for _, fix := range layer.Fixes {
			if fix.Success {
				successfulFixes++
			}
		}
	}

	result.TotalIssues = totalIssues
	result.TotalFixes = totalFixes
	safetyScore := 1.0
	if totalIssues > 0 {
		safetyScore -= float64(criticalIssues) * 0.3
		safetyScore -= float64(errorIssues) * 0.15
		safetyScore -= float64(warningIssues) * 0.05
		if totalFixes > 0 {
			fixRate := float64(successfulFixes) / float64(totalFixes)
			safetyScore += fixRate * 0.2
		}
	}
	if safetyScore < 0 {
		safetyScore = 0
	}
	result.SafetyScore = safetyScore
	result.IsSafe = safetyScore >= e.config.MinSafetyScore && criticalIssues == 0

	qualityScore := 1.0
	if totalIssues > 0 {
		qualityScore -= float64(totalIssues) * 0.1
	}
	if qualityScore < 0 {
		qualityScore = 0
	}
	result.QualityScore = qualityScore
	if totalFixes > 0 {
		result.SuccessRate = float64(successfulFixes) / float64(totalFixes)
	} else {
		result.SuccessRate = 1.0
	}
	result.CanProceed = result.IsSafe &&
		result.SafetyScore >= e.config.MinSafetyScore &&
		result.QualityScore >= e.config.MinQualityScore &&
		criticalIssues == 0
}

func (e *Engine) determineFinalStatus(result *Result) Status {
	anyPassed := false
	for _, layer := range result.Layers {
		switch layer.Status {
		case StatusFailed:
			return StatusFailed
		case StatusPassed:
			anyPassed = true
		}
	}
	if anyPassed {
		return StatusPassed
	}
	return StatusSkipped
}

func (e *Engine) generateRecommendation(result *Result) string {
	if result.CanProceed {
		return "All verification checks passed. Safe to proceed."
	}
	if !result.IsSafe {
		return fmt.Sprintf("Safety score (%.2f) below threshold (%.2f). Review critical issues before proceeding.",
			result.SafetyScore, e.config.MinSafetyScore)
	}
	if result.QualityScore < e.config.MinQualityScore {
		return fmt.Sprintf("Quality score (%.2f) below threshold (%.2f). Consider fixing issues before proceeding.",
			result.QualityScore, e.config.MinQualityScore)
	}
	return "Verification found issues. Review before proceeding."
}

func (e *Engine) generateSummary(result *Result) string {
	if len(result.Layers) == 0 {
		return "No verification layers executed"
	}
	passed := 0
	failed := 0
	for _, layer := range result.Layers {
		switch layer.Status {
		case StatusPassed:
			passed++
		case StatusFailed:
			failed++
		}
	}
	return fmt.Sprintf("Verification complete: %d passed, %d failed, %d issues found, %d fixes applied",
		passed, failed, result.TotalIssues, result.TotalFixes)
}

func (e *Engine) addToHistory(result *Result) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = append(e.history, *result)
	if len(e.history) > e.maxHistory {
		e.history = e.history[len(e.history)-e.maxHistory:]
	}
}

// GetHistory returns recent verification history.
func (e *Engine) GetHistory() []Result {
	e.mu.RLock()
	defer e.mu.RUnlock()
	history := make([]Result, len(e.history))
	copy(history, e.history)
	return history
}

// GetConfig returns the current configuration.
func (e *Engine) GetConfig() *Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// UpdateConfig updates the configuration.
func (e *Engine) UpdateConfig(config *Config) {
	if config == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = config
}

// SyntaxHook returns the built-in syntax verification hook.
func SyntaxHook() LayerHook { return runSyntaxChecks }

// SemanticHook returns the built-in semantic verification hook.
func SemanticHook() LayerHook { return runSemanticChecks }

// TestHook returns the built-in test verification hook.
func TestHook() LayerHook { return runTestChecks }

// IntegrationHook returns the built-in integration verification hook.
func IntegrationHook() LayerHook { return runIntegrationChecks }
