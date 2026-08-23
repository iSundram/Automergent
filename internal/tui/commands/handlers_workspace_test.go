package commands

import (
	"strings"
	"testing"
)

func TestRewindWithoutCheckpoints(t *testing.T) {
	m := NewMockHost()
	handleRewind(m, nil)
	if !strings.Contains(m.systemMessages[0], "No checkpoints yet") {
		t.Fatalf("expected empty-state message: %q", m.systemMessages[0])
	}
}

func TestRewindListsCheckpoints(t *testing.T) {
	m := NewMockHost()
	m.checkpoints = []CheckpointInfo{
		{Index: 1, Label: "fix parser", Messages: 4},
		{Index: 2, Label: "add tests", Messages: 8},
	}
	handleRewind(m, []string{"list"})
	out := strings.Join(m.systemMessages, "\n")
	if len(m.rewindToCalls) != 0 {
		t.Fatal("list path must not restore anything")
	}
	_ = out
}

func TestRewindBareOpensPicker(t *testing.T) {
	m := NewMockHost()
	m.checkpoints = []CheckpointInfo{{Index: 1, Label: "x"}}
	handleRewind(m, nil)
	if m.rewindPickerCalls != 1 {
		t.Fatal("bare /rewind with checkpoints must open the picker")
	}
}

func TestRewindRestoresByIndex(t *testing.T) {
	m := NewMockHost()
	m.checkpoints = []CheckpointInfo{{Index: 1, Label: "x", Messages: 2}, {Index: 2, Label: "y", Messages: 5}}
	handleRewind(m, []string{"2"})
	if len(m.rewindToCalls) != 1 || m.rewindToCalls[0] != 2 {
		t.Fatalf("restore not requested correctly: %v", m.rewindToCalls)
	}

	m = NewMockHost()
	handleRewind(m, []string{"99"})
	if len(m.errorMessages) == 0 {
		t.Fatal("out-of-range index must surface an error")
	}
}

func TestRewindRejectsNonNumericIndex(t *testing.T) {
	m := NewMockHost()
	m.checkpoints = []CheckpointInfo{{Index: 1}}
	handleRewind(m, []string{"abc"})
	if len(m.usageMessages) == 0 {
		t.Fatal("non-numeric index should show usage")
	}
}

func TestBranchRequiresNameAndDelegates(t *testing.T) {
	m := NewMockHost()
	handleBranch(m, nil)
	if len(m.usageMessages) == 0 || len(m.branchCalls) != 0 {
		t.Fatal("bare /branch must show usage without forking")
	}

	handleBranch(m, []string{"experiment"})
	if len(m.branchCalls) != 1 || m.branchCalls[0] != "experiment" {
		t.Fatalf("branch not delegated: %v", m.branchCalls)
	}
}

func TestFilesEmptyAndPopulated(t *testing.T) {
	m := NewMockHost()
	handleFiles(m, nil)
	if !strings.Contains(m.systemMessages[0], "No files touched") {
		t.Fatalf("empty state wrong: %q", m.systemMessages[0])
	}

	m = NewMockHost()
	m.contextFiles = []string{"a.go", "b.go"}
	m.extraDirs = []string{"/tmp/extra"}
	handleFiles(m, nil)
	out := m.systemMessages[0]
	for _, want := range []string{"(2)", "a.go", "b.go", "/tmp/extra"} {
		if !strings.Contains(out, want) {
			t.Fatalf("files output missing %q:\n%s", want, out)
		}
	}
}

func TestCostWithAndWithoutStorage(t *testing.T) {
	m := NewMockHost()
	handleCost(m, nil)
	if !strings.Contains(m.systemMessages[0], "unavailable") {
		t.Fatal("no-storage case should say totals unavailable")
	}

	m = NewMockHost()
	m.tokenSessions, m.tokenTotalIn, m.tokenTotalOut = 7, 12000, 3400
	handleCost(m, nil)
	out := m.systemMessages[0]
	for _, want := range []string{"in 100", "out 50", "$0.0010", "7", "12000"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cost output missing %q:\n%s", want, out)
		}
	}
}

