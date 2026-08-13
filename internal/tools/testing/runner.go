package testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/iSundram/Automergent/internal/tools"
)

// RunTestsTool auto-detects and runs tests.
type RunTestsTool struct{}

func (t *RunTestsTool) Name() string { return "run_tests" }
func (t *RunTestsTool) Description() string {
	return `Auto-detect test framework and run tests.

Supported frameworks:
- Go: go test
- Node.js: npm test, yarn test, pnpm test
- Python: pytest, python -m unittest
- Rust: cargo test
- Java/Maven: mvn test
- Java/Gradle: gradle test

Returns summary on success, full output on failure.`
}
func (t *RunTestsTool) RequiresConfirmation(mode string) bool {
	return mode == "plan"
}

func (t *RunTestsTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to run tests in (default: current dir).",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Test pattern/filter (e.g., 'TestUser*' for Go, '-k user' for pytest).",
			},
			"verbose": map[string]any{
				"type":        "boolean",
				"description": "Show verbose output even on success.",
			},
			"coverage": map[string]any{
				"type":        "boolean",
				"description": "Generate coverage report.",
			},
			"timeout": map[string]any{
				"type":        "string",
				"description": "Test timeout (e.g., '5m', '30s').",
			},
			"framework": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "go", "npm", "yarn", "pnpm", "pytest", "unittest", "cargo", "maven", "gradle"},
				"description": "Test framework to use (default: auto-detect).",
			},
		},
	}
}

func (t *RunTestsTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	if path == "" {
		path = "."
	}

	pattern, _ := tools.StringArg(args, "pattern")
	verbose, _ := tools.ArgBool(args, "verbose")
	coverage, _ := tools.ArgBool(args, "coverage")
	timeoutStr, _ := tools.StringArg(args, "timeout")
	framework, _ := tools.StringArg(args, "framework")

	if framework == "" || framework == "auto" {
		framework = detectTestFramework(path)
	}

	if framework == "" {
		return tools.Result{
			IsError: true,
			Content: "could not auto-detect test framework. Specify framework parameter.",
		}, nil
	}

	// Validate timeout
	if timeoutStr != "" {
		if err := validateTimeout(timeoutStr); err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("invalid timeout: %v", err)}, nil
		}
	}

	// Validate pattern
	if pattern != "" {
		if err := validatePattern(framework, pattern); err != nil {
			return tools.Result{IsError: true, Content: fmt.Sprintf("invalid pattern: %v", err)}, nil
		}
	}

	// Build command
	cmd, cmdArgs := buildTestCommand(framework, path, pattern, coverage, timeoutStr, verbose)
	if cmd == "" {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("unknown test framework: %s", framework),
		}, nil
	}

	// Set timeout
	timeout := 5 * time.Minute
	if timeoutStr != "" {
		d, _ := time.ParseDuration(timeoutStr)
		timeout = d
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run tests using robust runner that kills child processes on cancel
	output, err := runCommand(ctx, path, cmd, cmdArgs...)
	outputStr := string(output)

	if err != nil {
		// If context deadline exceeded, propagate a clear message
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return tools.Result{IsError: true, Content: "tests timed out"}, nil
		}
		// Test failure
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("tests failed:\n\n%s", outputStr),
			Metadata: map[string]any{
				"framework": framework,
				"passed":    false,
			},
		}, nil
	}

	// Test success
	if verbose {
		return tools.Result{
			Content: fmt.Sprintf("all tests passed:\n\n%s", outputStr),
			Metadata: map[string]any{
				"framework": framework,
				"passed":    true,
			},
		}, nil
	}

	// Summarize success
	summary := summarizeTestOutput(framework, outputStr)
	return tools.Result{
		Content: summary,
		Metadata: map[string]any{
			"framework": framework,
			"passed":    true,
		},
	}, nil
}

