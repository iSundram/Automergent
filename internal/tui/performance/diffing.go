package performance

import (
	"crypto/sha256"
	"strings"
	"sync"
)

// DiffResult represents the result of a diff operation.
type DiffResult struct {
	HasChanges   bool
	ChangedLines []int
	AddedLines   []int
	RemovedLines []int
}

// ContentDiffer performs efficient content diffing to minimize redraws.
type ContentDiffer struct {
	mu           sync.RWMutex
	lastContent  string
	lastHash     [32]byte
	lastLines    []string
	lineHashes   [][32]byte
	changeBuffer []LineChange
}

// LineChange represents a single line change.
type LineChange struct {
	Index   int
	OldLine string
	NewLine string
	Type    ChangeType
}

// ChangeType indicates the type of change.
type ChangeType int

const (
	ChangeNone ChangeType = iota
	ChangeModified
	ChangeAdded
	ChangeRemoved
)

// NewContentDiffer creates a new content differ.
func NewContentDiffer() *ContentDiffer {
	return &ContentDiffer{
		changeBuffer: make([]LineChange, 0, 64),
	}
}

// Diff compares new content against the last known content and returns changes.
func (d *ContentDiffer) Diff(newContent string) DiffResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Quick hash comparison for unchanged content
	newHash := sha256.Sum256([]byte(newContent))
	if d.lastHash == newHash {
		return DiffResult{HasChanges: false}
	}

	result := DiffResult{HasChanges: true}
	newLines := strings.Split(newContent, "\n")

	// First render - no comparison needed
	if d.lastContent == "" {
		d.lastContent = newContent
		d.lastHash = newHash
		d.lastLines = newLines
		d.lineHashes = computeLineHashes(newLines)
		for i := range newLines {
			result.AddedLines = append(result.AddedLines, i)
		}
		return result
	}

	// Compute per-line changes
	newLineHashes := computeLineHashes(newLines)
	oldLen := len(d.lastLines)
	newLen := len(newLines)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	// Check modified lines
	for i := 0; i < minLen; i++ {
		if d.lineHashes[i] != newLineHashes[i] {
			result.ChangedLines = append(result.ChangedLines, i)
		}
	}

	// Added lines
	for i := oldLen; i < newLen; i++ {
		result.AddedLines = append(result.AddedLines, i)
	}

	// Removed lines
	for i := newLen; i < oldLen; i++ {
		result.RemovedLines = append(result.RemovedLines, i)
	}

	// Update state
	d.lastContent = newContent
	d.lastHash = newHash
	d.lastLines = newLines
	d.lineHashes = newLineHashes

	return result
}

// GetChanges returns detailed line changes for incremental rendering.
func (d *ContentDiffer) GetChanges(newContent string) []LineChange {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.changeBuffer = d.changeBuffer[:0]
	newLines := strings.Split(newContent, "\n")

	if d.lastContent == "" {
		for i, line := range newLines {
			d.changeBuffer = append(d.changeBuffer, LineChange{
				Index:   i,
				NewLine: line,
				Type:    ChangeAdded,
			})
		}
		d.lastContent = newContent
		d.lastLines = newLines
		d.lastHash = sha256.Sum256([]byte(newContent))
		d.lineHashes = computeLineHashes(newLines)
		return d.changeBuffer
	}

	newLineHashes := computeLineHashes(newLines)
	oldLen := len(d.lastLines)
	newLen := len(newLines)
	minLen := oldLen
	if newLen < minLen {
		minLen = newLen
	}

	// Modified lines
	for i := 0; i < minLen; i++ {
		if d.lineHashes[i] != newLineHashes[i] {
			d.changeBuffer = append(d.changeBuffer, LineChange{
				Index:   i,
				OldLine: d.lastLines[i],
				NewLine: newLines[i],
				Type:    ChangeModified,
			})
		}
	}

	// Added lines
	for i := oldLen; i < newLen; i++ {
		d.changeBuffer = append(d.changeBuffer, LineChange{
			Index:   i,
			NewLine: newLines[i],
			Type:    ChangeAdded,
		})
	}

	// Removed lines
	for i := newLen; i < oldLen; i++ {
		d.changeBuffer = append(d.changeBuffer, LineChange{
			Index:   i,
			OldLine: d.lastLines[i],
			Type:    ChangeRemoved,
		})
	}

	// Update state
	d.lastContent = newContent
	d.lastHash = sha256.Sum256([]byte(newContent))
	d.lastLines = newLines
	d.lineHashes = newLineHashes

	return d.changeBuffer
}

// HasChanged returns true if the content would differ from last known state.
func (d *ContentDiffer) HasChanged(newContent string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	newHash := sha256.Sum256([]byte(newContent))
	return d.lastHash != newHash
}

// Reset clears the diff state.
func (d *ContentDiffer) Reset() {
	d.mu.Lock()
	d.lastContent = ""
	d.lastHash = [32]byte{}
	d.lastLines = nil
	d.lineHashes = nil
	d.changeBuffer = d.changeBuffer[:0]
	d.mu.Unlock()
}

// LastLineCount returns the number of lines in the last content.
func (d *ContentDiffer) LastLineCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.lastLines)
}

func computeLineHashes(lines []string) [][32]byte {
	hashes := make([][32]byte, len(lines))
	for i, line := range lines {
		hashes[i] = sha256.Sum256([]byte(line))
	}
	return hashes
}

// BatchDiffer handles diffing for multiple components efficiently.
type BatchDiffer struct {
	mu       sync.RWMutex
	differs  map[string]*ContentDiffer
	dirtySet map[string]bool
}

// NewBatchDiffer creates a batch differ for multiple components.
func NewBatchDiffer() *BatchDiffer {
	return &BatchDiffer{
		differs:  make(map[string]*ContentDiffer),
		dirtySet: make(map[string]bool),
	}
}

// Register registers a component for diffing.
func (b *BatchDiffer) Register(id string) {
	b.mu.Lock()
	if _, exists := b.differs[id]; !exists {
		b.differs[id] = NewContentDiffer()
	}
	b.mu.Unlock()
}

// Update updates content for a component and returns if it changed.
func (b *BatchDiffer) Update(id, content string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	differ, exists := b.differs[id]
	if !exists {
		differ = NewContentDiffer()
		b.differs[id] = differ
	}

	result := differ.Diff(content)
	b.dirtySet[id] = result.HasChanges
	return result.HasChanges
}

// IsDirty returns true if the component needs redraw.
func (b *BatchDiffer) IsDirty(id string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dirtySet[id]
}

// ClearDirty clears the dirty flag for a component.
func (b *BatchDiffer) ClearDirty(id string) {
	b.mu.Lock()
	delete(b.dirtySet, id)
	b.mu.Unlock()
}

// DirtyComponents returns all components that need redraw.
func (b *BatchDiffer) DirtyComponents() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dirty := make([]string, 0, len(b.dirtySet))
	for id, isDirty := range b.dirtySet {
		if isDirty {
			dirty = append(dirty, id)
		}
	}
	return dirty
}

// ClearAll clears all dirty flags.
func (b *BatchDiffer) ClearAll() {
	b.mu.Lock()
	for k := range b.dirtySet {
		delete(b.dirtySet, k)
	}
	b.mu.Unlock()
}
