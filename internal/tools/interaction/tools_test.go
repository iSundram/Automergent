package interaction

import (
	"context"
	"strings"
	"testing"
)

func TestParseQuestions(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{"question": "Which DB?", "options": []any{"sqlite", "postgres"}},
			map[string]any{"question": "Why?", "allow_custom": false},
			"junk",
			map[string]any{"noquestion": true},
		},
	}
	qs := parseQuestions(args)
	if len(qs) != 2 {
		t.Fatalf("expected 2 valid questions, got %d", len(qs))
	}
	if len(qs[0].Options) != 2 || !qs[0].AllowCustom {
		t.Fatalf("q0 = %+v", qs[0])
	}
	if qs[1].AllowCustom {
		t.Fatal("allow_custom=false must be honored")
	}
}

func TestStructuredAskHookReceivesAndReturns(t *testing.T) {
	var got QuestionnaireRequest
	SetQuestionnaire(func(req QuestionnaireRequest) (string, error) {
		got = req
		return "Q: Which DB?\nA: postgres\nQ: Migrate?\nA: yes", nil
	})
	t.Cleanup(func() { SetQuestionnaire(nil) })

	tool := NewAskUserTool(func(string) (string, error) {
		t.Fatal("legacy responder must not run when hook installed")
		return "", nil
	})
	res, err := tool.Execute(context.Background(), map[string]any{
		"questions": []any{
			map[string]any{"question": "Which DB?", "options": []any{"sqlite", "postgres"}},
			map[string]any{"question": "Migrate?", "options": []any{"yes", "no"}},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("execute failed: %v %v", err, res.Content)
	}
	if len(got.Questions) != 2 {
		t.Fatalf("hook got %d questions", len(got.Questions))
	}
	if !strings.Contains(res.Content, "A: postgres") {
		t.Fatalf("answer passthrough broken: %q", res.Content)
	}
}

func TestLegacyFallbackWithoutHook(t *testing.T) {
	SetQuestionnaire(nil)
	tool := NewAskUserTool(func(q string) (string, error) { return "custom answer", nil })
	res, err := tool.Execute(context.Background(), map[string]any{"question": "hello?"})
	if err != nil || res.IsError || res.Content != "custom answer" {
		t.Fatalf("legacy path broken: %v %v %+v", err, res.IsError, res)
	}
}

func TestQuestionsText(t *testing.T) {
	got := questionsText([]AskQuestion{{Text: "a"}, {Text: "b"}})
	if got != "a | b" {
		t.Fatalf("got %q", got)
	}
}
