package cache

import (
	"regexp"
	"strings"
)

// DynamicBoundaryMarker is the marker that separates static from dynamic content.
// Content before this marker is highly cacheable.
const DynamicBoundaryMarker = "<!-- SYSTEM_PROMPT_DYNAMIC_BOUNDARY -->"

// ContentClassification identifies the stability of content for caching decisions.
type ContentClassification string

const (
	// ClassificationStatic is for content that rarely changes (system instructions).
	ClassificationStatic ContentClassification = "static"
	// ClassificationSemiStatic is for content that changes occasionally (tool schemas).
	ClassificationSemiStatic ContentClassification = "semi_static"
	// ClassificationDynamic is for content that changes frequently (user input, context).
	ClassificationDynamic ContentClassification = "dynamic"
	// ClassificationVolatile is for content that changes every request (timestamps).
	ClassificationVolatile ContentClassification = "volatile"
)

// BoundaryDetector identifies cache boundaries in content.
type BoundaryDetector struct {
	dynamicPatterns  []*regexp.Regexp
	volatilePatterns []*regexp.Regexp
}

// NewBoundaryDetector creates a new boundary detector with default patterns.
func NewBoundaryDetector() *BoundaryDetector {
	return &BoundaryDetector{
		dynamicPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)current (time|date|timestamp)`),
			regexp.MustCompile(`(?i)user['']?s (name|input|query)`),
			regexp.MustCompile(`(?i)session (id|token)`),
			regexp.MustCompile(`(?i)(today|now|currently)`),
			regexp.MustCompile(`\$\{[^}]+\}`),   // Template variables
			regexp.MustCompile(`\{\{[^}]+\}\}`), // Mustache templates
		},
		volatilePatterns: []*regexp.Regexp{
			regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`), // ISO timestamp
			regexp.MustCompile(`\d{10,13}`),                           // Unix timestamp
		},
	}
}

// ClassifyContent determines the cacheability of content.
func (d *BoundaryDetector) ClassifyContent(content string) ContentClassification {
	// Check for volatile patterns first
	for _, p := range d.volatilePatterns {
		if p.MatchString(content) {
			return ClassificationVolatile
		}
	}

	// Check for dynamic patterns
	for _, p := range d.dynamicPatterns {
		if p.MatchString(content) {
			return ClassificationDynamic
		}
	}

	// If it contains the dynamic boundary marker, it has both
	if strings.Contains(content, DynamicBoundaryMarker) {
		return ClassificationSemiStatic
	}

	// Default to static
	return ClassificationStatic
}

// FindBoundaries locates cache-friendly split points in content.
func (d *BoundaryDetector) FindBoundaries(content string) []int {
	var boundaries []int

	// Look for explicit markers
	markerIdx := strings.Index(content, DynamicBoundaryMarker)
	if markerIdx != -1 {
		boundaries = append(boundaries, markerIdx)
	}

	// Look for natural boundaries (section separators)
	lines := strings.Split(content, "\n")
	pos := 0
	for i, line := range lines {
		if isSectionBoundary(line) && i > 0 {
			boundaries = append(boundaries, pos)
		}
		pos += len(line) + 1 // +1 for newline
	}

	return boundaries
}

// isSectionBoundary checks if a line represents a natural section break.
func isSectionBoundary(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Markdown headers
	if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "===") {
		return true
	}

	// HTML-style comments that might be markers
	if strings.HasPrefix(trimmed, "<!--") && strings.Contains(trimmed, "SECTION") {
		return true
	}

	// Separator lines
	if len(trimmed) > 3 && (strings.Repeat("-", len(trimmed)) == trimmed ||
		strings.Repeat("=", len(trimmed)) == trimmed) {
		return true
	}

	return false
}

// splitPromptForCaching splits a prompt into cacheable blocks.
func splitPromptForCaching(prompt string) []ContentBlock {
	detector := NewBoundaryDetector()
	boundaries := detector.FindBoundaries(prompt)

	if len(boundaries) == 0 {
		// No boundaries found, cache the whole thing
		return []ContentBlock{{
			Type:         "text",
			Text:         prompt,
			CacheControl: DefaultCacheControl(),
		}}
	}

	var blocks []ContentBlock
	lastPos := 0

	for _, boundary := range boundaries {
		if boundary <= lastPos {
			continue
		}

		section := prompt[lastPos:boundary]
		if len(strings.TrimSpace(section)) > 0 {
			classification := detector.ClassifyContent(section)

			block := ContentBlock{
				Type: "text",
				Text: section,
			}

			// Apply cache control based on classification
			switch classification {
			case ClassificationStatic:
				block.CacheControl = LongTTLCacheControl()
			case ClassificationSemiStatic:
				block.CacheControl = DefaultCacheControl()
			default:
				// Dynamic/volatile content doesn't get cache control
			}

			blocks = append(blocks, block)
		}
		lastPos = boundary
	}

	// Handle remaining content
	if lastPos < len(prompt) {
		remaining := prompt[lastPos:]
		if len(strings.TrimSpace(remaining)) > 0 {
			// Remove the marker itself if present
			remaining = strings.ReplaceAll(remaining, DynamicBoundaryMarker, "")
			remaining = strings.TrimSpace(remaining)

			if len(remaining) > 0 {
				classification := detector.ClassifyContent(remaining)
				block := ContentBlock{
					Type: "text",
					Text: remaining,
				}
				if classification == ClassificationStatic {
					block.CacheControl = DefaultCacheControl()
				}
				blocks = append(blocks, block)
			}
		}
	}

	return blocks
}

// SplitSystemPrompt splits a system prompt at the dynamic boundary marker.
func SplitSystemPrompt(prompt string) (static, dynamic string) {
	idx := strings.Index(prompt, DynamicBoundaryMarker)
	if idx == -1 {
		return prompt, ""
	}
	return prompt[:idx], prompt[idx+len(DynamicBoundaryMarker):]
}

// OptimizeForCaching reorders content to maximize cache efficiency.
// Static content comes first, followed by semi-static, then dynamic.
func OptimizeForCaching(sections []string, classifications []ContentClassification) []string {
	if len(sections) != len(classifications) {
		return sections
	}

	type classified struct {
		content        string
		classification ContentClassification
		originalIdx    int
	}

	items := make([]classified, len(sections))
	for i := range sections {
		items[i] = classified{sections[i], classifications[i], i}
	}

	// Sort by classification priority
	priority := map[ContentClassification]int{
		ClassificationStatic:     0,
		ClassificationSemiStatic: 1,
		ClassificationDynamic:    2,
		ClassificationVolatile:   3,
	}

	// Stable sort by classification
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if priority[items[j].classification] < priority[items[i].classification] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.content
	}
	return result
}

// InsertBoundaryMarker adds a dynamic boundary marker to content.
func InsertBoundaryMarker(staticPart, dynamicPart string) string {
	if dynamicPart == "" {
		return staticPart
	}
	return staticPart + "\n" + DynamicBoundaryMarker + "\n" + dynamicPart
}

// IdentifyStaticPrefix finds the longest static prefix in a prompt.
func IdentifyStaticPrefix(prompt string) int {
	detector := NewBoundaryDetector()
	lines := strings.Split(prompt, "\n")

	staticEnd := 0
	pos := 0

	for _, line := range lines {
		lineLen := len(line) + 1 // +1 for newline
		classification := detector.ClassifyContent(line)

		if classification == ClassificationStatic || classification == ClassificationSemiStatic {
			staticEnd = pos + lineLen
		} else {
			// First dynamic line breaks the static prefix
			break
		}
		pos += lineLen
	}

	return staticEnd
}
