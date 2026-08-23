package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestParseMarkdownCommandFullFrontmatter(t *testing.T) {
	content := "---\ndescription: Review the staged diff\naliases: rev, sg\nargument-hint: [focus]\nwhen-to-use: before committing\nsensitive: true\n---\nReview $ARGUMENTS with care. Focus on $1 first."
	cmd, body, err := ParseMarkdownCommand("git/staged.md", []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "git:staged" || cmd.Category != "Custom" || !cmd.Immediate || !cmd.SupportsHeadless {
		t.Fatalf("unexpected command: %#v", cmd)
	}
	if cmd.Description != "Review the staged diff" || cmd.ArgsHint != "[focus]" || cmd.WhenToUse != "before committing" {
		t.Fatalf("metadata mismatch: %#v", cmd)
	}
	if !cmd.Sensitive || len(cmd.Aliases) != 2 || cmd.Aliases[0] != "rev" {
		t.Fatalf("flags/aliases mismatch: %#v", cmd)
	}
	if body != "Review $ARGUMENTS with care. Focus on $1 first." {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestParseMarkdownCommandWithoutFrontmatter(t *testing.T) {
	cmd, _, err := ParseMarkdownCommand("explain.md", []byte("# Title\nExplain the selected code in plain language.\nMore detail."))
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "explain" || cmd.Description != "Explain the selected code in plain language." {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}

func TestParseMarkdownCommandRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"README.md":   "# readme",
		".hidden.md":  "secret",
		"empty.md":    "",
		"blank.md":    "\n\n",
		"dangling.md": "---\ndescription: no closing fence\nBody here.",
	}
	for name, content := range cases {
		if _, _, err := ParseMarkdownCommand(name, []byte(content)); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
}

func TestExpandPromptTemplate(t *testing.T) {
	body := "Deploy $1 to $2 with notes: $ARGUMENTS spare=$3"
	got := ExpandPromptTemplate(body, []string{"staging", "eu-west", "extra", "ignored"})
	want := "Deploy staging to eu-west with notes: staging eu-west extra ignored spare=extra"
	if got != want {
		t.Fatalf("ExpandPromptTemplate = %q, want %q", got, want)
	}
	if got := ExpandPromptTemplate("no args: $ARGUMENTS", nil); got != "no args:" {
		t.Fatalf("nil args expansion = %q", got)
	}
}

func TestLoadMarkdownCommandsRegistersAndSkips(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"deploy.md":           "---\ndescription: Ship it\n---\nDeploy $ARGUMENTS",
		"git/staged.md":       "Stage-aware review of $1",
		"README.md":           "# docs",
		".secrets/command.md": "should be skipped",
		"broken.md":           "---\nunclosed",
		"notes.txt":           "not markdown",
	})

	reg := Default()
	count, warnings := LoadMarkdownCommands(reg, dir)
	if count != 2 {
		t.Fatalf("loaded %d commands, want 2; warnings: %v", count, warnings)
	}

	cmd, ok := reg.Lookup("deploy")
	if !ok || cmd.Description != "Ship it" {
		t.Fatalf("deploy not registered correctly: %#v ok=%v", cmd, ok)
	}
	if _, ok := reg.Lookup("git:staged"); !ok {
		t.Fatal("namespaced git:staged missing")
	}
	if _, ok := reg.Lookup("broken"); ok {
		t.Fatal("broken file must not register")
	}

	m := NewMockHost()
	if _, err := reg.Dispatch(m, "git:staged", []string{"main.go"}); err != nil {
		t.Fatal(err)
	}
	if len(m.startAgentCalls) != 1 || m.startAgentCalls[0] != "Stage-aware review of main.go" {
		t.Fatalf("dispatch did not expand prompt: %v", m.startAgentCalls)
	}
}

func TestBuiltinsWinOverCustomConflicts(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"model.md":    "Override builtin model command",
		"stop.md":     "Alias collision with cancel's alias",
		"harmless.md": "Fine",
	})

	reg := Default()
	count, warnings := LoadMarkdownCommands(reg, dir)
	if count != 1 {
		t.Fatalf("expected only harmless.md to load, got %d; warnings: %v", count, warnings)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 conflict warnings, got: %v", warnings)
	}

	builtinModel, _ := reg.Lookup("model")
	if builtinModel.Category != "AI & Model" {
		t.Fatalf("builtin /model was shadowed: %#v", builtinModel)
	}
	m := NewMockHost()
	if _, err := reg.Dispatch(m, "model", nil); err != nil {
		t.Fatal(err)
	}
	if len(m.systemMessages) == 0 || !strings.Contains(m.systemMessages[0], "Current model") {
		t.Fatal("dispatching /model ran the custom handler instead of the builtin")
	}
}

func TestRegisterCustomReportsErrorsInsteadOfPanicking(t *testing.T) {
	reg := Default()
	err := reg.RegisterCustom(Command{Name: "help", Description: "d", Category: "Custom", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) })
	if err == nil {
		t.Fatal("name collision should error")
	}
	err = reg.RegisterCustom(Command{Name: "ok", Aliases: []string{"exit"}, Description: "d", Category: "Custom", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) })
	if err == nil {
		t.Fatal("alias collision should error")
	}
	if err := reg.RegisterCustom(Command{Name: "dup", Aliases: []string{"same", "same"}, Description: "d", Category: "Custom", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) }); err == nil {
		t.Fatal("duplicate self-alias should be rejected, not panic")
	}
	if err := reg.RegisterCustom(Command{Name: "fine", Description: "d", Category: "Custom", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) }); err != nil {
		t.Fatalf("clean registration failed: %v", err)
	}
}

func TestFindProjectCommandsDirWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(filepath.Join(root, userCommandsNm), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got := findProjectCommandsDir(nested)
	want, _ := filepath.Abs(filepath.Join(root, userCommandsNm))
	if got != want {
		t.Fatalf("found %q, want %q", got, want)
	}
	if other := findProjectCommandsDir(t.TempDir()); other != "" {
		t.Fatalf("unrelated tree should find nothing, got %q", other)
	}
}

func TestLoadProjectAndUserCommandsEndToEnd(t *testing.T) {
	home := t.TempDir()
	workDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(workDir, userCommandsNm), 0o755); err != nil {
		t.Fatal(err)
	}
	userDir := filepath.Join(home, "userhome")
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", userDir)
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	if err := os.MkdirAll(filepath.Join(userDir, userCommandsNm), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, userCommandsNm, "proj.md"), []byte("Project prompt $1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, userCommandsNm, "global.md"), []byte("---\ndescription: Global helper\n---\nGlobal prompt"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := Default()
	count, warnings := LoadProjectAndUserCommands(reg, workDir)
	if count != 2 || len(warnings) != 0 {
		t.Fatalf("count=%d warnings=%v", count, warnings)
	}
	if _, ok := reg.Lookup("proj"); !ok {
		t.Fatal("project command not loaded")
	}
	if cmd, ok := reg.Lookup("global"); !ok || cmd.Description != "Global helper" {
		t.Fatal("user command not loaded")
	}
}
