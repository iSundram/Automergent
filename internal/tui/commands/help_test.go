package commands

import (
	"strings"
	"testing"
)

func TestHelpRowsCoverEveryVisibleCommand(t *testing.T) {
	r := Default()
	rows := r.HelpRows()

	seen := map[string]bool{}
	for _, row := range rows {
		name := strings.Fields(row[0])[0]
		seen[name] = true
		if row[1] == "" {
			t.Fatalf("help row %q has empty description", row[0])
		}
	}
	for _, cmd := range r.List() {
		if cmd.Hidden {
			continue
		}
		if !seen["/"+cmd.Name] {
			t.Errorf("command /%s missing from help rows", cmd.Name)
		}
	}
}

func TestHelpRowsMergeAliasesAndHints(t *testing.T) {
	byKey := map[string]string{}
	for _, row := range Default().HelpRows() {
		byKey[strings.Fields(row[0])[0]] = row[0]
	}

	if got := byKey["/diff"]; !strings.Contains(got, "/changes") {
		t.Fatalf("aliases not merged into help key: %q", got)
	}
	if got := byKey["/model"]; !strings.Contains(got, "[name|reset]") {
		t.Fatalf("args hint missing from help key: %q", got)
	}
	if got := byKey["/sessions"]; !strings.Contains(got, "/session") {
		t.Fatalf("session alias missing from help key: %q", got)
	}
}

func TestHelpRowsExcludeHidden(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Command{Name: "shown", Description: "d", Category: "Test", Icon: "x"}, func(h Host, a []string) Result { return Done(nil) })
	r.MustRegister(Command{Name: "hidden", Description: "d", Category: "Test", Icon: "x", Hidden: true}, func(h Host, a []string) Result { return Done(nil) })

	for _, row := range r.HelpRows() {
		if strings.Contains(row[0], "hidden") {
			t.Fatalf("hidden command leaked into help rows: %v", row)
		}
	}
}
