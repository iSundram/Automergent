package app

import (
	"github.com/iSundram/Automergent/internal/tui/render"
	"github.com/iSundram/Automergent/internal/tui/themes"
	"regexp"
	"strings"
	"testing"
)

func TestSmokeThemedDiff(t *testing.T) {
	render.SetTheme(themes.Get("dracula"))
	d := computeSimpleDiff("main.go", "func a() {\n\tfmt.Println(\"old\")\n}\n", "func a() {\n\tfmt.Println(\"new world\")\n}\n")
	out := render.DiffWithWidth(d, 60)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI styling in diff output")
	}
	plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")
	for _, want := range []string{"old", "new world", "func a()", "@@ -1,3 +1,3 @@"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("diff missing %q in:\n%s", want, plain)
		}
	}
}
