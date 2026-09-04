package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kawaiipantsu/aiusagemonitor/internal/version"
)

// Help is the static reference tab. Its content can run longer than a short
// terminal, so it keeps its own scroll offset rather than relying on the
// app's blunt bottom-truncation.
type Help struct {
	scroll int
}

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

var helpKeys = []helpRow{
	{"↑/↓, j/k", "scroll this page"},
	{"pgup/pgdn", "scroll a page at a time"},
}

// Update lets Help scroll independently of the app's global keybindings.
func (h *Help) Update(msg tea.Msg, c Ctx) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	lines := len(h.lines(c))
	visible := c.ContentHeight() - 3
	maxScroll := lines - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	switch key.String() {
	case "up", "k":
		h.scroll--
	case "down", "j":
		h.scroll++
	case "pgup":
		h.scroll -= visible
	case "pgdown", "pgdn":
		h.scroll += visible
	case "home", "g":
		h.scroll = 0
	case "end", "G":
		h.scroll = maxScroll
	default:
		return
	}
	h.scroll = clampInt(h.scroll, 0, maxScroll)
}

// lines renders the full help text as individual lines (no border), so both
// View and Update can measure/window it consistently.
func (h *Help) lines(c Ctx) []string {
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
	section("Help", helpKeys)

	b.WriteString(s.Accent.Render("Waterfall") + "\n")
	b.WriteString(s.Muted.Render(
		"  A live rolling matrix: one row per busiest model, plus fixed \"Agent\" (subagent/Task-tool\n"+
			"  turns) and \"Background\" (usage the poll collectors picked up outside any interactive\n"+
			"  request) rows, over the same window as the Dashboard graph — watch a tool shift between\n"+
			"  models, or a subagent kick in, as it happens.\n") + "\n")

	b.WriteString(s.Accent.Render("Data sources") + "\n")
	b.WriteString(s.Muted.Render(
		"  logs   tail local AI-CLI session transcripts (Claude Code, Codex CLI, Gemini CLI) — no API key needed\n"+
			"  proxy  run the built-in local reverse proxy and point your SDK's base URL at it for exact, real-time\n"+
			"         token counts and rate-limit headers\n"+
			"  poll   periodically call a vendor's usage/admin API (coarser, needs an admin-scoped key)\n") + "\n")

	b.WriteString(s.Accent.Render("Claude account status") + "\n")
	b.WriteString(s.Muted.Render(
		"  When Claude's collector is \"logs\", the Dashboard also shows your Claude Code login type\n"+
			"  (Claude.ai subscription vs. a pay-as-you-go console API key) and, for a subscription login,\n"+
			"  the rolling 5-hour session and 7-day weekly usage allowance in both the top bar and the\n"+
			"  Rate limits box — including \"LOW PRIORITY\" once the session window is spent. Read from\n"+
			"  ~/.claude.json; only plan/quota metadata is read, never your OAuth tokens, email or name.\n"+
			"  This is Claude Code's OWN cached snapshot, refreshed only when the CLI itself decides to —\n"+
			"  not on every request — so it can lag well behind what /status or claude.ai shows live; the\n"+
			"  \"(Claude Code cache, Xm old)\" tag next to it is exactly how far behind it is right now.\n"+
			"  Turn it off in Settings ▸ Claude ▸ Account status.\n") + "\n")

	b.WriteString(s.Accent.Render("Appearance") + "\n")
	b.WriteString(s.Muted.Render(
		"  Settings ▸ Appearance ▸ Nerd Font icons swaps the tab bar and status bar to Nerd Font\n"+
			"  glyphs — off by default, since they render as boxes/tofu without a patched Nerd Font\n"+
			"  installed in your terminal.\n") + "\n")

	b.WriteString(s.Subtle.Render("Configure all of this from the Settings tab — changes to appearance apply instantly;\nprovider/collector changes apply when you Save & apply."))

	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (h *Help) View(c Ctx) string {
	s := c.Styles
	lines := h.lines(c)
	// Reserve 2 rows for the panel border and 1 for the scroll-position hint
	// below it, so the whole thing fits inside the app's own height budget
	// instead of getting blunt-truncated by it.
	visible := clampInt(c.ContentHeight()-3, 5, len(lines))
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	h.scroll = clampInt(h.scroll, 0, maxScroll)
	end := h.scroll + visible
	if end > len(lines) {
		end = len(lines)
	}
	body := s.Panel.Width(c.Width - 2).Render(strings.Join(lines[h.scroll:end], "\n"))

	if len(lines) <= visible {
		return body
	}
	scrollHint := s.Help.Render(fmt.Sprintf("↑/↓ scroll · line %d-%d of %d", h.scroll+1, end, len(lines)))
	return lipgloss.JoinVertical(lipgloss.Left, body, scrollHint)
}

func padKey(s string) string {
	const w = 16
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}
