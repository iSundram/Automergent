package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// IsRepo reports whether the given directory is inside a git repository.
func IsRepo(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

// RootDir returns the root directory of the repository containing dir.
func RootDir(ctx context.Context, dir string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git root: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

// CurrentBranch returns the current branch name.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// Status returns the short git status.
func Status(ctx context.Context, dir string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--short")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Diff returns the current diff.
func Diff(ctx context.Context, dir string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}

// StagedDiff returns the staged diff.
func StagedDiff(ctx context.Context, dir string) (string, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--cached")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
