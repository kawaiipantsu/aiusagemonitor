package views

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	"github.com/kawaiipantsu/aiusagemonitor/internal/store"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

type histRange struct {
	Label  string
	Span   time.Duration
	Bucket time.Duration
}

var histRanges = []histRange{
	{"1h", time.Hour, time.Minute},
	{"24h", 24 * time.Hour, time.Hour},
	{"7d", 7 * 24 * time.Hour, 6 * time.Hour},
	{"30d", 30 * 24 * time.Hour, 24 * time.Hour},
	{"90d", 90 * 24 * time.Hour, 24 * time.Hour},
}

// History shows token/cost totals over a selectable time range, sourced from
// the persisted event log (not just the in-memory dashboard window).
type History struct {
	RangeIdx int
}

func (h *History) Cycle(forward bool) {
	if forward {
		h.RangeIdx = (h.RangeIdx + 1) % len(histRanges)
	} else {
		h.RangeIdx = (h.RangeIdx - 1 + len(histRanges)) % len(histRanges)
	}
}

func (h *History) View(c Ctx) string {
	s := c.Styles
	r := histRanges[h.RangeIdx]
	to := time.Now()
	from := to.Add(-r.Span)

	var chips []string
	for i, rr := range histRanges {
		st := s.TabInactive
		if i == h.RangeIdx {
			st = s.TabActive
		}
		chips = append(chips, st.Render(rr.Label))
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, chips...) + "  " + s.Help.Render("←/→ change range")

	totals, err := c.Store.RangeTotals(c.DBCtx(), from, to)
	if err != nil {
		return header + "\n" + s.Danger.Render("query failed: "+err.Error())
	}
	buckets, _ := c.Store.Series(c.DBCtx(), from, to, r.Bucket)

	// Sum per-bucket across providers, preserving chronological order.
	byTime := map[int64]float64{}
	var order []int64
	for _, b := range buckets {
		k := b.Start.UnixMilli()
		if _, ok := byTime[k]; !ok {
			order = append(order, k)
		}
		byTime[k] += float64(b.Usage.Total())
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	var barRows []components.BarRow
	maxRows := clampInt(c.ContentHeight()-14, 4, 40)
	start := 0
	if len(order) > maxRows {
		start = len(order) - maxRows
	}
	for _, k := range order[start:] {
		t := time.UnixMilli(k)
		lbl := t.Format("Jan 2 15:04")
		if r.Bucket >= 24*time.Hour {
			lbl = t.Format("Mon Jan 2")
		} else if r.Bucket >= time.Hour {
			lbl = t.Format("Jan 2 15:00")
		}
		barRows = append(barRows, components.BarRow{Label: lbl, Value: byTime[k], Sub: components.CompactInt(int64(byTime[k])) + " tok"})
	}
	chart := components.BarChart(barRows, 16, clampInt(c.Width-40, 10, 60), s.Theme)
	chartPanel := s.Panel.Width(c.Width - 2).Render(s.PanelTitle.Render("Tokens per "+r.Bucket.String()+" bucket") + "\n" + chart)

	summary := h.renderSummary(c, totals, from, to)

	return lipgloss.JoinVertical(lipgloss.Left, header, chartPanel, summary)
}

func (h *History) renderSummary(c Ctx, totals store.Totals, from, to time.Time) string {
	s := c.Styles

	provRows := make([]components.BarRow, 0, len(totals.ByProvider))
	for p, u := range totals.ByProvider {
		provRows = append(provRows, components.BarRow{
			Label: p.Title(),
			Value: float64(u.Total()),
			Sub:   fmt.Sprintf("%s tok · %s · %d req", components.CompactInt(u.Total()), components.Money(totals.CostByProv[p]), u.Requests),
			Color: s.Theme.ProviderColor(string(p)),
		})
	}
	sort.Slice(provRows, func(i, j int) bool { return provRows[i].Value > provRows[j].Value })
	provChart := components.BarChart(provRows, 12, clampInt(c.Width/2-30, 8, 30), s.Theme)

	type modelRow struct {
		model string
		usage model.TokenUsage
		cost  float64
	}
	var modelRows []modelRow
	for m, u := range totals.ByModel {
		modelRows = append(modelRows, modelRow{m, u, totals.CostByModel[m]})
	}
	sort.Slice(modelRows, func(i, j int) bool { return modelRows[i].usage.Total() > modelRows[j].usage.Total() })
	if len(modelRows) > 8 {
		modelRows = modelRows[:8]
	}
	var modelBars []components.BarRow
	for _, mr := range modelRows {
		modelBars = append(modelBars, components.BarRow{
			Label: mr.model,
			Value: float64(mr.usage.Total()),
			Sub:   components.Money(mr.cost),
		})
	}
	modelChart := components.BarChart(modelBars, 20, clampInt(c.Width/2-30, 8, 30), s.Theme)

	stats := fmt.Sprintf("%s → %s   ·   %s tokens   ·   %s   ·   %d requests   ·   %d sessions   ·   %d events",
		from.Format("Jan 2 15:04"), to.Format("Jan 2 15:04"),
		components.CompactInt(totals.Usage.Total()), components.Money(totals.CostUSD), totals.Usage.Requests, totals.Sessions, totals.Events)

	left := s.Panel.Width(c.Width/2 - 1).Render(s.PanelTitle.Render("By provider") + "\n" + provChart)
	right := s.Panel.Width(c.Width/2 - 1).Render(s.PanelTitle.Render("By model") + "\n" + modelChart)
	cols := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	return lipgloss.JoinVertical(lipgloss.Left, s.Muted.Render(stats), cols)
}
