package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/iSundram/Automergent/internal/ai"
	"github.com/iSundram/Automergent/internal/config"
	"github.com/iSundram/Automergent/internal/session"
)

func TestEffectiveContextWindowReservesSummaryTokens(t *testing.T) {
	ag := &Agent{cfg: &config.Config{MaxContextTokens: 100_000}}
	got := ag.effectiveContextWindow(nil)
	if got != 100_000-maxOutputTokensForSummary {
		t.Fatalf("effective window = %d, want %d", got, 100_000-maxOutputTokensForSummary)
	}
}

func TestEffectiveContextWindowDegenerateConfig(t *testing.T) {
	ag := &Agent{cfg: &config.Config{MaxContextTokens: 10_000}}
	if got := ag.effectiveContextWindow(nil); got != 5_000 {
		t.Fatalf("degenerate effective window = %d, want half (5000)", got)
	}
}

func TestAutoCompactThresholds(t *testing.T) {
	ag := &Agent{cfg: &config.Config{MaxContextTokens: 100_000}}
	effective := ag.effectiveContextWindow(nil)

	// Default: effective minus the standard buffer.
	want := effective - autocompactBufferTokens
	if got := ag.autoCompactThreshold(nil); got != want {
		t.Fatalf("threshold = %d, want %d", got, want)
	}

	// A configured percentage can only pull the trigger earlier.
	ag.cfg.AutoCompressAt = 0.5
	pct := effective / 2
	if got := ag.autoCompactThreshold(nil); got != pct {
		t.Fatalf("percentage threshold = %d, want %d", got, pct)
	}

	// Blocking limit reserves the manual-compact buffer.
	if got := ag.blockingLimit(nil); got != effective-manualCompactBufferTokens {
		t.Fatalf("blocking limit = %d, want %d", got, effective-manualCompactBufferTokens)
	}
}

func TestAutoCompactThresholdScalesWithWindow(t *testing.T) {
	small := autocompactBuffer(200_000 - maxOutputTokensForSummary)
	large := autocompactBuffer(500_000 - maxOutputTokensForSummary)
	huge := autocompactBuffer(1_000_000 - maxOutputTokensForSummary)
	if small != autocompactBufferTokens || large != largeWindowBufferTokens || huge != hugeWindowBufferTokens {
		t.Fatalf("buffers not scaled: small=%d large=%d huge=%d", small, large, huge)
	}
}

func TestTokenCountWithEstimationAnchor(t *testing.T) {
	ag := &Agent{cfg: &config.Config{}}
	msgs := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, strings.Repeat("x", 400)),
		ai.NewTextMessage(ai.RoleAssistant, strings.Repeat("y", 400)),
	}

	// No anchor yet: pure estimation.
	unanchored := ag.tokenCountWithEstimation(msgs)
	if unanchored == 0 {
		t.Fatal("unanchored estimate should be positive")
	}

	// Anchor on the first message with a known usage total.
	ag.recordUsageAnchor(ai.Usage{InputTokens: 90, OutputTokens: 10}, 1)
	anchored := ag.tokenCountWithEstimation(msgs)
	want := 100 + len(msgs[1].PlaintextForHistory())/4
	if anchored != want {
		t.Fatalf("anchored count = %d, want %d", anchored, want)
	}

	// Compaction invalidates the anchor.
	ag.invalidateUsageAnchor()
	if ag.usageAnchor != 0 || ag.usageAnchoredTokens != 0 {
		t.Fatal("anchor not invalidated")
	}
}

func TestMicroCompactToolResults(t *testing.T) {
	build := func(n int) []ai.Message {
		msgs := []ai.Message{ai.NewTextMessage(ai.RoleUser, "request")}
		for i := 0; i < n; i++ {
			msgs = append(msgs,
				ai.Message{
					Role: ai.RoleAssistant,
					Content: []ai.ContentPart{{
						Type:     ai.ContentTypeToolCall,
						ToolCall: &ai.ToolCall{ID: fmt.Sprintf("c%d", i), Name: "read_file"},
					}},
				},
				ai.Message{
					Role: ai.RoleTool,
					Content: []ai.ContentPart{{
						Type: ai.ContentTypeToolResult,
						ToolResult: &ai.ToolResult{
							ToolCallID: fmt.Sprintf("c%d", i),
							Content:    strings.Repeat("z", 2000),
						},
					}},
				})
		}
		return msgs
	}

	msgs := build(10)
	out := microCompactToolResults(msgs, 2)

	cleared, kept := 0, 0
	for _, m := range out {
		if m.Role != ai.RoleTool {
			continue
		}
		c := m.Content[0].ToolResult.Content
		if strings.Contains(c, "[Old tool result cleared") {
			cleared++
		} else if len(c) == 2000 {
			kept++
		}
	}
	if cleared != 8 || kept != 2 {
		t.Fatalf("cleared=%d kept=%d, want 8 cleared and 2 kept", cleared, kept)
	}

	// Tool calls are untouched and the sequence stays valid.
	if err := ai.ValidateMessageSequence(out); err != nil {
		t.Fatalf("sequence invalid after micro-compact: %v", err)
	}

	// Idempotent: a second pass changes nothing.
	if again := microCompactToolResults(out, 2); !sameToolContents(again, out) {
		t.Fatal("micro-compact is not idempotent")
	}
}

func sameToolContents(a, b []ai.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != ai.RoleTool || b[i].Role != ai.RoleTool {
			continue
		}
		if len(a[i].Content) != len(b[i].Content) {
			return false
		}
		for j := range a[i].Content {
			pa, pb := a[i].Content[j].ToolResult, b[i].Content[j].ToolResult
			if pa != nil && pb != nil && pa.Content != pb.Content {
				return false
			}
		}
	}
	return true
}

