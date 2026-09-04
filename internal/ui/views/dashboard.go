package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/engine"
	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

// Dashboard is the live, at-a-glance tab: stat cards, a rolling graph, per
// provider rate-limit gauges, and a top-models breakdown.
type Dashboard struct {
	Filter model.Provider // "" = all providers
}

// CycleFilter advances the provider filter (used by '[' / ']').
func (d *Dashboard) CycleFilter(state *engine.DashboardState, forward bool) {
	opts := append([]model.Provider{""}, state.Order...)
	if len(opts) == 0 {
		return
	}
	idx := 0
	for i, p := range opts {
		if p == d.Filter {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(opts)
	} else {
		idx = (idx - 1 + len(opts)) % len(opts)
	}
	d.Filter = opts[idx]
}

// SetFilterDigit maps '0'..'4' to (all, provider1..4) in state.Order.
func (d *Dashboard) SetFilterDigit(state *engine.DashboardState, n int) {
	if n == 0 {
		d.Filter = ""
		return
	}
	if n-1 < len(state.Order) {
		d.Filter = state.Order[n-1]
	}
}

func (d *Dashboard) View(c Ctx) string {
	s := c.Styles
	th := s.Theme
	if c.State == nil {
		return s.Muted.Render("Waiting for the first data point…")
	}
	st := c.State

	visible := st.Order
	if d.Filter != "" {
		visible = []model.Provider{d.Filter}
	}

	header := d.renderFilterBar(c, visible)
	accountBar := d.renderAccountBar(c)

	// --- stat cards -------------------------------------------------------
	var sessTot model.TokenUsage
	var winTot model.TokenUsage
	var cost, rate float64
	for _, p := range visible {
		ps := st.ProviderOrDefault(p)
		sessTot = sessTot.Add(ps.Session)
		winTot = winTot.Add(ps.Window)
		cost += ps.Cost
		rate += ps.RatePerMin
	}
	if d.Filter == "" {
		sessTot, winTot, cost, rate = st.SessionTotals, st.Totals, st.TotalCost, st.RatePerMin
	}
	cards := []components.StatCard{
		{Label: "session tokens", Value: components.CompactInt(sessTot.Total()), Sub: components.Duration(c.Now.Sub(st.StartedAt)) + " running", Color: th.Primary},
		{Label: "rate", Value: components.Compact(rate) + "/min", Sub: fmt.Sprintf("last %dm window", st.WindowMin), Color: th.Accent},
		{Label: "window cost", Value: components.Money(cost), Sub: components.Money(st.SessionCost) + " total", Color: th.Success},
		{Label: "requests", Value: components.CompactInt(winTot.Requests), Sub: fmt.Sprintf("%d events seen", st.EventsSeen), Color: th.Secondary},
		{Label: "cache hit", Value: components.Pct(winTot.CacheHitRatio()), Sub: components.CompactInt(winTot.CacheReadTokens) + " tok read", Color: th.Warning},
	}
	cardW := 22
	statRow := components.RenderStatCards(cards, cardW, c.Width, th)

	// --- layout: graph (left) + limits (right) when wide ------------------
	accountLines := 0
	if accountBar != "" {
		accountLines = lipgloss.Height(accountBar)
	}
	graphH := clampInt(c.ContentHeight()-lipgloss.Height(statRow)-lipgloss.Height(header)-accountLines-10, 6, 16)
	wide := c.Width >= 100
	var body string
	if wide {
		rightW := 34
		leftW := c.Width - rightW - 3
		leftContent := d.graphContent(c, visible, leftW, graphH)
		rightContent := d.limitsContent(c, visible, rightW)
		// Both panels must share one border-box height, or the shorter one's
		// border closes early while the taller one keeps going — that reads
		// as a "broken" disconnected border rather than two aligned boxes.
		commonH := maxInt(lipgloss.Height(leftContent), lipgloss.Height(rightContent))
		left := s.Panel.Width(leftW - 2).Height(commonH).Render(leftContent)
		right := s.Panel.Width(rightW - 2).Height(commonH).Render(rightContent)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	} else {
		graph := s.Panel.Width(c.Width - 2).Render(d.graphContent(c, visible, c.Width, clampInt(graphH-2, 5, 12)))
		limits := s.Panel.Width(c.Width - 2).Render(d.limitsContent(c, visible, c.Width))
		body = lipgloss.JoinVertical(lipgloss.Left, graph, limits)
	}

	models := d.renderModelsPanel(c, visible, c.Width)

	parts := []string{header}
	if accountBar != "" {
		parts = append(parts, accountBar)
	}
	parts = append(parts, statRow, body, models)
	if notes := renderNotes(c); notes != "" {
		parts = append(parts, notes)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (d *Dashboard) renderFilterBar(c Ctx, visible []model.Provider) string {
	s := c.Styles
	var chips []string
	all := s.TabInactive
	if d.Filter == "" {
		all = s.TabActive
	}
	chips = append(chips, all.Render("ALL"))
	for i, p := range c.State.Order {
		var st lipgloss.Style
		if d.Filter == p {
			st = lipgloss.NewStyle().Background(lipgloss.Color(s.Theme.ProviderColor(string(p)))).Foreground(lipgloss.Color(s.Theme.Bg)).Bold(true).Padding(0, 2)
		} else {
			st = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.ProviderColor(string(p)))).Padding(0, 2)
		}
		badge := ""
		if c.State.ProviderOrDefault(p).Active(c.Now) {
			badge = "●"
		} else {
			badge = "○"
		}
		chips = append(chips, st.Render(fmt.Sprintf("%d:%s %s", i+1, p.Title(), badge)))
	}
	hint := s.Subtle.Render("  [ / ] cycle provider · p pause · t theme")
	return lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(chips, " "), hint)
}

