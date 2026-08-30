package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathScopeInBounds(t *testing.T) {
	dir := t.TempDir()
	ps := NewPathScope(dir)

	cases := []struct {
		path  string
		write bool
		want  bool
	}{
		{filepath.Join(dir, "main.go"), false, true},
		{filepath.Join(dir, "a", "b", "c.txt"), true, true},
		{dir, false, true},
		{"/etc/passwd", false, false},
		{"/etc/passwd", true, false},
		{"/usr/local/bin/tool", true, false},
	}
	for _, tc := range cases {
		if got := ps.Check(tc.path, tc.write).Allowed; got != tc.want {
			t.Errorf("Check(%q, write=%v) = %v, want %v", tc.path, tc.write, got, tc.want)
		}
	}
}

func TestPathScopeRelativeAndTilde(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Skip("cannot chdir")
	}
	defer os.Chdir(oldWd)

	ps := NewPathScope(dir)
	// Relative paths resolve against the process cwd, which is the scope root.
	if !ps.Check("main.go", false).Allowed {
		t.Error("relative path inside project must be in-bounds")
	}
	// ~/... expands to the home dir — outside the project unless it IS the
	// project; assert it does not crash and decides either way.
	_ = ps.Check("~/some-file", false)
}

func TestPathScopeGrantedDir(t *testing.T) {
	project := t.TempDir()
	ps := NewPathScope(project)

	outside := t.TempDir() // a second, distinct directory
	if ps.Check(filepath.Join(outside, "data.json"), false).Allowed {
		t.Fatal("second dir must be out-of-bounds before the grant")
	}

	ps.AddGrantedDir(outside)
	if !ps.Check(filepath.Join(outside, "data.json"), false).Allowed {
		t.Error("granted dir must allow reads")
	}
	if !ps.Check(filepath.Join(outside, "nested", "x"), true).Allowed {
		t.Error("granted dir must allow writes beneath it")
	}
}

func TestPathScopeProtectedWrites(t *testing.T) {
	dir := t.TempDir()
	ps := NewPathScope(dir)

	gitConfig := filepath.Join(dir, ".git", "config")
	if ps.Check(gitConfig, true).Allowed {
		t.Error("write into .git must require approval")
	}
	if !ps.Check(gitConfig, false).Allowed {
		t.Error("read from .git stays in-bounds (approval, not denial, is the write rule)")
	}
	sshKey := filepath.Join(dir, ".ssh", "id_rsa")
	if ps.Check(sshKey, true).Allowed {
		t.Error("write into .ssh must require approval")
	}
	normal := filepath.Join(dir, "src", "main.go")
	if !ps.Check(normal, true).Allowed {
		t.Error("ordinary in-bounds write must not prompt")
	}
}

func TestPathScopeDecisionMessage(t *testing.T) {
	ps := NewPathScope(t.TempDir())
	decision := ps.Check("/etc/passwd", false)
	if decision.Allowed || decision.OutsideDir != "/etc/passwd" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Reason == "" {
		t.Error("out-of-bounds decision must carry a reason for the prompt")
	}
}

func TestAbsolutePathsInCommand(t *testing.T) {
	cmd := `cat /etc/hosts && grep foo /var/log/system.log; echo done > /tmp/out.txt`
	paths := AbsolutePathsInCommand(cmd)
	want := map[string]bool{"/etc/hosts": true, "/var/log/system.log": true, "/tmp/out.txt": true}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want keys of %v", paths, want)
	}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

func TestCdTargets(t *testing.T) {
	cases := []struct {
		cmd   string
		want  []string
	}{
		{"cd /tmp && ls", []string{"/tmp"}},
		{"cd ../elsewhere; make", []string{"../elsewhere"}},
		{"echo hi", nil},
		{"cd -", nil},
		{"cd sub && cd other && ls", []string{"sub", "other"}},
	}
	for _, tc := range cases {
		got := CdTargets(tc.cmd)
		if len(got) != len(tc.want) {
			t.Errorf("CdTargets(%q) = %v, want %v", tc.cmd, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("CdTargets(%q)[%d] = %q, want %q", tc.cmd, i, got[i], tc.want[i])
			}
		}
	}
}

func TestHasCompoundCdWrite(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"cd .git && mv a b", true},       // write after cd
		{"cd /tmp && echo hi > out", true}, // redirection after cd
		{"cd /tmp && ls", false},           // read-only after cd
		{"ls > out.txt", false},            // write without cd
		{"cd /tmp", false},                 // bare cd
	}
	for _, tc := range cases {
		if got := HasCompoundCdWrite(tc.cmd); got != tc.want {
			t.Errorf("HasCompoundCdWrite(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestCheckBashCommand(t *testing.T) {
	dir := t.TempDir()
	ps := NewPathScope(dir)

	if d := ps.CheckBashCommand("cat "+filepath.Join(dir, "f.txt"), ""); !d.Allowed {
		t.Errorf("in-project read blocked: %+v", d)
	}
	if d := ps.CheckBashCommand("cat /etc/hosts", ""); d.Allowed {
		t.Error("out-of-project absolute path must be flagged")
	}
	if d := ps.CheckBashCommand("cd /etc && ls", ""); d.Allowed {
		t.Error("cd outside the project must be flagged")
	}
	// Relative cd resolves against the shell's persistent cwd.
	shellCwd := "/var/tmp"
	if d := ps.CheckBashCommand("cd other && ls", shellCwd); d.Allowed {
		t.Error("relative cd outside the project must be flagged")
	}
	if d := ps.CheckBashCommand("cd .git && touch x", dir); d.Allowed {
		t.Error("compound cd+write must require approval")
	}
}

func TestDirGrantScopes(t *testing.T) {
	scope := GrantScope("/some/dir")
	dir, ok := IsDirGrant(scope)
	if !ok || dir != "/some/dir" {
		t.Fatalf("round trip failed: %q -> %q, %v", scope, dir, ok)
	}
	if _, ok := IsDirGrant("name=\"bash\";action=write"); ok {
		t.Error("non-directory scope must not parse as a dir grant")
	}
}

func TestExtractToolPaths(t *testing.T) {
	args := map[string]any{
		"path":    "/a/b.txt",
		"unknown": "ignored",
	}
	paths := ExtractToolPaths("edit_file", args)
	if len(paths) != 1 || paths[0] != "/a/b.txt" {
		t.Fatalf("paths = %v", paths)
	}

	bashArgs := map[string]any{"command": "cat /etc/hosts"}
	paths = ExtractToolPaths("bash", bashArgs)
	if len(paths) != 1 || paths[0] != "/etc/hosts" {
		t.Fatalf("bash paths = %v", paths)
	}
}
