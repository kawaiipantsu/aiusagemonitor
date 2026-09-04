package views

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

// Profile surfaces usage-profiling summaries: an activity heatmap, busiest
// hour/day, cache efficiency and a simple burn-rate projection.
type Profile struct{}

var weekdayLabels = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

func (p *Profile) View(c Ctx) string {
	s := c.Styles
	to := time.Now()
	from := to.Add(-30 * 24 * time.Hour)

	grid, err := c.Store.Heatmap(c.DBCtx(), from, to)
	if err != nil {
		return s.Danger.Render("heatmap query failed: " + err.Error())
	}
	totals, err := c.Store.RangeTotals(c.DBCtx(), from, to)
	if err != nil {
		return s.Danger.Render("totals query failed: " + err.Error())
	}

	// Reorder Sun..Sat -> Mon..Sun for a conventional week view.
	rows := make([][]int64, 7)
	for i := 0; i < 7; i++ {
		src := (i + 1) % 7 // Mon=1 .. Sun=0
		row := make([]int64, 24)
		copy(row, grid[src][:])
		rows[i] = row
	}
	heat := components.Heatmap(rows, weekdayLabels, "0    4    8    12   16   20   24", s.Theme)

	busiestDay, busiestHour, dayTot, hourTot := busiest(rows)
	avgPerSession := 0.0
	if totals.Sessions > 0 {
		avgPerSession = float64(totals.Usage.Total()) / float64(totals.Sessions)
	}
	days := to.Sub(from).Hours() / 24
	if days < 1 {
		days = 1
	}
	dailyTok := float64(totals.Usage.Total()) / days
	dailyCost := totals.CostUSD / days

	cards := []components.StatCard{
		{Label: "avg tokens/session", Value: components.Compact(avgPerSession), Color: s.Theme.Primary},
		{Label: "cache hit ratio", Value: components.Pct(totals.Usage.CacheHitRatio()), Color: s.Theme.Accent},
		{Label: "busiest day", Value: weekdayLabels[busiestDay], Sub: components.CompactInt(dayTot) + " tok", Color: s.Theme.Secondary},
		{Label: "busiest hour", Value: fmt.Sprintf("%02d:00", busiestHour), Sub: components.CompactInt(hourTot) + " tok", Color: s.Theme.Warning},
		{Label: "daily burn rate", Value: components.Compact(dailyTok) + "/day", Sub: components.Money(dailyCost) + "/day", Color: s.Theme.Success},
		{Label: "30d projection", Value: components.Money(dailyCost * 30), Sub: components.CompactInt(int64(dailyTok*30)) + " tok", Color: s.Theme.Danger},
	}
	statRow := components.RenderStatCards(cards, 24, c.Width, s.Theme)

	heatPanel := s.Panel.Width(c.Width - 2).Render(s.PanelTitle.Render("Activity — last 30 days (Mon–Sun × hour of day)") + "\n\n" + heat)

	note := s.Subtle.Render("Profiling is computed from the persisted history database (not just the live window), so it survives restarts.")

	return lipgloss.JoinVertical(lipgloss.Left, statRow, heatPanel, note)
}

func busiest(rows [][]int64) (day, hour int, dayTot, hourTot int64) {
	hourSums := make([]int64, 24)
	for d, row := range rows {
		var sum int64
		for hIdx, v := range row {
			sum += v
			hourSums[hIdx] += v
		}
		if sum > dayTot {
			dayTot = sum
			day = d
		}
	}
	for h, v := range hourSums {
		if v > hourTot {
			hourTot = v
			hour = h
		}
	}
	return
}
