package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/iSundram/Automergent/internal/graph"
)

type ReplayEngine struct {
	store             *graph.Store
	mu                sync.RWMutex
	decisionRecorder  *DecisionRecorder
	memoryManager     *MemoryManager
}

func NewReplayEngine(store *graph.Store, decisionRecorder *DecisionRecorder, memoryManager *MemoryManager) *ReplayEngine {
	return &ReplayEngine{
		store:            store,
		decisionRecorder: decisionRecorder,
		memoryManager:    memoryManager,
	}
}

func (re *ReplayEngine) GetDecisionReplay(ctx context.Context, filePath string) ([]*DecisionReplay, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	return re.decisionRecorder.GetDecisionReplay(ctx, filePath)
}

func (re *ReplayEngine) GetDecisionTimeline(ctx context.Context, taskID uuid.UUID) ([]*DecisionTimelineEntry, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	decisions, err := re.decisionRecorder.GetDecisionsForTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("get decisions for task: %w", err)
	}

	var timeline []*DecisionTimelineEntry
	for i, d := range decisions {
		related := []uuid.UUID{}
		for _, opt := range d.Options {
			if opt.Metadata != nil {
				var meta map[string]interface{}
				if err := json.Unmarshal(opt.Metadata, &meta); err == nil {
					if relIDs, ok := meta["related_decisions"].([]interface{}); ok {
						for _, id := range relIDs {
							if uid, ok := id.(string); ok {
								if parsed, err := uuid.Parse(uid); err == nil {
									related = append(related, parsed)
								}
							}
						}
					}
				}
			}
		}
		timeline = append(timeline, &DecisionTimelineEntry{
			Decision:  d,
			Timestamp: d.CreatedAt,
			Sequence:  i,
			RelatedTo: related,
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp.Before(timeline[j].Timestamp)
	})

	for i := range timeline {
		timeline[i].Sequence = i
	}

	return timeline, nil
}

func (re *ReplayEngine) GetFailedAttempts(ctx context.Context, issue string) ([]*FailedAttempt, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	nodes, err := re.store.ListNodes(ctx, graph.NodeTypeMemory, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("list memory nodes: %w", err)
	}

	var attempts []*FailedAttempt
	issueLower := toLower(issue)

	for _, node := range nodes {
		var mem Memory
		if err := node.UnmarshalData(&mem); err != nil {
			continue
		}
		if mem.Type != MemoryTypeFailure {
			continue
		}

		if !containsIgnoreCase(mem.Content, issueLower) && !hasMatchingTag(mem.Tags, issueLower) {
			continue
		}

		attempt := &FailedAttempt{
			Issue:        extractIssue(mem.Content),
			Approach:     extractApproach(mem.Content),
			Reason:       extractReason(mem.Content),
			ErrorDetails: mem.Content,
			Timestamp:    mem.CreatedAt,
			Tags:         mem.Tags,
			Metadata:     mem.Metadata,
		}

		for _, refID := range mem.References {
			refNode, err := re.store.GetNode(ctx, refID)
			if err == nil && refNode.Type == graph.NodeTypeDecision {
				var d Decision
				if err := refNode.UnmarshalData(&d); err == nil {
					attempt.Decision = &d
					break
				}
			}
		}

		attempts = append(attempts, attempt)
	}

	sort.Slice(attempts, func(i, j int) bool {
		return attempts[i].Timestamp.After(attempts[j].Timestamp)
	})

	return attempts, nil
}

func (re *ReplayEngine) GetFixHistory(ctx context.Context, filePath string) ([]*FixAttempt, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	fileID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath))
	edges, err := re.store.GetEdgesTo(ctx, fileID, graph.EdgeTypeReferences)
	if err != nil {
		return nil, fmt.Errorf("get edges to file: %w", err)
	}

	var fixAttempts []*FixAttempt
	seen := make(map[uuid.UUID]bool)

	for _, edge := range edges {
		if seen[edge.FromID] {
			continue
		}
		node, err := re.store.GetNode(ctx, edge.FromID)
		if err != nil {
			continue
		}

		if node.Type == graph.NodeTypeDecision {
			var d Decision
			if err := node.UnmarshalData(&d); err == nil {
				if d.Type == DecisionTypeImplementation || d.Type == DecisionTypeVerification {
					fix := &FixAttempt{
						FilePath:   filePath,
						Issue:      extractIssueFromDecision(d),
						Fix:        extractFixFromDecision(d),
						Outcome:    d.Outcome,
						Decision:   &d,
						TestResult: extractTestResult(d),
						Timestamp:  d.CreatedAt,
						Tags:       d.Tags,
						Metadata:   d.Metadata,
					}
					fixAttempts = append(fixAttempts, fix)
					seen[edge.FromID] = true
				}
			}
		}

		if node.Type == graph.NodeTypeMemory {
			var mem Memory
			if err := node.UnmarshalData(&mem); err == nil {
				if mem.Type == MemoryTypeSolution || mem.Type == MemoryTypeFailure {
					fix := &FixAttempt{
						FilePath:   filePath,
						Issue:      extractIssue(mem.Content),
						Fix:        extractFix(mem.Content),
						Outcome:    determineOutcome(mem),
						Decision:   nil,
						TestResult: "",
						Timestamp:  mem.CreatedAt,
						Tags:       mem.Tags,
						Metadata:   mem.Metadata,
					}
					fixAttempts = append(fixAttempts, fix)
					seen[edge.FromID] = true
				}
			}
		}
	}

	sort.Slice(fixAttempts, func(i, j int) bool {
		return fixAttempts[i].Timestamp.Before(fixAttempts[j].Timestamp)
	})

	return fixAttempts, nil
}

