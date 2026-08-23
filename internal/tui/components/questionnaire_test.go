package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/iSundram/Automergent/internal/tools/interaction"
)

func newTestQuestionnaire(t *testing.T) *Questionnaire {
	t.Helper()
	return NewQuestionnaire(testStyles())
}

func TestQuestionnaireFlowOptionsAndReview(t *testing.T) {
	q := newTestQuestionnaire(t)
	var submitted string
	req := interaction.QuestionnaireRequest{
		Questions: []interaction.AskQuestion{
			{Text: "Which auth approach?", Options: []string{"JWT", "OAuth2"}, AllowCustom: true},
			{Text: "Migrate data now?", Options: []string{"yes", "no"}},
		},
	}
	if !q.Begin(req, func(s string) { submitted = s }, nil) {
		t.Fatal("Begin failed")
	}
	if !q.Visible() {
		t.Fatal("should be visible")
	}

	// Q1: quick-pick option 2 via number key.
	q.Update(tea.KeyPressMsg{Code: '2'})
	if q.state != qStateSelecting || q.current != 1 {
		t.Fatalf("expected advance to Q2 selecting, got state=%v cur=%d", q.state, q.current)
	}
	if q.answers[0] != "OAuth2" {
		t.Fatalf("answer0 = %q", q.answers[0])
	}

	// Q2 (no custom): enter picks highlighted option.
	q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if q.state != qStateReview {
		t.Fatalf("expected review, got %v", q.state)
	}

	// Review: submit with s.
	q.Update(tea.KeyPressMsg{Code: 's'})
	if q.Visible() {
		t.Fatal("should hide after submit")
	}
	for _, want := range []string{"Q: Which auth approach?", "A: OAuth2", "Q: Migrate data now?", "A: yes"} {
		if !strings.Contains(submitted, want) {
			t.Errorf("missing %q in:\n%s", want, submitted)
		}
	}
}

func TestQuestionnaireCustomAnswerPath(t *testing.T) {
	q := newTestQuestionnaire(t)
	var cancelled bool
	req := interaction.QuestionnaireRequest{
		Questions: []interaction.AskQuestion{{Text: "Name the service", AllowCustom: true}},
	}
	q.Begin(req, func(string) {}, func() { cancelled = true })

	// No options -> cursor lands on custom row; enter opens input.
	q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if q.state != qStateCustom {
		t.Fatalf("expected custom entry, got %v", q.state)
	}
	q.custom.SetValue("auth-svc")
	q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if q.state != qStateReview {
		t.Fatalf("expected review after custom, got %v", q.state)
	}
	// esc dismisses from review.
	q.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !cancelled {
		t.Error("onCancel should have fired")
	}
}

func TestQuestionnaireViewRenders(t *testing.T) {
	q := newTestQuestionnaire(t)
	q.SetSize(80, 24)
	q.Begin(interaction.QuestionnaireRequest{
		Questions: []interaction.AskQuestion{{Text: "Pick one", Options: []string{"alpha", "beta"}}},
	}, nil, nil)
	view := q.View()
	for _, want := range []string{"Question 1 of 1", "Pick one", "1. alpha", "2. beta"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
