package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/agent/agentdef"
	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
	subagent "github.com/iSundram/Automergent/internal/tools/agent"
)

func TestIsReadOnlyDefinition(t *testing.T) {
	cases := []struct {
		def  *agentdef.AgentDefinition
		want bool
	}{
		{&agentdef.AgentDefinition{Name: "explore"}, true},
		{&agentdef.AgentDefinition{Name: "review"}, true},
		{&agentdef.AgentDefinition{Name: "general-purpose", Tools: nil}, false}, // nil = all tools
		{&agentdef.AgentDefinition{Name: "scanner", Tools: []string{"read_file", "grep", "glob"}}, true},
		{&agentdef.AgentDefinition{Name: "builder", Tools: []string{"read_file", "edit_file"}}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := isReadOnlyDefinition(tc.def); got != tc.want {
			t.Errorf("isReadOnlyDefinition(%v) = %v, want %v", tc.def, got, tc.want)
		}
	}
}

func TestResolveChildModelFastRouting(t *testing.T) {
	ag := &Agent{cfg: &config.Config{Model: "gemini-2.5-pro", FastModel: "gemini-2.5-flash"}}
	explore := &agentdef.AgentDefinition{Name: "explore"}
	general := &agentdef.AgentDefinition{Name: "general-purpose"}
	pinned := &agentdef.AgentDefinition{Name: "explore", Model: "custom-model"}

	cases := []struct {
		explicit string
		def      *agentdef.AgentDefinition
		want     string
	}{
		{"", explore, "gemini-2.5-flash"},    // read-only -> fast model
		{"", general, ""},                     // full agents inherit the parent model
		{"", pinned, "custom-model"},          // definition pin wins
		{"override", explore, "override"},     // explicit call-site override wins
	}
	for _, tc := range cases {
		if got := ag.resolveChildModel(tc.explicit, tc.def); got != tc.want {
			t.Errorf("resolveChildModel(%q, %s) = %q, want %q", tc.explicit, tc.def.Name, got, tc.want)
		}
	}

	// No FastModel configured: read-only agents fall back to the parent model.
	bare := &Agent{cfg: &config.Config{Model: "main"}}
	if got := bare.resolveChildModel("", explore); got != "" {
		t.Errorf("no fast model configured: got %q, want empty", got)
	}
}

func TestReadOnlyChildOmitsProjectContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("project instructions here"), 0o644); err != nil {
		t.Fatal(err)
	}
	ag := &Agent{cfg: &config.Config{}, workDir: dir}
	if _, ok := ag.userContext()["projectInstructions"]; !ok {
		t.Fatal("full agent must receive project instructions")
	}

	ro := &Agent{cfg: &config.Config{}, workDir: dir, omitProjectContext: true}
	ctxMap := ro.userContext()
	if _, ok := ctxMap["projectInstructions"]; ok {
		t.Error("read-only child must not receive project instructions")
	}
	if _, ok := ctxMap["userRules"]; ok {
		t.Error("read-only child must not receive the rules block")
	}
}

func TestForkAndResumeContextTags(t *testing.T) {
	ctx := context.Background()
	if subagent.ResumeAgentIDFrom(ctx) != "" || subagent.ForkContextFrom(ctx) {
		t.Fatal("untagged context must read as no-resume, no-fork")
	}
	ctx = subagent.WithResumeAgentID(ctx, "agent-7")
	ctx = subagent.WithForkContext(ctx)
	if subagent.ResumeAgentIDFrom(ctx) != "agent-7" {
		t.Error("resume ID lost")
	}
	if !subagent.ForkContextFrom(ctx) {
		t.Error("fork flag lost")
	}
}

func TestForkContextMessagesRepaired(t *testing.T) {
	ag := &Agent{cfg: &config.Config{}, sess: session.New()}
	// A dangling tool call (interrupted before its result was recorded).
	ag.sess.AddMessage(ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{{
			Type:     ai.ContentTypeToolCall,
			ToolCall: &ai.ToolCall{ID: "c1", Name: "grep"},
		}},
	})
	forked := ag.forkContextMessages()
	// The repair must synthesize a result so no call dangles.
	pending := map[string]bool{}
	for _, m := range forked {
		for _, tc := range m.ToolCallParts() {
			pending[tc.ID] = true
		}
		for _, p := range m.Content {
			if p.ToolResult != nil {
				delete(pending, p.ToolResult.ToolCallID)
			}
		}
	}
	if len(pending) > 0 {
		t.Fatalf("fork context has dangling calls: %v", pending)
	}
}

func TestAgentMemoryPersistence(t *testing.T) {
	dir := t.TempDir()
	def := &agentdef.AgentDefinition{Name: "contexter", MemoryScope: agentdef.MemoryScopeProject}
	memory := LoadAgentMemory(dir, def)

	if err := memory.Append("auth flows live in internal/auth"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := memory.Append("auth flows live in internal/auth"); err != nil {
		t.Fatalf("duplicate append must be accepted: %v", err)
	}
	if got := memory.Entries(); len(got) != 1 {
		t.Fatalf("duplicate recorded twice: %v", got)
	}

	// A fresh load (next spawn) sees the persisted entries.
	reloaded := LoadAgentMemory(dir, def)
	if got := reloaded.Entries(); len(got) != 1 || got[0] != "auth flows live in internal/auth" {
		t.Fatalf("memory did not persist: %v", got)
	}
	if !strings.Contains(reloaded.Prompt(), "Agent Memory") {
		t.Fatal("memory prompt rendering broken")
	}

	if err := reloaded.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := reloaded.Entries(); len(got) != 0 {
		t.Fatalf("clear left entries: %v", got)
	}
}

func TestAgentMemoryGlobalScope(t *testing.T) {
	// Global scope resolves under the home dir; project under workDir. Test
	// the project path resolution directly and just check global != project.
	dir := t.TempDir()
	project := &agentdef.AgentDefinition{Name: "x", MemoryScope: agentdef.MemoryScopeProject}
	global := &agentdef.AgentDefinition{Name: "x", MemoryScope: agentdef.MemoryScopeGlobal}
	if memoryPath(dir, project) == memoryPath(dir, global) {
		t.Fatal("project and global memory must resolve to different files")
	}
	if !strings.Contains(memoryPath(dir, project), filepath.Join(dir, ".automergent", "agent-memory")) {
		t.Fatalf("project memory path = %q", memoryPath(dir, project))
	}
}

func TestSidechainTranscriptPath(t *testing.T) {
	dir := t.TempDir()
	child := &Agent{cfg: &config.Config{}, workDir: dir}
	child.setSidechainTranscript("agent-12")
	tm := child.ContextManager().TranscriptManager()
	if tm == nil {
		t.Fatal("sidechain transcript not installed")
	}
	path := filepath.Join(dir, ".automergent", "subagents", "agent-12.jsonl")
	if _, err := os.Stat(path); err != nil {
		// File is created lazily on first append; the directory must exist.
		if _, derr := os.Stat(filepath.Dir(path)); derr != nil {
			t.Fatalf("sidechain dir missing: %v", derr)
		}
	}
}

func TestTrackAndResumeChild(t *testing.T) {
	ag := &Agent{cfg: &config.Config{}}
	child := &Agent{cfg: &config.Config{}}
	ag.trackChild("agent-1", child)
	if got, ok := ag.resumeChild("agent-1"); !ok || got != child {
		t.Fatal("tracked child not resumable")
	}
	if _, ok := ag.resumeChild("agent-404"); ok {
		t.Fatal("unknown child must not resolve")
	}
}