func TestConfigPanelSmoke(t *testing.T) {
	m := NewMockHost()
	m.secBlocked = []string{"~/.ssh/**"}
	m.secAllowed = []string{"./**"}
	handleConfig(m, []string{"show"})
	out := m.systemMessages[0]
	for _, want := range []string{"google / gemini-3.6-flash", "set (hidden)", "1 blocked · 1 allowed", "Validation:", "automergent config show"} {
		if !strings.Contains(out, want) {
			t.Fatalf("config panel missing %q:\n%s", want, out)
		}
	}
}

func TestConfigBareOpensSettingsPicker(t *testing.T) {
	m := NewMockHost()
	handleConfig(m, nil)
	if m.settingsPickerCalls != 1 {
		t.Fatal("bare /config must open the settings picker")
	}
}

func TestPermissionsMergesApprovalsAndWriteRules(t *testing.T) {
	m := NewMockHost()
	m.secBlocked = []string{"secrets/**"}
	handlePermissions(m, []string{"revoke", "1"})
	if len(m.handleApprovalsCalls) != 1 {
		t.Fatal("approvals flow not invoked")
	}
	args := m.handleApprovalsCalls[0]
	if args[0] != "revoke" || args[1] != "1" {
		t.Fatalf("args not forwarded: %v", args)
	}
	if !strings.Contains(strings.Join(m.systemMessages, "\n"), "blocked: secrets/**") {
		t.Fatal("write-path rules missing from permissions output")
	}
}

func TestPermissionsBareOpensPicker(t *testing.T) {
	m := NewMockHost()
	handlePermissions(m, nil)
	if m.permissionsPickerCalls != 1 {
		t.Fatal("bare /permissions must open the picker")
	}
}

func TestAddDirFlows(t *testing.T) {
	m := NewMockHost()
	handleAddDir(m, nil)
	if len(m.usageMessages) == 0 {
		t.Fatal("bare add-dir needs usage")
	}

	m = NewMockHost()
	handleAddDir(m, []string{"../vendor"})
	if len(m.addDirCalls) != 1 || m.addDirCalls[0] != "../vendor" {
		t.Fatalf("path not passed through: %v", m.addDirCalls)
	}

	m = NewMockHost()
	m.extraDirs = []string{"/already/there"}
	handleAddDir(m, nil)
	if !strings.Contains(m.systemMessages[len(m.systemMessages)-1], "/already/there") {
		t.Fatal("bare add-dir with existing roots should list them")
	}
}

func TestSummaryPromptShape(t *testing.T) {
	m := NewMockHost()
	handleSummary(m, []string{"the", "migration"})
	prompt := m.startAgentCalls[0]
	for _, want := range []string{"goals", "open items", "Emphasis: the migration"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("summary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestSecurityReviewPromptShape(t *testing.T) {
	m := NewMockHost()
	handleSecurityReview(m, nil)
	prompt := m.startAgentCalls[0]
	for _, want := range []string{"git diff", "credentials", "severity: critical", "Do not modify"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("security review prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestIssueRequiresTitle(t *testing.T) {
	m := NewMockHost()
	handleIssue(m, nil)
	if len(m.usageMessages) == 0 || len(m.startAgentCalls) != 0 {
		t.Fatal("/issue without title must show usage and not start agent")
	}

	handleIssue(m, []string{"Fix", "login", "race"})
	prompt := m.startAgentCalls[0]
	if !strings.Contains(prompt, `"Fix login race"`) || !strings.Contains(prompt, "gh") {
		t.Fatalf("issue prompt malformed:\n%s", prompt)
	}
}

func TestPRCommentsNormalizesBareNumber(t *testing.T) {
	m := NewMockHost()
	handlePRComments(m, []string{"42"})
	if !strings.Contains(m.startAgentCalls[0], "#42") {
		t.Fatalf("bare number not normalized to #ref:\n%s", m.startAgentCalls[0])
	}

	m = NewMockHost()
	handlePRComments(m, []string{"https://github.com/o/r/pull/9"})
	if strings.Contains(m.startAgentCalls[0], "#https") {
		t.Fatal("URL must not be prefixed with #")
	}
}
