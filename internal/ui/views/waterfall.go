package views

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

// Waterfall is a live rolling matrix — one row per model (busiest first),
// plus fixed "Agent" (subagent/Task-tool turns) and "Background" (usage the
// poll collectors picked up outside any interactive request) rows — so you
// can watch a tool shift between models, or a subagent/background job kick
// in, as it happens.
type Waterfall struct{}

func (w *Waterfall) View(c Ctx) string {
	s := c.Styles
	th := s.Theme
	if c.State == nil || len(c.State.Waterfall) == 0 {
		return s.Muted.Render("Waiting for the first data point…")
	}

	labelWidth := 26
	totalColWidth := 8
	cellWidth := 2
	if c.State.WindowMin > 0 {
		available := c.Width - labelWidth - totalColWidth - 6
		if available/c.State.WindowMin < cellWidth {
			cellWidth = clampInt(available/c.State.WindowMin, 1, 2)
		}
	}

	rows := make([]components.WaterfallRow, 0, len(c.State.Waterfall))
	for _, r := range c.State.Waterfall {
		var color string
		switch {
		case r.Provider != "":
			// Leave empty: model rows are gradient-shaded by intensity so
			// they stay comparable to one another at a glance.
		case r.Label == "Agent":
			color = th.Secondary
		case r.Label == "Background":
			color = th.Warning
		}
		rows = append(rows, components.WaterfallRow{Label: r.Label, Color: color, Values: r.Series})
	}

	matrix := components.Waterfall(rows, labelWidth, cellWidth, th)

	title := fmt.Sprintf("Waterfall — model & agent activity, last %dm (rolling right →)", c.State.WindowMin)
	agentSwatch := lipgloss.NewStyle().Foreground(lipgloss.Color(th.Secondary)).Render("■")
	legend := s.Subtle.Render("gradient = model intensity") + "   " +
		agentSwatch + s.Subtle.Render(" agent turns") + "   " +
		s.Warning.Render("■") + s.Subtle.Render(" background (poll) usage")

	body := s.PanelTitle.Render(title) + "\n" + legend + "\n\n" + matrix
	return s.Panel.Width(c.Width - 2).Render(body)
}
