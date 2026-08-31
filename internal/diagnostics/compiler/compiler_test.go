package compiler

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/diagnostics/types"
)

func TestGoParser_BuildErrors(t *testing.T) {
	output := `# example.com/pkg
./internal/foo.go:12:5: undefined: someFunc
./internal/foo.go:20:2: missing return at end of function
`
	diags := ParseOutput(output, LangGo)
	if len(diags) != 2 {
		t.Fatalf("want 2 diagnostics, got %d: %+v", len(diags), diags)
	}
	if diags[0].FilePath != "./internal/foo.go" || diags[0].Line != 12 || diags[0].Column != 5 {
		t.Fatalf("first diag misparsed: %+v", diags[0])
	}
	if diags[0].Category != CategoryType {
		t.Fatalf("undefined should categorize as type: %+v", diags[0])
	}
	if !hasTag(diags[0].Tags, "undefined") {
		t.Fatalf("undefined tag missing: %+v", diags[0])
	}
}

// Regression: enrichment patterns used to attach to an unrelated earlier
// diagnostic when the matching line produced no diagnostic of its own.
func TestGoParser_EnrichmentStaysOnItsLine(t *testing.T) {
	output := `./a.go:5:1: something else
cannot find package "missing.example.com/x"
`
	diags := ParseOutput(output, LangGo)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Category != CategoryUnknown {
		t.Fatalf("unrelated diagnostic was re-categorized: %+v", diags[0])
	}
}

func TestGoParser_VetOutput(t *testing.T) {
	output := `internal/foo.go:42: printf: non-constant format string in call to fmt.Errorf
`
	diags := ParseOutput(output, LangGo)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Severity != "warning" {
		t.Fatalf("vet output should be warning severity: %+v", diags[0])
	}
}

func TestTypeScriptParser_TSC(t *testing.T) {
	output := `src/app.ts(10,3): error TS2345: Argument of type 'string' is not assignable to parameter of type 'number'.
`
	diags := ParseOutput(output, LangTypeScript)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].ErrorCode != "TS2345" || diags[0].Line != 10 || diags[0].Column != 3 {
		t.Fatalf("tsc diag misparsed: %+v", diags[0])
	}
	if diags[0].Category != CategoryType {
		t.Fatalf("TS2xxx should categorize as type: %+v", diags[0])
	}
	if !strings.Contains(diags[0].URL, "ts2345") {
		t.Fatalf("expected docs URL: %+v", diags[0])
	}
}

func TestPythonParser_Mypy(t *testing.T) {
	output := `main.py:8: error: Incompatible types (found "int", expected "str")  [arg-type]
`
	diags := ParseOutput(output, LangPython)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Line != 8 || diags[0].Severity != "error" {
		t.Fatalf("mypy diag misparsed: %+v", diags[0])
	}
}

func TestPythonParser_Traceback(t *testing.T) {
	output := `Traceback (most recent call last):
  File "app.py", line 3, in <module>
    boom()
  File "app.py", line 7, in boom
    return 1 /
SyntaxError: unexpected EOF while parsing
`
	diags := ParseOutput(output, LangPython)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].FilePath != "app.py" || diags[0].Line != 7 {
		t.Fatalf("syntax error bound to wrong location: %+v", diags[0])
	}
	if diags[0].Category != CategorySyntax {
		t.Fatalf("SyntaxError should be syntax category: %+v", diags[0])
	}
}

func TestRustParser_CargoErrors(t *testing.T) {
	output := `error[E0382]: borrow of moved value: ` + "`s`" + `
  --> src/main.rs:10:14
   |
10 |     println!("{}", s);
   |                  ^
   |
help: consider cloning the value
  |
10 |     println!("{}", s.clone());
   |                   ++++++++

error: aborting due to 1 previous error
`
	diags := ParseOutput(output, LangRust)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	d := diags[0]
	if d.ErrorCode != "E0382" || d.FilePath != "src/main.rs" || d.Line != 10 {
		t.Fatalf("rust diag misparsed: %+v", d)
	}
	var helpFound bool
	for _, s := range d.Suggestions {
		if strings.Contains(s, "cloning") {
			helpFound = true
		}
	}
	if !helpFound {
		t.Fatalf("help line not captured as suggestion: %+v", d)
	}
	if !strings.Contains(d.URL, "E0382") {
		t.Fatalf("expected error-code URL: %+v", d)
	}
}

func TestJavaParser_Javac(t *testing.T) {
	output := `src/Main.java:7: error: cannot find symbol
  symbol: variable undefinedVar
`
	diags := ParseOutput(output, LangJava)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].Line != 7 || diags[0].Severity != "error" || diags[0].Category != CategoryType {
		t.Fatalf("javac diag misparsed: %+v", diags[0])
	}
}

func TestGenericParser_CommonShapes(t *testing.T) {
	tests := []struct {
		name, line string
		file       string
		lineNo     int
		severity   string
	}{
		{"file:line:col", "src/x.zig:4:2: error: expected ';'", "src/x.zig", 4, "error"},
		{"file:line", "src/x.zig:9: error: unknown directive", "src/x.zig", 9, "error"},
		{"paren style", "src/x.pas(3,1): Error: Syntax error", "src/x.pas", 3, "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := ParseOutput(tt.line, LangGeneric)
			if len(diags) != 1 {
				t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
			}
			if diags[0].FilePath != tt.file || diags[0].Line != tt.lineNo || diags[0].Severity != tt.severity {
				t.Fatalf("misparsed: %+v", diags[0])
			}
		})
	}
}

// Regression: the bracketed pattern "[ERROR] file:line - msg" used to land
// in the file:line branch and parse "ERROR" as the file path.
func TestGenericParser_BracketedSeverity(t *testing.T) {
	output := "[ERROR] src/Main.java:12 - cannot find symbol\n"
	diags := ParseOutput(output, LangGeneric)
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	d := diags[0]
	if d.FilePath != "src/Main.java" {
		t.Fatalf("file path misparsed: %+v", d)
	}
	if d.Line != 12 {
		t.Fatalf("line misparsed: %+v", d)
	}
	if d.Severity != "error" {
		t.Fatalf("severity misparsed: %+v", d)
	}
}

func TestDetectLanguage_FromOutput(t *testing.T) {
	cases := map[string]Language{
		"# github.com/x/y\n./a.go:1:1: undefined: X": LangGo,
		"error[E0382]: borrow of moved value":       LangRust,
		"Traceback (most recent call last)":         LangPython,
		"src/App.tsx(1,1): error TS2304":            LangTypeScript,
		"something unrecognizable":                  LangGeneric,
	}
	for output, want := range cases {
		if got := DetectLanguage(output); got != want {
			t.Errorf("DetectLanguage(%q) = %q, want %q", output, got, want)
		}
	}
}

// CompilerDiagnostic must not shadow the embedded Diagnostic fields —
// converting to a plain Diagnostic has to keep range and suggestions.
func TestCompilerDiagnostic_NoShadowing(t *testing.T) {
	cd := CompilerDiagnostic{}
	cd.Suggestions = []string{"fix it"}
	cd.Tags = []string{"go"}
	cd.EndLine = 5
	cd.EndColumn = 9

	var base types.Diagnostic = cd.Diagnostic
	if len(base.Suggestions) != 1 || base.Suggestions[0] != "fix it" {
		t.Fatalf("suggestions lost in conversion: %+v", base)
	}
	if base.EndLine != 5 || base.EndColumn != 9 {
		t.Fatalf("range lost in conversion: %+v", base)
	}
	if len(base.Tags) != 1 {
		t.Fatalf("tags lost in conversion: %+v", base)
	}
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
