package tui

import "testing"

func TestSlashCommandRegistryNamesAndAliasesAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, command := range slashCommands {
		if command.Name == "" || command.Category == "" || command.Description == "" {
			t.Fatalf("incomplete command definition: %#v", command)
		}
		for _, name := range append([]string{command.Name}, command.Aliases...) {
			if owner, exists := seen[name]; exists {
				t.Fatalf("command name or alias %q belongs to both %q and %q", name, owner, command.Name)
			}
			seen[name] = command.Name
		}
	}
}

func TestLookupSlashCommandResolvesAliases(t *testing.T) {
	wants := map[string]string{
		"files": "tree", "changes": "diff", "diagnostics": "lsp",
		"tokens": "context", "stop": "cancel", "exit": "quit",
	}
	for alias, want := range wants {
		got, ok := lookupSlashCommand(alias)
		if !ok || got.Name != want {
			t.Fatalf("lookupSlashCommand(%q) = %#v, %v; want %q", alias, got, ok, want)
		}
	}
}

func TestCommandPaletteItemsExposeUsageAndSearchTerms(t *testing.T) {
	items := commandPaletteItems()
	if len(items) != len(slashCommands) {
		t.Fatalf("palette has %d commands, registry has %d", len(items), len(slashCommands))
	}
	foundRun := false
	for _, item := range items {
		if item.Value == "run" {
			foundRun = item.Hint == "<command>" && item.SearchTerms != ""
		}
	}
	if !foundRun {
		t.Fatal("run command is missing usage or searchable metadata")
	}
}
