package lsp

// Severity maps LSP diagnostic severity numbers to strings.
type Severity int

const (
	SeverityError       Severity = 1
	SeverityWarning     Severity = 2
	SeverityInformation Severity = 3
	SeverityHint        Severity = 4
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Position is a line/character pair (0-indexed).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a start/end position pair.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Diagnostic holds a single LSP diagnostic.
type Diagnostic struct {
	Range    Range    `json:"range"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Source   string   `json:"source"`
	Code     string   `json:"code"`
}
