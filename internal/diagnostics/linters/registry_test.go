package linters

import (
	"testing"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

func TestParseGolangCILintOutput(t *testing.T) {
	output := `{"Issues":[{"FromLinter":"govet","Text":"printf: non-constant format string in call to fmt.Printf","Severity":"WARNING","SourceLines":null,"Pos":{"Filename":"internal/tools/foo.go","Offset":0,"Line":42,"Column":5},"ExpectedNoLint":false},{"FromLinter":"staticcheck","Text":"SA9003: empty branch","Severity":"","Pos":{"Filename":"internal/tools/bar.go","Offset":0,"Line":7,"Column":1}}],"Report":{"Linters":[]}}`

	diags := parseGolangCILintOutput(output, "warning")
	if len(diags) != 2 {
		t.Fatalf("want 2 diagnostics, got %d: %+v", len(diags), diags)
	}
	first := diags[0]
	if first.Line != 42 || first.Column != 5 || first.FilePath != "internal/tools/foo.go" {
		t.Fatalf("position misparsed: %+v", first)
	}
	if first.Severity != "warning" {
		t.Fatalf("WARNING should map to warning: %+v", first)
	}
	if first.Code != "govet" || first.Source != "golangci-lint" {
		t.Fatalf("source metadata misparsed: %+v", first)
	}
	// Empty severity falls back to the configured default.
	if diags[1].Severity != "warning" {
		t.Fatalf("empty severity should use default: %+v", diags[1])
	}
}

func TestParseGolangCILintOutput_PrefixedWithLogs(t *testing.T) {
	output := "level=warning msg=\"[config_reader] .golangci.yml\"\n{\"Issues\":[]}"
	if diags := parseGolangCILintOutput(output, "warning"); diags != nil {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}

func TestParseRuffOutput(t *testing.T) {
	output := `[{"code":"F841","message":"Local variable ` + "`x`" + ` is assigned to but never used","filename":"app.py","location":{"row":10,"column":8},"end_location":{"row":10,"column":9},"noqa_row":10,"url":"https://docs.astral.sh/ruff/rules/f841"}]`

	diags := parseRuffOutput(output, "warning")
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Line != 10 || d.Column != 7 {
		t.Fatalf("location misparsed (want 1-indexed row, 0-indexed column): %+v", d)
	}
	if d.EndLine != 10 || d.EndColumn != 9 {
		t.Fatalf("end location misparsed: %+v", d)
	}
	if d.Code != "F841" || d.Source != "ruff" {
		t.Fatalf("metadata misparsed: %+v", d)
	}
}

func TestRegistry_OnlyParseableLintersRegistered(t *testing.T) {
	r := NewLinterRegistry()
	for _, l := range r.GetAllLinters() {
		switch l.Name() {
		case "golangci-lint", "ruff":
		default:
			t.Errorf("unexpected linter registered: %s (only linters with real output parsers belong here)", l.Name())
		}
	}
}

func TestRegistry_LintWithoutPathIsNoop(t *testing.T) {
	r := NewLinterRegistry()
	// Bare filenames (tests, in-memory content) must not shell out.
	diags, err := r.Lint(nil, types.LangGo, "main.go", "package main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diags != nil {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}

func TestApplyConfig_ReplacesArgs(t *testing.T) {
	r := NewLinterRegistry()
	r.ApplyConfig(LinterConfigMap{
		"ruff": {Enabled: true, Timeout: 5000000000, Args: []string{"check", "--fix"}},
	})
	for _, l := range r.GetAllLinters() {
		if l.Name() != "ruff" {
			continue
		}
		cfg := l.Config()
		if len(cfg.Args) != 2 || cfg.Args[1] != "--fix" {
			t.Fatalf("ApplyConfig did not replace args: %+v", cfg)
		}
	}
}
