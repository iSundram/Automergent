package components

import (
	"strings"
)

// Page is the structured content model for full-page command output. Command
// view builders (cmd_*_view.go) produce Pages as pure functions of the Host;
// the PageViewer renders them. Plain-text output keeps using FullPage — Page
// is the richer sibling for commands with sections, status flags and actions.

// PageStatus marks a flagged row as passing, warn-worthy or failed.
type PageStatus int

const (
	PageStatusOK   PageStatus = iota // ✓
	PageStatusWarn                   // !
	PageStatusFail                   // ✗
)

// PageSection is one titled block of a page. Any combination of Lines, Rows
// and Flagged entries may be present; the viewer renders them in that order.
type PageSection struct {
	Heading string
	Lines   []string
	// Rows are key/value pairs rendered as "key: value" with aligned keys.
	Rows [][2]string
	// Flagged entries render with a ✓ / ! / ✗ marker and status colouring.
	Flagged []PageFlag
}

// PageFlag is a status-marked line (health checks, validation results, ...).
type PageFlag struct {
	Label  string
	Detail string
	Status PageStatus
}

// PageAction is a keyboard shortcut that dispatches another slash command.
// Pressing Key while the page is open runs Command with Args — this is how
// pages wire commands into each other.
type PageAction struct {
	Key     string // single lowercase letter, e.g. "i"
	Label   string // shown in the action bar, e.g. "Init project"
	Command string // command name without the leading slash
	Args    []string
}

// Page is a full page of command output.
type Page struct {
	Title    string
	Subtitle string
	Sections []PageSection
	Actions  []PageAction
}

// Row is a convenience constructor for a key/value row.
func Row(key, value string) [2]string { return [2]string{key, value} }

// Lines converts a page into flat, renderable text lines (used by tests and
// by the plain FullPage fallback).
func (p Page) Lines(width int) []string {
	var out []string
	for _, sec := range p.Sections {
		if sec.Heading != "" {
			out = append(out, strings.ToUpper(sec.Heading))
		}
		for _, line := range sec.Lines {
			out = append(out, "  "+line)
		}
		keyW := 0
		for _, row := range sec.Rows {
			if len(row[0]) > keyW {
				keyW = len(row[0])
			}
		}
		for _, row := range sec.Rows {
			pad := strings.Repeat(" ", max(0, keyW-len(row[0])))
			out = append(out, "  "+row[0]+pad+"  "+row[1])
		}
		for _, flag := range sec.Flagged {
			marker := "✓"
			switch flag.Status {
			case PageStatusWarn:
				marker = "!"
			case PageStatusFail:
				marker = "✗"
			}
			line := "  " + marker + " " + flag.Label
			if flag.Detail != "" {
				line += ": " + flag.Detail
			}
			out = append(out, line)
		}
	}
	return out
}
