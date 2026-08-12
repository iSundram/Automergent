package planning

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Planner turns user requests into dependency-aware execution plans.
type Planner struct {
	rootDir string
}

// NewPlanner creates a planner rooted at the current working directory.
func NewPlanner(rootDir string) *Planner {
	if rootDir == "" {
		rootDir = "."
	}
	return &Planner{rootDir: rootDir}
}

// AnalyzeRequest derives structured meaning from a user request.
func (p *Planner) AnalyzeRequest(request string) RequestAnalysis {
	lower := strings.ToLower(request)
	keywords := extractKeywords(lower)
	files := extractFileMentions(request)

	analysis := RequestAnalysis{
		RawRequest:    request,
		Intent:        trimIntent(request),
		RequestType:   classifyRequest(lower),
		Scope:         classifyScope(lower, files),
		Complexity:    classifyComplexity(lower, files),
		Keywords:      keywords,
		ExplicitFiles: files,
		Risks:         inferRisks(lower),
		Assumptions:   inferAssumptions(lower),
		Confidence:    confidenceScore(lower, files, keywords),
		AnalyzedAt:    time.Now(),
	}
	return analysis
}

func trimIntent(request string) string {
	request = strings.TrimSpace(request)
	if request == "" {
		return request
	}
	if idx := strings.IndexAny(request, ".!?"); idx > 0 {
		return strings.TrimSpace(request[:idx])
	}
	if len(request) > 120 {
		return strings.TrimSpace(request[:120]) + "..."
	}
	return request
}

