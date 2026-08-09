package components

import (
	"fmt"
	"time"

	"github.com/iSundram/Automergent/internal/tui/themes"
)

// Stats tracks session statistics.
type Stats struct {
	styles        *themes.Styles
	InputTokens   int
	OutputTokens  int
	TotalCost     float64
	StartTime     time.Time
	ToolCallCount int
}

// NewStats creates a Stats tracker.
func NewStats(styles *themes.Styles) Stats {
	return Stats{styles: styles, StartTime: time.Now()}
}

// View renders a compact stats line.
func (s Stats) View() string {
	elapsed := time.Since(s.StartTime).Round(time.Second)
	totalTok := s.InputTokens + s.OutputTokens
	return fmt.Sprintf(
		"in:%d  out:%d  total:%d  tools:%d  elapsed:%s",
		s.InputTokens, s.OutputTokens, totalTok, s.ToolCallCount, elapsed,
	)
}
