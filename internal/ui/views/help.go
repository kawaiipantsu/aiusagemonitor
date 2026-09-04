package views

import (
	"strings"

	"github.com/kawaiipantsu/aiusagemonitor/internal/version"
)

// Help is the static reference tab.
type Help struct{}

type helpRow struct{ key, desc string }

var globalKeys = []helpRow{
	{"tab / shift+tab", "next / previous view"},
	{"1-6", "jump directly to a view"},
	{"t / T", "next / previous theme"},
	{"p", "pause / resume live updates"},
	{"r", "force an immediate refresh"},
	{"?", "toggle this help"},
	{"q, ctrl+c", "quit"},
}

var dashKeys = []helpRow{
	{"[ / ]", "cycle the provider filter (all → each provider)"},
}

var sessionsKeys = []helpRow{
	{"↑/↓, j/k", "select a session"},
	{"pgup/pgdn", "page the list"},
}

var historyKeys = []helpRow{
	{"←/→", "change the time range"},
}

var settingsKeys = []helpRow{
	{"↑/↓", "move between fields"},
	{"←/→", "change an enum value"},
	{"enter", "edit a text field / trigger an action"},
	{"esc", "cancel an edit"},
}

func (h *Help) View(c Ctx) string {
	s := c.Styles
	var b strings.Builder
	b.WriteString(s.PanelTitle.Render("aiusagemonitor") + "  " + s.Muted.Render(version.Short()) + "\n")
	b.WriteString(s.Text.Render("Live token/limit/cost monitoring for OpenAI, Claude, Google Gemini and xAI.") + "\n\n")

	section := func(title string, rows []helpRow) {
		b.WriteString(s.Accent.Render(title) + "\n")
		for _, r := range rows {
			b.WriteString("  " + s.Bold.Render(padKey(r.key)) + "  " + s.Muted.Render(r.desc) + "\n")
		}
		b.WriteString("\n")
	}
	section("Global", globalKeys)
	section("Dashboard", dashKeys)
	section("Sessions", sessionsKeys)
	section("History", historyKeys)
	section("Settings", settingsKeys)

	b.WriteString(s.Accent.Render("Data sources") + "\n")
	b.WriteString(s.Muted.Render(
		"  logs   tail local AI-CLI session transcripts (Claude Code, Codex CLI, Gemini CLI) — no API key needed\n"+
			"  proxy  run the built-in local reverse proxy and point your SDK's base URL at it for exact, real-time\n"+
			"         token counts and rate-limit headers\n"+
			"  poll   periodically call a vendor's usage/admin API (coarser, needs an admin-scoped key)\n") + "\n")

	b.WriteString(s.Subtle.Render("Configure all of this from the Settings tab — changes to appearance apply instantly;\nprovider/collector changes apply when you Save & apply."))

	return s.Panel.Width(c.Width - 2).Render(strings.TrimRight(b.String(), "\n"))
}

func padKey(s string) string {
	const w = 16
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}
