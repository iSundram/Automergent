package testing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Build command
	cmd, cmdArgs := buildTestCommand(framework, path, pattern, coverage, timeoutStr)
	if cmd == "" {
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("unknown test framework: %s", framework),
		}, nil
	}

	// Set timeout
	timeout := 5 * time.Minute
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run tests
	execCmd := exec.CommandContext(ctx, cmd, cmdArgs...)
	execCmd.Dir = path

	output, err := execCmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
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

	// Python
	if fileExists(filepath.Join(path, "pytest.ini")) ||
		fileExists(filepath.Join(path, "pyproject.toml")) ||
		fileExists(filepath.Join(path, "setup.py")) {
		return "pytest"
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
	if fileExists(filepath.Join(path, "build.gradle")) ||
		fileExists(filepath.Join(path, "build.gradle.kts")) {
		return "gradle"
	}

	return ""
}

func buildTestCommand(framework, path, pattern string, coverage bool, timeout string) (string, []string) {
	switch framework {
	case "go":
		args := []string{"test", "./..."}
		if pattern != "" {
			args = append(args, "-run", pattern)
		}
		if coverage {
			args = append(args, "-cover")
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
	lines := strings.Split(output, "\n")
	var summary []string

	switch framework {
	case "go":
		for _, line := range lines {
			if strings.Contains(line, "PASS") || strings.Contains(line, "ok") {
				summary = append(summary, line)
			}
		}
		if len(summary) > 0 {
			return fmt.Sprintf("✅ All tests passed\n%s", strings.Join(summary[:min(5, len(summary))], "\n"))
		}

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
	cmd := exec.CommandContext(ctx, "go", "test", "-coverprofile="+coverFile, "./...")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage failed: %v", err)}, nil
	}

	// Get coverage report
	var args []string
	switch format {
	case "html":
		htmlFile := filepath.Join(path, "coverage.html")
		args = []string{"tool", "cover", "-html=" + coverFile, "-o", htmlFile}
	default:
		args = []string{"tool", "cover", "-func=" + coverFile}
	}

	cmd = exec.CommandContext(ctx, "go", args...)
	cmd.Dir = path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage report failed: %v", err)}, nil
	}

	return tools.Result{Content: string(output)}, nil
}

func runPytestCoverage(ctx context.Context, path, format string) (tools.Result, error) {
	args := []string{"--cov=" + path, "--cov-report=" + format}
	cmd := exec.CommandContext(ctx, "pytest", args...)
	cmd.Dir = path
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tools.Result{IsError: true, Content: fmt.Sprintf("coverage failed: %v\n%s", err, output)}, nil
	}

	return tools.Result{Content: string(output)}, nil
}

// EstimatedCost returns cost estimates for the run tests tool.
func (t *RunTestsTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 1000, LatencyMs: 5000, RiskLevel: "low"}
}

// EstimatedCost returns cost estimates for the test coverage tool.
func (t *TestCoverageTool) EstimatedCost() tools.ToolCost {
	return tools.ToolCost{TokensApprox: 500, LatencyMs: 3000, RiskLevel: "low"}
}
