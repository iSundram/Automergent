package git

import (
	"context"
	"os/exec"
	"testing"
)

func TestCategorizeFile(t *testing.T) {
	tests := []struct {
		path     string
		expected CommitType
	}{
		{"internal/git/git_test.go", CommitTypeTest},
		{"pkg/api/handler_test.go", CommitTypeTest},
		{"tests/integration/main.go", CommitTypeTest},
		{"README.md", CommitTypeDocs},
		{"docs/guide.rst", CommitTypeDocs},
		{"Makefile", CommitTypeBuild},
		{"Dockerfile", CommitTypeBuild},
		{"go.mod", CommitTypeBuild},
		{".github/workflows/ci.yml", CommitTypeCI},
		{"styles/main.css", CommitTypeStyle},
		{"internal/git/semantic.go", CommitTypeFeat},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := categorizeFile(tc.path)
			if got != tc.expected {
				t.Errorf("categorizeFile(%s) = %v, want %v", tc.path, got, tc.expected)
			}
		})
	}
}

func TestGetFileCategory(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/git/semantic.go", "git"},
		{"pkg/api/handler.go", "api"},
		{"cmd/main.go", "cmd"},
		{"main.go", ""},
		{"docs/guide.md", "docs"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := getFileCategory(tc.path)
			if got != tc.expected {
				t.Errorf("getFileCategory(%s) = %v, want %v", tc.path, got, tc.expected)
			}
		})
	}
}

func TestDeterminePrimaryType(t *testing.T) {
	tests := []struct {
		name     string
		files    []FileChange
		expected CommitType
	}{
		{
			name: "mostly tests",
			files: []FileChange{
				{Path: "test1_test.go", ChangeType: CommitTypeTest},
				{Path: "test2_test.go", ChangeType: CommitTypeTest},
				{Path: "main.go", ChangeType: CommitTypeFeat},
			},
			expected: CommitTypeFeat, // feat takes priority
		},
		{
			name: "only docs",
			files: []FileChange{
				{Path: "README.md", ChangeType: CommitTypeDocs},
				{Path: "CHANGELOG.md", ChangeType: CommitTypeDocs},
			},
			expected: CommitTypeDocs,
		},
		{
			name: "fix has priority",
			files: []FileChange{
				{Path: "main.go", ChangeType: CommitTypeFeat},
				{Path: "bug.go", ChangeType: CommitTypeFix},
			},
			expected: CommitTypeFix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := determinePrimaryType(tc.files)
			if got != tc.expected {
				t.Errorf("determinePrimaryType() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestSemanticCommitFormat(t *testing.T) {
	tests := []struct {
		name     string
		commit   SemanticCommit
		expected string
	}{
		{
			name: "simple feat",
			commit: SemanticCommit{
				Type:        CommitTypeFeat,
				Description: "add new feature",
			},
			expected: "feat: add new feature",
		},
		{
			name: "feat with scope",
			commit: SemanticCommit{
				Type:        CommitTypeFeat,
				Scope:       "git",
				Description: "add semantic commits",
			},
			expected: "feat(git): add semantic commits",
		},
		{
			name: "breaking change",
			commit: SemanticCommit{
				Type:           CommitTypeFeat,
				Scope:          "api",
				Description:    "change response format",
				BreakingChange: true,
				Footer:         "BREAKING CHANGE: response format changed",
			},
			expected: "feat(api)!: change response format\n\nBREAKING CHANGE: response format changed",
		},
		{
			name: "with body",
			commit: SemanticCommit{
				Type:        CommitTypeFix,
				Description: "resolve race condition",
				Body:        "- Fixed mutex handling\n- Added tests",
			},
			expected: "fix: resolve race condition\n\n- Fixed mutex handling\n- Added tests",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.commit.Format()
			if got != tc.expected {
				t.Errorf("Format() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestGenerateChangeSummary(t *testing.T) {
	tests := []struct {
		name     string
		files    []FileChange
		contains string
	}{
		{
			name:     "no files",
			files:    []FileChange{},
			contains: "no changes",
		},
		{
			name: "single file",
			files: []FileChange{
				{Path: "main.go", Status: "M"},
			},
			contains: "update main.go",
		},
		{
			name: "multiple additions",
			files: []FileChange{
				{Path: "file1.go", Status: "A"},
				{Path: "file2.go", Status: "A"},
			},
			contains: "add 2 files",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := generateChangeSummary(tc.files)
			if !contains(got, tc.contains) {
				t.Errorf("generateChangeSummary() = %q, want to contain %q", got, tc.contains)
			}
		})
	}
}

func TestAnalyzeChanges(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial commit
	addFile(t, dir, "internal/git/main.go", "package git\n\nfunc Main() {}\n")
	stageAndCommit(t, dir, "initial")

	// Stage new changes
	addFile(t, dir, "internal/git/semantic.go", "package git\n\nfunc Semantic() {}\n")
	addFile(t, dir, "internal/git/main.go", "package git\n\nfunc Main() { return }\n")

	// Stage the files
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "add", "-A")
	cmd.Run()

	analysis, err := AnalyzeChanges(ctx, dir)
	if err != nil {
		t.Fatalf("AnalyzeChanges error: %v", err)
	}

	if len(analysis.Files) == 0 {
		t.Error("expected files in analysis")
	}
}

func TestGenerateCommitMessage(t *testing.T) {
	analysis := &ChangeAnalysis{
		Files: []FileChange{
			{Path: "internal/git/semantic.go", Status: "A", ChangeType: CommitTypeFeat},
			{Path: "internal/git/semantic_test.go", Status: "A", ChangeType: CommitTypeTest},
		},
		PrimaryType: CommitTypeFeat,
		Scope:       "git",
		Summary:     "add semantic commit support",
	}

	commit := GenerateCommitMessage(analysis)

	if commit.Type != CommitTypeFeat {
		t.Errorf("expected type feat, got %s", commit.Type)
	}

	if commit.Scope != "git" {
		t.Errorf("expected scope git, got %s", commit.Scope)
	}

	formatted := commit.Format()
	if !contains(formatted, "feat(git):") {
		t.Errorf("expected formatted to contain 'feat(git):', got %s", formatted)
	}
}

func TestGetActionVerb(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"A", "add"},
		{"M", "update"},
		{"D", "remove"},
		{"R", "rename"},
		{"C", "copy"},
		{"X", "modify"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			got := getActionVerb(tc.status)
			if got != tc.expected {
				t.Errorf("getActionVerb(%s) = %s, want %s", tc.status, got, tc.expected)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
