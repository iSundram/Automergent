package app

// Diff-tab tracking for file-writing tools.
//
// The confirm flow (EventConfirm) opens a diff tab pre-execution, but only
// when approval is actually asked for: in accept-edits/auto mode, or after an
// always-allow grant, writes never confirm — and multi_edit never did — so
// those files (including brand-new ones) never reached the diff pane. These
// helpers snapshot the file before the write and open the tab afterwards, so
// every touched file lands in the strip regardless of the approval path.

import (
	"os"

	"github.com/iSundram/Automergent/internal/tools"
)

// fileWriteSnapshot is the pre-write state of one file, keyed by tool ID.
type fileWriteSnapshot struct {
	path   string
	before string
}

// fileWriteTools lists the tools whose effect on a file is worth a diff tab.
var fileWriteTools = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"multi_edit": true,
}

// snapshotFileWrite records the on-disk content of the file a write tool is
// about to touch. A missing file snapshots as "" so a newly created file
// diffs as a pure addition.
func (a *App) snapshotFileWrite(toolID, toolName string, args map[string]any) {
	if !fileWriteTools[toolName] || args == nil {
		return
	}
	path, ok := tools.StringArg(args, "path")
	if !ok || path == "" {
		return
	}
	data, _ := os.ReadFile(path) // absent file = new file = empty "before"
	a.fileWriteSnapshots[toolID] = fileWriteSnapshot{path: path, before: string(data)}
}

// openDiffTabForCompletedWrite diffs the post-write file against its snapshot
// and opens (or refreshes) its diff tab. The snapshot is always consumed so
// cancelled runs cannot leak entries.
func (a *App) openDiffTabForCompletedWrite(toolID, toolName string, failed bool) {
	snap, ok := a.fileWriteSnapshots[toolID]
	if ok {
		delete(a.fileWriteSnapshots, toolID)
	}
	if !ok || failed {
		return
	}
	after, err := os.ReadFile(snap.path)
	if err != nil {
		after = nil // write claimed success but the file is unreadable — show all-add
	}
	afterStr := string(after)
	if afterStr == snap.before {
		return // nothing actually changed (an all-context diff is noise)
	}
	diff := computeSimpleDiff(snap.path, snap.before, afterStr)
	if diff == "" {
		return
	}
	a.diffPane.OpenFile(snap.path, diff)
}
