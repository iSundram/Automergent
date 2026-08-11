package security

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iSundram/Automergent/internal/tools"
)

// SecretPatterns defines regex patterns for detecting secrets.
var SecretPatterns = []struct {
	Name    string
	Pattern *regexp.Regexp
}{
	{"AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"AWS Secret Key", regexp.MustCompile(`(?i)aws.{0,20}secret.{0,20}['\"][0-9a-zA-Z/+]{40}['\"]`)},
	{"GitHub Token", regexp.MustCompile(`ghp_[0-9a-zA-Z]{36}`)},
	{"GitHub OAuth", regexp.MustCompile(`gho_[0-9a-zA-Z]{36}`)},
	{"GitHub App Token", regexp.MustCompile(`(ghu|ghs)_[0-9a-zA-Z]{36}`)},
	{"GitLab Token", regexp.MustCompile(`glpat-[0-9a-zA-Z\-]{20}`)},
	{"Slack Token", regexp.MustCompile(`xox[baprs]-[0-9a-zA-Z]{10,48}`)},
	{"Slack Webhook", regexp.MustCompile(`https://hooks\.slack\.com/services/T[a-zA-Z0-9_]{8}/B[a-zA-Z0-9_]{8,12}/[a-zA-Z0-9_]{24}`)},
	{"Google API Key", regexp.MustCompile(`AIza[0-9A-Za-z\\-_]{35}`)},
	{"Google OAuth", regexp.MustCompile(`[0-9]+-[0-9A-Za-z_]{32}\.apps\.googleusercontent\.com`)},
	{"Private Key", regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)},
	{"JWT Token", regexp.MustCompile(`eyJ[A-Za-z0-9-_=]+\.eyJ[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_.+/=]*`)},
	{"Stripe Key", regexp.MustCompile(`sk_live_[0-9a-zA-Z]{24}`)},
	{"Stripe Test Key", regexp.MustCompile(`sk_test_[0-9a-zA-Z]{24}`)},
	{"Twilio API Key", regexp.MustCompile(`SK[0-9a-fA-F]{32}`)},
	{"SendGrid API Key", regexp.MustCompile(`SG\.[0-9A-Za-z\-_]{22}\.[0-9A-Za-z\-_]{43}`)},
	{"Mailgun API Key", regexp.MustCompile(`key-[0-9a-zA-Z]{32}`)},
	{"npm Token", regexp.MustCompile(`npm_[0-9a-zA-Z]{36}`)},
	{"PyPI Token", regexp.MustCompile(`pypi-[0-9a-zA-Z]{50,}`)},
	{"Generic API Key", regexp.MustCompile(`(?i)(api[_-]?key|apikey|api[_-]?secret)['\"]?\s*[:=]\s*['\"][0-9a-zA-Z]{16,}['\"]`)},
	{"Generic Secret", regexp.MustCompile(`(?i)(secret|password|passwd|pwd)['\"]?\s*[:=]\s*['\"][^'\"]{8,}['\"]`)},
	{"Database URL", regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis)://[^:]+:[^@]+@`)},
}

// SecretsScanTool scans for hardcoded secrets.
type SecretsScanTool struct{}

func (t *SecretsScanTool) Name() string { return "secrets_scan" }
func (t *SecretsScanTool) Description() string {
	return `Scan files for hardcoded secrets, API keys, and credentials.

Detects:
- AWS, GitHub, GitLab, Google, Slack tokens
- Private keys, JWTs
- Stripe, Twilio, SendGrid, Mailgun keys
- Database URLs with credentials
- Generic API keys and secrets`
}
func (t *SecretsScanTool) RequiresConfirmation(mode string) bool { return false }

func (t *SecretsScanTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory or file to scan (default: current dir).",
			},
			"include_tests": map[string]any{
				"type":        "boolean",
				"description": "Include test files in scan (default: false).",
			},
			"exclude": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Patterns to exclude.",
			},
		},
	}
}

func (t *SecretsScanTool) Execute(_ context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	if path == "" {
		path = "."
	}

	includeTests := false
	if v, ok := tools.ArgBool(args, "include_tests"); ok {
		includeTests = v
	}

	var findings []SecretFinding
	maxFindings := 100

	err := filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "__pycache__", ".venv", "venv", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}

		// Skip test files unless requested
		if !includeTests && isTestFile(d.Name()) {
			return nil
		}

		// Skip binary and non-code files
		if !isCodeFile(d.Name()) {
			return nil
		}

		// Scan file
		fileFindings, err := scanFile(filePath)
		if err != nil {
			return nil
		}

		findings = append(findings, fileFindings...)
		if len(findings) >= maxFindings {
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return tools.Result{IsError: true, Content: fmt.Sprintf("scan error: %v", err)}, nil
	}

	if len(findings) == 0 {
		return tools.Result{Content: "✅ No secrets detected"}, nil
	}

	// Format findings
	var lines []string
	lines = append(lines, fmt.Sprintf("⚠️  Found %d potential secret(s):\n", len(findings)))

	for i, f := range findings {
		if i >= maxFindings {
			lines = append(lines, fmt.Sprintf("\n... and %d more (truncated)", len(findings)-maxFindings))
			break
		}
		lines = append(lines, fmt.Sprintf("• %s:%d - %s", f.File, f.Line, f.Type))
		lines = append(lines, fmt.Sprintf("  %s", maskSecret(f.Match)))
	}

	return tools.Result{
		Content: strings.Join(lines, "\n"),
		Metadata: map[string]any{
			"findings_count": len(findings),
		},
	}, nil
}