func TestAdjustIndexToPreservePairs(t *testing.T) {
	msgs := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "request"),
		ai.NewTextMessage(ai.RoleAssistant, "plan"), // 1
		ai.Message{ // 2: assistant with tool call
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{{
				Type:     ai.ContentTypeToolCall,
				ToolCall: &ai.ToolCall{ID: "t1", Name: "grep"},
			}},
		},
		ai.Message{ // 3: its result
			Role: ai.RoleTool,
			Content: []ai.ContentPart{{
				Type:       ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{ToolCallID: "t1", Content: "hits"},
			}},
		},
		ai.NewTextMessage(ai.RoleAssistant, "findings"), // 4
		ai.NewTextMessage(ai.RoleAssistant, "wrap"),     // 5
	}

	// A naive split at 3 would summarize away the tool call at 2 but keep
	// its orphaned result; the adjustment must move the boundary to 2.
	if got := adjustIndexToPreservePairs(msgs, 3); got != 2 {
		t.Fatalf("adjusted index = %d, want 2", got)
	}

	// A split that already lands on a clean boundary is untouched.
	if got := adjustIndexToPreservePairs(msgs, 4); got != 4 {
		t.Fatalf("clean boundary moved: %d, want 4", got)
	}
}

func TestCompactSessionMessagesMarksBoundary(t *testing.T) {
	ag := &Agent{
		cfg:      &config.Config{CompressionKeepRecent: 2},
		provider: &mockProvider{summaryResponse: "summary"},
		sess:     session.New(),
	}

	msgs := make([]ai.Message, 15)
	msgs[0] = ai.NewTextMessage(ai.RoleUser, "Initial intent")
	for i := 1; i < 15; i++ {
		msgs[i] = ai.NewTextMessage(ai.RoleAssistant, "filler")
	}

	compacted := ag.CompactSessionMessages(context.Background(), msgs)
	foundBoundary := false
	for _, m := range compacted {
		if m.Metadata != nil {
			if v, ok := m.Metadata[compactionBoundaryKey].(bool); ok && v {
				foundBoundary = true
			}
		}
	}
	if !foundBoundary {
		t.Fatal("compaction boundary marker missing from summary message")
	}
}

func TestCompactSessionMessagesKeepsPairsTogether(t *testing.T) {
	ag := &Agent{
		cfg:      &config.Config{CompressionKeepRecent: 2},
		provider: &mockProvider{summaryResponse: "summary"},
		sess:     session.New(),
	}

	// user, assistant(no tools), filler padding, then a call/result pair
	// straddling the naive keep-recent boundary, then one trailing message.
	msgs := []ai.Message{
		ai.NewTextMessage(ai.RoleUser, "request"),
		ai.NewTextMessage(ai.RoleAssistant, "plan"),
	}
	for i := 0; i < 12; i++ {
		msgs = append(msgs, ai.NewTextMessage(ai.RoleAssistant, "filler"))
	}
	msgs = append(msgs,
		ai.Message{
			Role: ai.RoleAssistant,
			Content: []ai.ContentPart{{
				Type:     ai.ContentTypeToolCall,
				ToolCall: &ai.ToolCall{ID: "t1", Name: "grep"},
			}},
		},
		ai.Message{
			Role: ai.RoleTool,
			Content: []ai.ContentPart{{
				Type:       ai.ContentTypeToolResult,
				ToolResult: &ai.ToolResult{ToolCallID: "t1", Content: "hits"},
			}},
		},
		ai.NewTextMessage(ai.RoleAssistant, "wrap"),
	)

	compacted := ag.CompactSessionMessages(context.Background(), msgs)
	if err := ai.ValidateMessageSequence(compacted); err != nil {
		t.Fatalf("compacted sequence invalid: %v", err)
	}
	// The kept suffix must not start with an orphaned tool result.
	for i, m := range compacted {
		if m.Role == ai.RoleTool && i > 0 {
			// Every tool result's call must appear at or after the boundary.
			seen := map[string]bool{}
			for _, earlier := range compacted[:i] {
				for _, tc := range earlier.ToolCallParts() {
					seen[tc.ID] = true
				}
			}
			for _, p := range m.Content {
				if p.ToolResult != nil && !seen[p.ToolResult.ToolCallID] {
					t.Fatalf("orphaned tool result for call %s at position %d", p.ToolResult.ToolCallID, i)
				}
			}
		}
	}
}

func TestContextOverflowErrorMatching(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"request failed: maximum context length exceeded", true},
		{"input tokens exceed the model's limit", true},
		{"400 Bad Request: request too large", true},
		{"connection reset by peer", false},
		{"rate limited", false},
	}
	for _, tc := range cases {
		if got := isContextOverflowError(fmt.Errorf("%s", tc.msg)); got != tc.want {
			t.Errorf("isContextOverflowError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestSteeredNotificationsInjectAsUserMessages(t *testing.T) {
	ag := &Agent{
		cfg:   &config.Config{},
		sess:  session.New(),
		events: make(chan Event, 128),
		steer: make(chan string, 8),
	}
	if !ag.Steer("<task-notification>agent done</task-notification>") {
		t.Fatal("steer rejected the notification")
	}
	if injected := ag.drainSteer(); injected != 1 {
		t.Fatalf("drainSteer injected %d messages, want 1", injected)
	}
	if len(ag.sess.Messages) != 1 || ag.sess.Messages[0].Role != ai.RoleUser {
		t.Fatalf("notification not recorded as user message: %+v", ag.sess.Messages)
	}
	if !strings.Contains(ag.sess.Messages[0].TextContent(), "<task-notification>") {
		t.Fatalf("notification text lost: %q", ag.sess.Messages[0].TextContent())
	}
}
