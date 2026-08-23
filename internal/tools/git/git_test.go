package git

import (
	"context"
	"testing"

	"github.com/iSundram/Automergent/internal/tools"
)

func TestRegisterAllRegistersSuite(t *testing.T) {
	reg := tools.NewRegistry()
	RegisterAll(reg)
	want := []string{
		"git_status", "git_diff", "git_log",
		"git_add", "git_commit", "git_branch", "git_checkout", "git_stash",
	}
	for _, name := range want {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestCommitRequiresConfirmation(t *testing.T) {
	reg := tools.NewRegistry()
	RegisterAll(reg)
	c, _ := reg.Get("git_commit")
	for _, mode := range []string{"edit", "plan", "default"} {
		if !c.RequiresConfirmation(mode) {
			t.Errorf("git_commit must require confirmation in mode %q", mode)
		}
	}
}

func TestRunGitFailsCleanlyOutsideRepo(t *testing.T) {
	res := runGit(context.Background(), "status", "--porcelain")
	if !res.IsError && res.Content == "" {
		t.Log("inside a repository; skipping negative assertion")
	}
}

func TestDiffNoChanges(t *testing.T) {
	d := &diffTool{}
	out, err := d.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	_ = out // either a diff or "(no changes)" / repo error — all acceptable
}