func (re *ReplayEngine) GetRelatedDecisions(ctx context.Context, filePath string, lineStart, lineEnd int) ([]*DecisionReplay, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	replays, err := re.decisionRecorder.GetDecisionReplay(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if lineStart > 0 && lineEnd > 0 {
		var filtered []*DecisionReplay
		for _, r := range replays {
			if r.LineRange != nil {
				if rangesOverlap(r.LineRange.Start, r.LineRange.End, lineStart, lineEnd) {
					filtered = append(filtered, r)
				}
			} else {
				filtered = append(filtered, r)
			}
		}
		return filtered, nil
	}

	return replays, nil
}

func (re *ReplayEngine) GetDecisionContext(ctx context.Context, decisionID uuid.UUID) (*DecisionContext, error) {
	re.mu.RLock()
	defer re.mu.RUnlock()

	node, err := re.store.GetNode(ctx, decisionID)
	if err != nil {
		return nil, err
	}
	var decision Decision
	if err := node.UnmarshalData(&decision); err != nil {
		return nil, err
	}

	context := &DecisionContext{
		Decision:      &decision,
		RelatedFiles:  decision.FilePaths,
		RelatedTasks:  []uuid.UUID{decision.TaskID},
		RelatedMemories: []*Memory{},
		Alternatives:  decision.Options,
		Evidence:      decision.Evidence,
	}

	if decision.ContextBucket != uuid.Nil {
		bucketNode, err := re.store.GetNode(ctx, decision.ContextBucket)
		if err == nil {
			var bucket graph.ContextBucket
			if err := bucketNode.UnmarshalData(&bucket); err == nil {
				context.ContextBucket = &bucket
			}
		}
	}

	if decision.TaskID != uuid.Nil {
		taskNode, err := re.store.GetNode(ctx, decision.TaskID)
		if err == nil {
			var task graph.Task
			if err := taskNode.UnmarshalData(&task); err == nil {
				context.Task = &task
			}
		}
	}

	memories, _ := re.memoryManager.GetMemoriesForContext(ctx, []uuid.UUID{decision.ContextBucket})
	context.RelatedMemories = memories

	return context, nil
}

type DecisionContext struct {
	Decision        *Decision       `json:"decision"`
	ContextBucket   *graph.ContextBucket `json:"context_bucket,omitempty"`
	Task            *graph.Task     `json:"task,omitempty"`
	RelatedFiles    []string        `json:"related_files"`
	RelatedTasks    []uuid.UUID     `json:"related_tasks"`
	RelatedMemories []*Memory       `json:"related_memories"`
	Alternatives    []Option        `json:"alternatives"`
	Evidence        []EvidenceRef   `json:"evidence"`
}

func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart <= bEnd && bStart <= aEnd
}

func toLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		containsIgnoreCaseHelper(toLower(s), toLower(substr)))
}

func containsIgnoreCaseHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func hasMatchingTag(tags []string, query string) bool {
	query = toLower(query)
	for _, tag := range tags {
		if containsIgnoreCaseHelper(toLower(tag), query) {
			return true
		}
	}
	return false
}

func extractIssue(content string) string {
	lines := splitLines(content)
	for _, line := range lines {
		if len(line) > 10 {
			return line
		}
	}
	return content
}

func extractApproach(content string) string {
	lines := splitLines(content)
	for i, line := range lines {
		if containsIgnoreCase(line, "approach") || containsIgnoreCase(line, "tried") || containsIgnoreCase(line, "attempt") {
			if i+1 < len(lines) {
				return lines[i+1]
			}
			return line
		}
	}
	return ""
}

func extractReason(content string) string {
	lines := splitLines(content)
	for i, line := range lines {
		if containsIgnoreCase(line, "reason") || containsIgnoreCase(line, "because") || containsIgnoreCase(line, "failed") {
			if i+1 < len(lines) {
				return lines[i+1]
			}
			return line
		}
	}
	return ""
}

func extractIssueFromDecision(d Decision) string {
	if d.Description != "" {
		return d.Description
	}
	return d.Title
}

func extractFixFromDecision(d Decision) string {
	if d.Rationale != "" {
		return d.Rationale
	}
	if d.Outcome != "" {
		return d.Outcome
	}
	return ""
}

func extractTestResult(d Decision) string {
	if d.Metadata != nil {
		var meta map[string]interface{}
		if err := json.Unmarshal(d.Metadata, &meta); err == nil {
			if tr, ok := meta["test_result"].(string); ok {
				return tr
			}
		}
	}
	return ""
}

func extractFix(content string) string {
	lines := splitLines(content)
	for i, line := range lines {
		if containsIgnoreCase(line, "fix") || containsIgnoreCase(line, "solution") || containsIgnoreCase(line, "resolved") {
			if i+1 < len(lines) {
				return lines[i+1]
			}
			return line
		}
	}
	return content
}

func determineOutcome(mem Memory) string {
	if mem.Type == MemoryTypeSolution {
		return "success"
	}
	if mem.Type == MemoryTypeFailure {
		return "failed"
	}
	return "unknown"
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}