func detectTestFramework(path string) string {
	// Go
	if fileExists(filepath.Join(path, "go.mod")) {
		return "go"
	}

	// Node.js
	if fileExists(filepath.Join(path, "package.json")) {
		// Check for package manager lock files
		if fileExists(filepath.Join(path, "pnpm-lock.yaml")) {
			return "pnpm"
		}
		if fileExists(filepath.Join(path, "yarn.lock")) {
			return "yarn"
		}
		return "npm"
	}

	// Python: be conservative. Prefer pytest only when explicit pytest config is present.
	if fileExists(filepath.Join(path, "pytest.ini")) || fileExists(filepath.Join(path, "tox.ini")) {
		return "pytest"
	}
	if fileExists(filepath.Join(path, "pyproject.toml")) {
		// inspect pyproject for [tool.pytest] or pytest in deps
		if data, err := os.ReadFile(filepath.Join(path, "pyproject.toml")); err == nil {
			s := string(data)
			if strings.Contains(s, "[tool.pytest]") || strings.Contains(s, "pytest") {
				return "pytest"
			}
		}
	}
	if fileExists(filepath.Join(path, "setup.cfg")) {
		if data, err := os.ReadFile(filepath.Join(path, "setup.cfg")); err == nil {
			s := string(data)
			if strings.Contains(s, "pytest") {
				return "pytest"
			}
		}
	}
	if fileExists(filepath.Join(path, "requirements.txt")) {
		if data, err := os.ReadFile(filepath.Join(path, "requirements.txt")); err == nil {
			if strings.Contains(string(data), "pytest") {
				return "pytest"
			}
		}
	}
	// If setup.py exists but no explicit pytest markers, assume unittest
	if fileExists(filepath.Join(path, "setup.py")) {
		return "unittest"
	}

	// Rust
	if fileExists(filepath.Join(path, "Cargo.toml")) {
		return "cargo"
	}

	// Maven
	if fileExists(filepath.Join(path, "pom.xml")) {
		return "maven"
	}

	// Gradle
	if fileExists(filepath.Join(path, "build.gradle")) || fileExists(filepath.Join(path, "build.gradle.kts")) {
		return "gradle"
	}

	return ""
}

func buildTestCommand(framework, path, pattern string, coverage bool, timeout string, verbose bool) (string, []string) {
	switch framework {
	case "go":
		args := []string{"test", "./..."}
		// Use -json for stable parsing when not verbose
		if !verbose {
			args = append(args, "-json")
		}
		if pattern != "" {
			args = append(args, "-run", pattern)
		}
		if coverage {
			args = append(args, "-coverprofile=coverage.out")
		}
		if timeout != "" {
			args = append(args, "-timeout", timeout)
		}
		return "go", args

	case "npm":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, "--", pattern)
		}
		return "npm", args

	case "yarn":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, pattern)
		}
		return "yarn", args

	case "pnpm":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, "--", pattern)
		}
		return "pnpm", args

	case "pytest":
		args := []string{}
		if pattern != "" {
			args = append(args, "-k", pattern)
		}
		if coverage {
			args = append(args, "--cov=.")
		}
		return "pytest", args

	case "unittest":
		args := []string{"-m", "unittest"}
		if pattern != "" {
			args = append(args, pattern)
		} else {
			args = append(args, "discover")
		}
		return "python", args

	case "cargo":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, pattern)
		}
		return "cargo", args

	case "maven":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, fmt.Sprintf("-Dtest=%s", pattern))
		}
		return "mvn", args

	case "gradle":
		args := []string{"test"}
		if pattern != "" {
			args = append(args, "--tests", pattern)
		}
		return "gradle", args

	default:
		return "", nil
	}
}

func summarizeTestOutput(framework, output string) string {
	// Prefer structured (JSON) parsing when available
	if framework == "go" {
		// go test -json emits JSON per-line
		scanner := strings.Split(output, "\n")
		pass := 0
		fail := 0
		for _, line := range scanner {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var ev struct {
				Action string `json:"Action"`
				Test   string `json:"Test"`
			}
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				if ev.Test != "" {
					if ev.Action == "pass" {
						pass++
					} else if ev.Action == "fail" {
						fail++
					}
				}
			}
		}
		if fail > 0 {
			return fmt.Sprintf("❌ %d tests failed, %d passed", fail, pass)
		}
		return fmt.Sprintf("✅ %d tests passed", pass)
	}

	// Fallback: simple string parsing
	lines := strings.Split(output, "\n")
	var summary []string

	switch framework {
	case "pytest":
		for _, line := range lines {
			if strings.Contains(line, "passed") || strings.Contains(line, "PASSED") {
				return fmt.Sprintf("✅ %s", line)
			}
		}

	case "npm", "yarn", "pnpm":
		for _, line := range lines {
			if strings.Contains(line, "Tests:") || strings.Contains(line, "passing") {
				summary = append(summary, line)
			}
		}
		if len(summary) > 0 {
			return fmt.Sprintf("✅ %s", strings.Join(summary, "\n"))
		}
	}

	// Default summary
	return fmt.Sprintf("✅ Tests passed (output: %d lines)", len(lines))
}

