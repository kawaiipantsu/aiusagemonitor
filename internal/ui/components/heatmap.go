package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

var heatBlocks = []rune{'·', '░', '▒', '▓', '█'}

// Heatmap renders a grid (e.g. 7 rows × 24 cols) shaded by relative intensity.
// rowLabels, if non-nil, is printed left of each row.
func Heatmap(grid [][]int64, rowLabels []string, colHeader string, th theme.Theme) string {
	maxV := int64(0)
	for _, row := range grid {
		for _, v := range row {
			if v > maxV {
				maxV = v
			}
		}
	}
	labelW := 0
	for _, l := range rowLabels {
		if len(l) > labelW {
			labelW = len(l)
		}
	}
	var b strings.Builder
	if colHeader != "" {
		b.WriteString(strings.Repeat(" ", labelW+1))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render(colHeader))
		b.WriteString("\n")
	}
	for ri, row := range grid {
		if rowLabels != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted)).Render(PadLeft(rowLabels[ri], labelW)))
			b.WriteString(" ")
		}
		for _, v := range row {
			frac := 0.0
			if maxV > 0 {
				frac = float64(v) / float64(maxV)
			}
			idx := int(frac * float64(len(heatBlocks)-1))
			if idx >= len(heatBlocks) {
				idx = len(heatBlocks) - 1
			}
			ch := heatBlocks[idx]
			col := th.Border
			if idx > 0 {
				col = th.GradientAt(frac)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(string(ch)))
		}
		if ri < len(grid)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
