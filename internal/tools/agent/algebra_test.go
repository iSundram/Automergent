package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAgentDefinition(t *testing.T) {
	content := "---\nname: db-migrator\ndescription: Runs schema migrations\nmodel: gemini-3-flash\ntools: [bash, sql]\n---\n\nYou migrate schemas carefully.\n"
	def := parseAgentDefinition(content)
	if def == nil {
		t.Fatal("expected definition")
	}
	if def.Name != "db-migrator" || def.Model != "gemini-3-flash" || len(def.Tools) != 2 {
		t.Fatalf("bad parse: %+v", def)
	}
	if def.SystemBody != "You migrate schemas carefully." {
		t.Fatalf("body = %q", def.SystemBody)
	}
}

func TestParseAgentDefinitionNoFrontmatter(t *testing.T) {
	if parseAgentDefinition("# just markdown") != nil {
		t.Fatal("expected nil for plain markdown")
	}
}

func TestLoadAgentDefinitions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\n---\nReview hard."), 0o644)
	os.WriteFile(filepath.Join(dir, "not-an-agent.md"), []byte("plain notes"), 0o644)

	names, err := LoadAgentDefinitions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "reviewer" {
		t.Fatalf("loaded %v", names)
	}
	if _, ok := LookupCustomAgent("Reviewer"); !ok {
		t.Fatal("custom agent not registered case-insensitively")
	}
}

func TestRolePreambleForCustom(t *testing.T) {
	RegisterCustomAgent(&customAgentDef{Name: "scout", SystemBody: "Be fast."})
	if got := rolePreambleFor(AgentType("scout")); got == "" {
		t.Fatal("expected preamble for custom type")
	}
	if got := rolePreambleFor(AgentTypeExplore); got != "" {
		t.Fatalf("builtin types must not get custom preamble, got %q", got)
	}
}
