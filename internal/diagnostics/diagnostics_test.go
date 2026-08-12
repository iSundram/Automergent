package diagnostics_test

import (
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/diagnostics"
)

// ─── Registry tests ───────────────────────────────────────────────────────────

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.py", "python"},
		{"index.js", "javascript"},
		{"App.jsx", "jsx"},
		{"src/lib.ts", "typescript"},
		{"Component.tsx", "tsx"},
		{"lib.rs", "rust"},
		{"Main.java", "java"},
		{"util.c", "c"},
		{"util.h", "c"},
		{"app.cpp", "cpp"},
		{"app.hpp", "cpp"},
		{"app.cc", "cpp"},
		{"App.cs", "csharp"},
		{"script.rb", "ruby"},
		{"index.php", "php"},
		{"App.swift", "swift"},
		{"Main.kt", "kotlin"},
		{"main.lua", "lua"},
		{"data.json", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"Cargo.toml", "toml"},
		{"unknown.xyz", ""},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := string(diagnostics.DetectLanguage(tc.path))
			if got != tc.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// ─── Go diagnostics ───────────────────────────────────────────────────────────

func TestGoParser_MissingParen(t *testing.T) {
	code := `package main

func main() {
    println("test"
}
`
	diags := diagnostics.Analyze("test.go", code)
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic, got none")
	}
	assertContainsCode(t, diags, "missing-token")
}

func TestGoParser_MissingPackage(t *testing.T) {
	code := `func main() {}`
	diags := diagnostics.Analyze("test.go", code)
	assertContainsCode(t, diags, "missing-package")
}

func TestGoParser_ValidCode(t *testing.T) {
	code := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	diags := diagnostics.Analyze("test.go", code)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid Go code, got %d: %v", len(diags), diags)
	}
}

// ─── Python diagnostics ───────────────────────────────────────────────────────

func TestPythonParser_UnclosedString(t *testing.T) {
	code := `def main():
    print("hello
`
	diags := diagnostics.Analyze("test.py", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for Python with unclosed string")
	}
}

func TestPythonParser_MissingColon(t *testing.T) {
	code := `def main()
    pass
`
	diags := diagnostics.Analyze("test.py", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for Python with missing colon")
	}
}

func TestPythonParser_ValidCode(t *testing.T) {
	code := `def greet(name):
    return f"Hello, {name}"

print(greet("world"))
`
	diags := diagnostics.Analyze("test.py", code)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid Python, got %d: %v", len(diags), diags)
	}
}

// ─── JavaScript diagnostics ───────────────────────────────────────────────────

func TestJSParser_MissingBrace(t *testing.T) {
	code := `function greet(name) {
    return "Hello, " + name;
`
	diags := diagnostics.Analyze("test.js", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for JS with missing closing brace")
	}
}

func TestJSParser_ValidCode(t *testing.T) {
	code := `function greet(name) {
    return "Hello, " + name;
}
console.log(greet("World"));
`
	diags := diagnostics.Analyze("test.js", code)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid JS, got %d: %v", len(diags), diags)
	}
}

// ─── TypeScript diagnostics ───────────────────────────────────────────────────

func TestTSParser_MissingBrace(t *testing.T) {
	code := `interface User {
    name: string;
    age: number;

function greet(user: User): string {
    return user.name;
}
`
	diags := diagnostics.Analyze("test.ts", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for TS with missing closing brace in interface")
	}
}

// ─── Rust diagnostics ─────────────────────────────────────────────────────────

func TestRustParser_MissingSemicolon(t *testing.T) {
	code := `fn main() {
    let x = 5
    println!("{}", x);
}
`
	diags := diagnostics.Analyze("test.rs", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for Rust with missing semicolon")
	}
}

func TestRustParser_ValidCode(t *testing.T) {
	code := `fn main() {
    let x: i32 = 5;
    println!("{}", x);
}
`
	diags := diagnostics.Analyze("test.rs", code)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid Rust, got %d: %v", len(diags), diags)
	}
}

// ─── JSON diagnostics ─────────────────────────────────────────────────────────

func TestJSONParser_InvalidJSON(t *testing.T) {
	code := `{"name": "test", "value":}`
	diags := diagnostics.Analyze("test.json", code)
	assertContainsCode(t, diags, "json-syntax-error")
}

func TestJSONParser_ValidJSON(t *testing.T) {
	code := `{"name": "test", "value": 42}`
	diags := diagnostics.Analyze("test.json", code)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid JSON, got %d", len(diags))
	}
}

// ─── YAML diagnostics ────────────────────────────────────────────────────────

func TestYAMLParser_InvalidYAML(t *testing.T) {
	code := "name: test\nvalue: 42\n  bad_indent: here\n"
	diags := diagnostics.Analyze("test.yaml", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for invalid YAML")
	}
}

// ─── TOML diagnostics ────────────────────────────────────────────────────────

func TestTOMLParser_InvalidTOML(t *testing.T) {
	code := "[section]\nbad = \n"
	diags := diagnostics.Analyze("test.toml", code)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for invalid TOML")
	}
}

// ─── Unsupported extension ───────────────────────────────────────────────────

func TestAnalyze_UnsupportedExtension(t *testing.T) {
	diags := diagnostics.Analyze("file.unknown", "some content")
	if diags != nil {
		t.Errorf("expected nil for unsupported extension, got %v", diags)
	}
}

// ─── Compare / Delta ─────────────────────────────────────────────────────────

func TestCompare_FixedError(t *testing.T) {
	before := diagnostics.Analyze("test.go", `func main() {
    println("test"
}`)
	after := diagnostics.Analyze("test.go", `package main

func main() {
    println("test")
}`)
	delta := diagnostics.Compare(before, after)
	if delta.FixedCount == 0 {
		t.Error("expected at least one fixed error")
	}
}

func TestCompare_IntroducedError(t *testing.T) {
	good := `package main

import "fmt"

func main() {
	fmt.Println("ok")
}
`
	bad := `import "fmt"

func main() {
	fmt.Println("ok")
}
`
	before := diagnostics.Analyze("test.go", good)
	after := diagnostics.Analyze("test.go", bad)
	delta := diagnostics.Compare(before, after)

	if delta.IsSafe {
		t.Error("expected delta to be unsafe (new error introduced)")
	}
	if delta.IntroducedCount == 0 {
		t.Error("expected at least one introduced error")
	}
}

func TestCompare_NoChange(t *testing.T) {
	code := `package main
func main() {}`
	diags := diagnostics.Analyze("test.go", code)
	delta := diagnostics.Compare(diags, diags)

	if delta.FixedCount != 0 || delta.IntroducedCount != 0 {
		t.Errorf("expected zero fixed/introduced for identical diagnostics, got fixed=%d introduced=%d",
			delta.FixedCount, delta.IntroducedCount)
	}
}

// ─── Cache ───────────────────────────────────────────────────────────────────

func TestAnalyze_CacheHit(t *testing.T) {
	code := `package main

func main() {}
`
	// Call twice – second call should hit the cache.
	first := diagnostics.Analyze("test.go", code)
	second := diagnostics.Analyze("test.go", code)

	if len(first) != len(second) {
		t.Errorf("cache returned different results: %d vs %d", len(first), len(second))
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func assertContainsCode(t *testing.T, diags []diagnostics.Diagnostic, code string) {
	t.Helper()
	for _, d := range diags {
		if strings.EqualFold(d.Code, code) {
			return
		}
	}
	t.Errorf("expected a diagnostic with code %q but got: %v", code, diags)
}