// renderAccountBar shows the CLI's own login/plan state — Claude.ai
// subscription (Pro/Max/...) vs. a pay-as-you-go console API key, plus the
// session/weekly usage allowance and whether Claude Code has dropped into
// "low priority" mode. Empty when no collector reports this (e.g. the
// account-status reader is off, or the provider doesn't support it yet).
func (d *Dashboard) renderAccountBar(c Ctx) string {
	s := c.Styles
	a, ok := c.State.Accounts[model.ProviderAnthropic]
	if !ok {
		return ""
	}

	label := a.PlanLabel
	switch a.Login {
	case model.LoginSubscription:
		label += " subscription"
	case model.LoginAPIKey:
		// PlanLabel is already "API Console (pay-as-you-go)".
	default:
		return ""
	}
	segs := []string{s.ProviderStyle(string(model.ProviderAnthropic)).Bold(true).Render("Claude") + " " + s.Text.Render(label)}
	if age := staleness(s, a.FetchedAt, c.Now); age != "" {
		segs = append(segs, age)
	}

	for _, w := range a.Windows {
		name := w.Kind
		st := s.Muted
		flag := ""
		if w.Active && w.Severity == "critical" {
			st = s.Danger
			flag = " ⚠ LOW PRIORITY"
		} else if w.Used >= 90 {
			st = s.Warning
		}
		seg := fmt.Sprintf("%s %.0f%% used%s", name, w.Used, flag)
		if !w.ResetAt.IsZero() {
			seg += " ↺" + components.Countdown(w.ResetIn(c.Now))
		}
		segs = append(segs, st.Render(seg))
	}

	return strings.Join(segs, s.Subtle.Render("  ·  "))
}

// graphContent returns the graph panel's unwrapped content (no border) so
// the caller can force a common height across side-by-side panels.
func (d *Dashboard) graphContent(c Ctx, visible []model.Provider, width, height int) string {
	s := c.Styles
	th := s.Theme
	series := make([]float64, c.State.WindowMin)
	for _, p := range visible {
		ps := c.State.ProviderOrDefault(p)
		for i, pt := range ps.Series {
			if i < len(series) {
				series[i] += float64(pt.Usage.Total())
			}
		}
	}
	innerW := width - 4
	if innerW < 10 {
		innerW = 10
	}
	graph := components.Braille([]components.Series{{Values: series, Color: th.Accent, Fill: true}}, innerW, height, th)

	var lines []string
	title := fmt.Sprintf("Tokens / minute — last %dm", c.State.WindowMin)
	lines = append(lines, s.PanelTitle.Render(title))
	lines = append(lines, graph)
	lines = append(lines, "")
	for _, p := range visible {
		ps := c.State.ProviderOrDefault(p)
		vals := make([]float64, len(ps.Series))
		for i, pt := range ps.Series {
			vals[i] = float64(pt.Usage.Total())
		}
		spark := components.Sparkline(vals, clampInt(innerW/2, 8, 40), th)
		row := fmt.Sprintf("%-9s %s %s", p.Title(), spark, components.Compact(ps.RatePerMin)+"/min")
		lines = append(lines, s.ProviderStyle(string(p)).Render(row))
	}
	return strings.Join(lines, "\n")
}

