package prompt

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed compact/summary.txt
var compactSummaryPrompt string

// CompactSummaryPrompt returns the structured conversation-summarization
// prompt used by the agent's compaction path. The structure (nine fixed
// sections, verbatim user messages, security constraints preserved, an
// <analysis> drafting scratchpad that is stripped afterward) follows the
// reference agent's compaction prompt: an unstructured "summarize this"
// loses exactly the details the next turn needs — user feedback, failed
// attempts, and the precise point work stopped.
func CompactSummaryPrompt() string { return compactSummaryPrompt }

// analysisRe matches the <analysis> drafting scratchpad the summary prompt
// requests. The scratchpad improves summary quality but has no informational
// value once the summary is written, so it never enters the session.
var analysisRe = regexp.MustCompile(`(?s)<analysis>.*?</analysis>`)

// summaryRe matches the <summary> block wrapping the final summary.
var summaryRe = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)

// FormatCompactSummary strips the <analysis> scratchpad and unwraps the
// <summary> tags from a raw summarizer response. Input that uses neither
// marker is returned trimmed unchanged.
func FormatCompactSummary(summary string) string {
	formatted := analysisRe.ReplaceAllString(summary, "")
	if m := summaryRe.FindStringSubmatch(formatted); m != nil {
		formatted = strings.TrimSpace(m[1])
	}
	// Collapse the whitespace the stripped blocks leave behind.
	formatted = regexp.MustCompile(`\n{3,}`).ReplaceAllString(formatted, "\n\n")
	return strings.TrimSpace(formatted)
}

// CompactContinuationSuffix is appended to the summary message when
// compaction fires mid-run (auto, predictive, or reactive — not the user's
// manual /compact). Without it the model treats the summary as a stopping
// point and ends the turn with a recap, stalling the phase arc.
const CompactContinuationSuffix = `
Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with "I'll continue" or similar. Pick up the last task as if the break never happened.`

// CompactRecentPreservedNote tells the model the kept suffix is verbatim, so
// it does not re-derive or distrust the most recent exchanges.
const CompactRecentPreservedNote = "Recent messages are preserved verbatim below the summary."

// CompactSummaryHeader frames the summary message the model receives after a
// compaction.
const CompactSummaryHeader = `# Neural Context Summary

> This is a compressed representation of the earlier conversation. The summary below covers the earlier portion; the verbatim recent messages follow it.`
