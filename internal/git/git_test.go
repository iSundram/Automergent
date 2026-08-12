package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testGitDir creates a temporary git repository for testing
func testGitDir(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(dir)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init", dir)
	if err := cmd.Run(); err != nil {
		cleanup()
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure user
	cmd = exec.Command("git", "-C", dir, "config", "user.name", "Test User")
	cmd.Run()
	cmd = exec.Command("git", "-C", dir, "config", "user.email", "test@example.com")
	cmd.Run()

	return dir, cleanup
}

// addFile adds a file to the repo
func addFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
}

// stageAndCommit stages and commits all changes
func stageAndCommit(t *testing.T, dir, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "add", "-A")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to stage files: %v", err)
	}

	cmd = exec.Command("git", "-C", dir, "commit", "-m", message)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}
}

func TestIsRepo(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	if !IsRepo(ctx, dir) {
		t.Error("expected directory to be a git repo")
	}

	nonRepo, _ := os.MkdirTemp("", "non-git-*")
	defer os.RemoveAll(nonRepo)

	if IsRepo(ctx, nonRepo) {
		t.Error("expected non-git directory to not be a repo")
	}
}

func TestRootDir(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Create subdirectory
	subDir := filepath.Join(dir, "subdir")
	os.Mkdir(subDir, 0755)

	root, err := RootDir(ctx, subDir)
	if err != nil {
		t.Fatalf("RootDir error: %v", err)
	}

	if root != dir {
		t.Errorf("expected root %s, got %s", dir, root)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Need at least one commit to have a branch
	addFile(t, dir, "test.txt", "content")
	stageAndCommit(t, dir, "initial commit")

	branch, err := CurrentBranch(ctx, dir)
	if err != nil {
		t.Fatalf("CurrentBranch error: %v", err)
	}

	// Could be "main" or "master" depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("unexpected branch name: %s", branch)
	}
}

func TestStatus(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Add initial commit
	addFile(t, dir, "test.txt", "content")
	stageAndCommit(t, dir, "initial")

	// Modify file
	addFile(t, dir, "test.txt", "modified content")

	status, err := Status(ctx, dir)
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}

	if status == "" {
		t.Error("expected non-empty status")
	}
}

func TestDiff(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Add initial commit
	addFile(t, dir, "test.txt", "line1\nline2\n")
	stageAndCommit(t, dir, "initial")

	// Modify file
	addFile(t, dir, "test.txt", "line1\nline2\nline3\n")

	diff, err := Diff(ctx, dir)
	if err != nil {
		t.Fatalf("Diff error: %v", err)
	}

	if diff == "" {
		t.Error("expected non-empty diff")
	}
}

func TestStagedDiff(t *testing.T) {
	dir, cleanup := testGitDir(t)
	defer cleanup()

	ctx := context.Background()

	// Add initial commit
	addFile(t, dir, "test.txt", "line1\n")
	stageAndCommit(t, dir, "initial")

	// Modify and stage
	addFile(t, dir, "test.txt", "line1\nline2\n")
	exec.Command("git", "-C", dir, "add", "test.txt").Run()

	diff, err := StagedDiff(ctx, dir)
	if err != nil {
		t.Fatalf("StagedDiff error: %v", err)
	}

	if diff == "" {
		t.Error("expected non-empty staged diff")
	}
}