type SecretFinding struct {
	File  string
	Line  int
	Type  string
	Match string
}

func scanFile(path string) ([]SecretFinding, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var findings []SecretFinding
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, sp := range SecretPatterns {
			if sp.Pattern.MatchString(line) {
				match := sp.Pattern.FindString(line)
				findings = append(findings, SecretFinding{
					File:  path,
					Line:  lineNum,
					Type:  sp.Name,
					Match: match,
				})
			}
		}
	}

	return findings, nil
}

func maskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func isTestFile(name string) bool {
	return strings.Contains(name, "_test.") ||
		strings.Contains(name, ".test.") ||
		strings.Contains(name, ".spec.") ||
		strings.HasPrefix(name, "test_")
}

func isCodeFile(name string) bool {
	exts := []string{
		".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".rb", ".java",
		".rs", ".c", ".cpp", ".h", ".hpp", ".cs", ".php", ".swift",
		".kt", ".scala", ".sh", ".bash", ".zsh", ".yaml", ".yml",
		".json", ".xml", ".env", ".toml", ".ini", ".cfg", ".conf",
	}
	for _, ext := range exts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// DependencyAuditTool checks for vulnerable dependencies.
type DependencyAuditTool struct{}

func (t *DependencyAuditTool) Name() string { return "dependency_audit" }
func (t *DependencyAuditTool) Description() string {
	return `Audit dependencies for known vulnerabilities.

Supports:
- Go: govulncheck
- Node.js: npm audit
- Python: pip-audit, safety
- Rust: cargo audit`
}
func (t *DependencyAuditTool) RequiresConfirmation(mode string) bool { return false }

func (t *DependencyAuditTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Project directory to audit.",
			},
			"framework": map[string]any{
				"type":        "string",
				"enum":        []string{"auto", "go", "npm", "pip", "cargo"},
				"description": "Package manager/framework (default: auto-detect).",
			},
		},
	}
}

func (t *DependencyAuditTool) Execute(ctx context.Context, args map[string]any) (tools.Result, error) {
	path, _ := tools.StringArg(args, "path")
	if path == "" {
		path = "."
	}

	framework, _ := tools.StringArg(args, "framework")
	if framework == "" || framework == "auto" {
		framework = detectPackageManager(path)
	}

	if framework == "" {
		return tools.Result{
			IsError: true,
			Content: "could not detect package manager",
		}, nil
	}

	var cmd *exec.Cmd
	switch framework {
	case "go":
		cmd = exec.CommandContext(ctx, "govulncheck", "./...")
	case "npm":
		cmd = exec.CommandContext(ctx, "npm", "audit", "--json")
	case "pip":
		cmd = exec.CommandContext(ctx, "pip-audit")
	case "cargo":
		cmd = exec.CommandContext(ctx, "cargo", "audit")
	default:
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("unsupported framework: %s", framework),
		}, nil
	}

	cmd.Dir = path
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for vulnerabilities
	if err != nil {
		// For npm audit, exit code 1 means vulnerabilities found
		if framework == "npm" && strings.Contains(outputStr, "vulnerabilities") {
			return tools.Result{
				Content: fmt.Sprintf("⚠️  Vulnerabilities found:\n\n%s", summarizeNpmAudit(outputStr)),
				Metadata: map[string]any{
					"framework":       framework,
					"vulnerabilities": true,
				},
			}, nil
		}
		return tools.Result{
			IsError: true,
			Content: fmt.Sprintf("audit failed: %v\n%s", err, outputStr),
		}, nil
	}

	if strings.Contains(outputStr, "no vulnerabilities") ||
		strings.Contains(outputStr, "No vulnerabilities") ||
		strings.Contains(outputStr, "0 vulnerabilities") {
		return tools.Result{
			Content: "✅ No known vulnerabilities found",
			Metadata: map[string]any{
				"framework":       framework,
				"vulnerabilities": false,
			},
		}, nil
	}

	return tools.Result{
		Content: outputStr,
		Metadata: map[string]any{
			"framework": framework,
		},
	}, nil
}

func detectPackageManager(path string) string {
	if fileExists(filepath.Join(path, "go.mod")) {
		return "go"
	}
	if fileExists(filepath.Join(path, "package.json")) {
		return "npm"
	}
	if fileExists(filepath.Join(path, "requirements.txt")) ||
		fileExists(filepath.Join(path, "pyproject.toml")) {
		return "pip"
	}
	if fileExists(filepath.Join(path, "Cargo.toml")) {
		return "cargo"
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func summarizeNpmAudit(output string) string {
	lines := strings.Split(output, "\n")
	var summary []string
	for _, line := range lines {
		if strings.Contains(line, "Severity") ||
			strings.Contains(line, "vulnerabilities") ||
			strings.Contains(line, "fix available") {
			summary = append(summary, strings.TrimSpace(line))
		}
	}
	if len(summary) > 0 {
		return strings.Join(summary, "\n")
	}
	return output
}
