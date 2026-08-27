package render

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// SimpleDiff renders an old→new pair as a unified-diff fragment: word-level
// «del»/‹ins› markers for single-line replacements, plain -/+ blocks otherwise.
// No file headers or @@ wrapper — callers add whatever chrome their context
// needs. This is the engine behind the fullscreen proposal viewer; the log's
// edit cards synthesize their previews through the same code so both views
// agree on what changed.
func SimpleDiff(oldText, newText string) string {
	dmp := diffmatchpatch.New()

	a, b, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	// Semantic cleanup MUST run while text is still line-tokenized: on decoded
	// strings it re-cuts runs mid-line, producing phantom context rows and
	// blank gaps (the old overlay renderer carried exactly this artifact).
	diffs = dmp.DiffCleanupSemantic(diffs)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = normalizeLineBoundaries(diffs)

	var sb strings.Builder
	for i, d := range diffs {
		if d.Text == "" {
			continue // consumed by the word-diff pairing below
		}
		text := strings.TrimSuffix(d.Text, "\n")
		lines := strings.Split(text, "\n")

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			for _, line := range lines {
				sb.WriteString(" " + line + "\n")
			}
		case diffmatchpatch.DiffDelete:
			// Replacement scenario: pair with the following insert and emit a
			// word-level diff so the shared part of a changed line stays quiet.
			if i+1 < len(diffs) && diffs[i+1].Type == diffmatchpatch.DiffInsert {
				insertText := strings.TrimSuffix(diffs[i+1].Text, "\n")
				insertLines := strings.Split(insertText, "\n")
				if len(lines) == len(insertLines) {
					for j, oldLine := range lines {
						sb.WriteString(wordDiff(dmp, oldLine, insertLines[j]))
					}
					diffs[i+1] = diffmatchpatch.Diff{Type: diffmatchpatch.DiffEqual, Text: ""}
					continue
				}
			}
			for _, line := range lines {
				sb.WriteString("-" + line + "\n")
			}
		case diffmatchpatch.DiffInsert:
			if d.Text == "" {
				continue // already consumed by the word-diff pairing above
			}
			for _, line := range lines {
				sb.WriteString("+" + line + "\n")
			}
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// normalizeLineBoundaries repairs runs left cut mid-line: an Equal run ending
// on a partial line hands that whole physical line to the run that continues
// it (so a changed row renders complete), and any other run merely borrows a
// missing terminator from its neighbor. Runs are already line-aligned when
// cleanup runs before decoding; this guards the ragged first/last segments.
func normalizeLineBoundaries(diffs []diffmatchpatch.Diff) []diffmatchpatch.Diff {
	for i := 0; i < len(diffs)-1; i++ {
		cur := &diffs[i]
		if cur.Text == "" || strings.HasSuffix(cur.Text, "\n") {
			continue
		}
		nxt := &diffs[i+1]
		if cur.Type != diffmatchpatch.DiffEqual {
			// The changed region owns its partial last line.
			if strings.HasPrefix(nxt.Text, "\n") {
				cur.Text += "\n"
				nxt.Text = nxt.Text[1:]
			}
			continue
		}
		nl := strings.LastIndexByte(cur.Text, '\n')
		partial := cur.Text[nl+1:]
		cur.Text = cur.Text[:nl+1]
		nxt.Text = partial + nxt.Text
	}
	return diffs
}

// wordDiff marks intra-line changes: deleted spans wrapped in «», inserted
// spans in ‹›. Lines with no real change pass through as plain context.
func wordDiff(dmp *diffmatchpatch.DiffMatchPatch, oldLine, newLine string) string {
	wordDiffs := dmp.DiffMain(oldLine, newLine, false)
	wordDiffs = dmp.DiffCleanupSemantic(wordDiffs)

	var oldSb, newSb strings.Builder
	hasChanges := false

	for _, wd := range wordDiffs {
		switch wd.Type {
		case diffmatchpatch.DiffEqual:
			oldSb.WriteString(wd.Text)
			newSb.WriteString(wd.Text)
		case diffmatchpatch.DiffDelete:
			oldSb.WriteString("«" + wd.Text + "»")
			hasChanges = true
		case diffmatchpatch.DiffInsert:
			newSb.WriteString("‹" + wd.Text + "›")
			hasChanges = true
		}
	}

	if hasChanges {
		return "-" + oldSb.String() + "\n+" + newSb.String() + "\n"
	}
	return " " + oldLine + "\n"
}

// AllAdds renders fresh-file content as an all-insertions diff fragment — the
// preview a create/write deserves instead of a bare "wrote N lines" sentence.
func AllAdds(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "+" + l
	}
	return strings.Join(lines, "\n")
}

// HunkLabel emits a display-only hunk separator understood by DiffWithWidth's
// @@ styling ("@@ edit 1 of 3 @@").
func HunkLabel(format string, args ...any) string {
	return "@@ " + fmt.Sprintf(format, args...) + " @@"
}
