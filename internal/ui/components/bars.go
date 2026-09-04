package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// BarRow is one labelled horizontal bar.
type BarRow struct {
	Label string
	Value float64
	Sub   string // right-hand annotation, e.g. cost or percentage
	Color string // empty = theme.Primary
}

// BarChart renders a set of horizontal bars scaled to a shared maximum,
// labels left-aligned to labelWidth and the bar itself barWidth cells wide.
func BarChart(rows []BarRow, labelWidth, barWidth int, th theme.Theme) string {
	if len(rows) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render("no data yet")
	}
	maxV := 0.0
	for _, r := range rows {
		if r.Value > maxV {
			maxV = r.Value
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Text))
	subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Border))

	var b strings.Builder
	for i, r := range rows {
		col := r.Color
		if col == "" {
			col = th.Primary
		}
		frac := r.Value / maxV
		filled := int(frac*float64(barWidth) + 0.5)
		if filled > barWidth {
			filled = barWidth
		}
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render(strings.Repeat("█", filled)) +
			trackStyle.Render(strings.Repeat("·", barWidth-filled))
		b.WriteString(labelStyle.Render(Pad(r.Label, labelWidth)))
		b.WriteString(" ")
		b.WriteString(bar)
		if r.Sub != "" {
			b.WriteString(" ")
			b.WriteString(subStyle.Render(r.Sub))
		}
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
