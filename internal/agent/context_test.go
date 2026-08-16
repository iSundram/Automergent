package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/cache"
	"github.com/iSundram/Automergent/internal/config"
	contextmgr "github.com/iSundram/Automergent/internal/context"
)

func TestAssemblePromptSectionsOrderAndBoundary(t *testing.T) {
	assembled := assemblePromptSections([]promptSection{
		{name: "identity", content: "# Identity\nStatic", classifyAs: cache.ClassificationStatic},
		{name: "protocol", content: "# Protocol\nAlso static", classifyAs: cache.ClassificationSemiStatic},
		{name: "cache-break", isCacheBreak: true},
		{name: "project", content: "# Project Context\nDynamic", classifyAs: cache.ClassificationDynamic},
	})

	if !strings.Contains(assembled, cache.DynamicBoundaryMarker) {
		t.Fatalf("expected dynamic boundary marker in assembled prompt")
	}

	staticPart, dynamicPart := cache.SplitSystemPrompt(assembled)
	if !strings.Contains(staticPart, "# Identity") || !strings.Contains(staticPart, "# Protocol") {
		t.Fatalf("expected static sections in static prompt part: %q", staticPart)
	}
	if strings.Contains(staticPart, "# Project Context") {
		t.Fatalf("did not expect dynamic section in static part")
	}
	if !strings.Contains(dynamicPart, "# Project Context") {
		t.Fatalf("expected dynamic section in dynamic part: %q", dynamicPart)
	}
}

func TestRenderManagedContextSelectionRespectsFileLimit(t *testing.T) {
	dir := t.TempDir()
	files := make([]string, 0, maxPromptContextFiles+1)
	for i := 0; i < maxPromptContextFiles+1; i++ {
		path := filepath.Join(dir, "file"+string(rune('A'+i))+".txt")
		content := "context " + strings.Repeat("data ", 300)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write test file: %v", err)
		}
		files = append(files, path)
	}

	cfg := config.Default()
	cfg.ContextFiles = files
	cm := contextmgr.NewManager(dir, contextmgr.DefaultManagerConfig())

	var sb strings.Builder
	renderManagedContextSelection(&sb, cfg, []ai.Message{ai.NewTextMessage(ai.RoleUser, "inspect context")}, dir, cm)
	rendered := sb.String()
	if rendered == "" {
		t.Fatalf("expected managed context rendering")
	}
	if got := strings.Count(rendered, "```text"); got != maxPromptContextFiles {
		t.Fatalf("expected %d context blocks, got %d\n%s", maxPromptContextFiles, got, rendered)
	}
}

func TestTruncateContextContentLimit(t *testing.T) {
	content := strings.Repeat("x", maxPromptContextChars+200)
	truncated := truncateContextContent(content, maxPromptContextChars)
	if len(truncated) > maxPromptContextChars+len("\n... [truncated] ...\n") {
		t.Fatalf("truncated content exceeded expected limit: %d", len(truncated))
	}
	if !strings.Contains(truncated, "... [truncated] ...") {
		t.Fatalf("expected truncation marker in content")
	}
}

func TestDetectPhaseResearchFromToolResultMappedByCallID(t *testing.T) {
	messages := []ai.Message{
		assistantToolCall("call_123", "grep"),
		toolResult("call_123", nil),
	}

	if got := DetectPhase(messages); got != PhasePlan {
		t.Fatalf("expected %q, got %q", PhasePlan, got)
	}
}

func TestDetectPhaseIgnoresToolCallIDStringHeuristics(t *testing.T) {
	messages := []ai.Message{
		assistantToolCall("read_like_id", "edit_file"),
		toolResult("read_like_id", nil),
	}

	if got := DetectPhase(messages); got != PhaseResearch {
		t.Fatalf("expected %q, got %q", PhaseResearch, got)
	}
}

func TestDetectPhaseResearchFromToolMetadataFallback(t *testing.T) {
	messages := []ai.Message{
		toolResult("missing_call", map[string]any{"tool_name": "glob"}),
	}

	if got := DetectPhase(messages); got != PhasePlan {
		t.Fatalf("expected %q, got %q", PhasePlan, got)
	}
}

func TestDetectPhaseExecuteRequiresPlanFile(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	messages := []ai.Message{
		assistantToolCall("call_42", "read_file"),
		toolResult("call_42", nil),
	}
	if got := DetectPhase(messages); got != PhasePlan {
		t.Fatalf("expected %q without plan file, got %q", PhasePlan, got)
	}

	planPath := filepath.Join(dir, "AUTOMERGENT_PLAN.md")
	if err := os.WriteFile(planPath, []byte("# plan"), 0o600); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	if got := DetectPhase(messages); got != PhaseExecute {
		t.Fatalf("expected %q with plan file, got %q", PhaseExecute, got)
	}
}

func assistantToolCall(id, name string) ai.Message {
	return ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{
			{
				Type: ai.ContentTypeToolCall,
				ToolCall: &ai.ToolCall{
					ID:   id,
					Name: name,
					Args: map[string]any{},
				},
			},
		},
	}
}

func toolResult(callID string, metadata map[string]any) ai.Message {
	return ai.Message{
		Role:     ai.RoleTool,
		Metadata: metadata,
		Content: []ai.ContentPart{
			{
				Type: ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{
					ToolCallID: callID,
					Content:    "ok",
				},
			},
		},
	}
}