func extractKeywords(request string) []string {
	raw := strings.FieldsFunc(request, func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	stop := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"this": true, "that": true, "into": true, "your": true, "need": true,
		"make": true, "add": true, "create": true, "update": true, "fix": true,
	}
	for _, w := range raw {
		if len(w) < 3 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

func extractFileMentions(request string) []string {
	fields := strings.Fields(request)
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, f := range fields {
		clean := strings.Trim(f, ".,;:()[]{}<>\"'`")
		if strings.Contains(clean, "/") && strings.Contains(clean, ".") {
			if !seen[clean] {
				seen[clean] = true
				out = append(out, clean)
			}
			continue
		}
		if strings.Contains(clean, ".go") || strings.Contains(clean, ".ts") || strings.Contains(clean, ".py") || strings.Contains(clean, ".md") || strings.Contains(clean, ".json") || strings.Contains(clean, ".yaml") || strings.Contains(clean, ".yml") {
			if !seen[clean] {
				seen[clean] = true
				out = append(out, clean)
			}
		}
	}
	return out
}

func classifyRequest(request string) RequestType {
	switch {
	case containsAny(request, []string{"fix", "bug", "broken", "error", "fails"}):
		return RequestTypeBugFix
	case containsAny(request, []string{"test", "coverage", "spec"}):
		return RequestTypeTest
	case containsAny(request, []string{"doc", "readme", "documentation", "explain"}):
		return RequestTypeDocumentation
	case containsAny(request, []string{"refactor", "restructure", "simplify", "cleanup", "clean up"}):
		return RequestTypeRefactor
	case containsAny(request, []string{"investigate", "analyze", "understand", "why", "how"}):
		return RequestTypeInvestigation
	case containsAny(request, []string{"across files", "multiple files", "project-wide", "all files"}):
		return RequestTypeMultiFile
	default:
		return RequestTypeFeature
	}
}

func classifyScope(request string, files []string) Scope {
	switch {
	case len(files) > 1:
		return ScopeMultiFile
	case containsAny(request, []string{"project-wide", "codebase", "entire repo", "whole repo", "all files"}):
		return ScopeProjectWide
	case strings.Contains(request, "multiple files"):
		return ScopeMultiFile
	default:
		return ScopeSingleFile
	}
}

func classifyComplexity(request string, files []string) Complexity {
	switch {
	case len(files) > 3 || containsAny(request, []string{"architecture", "dependency", "orchestrate", "replan"}):
		return ComplexityComplex
	case len(files) > 1:
		return ComplexityModerate
	case containsAny(request, []string{"simple", "small", "minor", "quick"}):
		return ComplexitySimple
	default:
		return ComplexityModerate
	}
}

func inferRisks(request string) []string {
	risks := make([]string, 0, 3)
	if containsAny(request, []string{"refactor", "restructure"}) {
		risks = append(risks, "behavior regressions during refactor")
	}
	if containsAny(request, []string{"test", "verification"}) {
		risks = append(risks, "verification may expose hidden failures")
	}
	if containsAny(request, []string{"project-wide", "all files", "codebase"}) {
		risks = append(risks, "broad blast radius")
	}
	return risks
}

func inferAssumptions(request string) []string {
	assumptions := []string{"workspace is accessible", "requested files exist or can be discovered"}
	if containsAny(request, []string{"plan", "replan"}) {
		assumptions = append(assumptions, "plan can be revised after feedback")
	}
	return assumptions
}

func confidenceScore(request string, files []string, keywords []string) float64 {
	score := 0.35
	score += float64(len(files)) * 0.15
	score += float64(len(keywords)) * 0.03
	if containsAny(request, []string{"explicit", "file", "path"}) {
		score += 0.1
	}
	if score > 1 {
		score = 1
	}
	return score
}

// DiscoverFiles finds files relevant to the request.
func (p *Planner) DiscoverFiles(ctx context.Context, analysis RequestAnalysis) ([]FileCandidate, error) {
	seen := map[string]bool{}
	candidates := make([]FileCandidate, 0)

	add := func(path, reason string, score float64, required bool) {
		if path == "" || seen[path] {
			return
		}
		if _, err := os.Stat(path); err != nil {
			return
		}
		seen[path] = true
		candidates = append(candidates, FileCandidate{Path: path, Reason: reason, Score: score, Required: required})
	}

	for _, explicit := range analysis.ExplicitFiles {
		candidates = append(candidates, FileCandidate{
			Path:     explicit,
			Reason:   "explicit file mention",
			Score:    1,
			Required: true,
		})
		seen[explicit] = true
	}

	err := filepath.WalkDir(p.rootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "build", "bin", ".cache":
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && path != p.rootDir {
				return filepath.SkipDir
			}
			return nil
		}
		score, reason := scoreFile(path, analysis)
		if score > 0 {
			add(path, reason, score, false)
		}
		return nil
	})
	if err != nil && err != context.Canceled {
		return nil, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > 12 {
		candidates = candidates[:12]
	}
	return candidates, nil
}

// GeneratePlan builds an execution plan from a user request.
func (p *Planner) GeneratePlan(ctx context.Context, request string) (*Plan, error) {
	analysis := p.AnalyzeRequest(request)
	return p.GeneratePlanWithAnalysis(ctx, analysis)
}

// GeneratePlanWithAnalysis builds an execution plan from a structured analysis.
func (p *Planner) GeneratePlanWithAnalysis(ctx context.Context, analysis RequestAnalysis) (*Plan, error) {
	files, err := p.DiscoverFiles(ctx, analysis)
	if err != nil {
		return nil, err
	}
	return p.buildPlan(analysis, files, 0, nil), nil
}

// Replan updates an existing plan using new feedback.
func (p *Planner) Replan(ctx context.Context, existing *Plan, feedback string) (*Plan, error) {
	if existing == nil {
		return p.GeneratePlan(ctx, feedback)
	}
	analysis := p.AnalyzeRequest(existing.Analysis.RawRequest + " " + feedback)
	files, err := p.DiscoverFiles(ctx, analysis)
	if err != nil {
		return nil, err
	}
	return p.buildPlan(analysis, files, existing.ReplanCount+1, existing), nil
}

func (p *Planner) buildPlan(analysis RequestAnalysis, files []FileCandidate, replans int, previous *Plan) *Plan {
	steps := make([]PlanStep, 0)
	order := make([][]string, 0)
	stepByFile := map[string]string{}

	if len(files) == 0 {
		steps = append(steps, PlanStep{
			ID:           "step-1",
			Title:        "Investigate request",
			Description:  analysis.Intent,
			Priority:     10,
			Estimated:    estimateDuration(analysis.Complexity),
			Verification: []string{"confirm scope"},
			Status:       StepPending,
		})
		return &Plan{
			ID:             fmt.Sprintf("plan-%d", time.Now().UnixNano()),
			Analysis:       analysis,
			Files:          files,
			Steps:          steps,
			ExecutionOrder: [][]string{{"step-1"}},
			ReplanCount:    replans,
			Notes:          mergeNotes(previous, analysis, files),
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	}

	for i, file := range files {
		id := fmt.Sprintf("step-%d", i+1)
		stepByFile[file.Path] = id
		desc := fmt.Sprintf("%s (%s)", file.Reason, file.Path)
		if file.Staleness != "" {
			desc += " [" + file.Staleness + "]"
		}
		steps = append(steps, PlanStep{
			ID:           id,
			Title:        stepTitle(analysis.RequestType, file.Path),
			Description:  desc,
			Files:        []string{file.Path},
			Priority:     priorityFor(analysis, file),
			Estimated:    estimateDuration(analysis.Complexity),
			Verification: verificationFor(analysis.RequestType, file.Path),
			Status:       StepPending,
		})
	}

	deps := inferDependencies(files)
	for i := range steps {
		for _, dep := range deps[steps[i].Files[0]] {
			if depID, ok := stepByFile[dep]; ok && depID != steps[i].ID {
				steps[i].DependsOn = appendUnique(steps[i].DependsOn, depID)
			}
		}
	}

	steps = addVerificationSteps(analysis, steps)
	order = buildExecutionOrder(steps)
	for i := range steps {
		if len(steps[i].DependsOn) == 0 {
			steps[i].Parallel = true
		}
	}

	for _, sig := range analysis.ContextSignals {
		for i := range steps {
			if len(steps[i].Files) > 0 && steps[i].Files[0] == sig.Path {
				if sig.Required {
					steps[i].Priority += 20
				}
				if sig.Staleness != "" {
					steps[i].ReplanReason = sig.Staleness
				}
			}
		}
	}

	return &Plan{
		ID:             fmt.Sprintf("plan-%d", time.Now().UnixNano()),
		Analysis:       analysis,
		Files:          files,
		Steps:          steps,
		ExecutionOrder: order,
		ReplanCount:    replans,
		Notes:          mergeNotes(previous, analysis, files),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func stepTitle(requestType RequestType, path string) string {
	base := filepath.Base(path)
	switch requestType {
	case RequestTypeTest:
		return "Validate " + base
	case RequestTypeDocumentation:
		return "Document " + base
	case RequestTypeRefactor:
		return "Refine " + base
	case RequestTypeBugFix:
		return "Fix " + base
	default:
		return "Inspect " + base
	}
}

func priorityFor(analysis RequestAnalysis, file FileCandidate) int {
	score := int(file.Score * 100)
	if file.Required {
		score += 25
	}
	switch analysis.RequestType {
	case RequestTypeBugFix:
		score += 15
	case RequestTypeTest:
		score += 10
	case RequestTypeDocumentation:
		score += 5
	}
	return score
}

func verificationFor(requestType RequestType, path string) []string {
	switch requestType {
	case RequestTypeTest:
		return []string{"run targeted tests", "inspect failing assertions"}
	case RequestTypeDocumentation:
		return []string{"check markdown rendering", "review broken links"}
	default:
		return []string{"review changed file: " + path}
	}
}

func mergeNotes(previous *Plan, analysis RequestAnalysis, files []FileCandidate) []string {
	notes := []string{
		fmt.Sprintf("request type: %s", analysis.RequestType),
		fmt.Sprintf("confidence: %.2f", analysis.Confidence),
	}
	if previous != nil {
		notes = append(notes, fmt.Sprintf("replanned from %d prior iterations", previous.ReplanCount))
	}
	if len(files) == 0 {
		notes = append(notes, "no files discovered; start with investigation")
	}
	return notes
}

func scoreFile(path string, analysis RequestAnalysis) (float64, string) {
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.Dir(path))
	score := 0.0
	reasons := make([]string, 0, 3)

	for _, kw := range analysis.Keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(base, kw) {
			score += 0.4
			reasons = append(reasons, "filename match: "+kw)
		}
		if strings.Contains(dir, kw) {
			score += 0.15
			reasons = append(reasons, "path match: "+kw)
		}
	}

	switch analysis.RequestType {
	case RequestTypeTest:
		if strings.HasSuffix(base, "_test.go") || strings.Contains(base, "test") {
			score += 0.4
			reasons = append(reasons, "test file")
		}
	case RequestTypeDocumentation:
		if strings.HasSuffix(base, ".md") || strings.Contains(base, "readme") {
			score += 0.35
			reasons = append(reasons, "documentation file")
		}
	case RequestTypeBugFix, RequestTypeRefactor:
		if strings.HasSuffix(base, ".go") || strings.HasSuffix(base, ".ts") || strings.HasSuffix(base, ".py") {
			score += 0.2
			reasons = append(reasons, "source file")
		}
	}

	if strings.Contains(base, "main.") || strings.Contains(base, "agent") || strings.Contains(base, "planner") {
		score += 0.1
		reasons = append(reasons, "core code path")
	}

	if score == 0 {
		return 0, ""
	}
	return score, strings.Join(reasons, ", ")
}

func inferDependencies(files []FileCandidate) map[string][]string {
	deps := make(map[string][]string)
	bases := make(map[string]string)
	for _, file := range files {
		bases[file.Path] = strings.TrimSuffix(strings.ToLower(filepath.Base(file.Path)), filepath.Ext(file.Path))
	}
	for _, file := range files {
		content, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}
		text := strings.ToLower(string(content))
		for otherPath, base := range bases {
			if otherPath == file.Path {
				continue
			}
			if strings.Contains(text, base) {
				deps[file.Path] = appendUnique(deps[file.Path], otherPath)
			}
		}
	}
	return deps
}

func addVerificationSteps(analysis RequestAnalysis, steps []PlanStep) []PlanStep {
	needsTest := analysis.RequestType == RequestTypeFeature || analysis.RequestType == RequestTypeBugFix || analysis.RequestType == RequestTypeRefactor || analysis.RequestType == RequestTypeTest
	if !needsTest {
		return steps
	}
	verify := PlanStep{
		ID:           fmt.Sprintf("step-%d", len(steps)+1),
		Title:        "Verify changes",
		Description:  "Run targeted validation for the requested changes",
		Priority:     1,
		Estimated:    10 * time.Minute,
		Verification: []string{"run tests", "confirm no regressions"},
		Status:       StepPending,
	}
	for _, step := range steps {
		verify.DependsOn = appendUnique(verify.DependsOn, step.ID)
	}
	return append(steps, verify)
}

func buildExecutionOrder(steps []PlanStep) [][]string {
	indegree := make(map[string]int)
	graph := make(map[string][]string)
	for _, step := range steps {
		indegree[step.ID] = len(step.DependsOn)
		for _, dep := range step.DependsOn {
			graph[dep] = append(graph[dep], step.ID)
		}
	}

	ready := make([]string, 0)
	for _, step := range steps {
		if indegree[step.ID] == 0 {
			ready = append(ready, step.ID)
		}
	}
	sort.Strings(ready)

	order := make([][]string, 0)
	for len(ready) > 0 {
		phase := append([]string(nil), ready...)
		order = append(order, phase)
		next := make([]string, 0)
		for _, id := range ready {
			for _, dependent := range graph[id] {
				indegree[dependent]--
				if indegree[dependent] == 0 {
					next = append(next, dependent)
				}
			}
		}
		sort.Strings(next)
		ready = next
	}
	return order
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func estimateDuration(c Complexity) time.Duration {
	switch c {
	case ComplexityTrivial:
		return 5 * time.Minute
	case ComplexitySimple:
		return 15 * time.Minute
	case ComplexityModerate:
		return 45 * time.Minute
	case ComplexityComplex:
		return 2 * time.Hour
	default:
		return 6 * time.Hour
	}
}

func containsAny(request string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(request, needle) {
			return true
		}
	}
	return false
}