// limitsContent returns the rate-limits panel's unwrapped content (no
// border); see graphContent.
func (d *Dashboard) limitsContent(c Ctx, visible []model.Provider, width int) string {
	s := c.Styles
	th := s.Theme
	var b strings.Builder
	b.WriteString(s.PanelTitle.Render("Rate limits") + "\n")
	any := false
	gw := width - 20
	if gw < 8 {
		gw = 8
	}
	for _, p := range visible {
		ps := c.State.ProviderOrDefault(p)
		acct, hasAcct := c.State.Accounts[p]
		if len(ps.Limits) == 0 && (!hasAcct || len(acct.Windows) == 0) {
			continue
		}
		any = true
		header := s.ProviderStyle(string(p)).Bold(true).Render(p.Title())
		if hasAcct && len(acct.Windows) > 0 {
			if age := staleness(s, acct.FetchedAt, c.Now); age != "" {
				header += " " + age
			}
		}
		b.WriteString(header + "\n")
		for _, l := range ps.Limits {
			row := fmt.Sprintf(" %-4s %s %s", l.Kind.Short(), components.GaugeWithLabel(l.Fraction(), gw, th),
				s.Subtle.Render("↺"+components.Countdown(l.ResetIn(c.Now))))
			b.WriteString(row + "\n")
		}
		// The account-status collector ("logs" mode) reports plan-level
		// session/weekly allowance windows — a different kind of limit than
		// the proxy/poll collectors' per-minute API headers, but it belongs
		// in the same box rather than leaving it empty for "logs" users.
		for _, w := range acct.Windows {
			flag := ""
			if w.Active && w.Severity == "critical" {
				flag = " " + s.Danger.Render("⚠ LOW PRIORITY")
			}
			row := fmt.Sprintf(" %-4s %s %s%s", windowShortLabel(w.Kind), components.GaugeWithLabel(w.Used/100, gw, th),
				s.Subtle.Render("↺"+components.Countdown(w.ResetIn(c.Now))), flag)
			b.WriteString(row + "\n")
		}
	}
	if !any {
		b.WriteString(s.Subtle.Render(" no limit data observed yet\n (logs: Claude account status · proxy/poll: API rate-limit headers)"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// staleness renders how long ago Claude Code itself last refreshed its
// cached usage snapshot — this is the CLI's own refresh time, not when we
// read the file, and the CLI can go a long time between refreshes. Making
// this visible matters: it explains why the number here can disagree with
// what `/status` or claude.ai shows live, rather than that being a bug.
func staleness(s ui.Styles, fetchedAt, now time.Time) string {
	if fetchedAt.IsZero() {
		return ""
	}
	age := now.Sub(fetchedAt)
	if age < 0 {
		age = 0
	}
	txt := "(Claude Code cache, " + components.Duration(age) + " old)"
	if age > 15*time.Minute {
		return s.Warning.Render(txt)
	}
	return s.Subtle.Render(txt)
}

// windowShortLabel mirrors model.LimitKind.Short()'s style for
// model.UsageWindow's plan-level buckets.
func windowShortLabel(kind string) string {
	switch kind {
	case "session":
		return "5h"
	case "weekly":
		return "7d"
	default:
		return kind
	}
}

func (d *Dashboard) renderModelsPanel(c Ctx, visible []model.Provider, width int) string {
	s := c.Styles
	type agg struct {
		model string
		prov  model.Provider
		usage model.TokenUsage
		cost  float64
	}
	byModel := map[string]*agg{}
	for _, p := range visible {
		ps := c.State.ProviderOrDefault(p)
		for _, mu := range ps.TopModels(0) {
			key := string(p) + "/" + mu.Model
			byModel[key] = &agg{model: mu.Model, prov: p, usage: mu.Usage, cost: mu.Cost}
		}
	}
	rows := make([]*agg, 0, len(byModel))
	for _, a := range byModel {
		rows = append(rows, a)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].usage.Total() > rows[j].usage.Total() })
	if len(rows) > 6 {
		rows = rows[:6]
	}
	var barRows []components.BarRow
	for _, a := range rows {
		lbl := a.model
		if lbl == "" {
			lbl = "(unknown)"
		}
		barRows = append(barRows, components.BarRow{
			Label: fmt.Sprintf("%s · %s", a.prov.Title(), lbl),
			Value: float64(a.usage.Total()),
			Sub:   components.CompactInt(a.usage.Total()) + " tok · " + components.Money(a.cost),
			Color: s.Theme.ProviderColor(string(a.prov)),
		})
	}
	chart := components.BarChart(barRows, 28, clampInt(width-50, 10, 40), s.Theme)
	content := s.PanelTitle.Render("Top models (window)") + "\n" + chart
	return s.Panel.Width(width - 2).Render(content)
}

func renderNotes(c Ctx) string {
	if len(c.State.Errors) == 0 {
		return ""
	}
	last := c.State.Errors[len(c.State.Errors)-1]
	return c.Styles.Warning.Render(fmt.Sprintf("⚠ %s: %s", last.Source, last.Err))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
