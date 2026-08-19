package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/iSundram/Automergent/internal/config"
)

func testApprovalCfg() *config.Config {
	return config.Default()
}

func press(model tea.Model, keys ...tea.KeyMsg) approvalModel {
	cur := model
	for _, k := range keys {
		next, _ := cur.Update(k)
		cur = next
	}
	return cur.(approvalModel)
}

func TestApprovalTrustScreenShowsWorkspaceAndOptions(t *testing.T) {
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	m.step = stepTrust
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Accessing workspace:", "/root/testproj", "Do you trust the contents of this project?", "Yes, I trust this folder", "Yes, and remember this folder", "No, exit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("approval view missing %q:\n%s", want, view)
		}
	}
}

func TestApprovalSelectTrustApprovesSessionOnly(t *testing.T) {
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	m.step = stepTrust
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.result.approved || m.result.remember {
		t.Fatalf("expected session-only approval, got %+v", m.result)
	}
}

func TestApprovalSelectRememberApprovesAndRemembers(t *testing.T) {
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	m.step = stepTrust
	m = press(m, tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.result.approved || !m.result.remember {
		t.Fatalf("expected remember approval, got %+v", m.result)
	}
}

func TestApprovalSelectExitRejects(t *testing.T) {
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	m.step = stepTrust
	m = press(m,
		tea.KeyPressMsg{Code: tea.KeyDown},
		tea.KeyPressMsg{Code: tea.KeyDown},
		tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.result.approved {
		t.Fatal("expected rejection on exit")
	}
}

func TestApprovalEscRejects(t *testing.T) {
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	m.step = stepTrust
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.result.approved {
		t.Fatal("expected rejection on esc")
	}
}

func TestApprovalRootRiskShowsWarningFirstTime(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	m := newApprovalModel(testApprovalCfg(), "/root/testproj")
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "root/admin privileges") {
		t.Fatalf("expected root warning first:\n%s", view)
	}
}

func TestApprovalRootRiskAcceptPersistsAndContinues(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	cfg := testApprovalCfg()
	m := newApprovalModel(cfg, "/root/testproj")
	m = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !cfg.Security.RootRiskAcknowledged {
		t.Fatal("expected root risk to be acknowledged")
	}
	if m.step != stepTrust {
		t.Fatalf("expected to advance to trust step after accepting risk, step=%d", m.step)
	}
}

func TestApprovalRootRiskSkipWhenAcknowledged(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	cfg := testApprovalCfg()
	cfg.Security.RootRiskAcknowledged = true
	m := newApprovalModel(cfg, "/root/testproj")
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "root/admin privileges") {
		t.Fatalf("root warning must be skipped when acknowledged:\n%s", view)
	}
}