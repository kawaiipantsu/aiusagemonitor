package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// WaterfallRow is one row of a Waterfall matrix: a label and its per-bucket
// intensity values, oldest to newest (left to right).
type WaterfallRow struct {
	Label  string
	Color  string // fixed accent hue; empty = shade along the theme gradient instead
	Values []float64
}

// Waterfall renders a live rolling matrix — one row per series, time flowing
// left→right — so you can see at a glance how usage shifts between rows
// (e.g. which model is active, or when a subagent kicks in). Rows with a
// fixed Color are shaded by density (·░▒▓█) in that one hue, so they read as
// visually distinct from the gradient-shaded rows; a shared global maximum
// keeps every row's intensity comparable.
func Waterfall(rows []WaterfallRow, labelWidth, cellWidth int, th theme.Theme) string {
	if len(rows) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render("no data yet")
	}
	globalMax := 0.0
	for _, r := range rows {
		for _, v := range r.Values {
			if v > globalMax {
				globalMax = v
			}
		}
	}
	if globalMax <= 0 {
		globalMax = 1
	}

	labelSt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Text))
	totalSt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted))
	block := strings.Repeat("█", maxInt(1, cellWidth))

	var lines []string
	for _, r := range rows {
		var cells strings.Builder
		var sum float64
		for _, v := range r.Values {
			sum += v
			frac := v / globalMax
			var col string
			var glyph string
			if r.Color != "" {
				col = r.Color
				glyph = strings.Repeat(string(densityGlyph(frac)), maxInt(1, cellWidth))
			} else {
				col = th.GradientAt(frac)
				glyph = block
				if frac <= 0 {
					col = th.Border
					glyph = strings.Repeat("·", maxInt(1, cellWidth))
				}
			}
			cells.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(glyph))
		}
		row := labelSt.Render(Pad(r.Label, labelWidth)) + " " + cells.String() + " " + totalSt.Render(CompactInt(int64(sum)))
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func densityGlyph(frac float64) rune {
	idx := int(frac*float64(len(heatBlocks)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(heatBlocks) {
		idx = len(heatBlocks) - 1
	}
	return heatBlocks[idx]
}
