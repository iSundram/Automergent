package components

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxHistoryMem is the maximum number of entries kept in memory.
	maxHistoryMem = 100
	// maxHistoryDisk is the maximum number of entries persisted to disk.
	maxHistoryDisk = 1000
)

// loadHistory reads history entries from path. Missing files are silently
// ignored. Each non-empty line is one entry (oldest first).
func loadHistory(path string) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			// Unescape newlines that were encoded during write.
			entries = append(entries, strings.ReplaceAll(line, "\\n", "\n"))
		}
	}
	// Keep only the most-recent maxHistoryMem in memory.
	if len(entries) > maxHistoryMem {
		entries = entries[len(entries)-maxHistoryMem:]
	}
	return entries
}

// appendHistory appends val to the history file at path, capping the file at
// maxHistoryDisk entries. It deduplicates the new entry against the last
// written line on disk. A best-effort write; errors are silently swallowed so
// a read-only filesystem never crashes the UI.
func appendHistory(path, val string) {
	if path == "" || val == "" {
		return
	}

	// Make sure the directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}

	// Read existing entries (full, for rewrite).
	existing := loadHistoryFull(path)

	// Deduplicate consecutive tail.
	if len(existing) > 0 && existing[len(existing)-1] == val {
		return
	}

	existing = append(existing, val)

	// Trim to disk limit.
	if len(existing) > maxHistoryDisk {
		existing = existing[len(existing)-maxHistoryDisk:]
	}

	// Write atomically via a temp file.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range existing {
		// Escape embedded newlines so every history entry is exactly one line.
		_, _ = w.WriteString(strings.ReplaceAll(e, "\n", "\\n") + "\n")
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return
	}
	f.Close()
	os.Rename(tmp, path)
}

// loadHistoryFull reads all entries from path without the memory cap, used
// internally when rewriting the file.
func loadHistoryFull(path string) []string {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			entries = append(entries, strings.ReplaceAll(line, "\\n", "\n"))
		}
	}
	return entries
}
