package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeFile writes content to a file
func writeFile(dir, path, content string) error {
	fullPath := filepath.Join(dir, path)
	return os.WriteFile(fullPath, []byte(content), 0644)
}

// GetRemoteURL returns the URL of the origin remote
func GetRemoteURL(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetLastCommit returns info about the last commit
func GetLastCommit(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "log", "-1", "--format=%H|%an|%s")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetStagedFiles returns list of staged files
func GetStagedFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := runGit(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}

	var files []string
	for _, f := range strings.Split(out, "\n") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// StageFiles stages the specified files
func StageFiles(ctx context.Context, dir string, files ...string) error {
	args := append([]string{"add"}, files...)
	_, err := runGit(ctx, dir, args...)
	return err
}

// Commit creates a new commit with the given message
func Commit(ctx context.Context, dir, message string) error {
	_, err := runGit(ctx, dir, "commit", "-m", message)
	return err
}

// CommitWithBody creates a commit with a message and body
func CommitWithBody(ctx context.Context, dir, message, body string) error {
	_, err := runGit(ctx, dir, "commit", "-m", message, "-m", body)
	return err
}

// GetTags returns all tags
func GetTags(ctx context.Context, dir string) ([]string, error) {
	out, err := runGit(ctx, dir, "tag", "--list")
	if err != nil {
		return nil, err
	}

	var tags []string
	for _, t := range strings.Split(out, "\n") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

// GetCommitCount returns the number of commits
func GetCommitCount(ctx context.Context, dir string) (int, error) {
	out, err := runGit(ctx, dir, "rev-list", "--count", "HEAD")
	if err != nil {
		return 0, err
	}

	var count int
	fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count, nil
}

// IsClean returns true if the working tree is clean
func IsClean(ctx context.Context, dir string) (bool, error) {
	out, err := runGit(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// HasUncommittedChanges returns true if there are uncommitted changes
func HasUncommittedChanges(ctx context.Context, dir string) (bool, error) {
	clean, err := IsClean(ctx, dir)
	return !clean, err
}

// GetCurrentCommitHash returns the current commit hash
func GetCurrentCommitHash(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetShortHash returns the short hash for a commit
func GetShortHash(ctx context.Context, dir, hash string) (string, error) {
	out, err := runGit(ctx, dir, "rev-parse", "--short", hash)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Fetch fetches from remote
func Fetch(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "fetch", "--all", "--prune")
	return err
}

// Pull pulls changes from remote
func Pull(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "pull")
	return err
}

// Push pushes changes to remote
func Push(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "push")
	return err
}

// CreateBranch creates a new branch
func CreateBranch(ctx context.Context, dir, name string) error {
	_, err := runGit(ctx, dir, "checkout", "-b", name)
	return err
}

// SwitchBranch switches to an existing branch
func SwitchBranch(ctx context.Context, dir, name string) error {
	_, err := runGit(ctx, dir, "checkout", name)
	return err
}

// DeleteBranch deletes a branch
func DeleteBranch(ctx context.Context, dir, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := runGit(ctx, dir, "branch", flag, name)
	return err
}

// Stash stashes current changes
func Stash(ctx context.Context, dir string, message string) error {
	if message != "" {
		_, err := runGit(ctx, dir, "stash", "push", "-m", message)
		return err
	}
	_, err := runGit(ctx, dir, "stash")
	return err
}

// StashPop pops the last stash
func StashPop(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "stash", "pop")
	return err
}

// Rebase rebases current branch onto target
func Rebase(ctx context.Context, dir, target string) error {
	_, err := runGit(ctx, dir, "rebase", target)
	return err
}

// AbortRebase aborts an in-progress rebase
func AbortRebase(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "rebase", "--abort")
	return err
}

// ContinueRebase continues a rebase after conflict resolution
func ContinueRebase(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "rebase", "--continue")
	return err
}

// Merge merges a branch into current branch
func Merge(ctx context.Context, dir, branch string) error {
	_, err := runGit(ctx, dir, "merge", branch)
	return err
}

// AbortMerge aborts an in-progress merge
func AbortMerge(ctx context.Context, dir string) error {
	_, err := runGit(ctx, dir, "merge", "--abort")
	return err
}

// Reset resets to a commit
func Reset(ctx context.Context, dir, commit string, hard bool) error {
	mode := "--mixed"
	if hard {
		mode = "--hard"
	}
	_, err := runGit(ctx, dir, "reset", mode, commit)
	return err
}

// GetConfig gets a git config value
func GetConfig(ctx context.Context, dir, key string) (string, error) {
	out, err := runGit(ctx, dir, "config", "--get", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SetConfig sets a git config value
func SetConfig(ctx context.Context, dir, key, value string) error {
	_, err := runGit(ctx, dir, "config", key, value)
	return err
}

// GetUserName returns the configured user name
func GetUserName(ctx context.Context, dir string) (string, error) {
	return GetConfig(ctx, dir, "user.name")
}

// GetUserEmail returns the configured user email
func GetUserEmail(ctx context.Context, dir string) (string, error) {
	return GetConfig(ctx, dir, "user.email")
}
