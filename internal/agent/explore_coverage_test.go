package agent

import (
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/shared"
)

func specWithFiles(files []string) shared.TaskSpec {
	return shared.TaskSpec{
		Type:        "explore",
		Phase:       shared.PhaseExplore,
		Description: "explore the codebase",
		Context:     map[string]any{"files_found": files},
	}
}

func readCallMsg(path string) ai.Message {
	return ai.Message{
		Role: ai.RoleAssistant,
		Content: []ai.ContentPart{{
			Type:     ai.ContentTypeToolCall,
			ToolCall: &ai.ToolCall{ID: "c1", Name: "read_file", Args: map[string]any{"path": path}},
		}},
	}
}

// The regression from the field report: 24 files discovered, ONE read
// (manager.go L1-50) — the old gate let the exit through; it must nudge.
func TestExploreCoverageOneReadOfManyFails(t *testing.T) {
	ag := &Agent{sess: newTestSession()}
	files := make([]string, 24)
	for i := range files {
		files[i] = "internal/context/file" + string(rune('a'+i)) + ".go"
	}
	spec := specWithFiles(files)
	// The field report: the model read ONE of the discovered files (the
	// map's manager.go), 50 lines of it, then answered.
	ag.sess.SetMessages([]ai.Message{readCallMsg(files[0])})

	if !ag.phaseExploreUndercovered(spec) {
		t.Fatalf("1 read of 24 discovered files must count as under-covered")
	}
	read, total, unread := ag.exploreCoverage(spec)
	if read != 1 || total != 24 || len(unread) != 23 {
		t.Fatalf("coverage = %d/%d, %d unread; want 1/24, 23 unread", read, total, len(unread))
	}
}

func TestExploreCoverageQuarterReadPasses(t *testing.T) {
	ag := &Agent{sess: newTestSession()}
	files := make([]string, 24)
	var msgs []ai.Message
	for i := range files {
		files[i] = "internal/context/file" + string(rune('a'+i)) + ".go"
		if i < 6 { // quarter of 24
			msgs = append(msgs, readCallMsg(files[i]))
		}
	}
	ag.sess.SetMessages(msgs)
	if ag.phaseExploreUndercovered(specWithFiles(files)) {
		t.Fatalf("6 reads of 24 files (>= quarter) should pass the gate")
	}
}

func TestExploreCoverageBasenameMatch(t *testing.T) {
	ag := &Agent{sess: newTestSession()}
	// Init glob carries bare names; reads use full paths.
	ag.sess.SetMessages([]ai.Message{readCallMsg("internal/context/budget.go")})
	spec := specWithFiles([]string{"budget.go", "ranking.go"})
	read, total, unread := ag.exploreCoverage(spec)
	if read != 1 || total != 2 || len(unread) != 1 {
		t.Fatalf("basename match failed: %d/%d read, unread=%v", read, total, unread)
	}
}

func TestExploreCoverageTinyMapOneReadPasses(t *testing.T) {
	ag := &Agent{sess: newTestSession()}
	ag.sess.SetMessages([]ai.Message{readCallMsg("plan.go")})
	if ag.phaseExploreUndercovered(specWithFiles([]string{"plan.go", "core.go"})) {
		t.Fatalf("tiny map: reading the key file should suffice")
	}
}
