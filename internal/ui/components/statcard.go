package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// StatCard is a small bordered box: a label, a big value, and an optional
// trailing sublabel (delta, unit, etc).
type StatCard struct {
	Label string
	Value string
	Sub   string
	Color string // accent colour for the value; empty = theme.Primary
}

// RenderStatCards lays out cards in a single row, each `width` cells wide,
// wrapping to additional rows if they would overflow `totalWidth`.
func RenderStatCards(cards []StatCard, width, totalWidth int, th theme.Theme) string {
	if width <= 0 {
		width = 18
	}
	perRow := max(1, totalWidth/(width+1))
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(th.Border)).
		Background(lipgloss.Color(th.Surface)).
		Width(width-2).Padding(0, 1)
	labelSt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle))
	subSt := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted))

	var rows []string
	for start := 0; start < len(cards); start += perRow {
		end := start + perRow
		if end > len(cards) {
			end = len(cards)
		}
		var rendered []string
		for _, c := range cards[start:end] {
			col := c.Color
			if col == "" {
				col = th.Primary
			}
			valSt := lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Bold(true)
			body := labelSt.Render(strings.ToUpper(c.Label)) + "\n" + valSt.Render(c.Value)
			if c.Sub != "" {
				body += "\n" + subSt.Render(c.Sub)
			}
			rendered = append(rendered, style.Render(body))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	}
	return strings.Join(rows, "\n")
}
