package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// Gauge renders a horizontal fill bar of the given width, coloured by the
// theme's success/warning/danger thresholds.
func Gauge(frac float64, width int, th theme.Theme) string {
	if width <= 2 {
		width = 3
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	inner := width - 2
	filled := int(frac*float64(inner) + 0.5)
	if filled > inner {
		filled = inner
	}
	col := th.GaugeColor(frac)
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(col))
	bg := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Border))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render("["))
	b.WriteString(fg.Render(strings.Repeat("█", filled)))
	b.WriteString(bg.Render(strings.Repeat("░", inner-filled)))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)).Render("]"))
	return b.String()
}

// GaugeWithLabel renders a gauge followed by a right-aligned percentage.
func GaugeWithLabel(frac float64, width int, th theme.Theme) string {
	g := Gauge(frac, width, th)
	lbl := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Muted)).Render(" " + Pct(frac))
	return g + lbl
}
