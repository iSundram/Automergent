package planning

import (
	"fmt"
	"time"
)

// RequestType classifies the incoming work request.
type RequestType string

const (
	RequestTypeFeature       RequestType = "feature"
	RequestTypeBugFix        RequestType = "bug_fix"
	RequestTypeRefactor      RequestType = "refactor"
	RequestTypeDocumentation RequestType = "documentation"
	RequestTypeTest          RequestType = "test"
	RequestTypeInvestigation RequestType = "investigation"
	RequestTypeMultiFile     RequestType = "multi_file"
)

// Scope describes the breadth of work.
type Scope string

const (
	ScopeSingleFile  Scope = "single_file"
	ScopeMultiFile   Scope = "multi_file"
	ScopeProjectWide Scope = "project_wide"
)

// Complexity estimates how much work is likely needed.
type Complexity string

const (
	ComplexityTrivial  Complexity = "trivial"
	ComplexitySimple   Complexity = "simple"
	ComplexityModerate Complexity = "moderate"
	ComplexityComplex  Complexity = "complex"
	ComplexityMajor    Complexity = "major"
)

// RequestAnalysis is the structured interpretation of a user request.
type RequestAnalysis struct {
	RawRequest     string
	Intent         string
	RequestType    RequestType
	Scope          Scope
	Complexity     Complexity
	Keywords       []string
	ExplicitFiles  []string
	Risks          []string
	Assumptions    []string
	Confidence     float64
	ContextSignals []ContextSignal
	AnalyzedAt     time.Time
}

// FileCandidate is a discovered file that may matter to the request.
type FileCandidate struct {
	Path      string
	Reason    string
	Score     float64
	DependsOn []string
	Required  bool
	Freshness float64
	Staleness string
}

// ContextSignal mirrors the context layer’s selection signals for planning.
type ContextSignal struct {
	Path            string
	Score           float64
	Required        bool
	DependencyDepth int
	Freshness       float64
	Staleness       string
	Modified        bool
}

// StepStatus tracks planning state.
type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepBlocked    StepStatus = "blocked"
	StepComplete   StepStatus = "complete"
	StepFailed     StepStatus = "failed"
)

// PlanStep describes one ordered unit of work.
type PlanStep struct {
	ID           string
	Title        string
	Description  string
	Files        []string
	DependsOn    []string
	Parallel     bool
	Priority     int
	Estimated    time.Duration
	Verification []string
	Status       StepStatus
	ReplanReason string
}

// Plan is the full dependency-aware plan.
type Plan struct {
	ID             string
	Analysis       RequestAnalysis
	Files          []FileCandidate
	Steps          []PlanStep
	ExecutionOrder [][]string
	ReplanCount    int
	Notes          []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Summary returns a compact human-readable representation.
func (p *Plan) Summary() string {
	if p == nil {
		return ""
	}
	out := "Plan:\n"
	out += "Intent: " + p.Analysis.Intent + "\n"
	out += "Files: "
	if len(p.Files) == 0 {
		out += "none\n"
	} else {
		out += p.Files[0].Path
		for _, f := range p.Files[1:] {
			out += ", " + f.Path
		}
		out += "\n"
	}
	for i, phase := range p.ExecutionOrder {
		out += "Phase " + fmtInt(i+1) + ": "
		for j, id := range phase {
			if j > 0 {
				out += ", "
			}
			out += id
		}
		out += "\n"
	}
	return out
}

func fmtInt(n int) string {
	return fmt.Sprintf("%d", n)
}