func validatePattern(framework, pattern string) error {
	switch framework {
	case "go":
		// go -run accepts a regexp
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("go regexp invalid: %w", err)
		}
	default:
		// Basic sanity check: no null bytes and reasonable length
		if strings.Contains(pattern, "\x00") {
			return errors.New("pattern contains NUL byte")
		}
		if len(pattern) > 2000 {
			return errors.New("pattern too long")
		}
	}
	return nil
}

func validateTimeout(timeoutStr string) error {
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return err
	}
	if d <= 0 {
		return errors.New("timeout must be positive")
	}
	// cap to 24h
	if d > 24*time.Hour {
		return errors.New("timeout unreasonably large")
	}
	return nil
}

// runCommand runs a command in its own process group and ensures that when ctx is
// cancelled all child processes are killed as well.
func runCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Create new process group
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var b strings.Builder
	cmd.Stdout = &b
	cmd.Stderr = &b

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// kill process group
		pgid := cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		// wait for process to exit
		<-done
		return []byte(b.String()), ctx.Err()
	case err := <-done:
		return []byte(b.String()), err
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestCoverageTool generates test coverage report.
type TestCoverageTool struct{}

func (t *TestCoverageTool) Name() string { return "test_coverage" }
func (t *TestCoverageTool) Description() string {
	return "Generate test coverage report for the project."
}
func (t *TestCoverageTool) RequiresConfirmation(mode string) bool { return false }

func (t *TestCoverageTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to analyze.",
			},
			"format": map[string]any{
				"type":        "string",
				"enum":        []string{"text", "html", "json"},
				"description": "Output format (default: text).",
			},
		},
	}
}

func (t *TestCoverageTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	if path == "" {
		path = "."
	}

	format := "text"
	if f, ok := tools.StringArg(args, "format"); ok {
		format = f
	}

	framework := detectTestFramework(path)

	switch framework {
	case "go":
		return runGoCoverage(ctx, path, format)
	case "pytest":
		return runPytestCoverage(ctx, path, format)
	default:
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("coverage not supported for framework: %s", framework),
		}, nil
	}
}

func runGoCoverage(ctx context.Context, path, format string) (tools.Result, error) {
	// Run tests with coverage
	coverFile := filepath.Join(path, "coverage.out")
	output, err := runCommand(ctx, path, "go", "test", "-coverprofile="+coverFile, "./...")
	if err != nil {
		// cleanup
		_ = os.Remove(coverFile)
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage failed: %v\n%s", err, output)}, nil
	}

	// Get coverage report
	var args []string
	htmlFile := filepath.Join(path, "coverage.html")
	switch format {
	case "html":
		args = []string{"tool", "cover", "-html=" + coverFile, "-o", htmlFile}
	default:
		args = []string{"tool", "cover", "-func=" + coverFile}
	}

	out, err := runCommand(ctx, path, "go", args...)
	// cleanup coverage files
	_ = os.Remove(coverFile)
	if format == "html" {
		_ = os.Remove(htmlFile)
	}
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage report failed: %v\n%s", err, out)}, nil
	}

	return tools.Result{Content: string(out)}, nil
}

func runPytestCoverage(ctx context.Context, path, format string) (tools.Result, error) {
	args := []string{"--cov=" + path, "--cov-report=" + format}
	out, err := runCommand(ctx, path, "pytest", args...)
	// cleanup common pytest coverage artifacts
	_ = os.Remove(filepath.Join(path, ".coverage"))
	_ = os.Remove(filepath.Join(path, "coverage.xml"))
	_ = os.RemoveAll(filepath.Join(path, "htmlcov"))
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage failed: %v\n%s", err, out)}, nil
	}

	return tools.Result{Content: string(out)}, nil
}

// EstimatedCost returns cost estimates for the run tests tool.
func (t *RunTestsTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 1000, LatencyMs: 5000, RiskLevel: "low"}
}

// EstimatedCost returns cost estimates for the test coverage tool.
func (t *TestCoverageTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 3000, RiskLevel: "low"}
}
