package views

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/engine"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

// Sessions lists every session seen in this run with a detail pane for the
// selected one.
type Sessions struct {
	table  table.Model
	ids    []string // row index -> session id, kept in sync with table rows
	width  int
	height int
	inited bool
}

func NewSessions() *Sessions {
	cols := []table.Column{
		{Title: "PROVIDER", Width: 10},
		{Title: "SESSION", Width: 18},
		{Title: "TOKENS", Width: 10},
		{Title: "RATE", Width: 10},
		{Title: "COST", Width: 9},
		{Title: "EVENTS", Width: 7},
		{Title: "LAST ACTIVE", Width: 12},
	}
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(10))
	return &Sessions{table: t}
}

func (v *Sessions) SetSize(w, h int) {
	v.width, v.height = w, h
	v.table.SetWidth(w)
	th := clampInt(h-10, 4, 30)
	v.table.SetHeight(th)
}

func (v *Sessions) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return cmd
}

func (v *Sessions) sync(c Ctx) {
	st := c.Styles
	v.table.SetStyles(table.Styles{
		Header:   lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color(st.Theme.Bg)).Background(lipgloss.Color(st.Theme.Primary)),
		Cell:     lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color(st.Theme.Text)),
		Selected: lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color(st.Theme.Bg)).Background(lipgloss.Color(st.Theme.Accent)),
	})
	rows := make([]table.Row, 0, len(c.State.Sessions))
	v.ids = v.ids[:0]
	for _, sess := range c.State.Sessions {
		label := sess.Label
		if label == "" {
			label = sess.ID
		}
		rows = append(rows, table.Row{
			sess.Provider.Title(),
			components.Pad(label, 18),
			components.CompactInt(sess.Usage.Total()),
			components.Compact(sess.RatePerMin) + "/m",
			components.Money(sess.CostUSD),
			fmt.Sprintf("%d", sess.Events),
			humanAgo(c.Now.Sub(sess.LastSeen)),
		})
		v.ids = append(v.ids, sess.ID)
	}
	v.table.SetRows(rows)
	if !v.inited && len(rows) > 0 {
		v.inited = true
	}
}

func (v *Sessions) View(c Ctx) string {
	s := c.Styles
	if c.State == nil || len(c.State.Sessions) == 0 {
		return s.Muted.Render("No sessions observed yet. Once a collector reports activity, sessions appear here.")
	}
	v.sync(c)

	tableView := s.Panel.Width(v.width - 2).Render(v.table.View())

	var detail string
	if idx := v.table.Cursor(); idx >= 0 && idx < len(c.State.Sessions) {
		detail = v.renderDetail(c, c.State.Sessions[idx])
	}
	help := s.Help.Render("↑/↓ select · pgup/pgdn page · home/end jump")
	return lipgloss.JoinVertical(lipgloss.Left, tableView, detail, help)
}

func (v *Sessions) renderDetail(c Ctx, sess engine.SessionAgg) string {
	s := c.Styles
	vals := make([]float64, len(sess.Series))
	for i, pt := range sess.Series {
		vals[i] = float64(pt.Usage.Total())
	}
	graph := components.Braille([]components.Series{{Values: vals, Color: s.Theme.ProviderColor(string(sess.Provider)), Fill: true}}, clampInt(v.width-10, 20, 120), 5, s.Theme)
	label := sess.Label
	if label == "" {
		label = sess.ID
	}
	title := fmt.Sprintf("%s · %s", sess.Provider.Title(), label)
	meta := fmt.Sprintf("first seen %s · duration %s · in %s / out %s / cache %s",
		humanAgo(c.Now.Sub(sess.FirstSeen)), components.Duration(sess.Duration()),
		components.CompactInt(sess.Usage.InputTokens), components.CompactInt(sess.Usage.OutputTokens),
		components.CompactInt(sess.Usage.CacheReadTokens))
	content := s.PanelTitle.Render(title) + "\n" + s.Muted.Render(meta) + "\n\n" + graph
	return s.Panel.Width(v.width - 2).Render(content)
}

// humanAgo renders a duration as a short "Xs/Xm/Xh ago" style string,
// clamping negatives (clock skew) to "now".
func humanAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return components.Duration(d) + " ago"
}
