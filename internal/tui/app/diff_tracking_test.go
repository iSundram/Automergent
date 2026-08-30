package app

import (
	"os"
	"path/filepath"
	"testing"
)

// snapshotFileWrite + openDiffTabForCompletedWrite back the diff pane for
// writes that never asked for confirmation (accept-edits/auto, grants) and
// for newly created files. These tests pin the tracking contract directly.

func TestDiffTabOpensForNewFileWrite(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "added.go") // does not exist yet

	app.snapshotFileWrite("t1", "write_file", map[string]any{"path": path})
	if _, ok := app.fileWriteSnapshots["t1"]; !ok {
		t.Fatal("write_file with a path must snapshot")
	}

	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.openDiffTabForCompletedWrite("t1", "write_file", false)

	if app.diffPane.TabCount() != 1 {
		t.Fatalf("new file write should open a diff tab, got %d", app.diffPane.TabCount())
	}
	if app.diffPane.ActiveLabel() != path {
		t.Fatalf("active tab should be the written file, got %q", app.diffPane.ActiveLabel())
	}
}

func TestDiffTabOpensForUnconfirmedEdit(t *testing.T) {
	app := newTestApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.go")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app.snapshotFileWrite("t2", "edit_file", map[string]any{"path": path})
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.openDiffTabForCompletedWrite("t2", "edit_file", false)

	if app.diffPane.TabCount() != 1 {
		t.Fatalf("edit without confirmation should still open a diff tab, got %d", app.diffPane.TabCount())
	}
}

func TestFailedWriteConsumesSnapshotWithoutTab(t *testing.T) {
	app := newTestApp(t)
	app.snapshotFileWrite("t3", "write_file", map[string]any{"path": "x.go"})
	app.openDiffTabForCompletedWrite("t3", "write_file", true)

	if app.diffPane.TabCount() != 0 {
		t.Fatalf("failed write must not open a tab, got %d", app.diffPane.TabCount())
	}
	if _, ok := app.fileWriteSnapshots["t3"]; ok {
		t.Fatal("failed write must still consume its snapshot")
	}
}

func TestNonWriteToolsAndUnchangedFilesIgnored(t *testing.T) {
	app := newTestApp(t)
	app.snapshotFileWrite("t4", "read_file", map[string]any{"path": "x.go"})
	if _, ok := app.fileWriteSnapshots["t4"]; ok {
		t.Fatal("read tools must not snapshot")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "same.go")
	if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app.snapshotFileWrite("t5", "write_file", map[string]any{"path": path})
	app.openDiffTabForCompletedWrite("t5", "write_file", false)
	if app.diffPane.TabCount() != 0 {
		t.Fatalf("unchanged file must not open a tab, got %d", app.diffPane.TabCount())
	}
}
