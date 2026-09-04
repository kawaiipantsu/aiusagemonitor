// Package app hosts the Bubble Tea root model: tab routing, global
// keybindings, window-resize handling and the bridge to the engine's live
// DashboardState stream.
package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/engine"
	"github.com/kawaiipantsu/aiusagemonitor/internal/store"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/views"
)

type tab int

const (
	tabDashboard tab = iota
	tabSessions
	tabHistory
	tabProfile
	tabWaterfall
	tabSettings
	tabHelp
)

var tabNames = []string{"Dashboard", "Sessions", "History", "Profile", "Waterfall", "Settings", "Help"}

// Model is the Bubble Tea root model.
type Model struct {
	cfg *config.Config
	st  *store.Store
	eng *engine.Engine
	sub chan *engine.DashboardState

	ctx    context.Context
	width  int
	height int
	active tab
	state  *engine.DashboardState
	paused bool

	dashboard *views.Dashboard
	sessions  *views.Sessions
	history   *views.History
	profile   *views.Profile
	waterfall *views.Waterfall
	settings  *views.Settings
	help      *views.Help

	quitting bool
}

// New builds the root model. The engine must already be started by the
// caller; Model only subscribes to it.
func New(ctx context.Context, cfg *config.Config, st *store.Store, eng *engine.Engine) *Model {
	m := &Model{
		ctx:       ctx,
		cfg:       cfg,
		st:        st,
		eng:       eng,
		sub:       eng.Subscribe(),
		dashboard: &views.Dashboard{},
		sessions:  views.NewSessions(),
		history:   &views.History{},
		profile:   &views.Profile{},
		waterfall: &views.Waterfall{},
		settings:  views.NewSettings(),
		help:      &views.Help{},
	}
	switch strings.ToLower(cfg.UI.StartView) {
	case "sessions":
		m.active = tabSessions
	case "history":
		m.active = tabHistory
	case "profile":
		m.active = tabProfile
	case "waterfall":
		m.active = tabWaterfall
	case "settings":
		m.active = tabSettings
	case "help":
		m.active = tabHelp
	}
	return m
}

type stateMsg struct{ s *engine.DashboardState }
type tickMsg time.Time

func waitForState(sub chan *engine.DashboardState) tea.Cmd {
	return func() tea.Msg {
		s, ok := <-sub
		if !ok {
			return nil
		}
		return stateMsg{s}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(waitForState(m.sub), tickCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sessions.SetSize(m.width, m.height)
		return m, nil
	case stateMsg:
		if !m.paused {
			m.state = msg.s
		}
		return m, waitForState(m.sub)
	case tickMsg:
		return m, tickCmd()
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While editing a settings field, every keystroke belongs to the text box.
	if m.active == tabSettings && m.settings.Editing() {
		action, cmd := m.settings.Update(msg, m.viewCtx())
		m.applyAction(action)
		return m, cmd
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "tab":
		m.active = tab((int(m.active) + 1) % len(tabNames))
		return m, nil
	case "shift+tab":
		m.active = tab((int(m.active) - 1 + len(tabNames)) % len(tabNames))
		return m, nil
	case "t":
		m.cfg.UI.Theme = theme.Next(m.cfg.UI.Theme)
		return m, nil
	case "T":
		m.cfg.UI.Theme = theme.Prev(m.cfg.UI.Theme)
		return m, nil
	case "p":
		m.paused = !m.paused
		return m, nil
	case "r":
		m.state = m.eng.Snapshot()
		return m, nil
	case "?":
		m.active = tabHelp
		return m, nil
	}
	if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(tabNames) {
		m.active = tab(n - 1)
		return m, nil
	}

	switch m.active {
	case tabDashboard:
		if m.state != nil {
			switch msg.String() {
			case "[":
				m.dashboard.CycleFilter(m.state, false)
			case "]":
				m.dashboard.CycleFilter(m.state, true)
			}
		}
	case tabSessions:
		return m, m.sessions.Update(msg)
	case tabSettings:
		action, cmd := m.settings.Update(msg, m.viewCtx())
		m.applyAction(action)
		return m, cmd
	case tabHistory:
		switch msg.String() {
		case "left", "h":
			m.history.Cycle(false)
		case "right", "l":
			m.history.Cycle(true)
		}
	case tabHelp:
		m.help.Update(msg, m.viewCtx())
	}
	return m, nil
}

func (m *Model) applyAction(a views.Action) {
	switch a {
	case views.ActionSave:
		if err := m.cfg.Save(); err != nil {
			m.settings.Notify("save failed: " + err.Error())
			return
		}
		m.eng.Reconfigure(m.ctx, m.cfg)
		m.settings.Notify("saved to " + m.cfg.Path() + " and applied")
	case views.ActionReload:
		fresh, err := config.Load(m.cfg.Path())
		if err != nil {
			m.settings.Notify("reload failed: " + err.Error())
			return
		}
		m.cfg = fresh
		m.eng.Reconfigure(m.ctx, m.cfg)
		m.settings.Notify("reloaded from disk")
	}
}

func (m *Model) viewCtx() views.Ctx {
	return views.Ctx{
		Styles: ui.NewStyles(theme.Get(m.cfg.UI.Theme)),
		State:  m.state,
		Store:  m.st,
		Cfg:    m.cfg,
		Eng:    m.eng,
		Width:  m.width,
		Height: m.height,
		Now:    time.Now(),
	}
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "starting aiusagemonitor…"
	}
	c := m.viewCtx()
	s := c.Styles

	tabBar := m.renderTabBar(s)
	var content string
	switch m.active {
	case tabDashboard:
		content = m.dashboard.View(c)
	case tabSessions:
		content = m.sessions.View(c)
	case tabHistory:
		content = m.history.View(c)
	case tabProfile:
		content = m.profile.View(c)
	case tabWaterfall:
		content = m.waterfall.View(c)
	case tabSettings:
		content = m.settings.View(c)
	case tabHelp:
		content = m.help.View(c)
	}
	content = clipHeight(content, c.ContentHeight())
	status := m.renderStatusBar(s)
	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status)
}

// tabIcons mirrors tabNames' order.
func tabIcons(ic theme.Icons) []string {
	return []string{ic.Dashboard, ic.Sessions, ic.History, ic.Profile, ic.Waterfall, ic.Settings, ic.Help}
}

func (m *Model) renderTabBar(s ui.Styles) string {
	icons := tabIcons(theme.IconsFor(m.cfg.UI.NerdFont))
	var b strings.Builder
	for i, name := range tabNames {
		icon := ""
		if i < len(icons) {
			icon = icons[i] + " "
		}
		label := fmt.Sprintf("%d %s%s", i+1, icon, name)
		if tab(i) == m.active {
			b.WriteString(s.TabActive.Render(label))
		} else {
			b.WriteString(s.TabInactive.Render(label))
		}
	}
	bar := lipgloss.NewStyle().Background(lipgloss.Color(s.Theme.Surface)).Width(m.width).Render(b.String())
	return bar
}

func (m *Model) renderStatusBar(s ui.Styles) string {
	ic := theme.IconsFor(m.cfg.UI.NerdFont)
	left := "aiusagemonitor"
	if m.paused {
		left = s.Warning.Render(ic.Paused+" PAUSED") + "  " + left
	} else {
		left = s.Success.Render(ic.Live+" live") + "  " + left
	}
	if m.state != nil && len(m.state.Collectors) > 0 {
		left += s.Subtle.Render("  ·  " + strings.Join(m.state.Collectors, ", "))
	}
	right := time.Now().Format(clockLayout(m.cfg.UI.Use24Hour)) + "  ·  ? help  ·  q quit"
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return s.StatusBar.Width(m.width).Render(line)
}

func clockLayout(use24h bool) string {
	if use24h {
		return "15:04:05"
	}
	return "3:04:05 PM"
}

// clipHeight caps s at maxLines lines, marking truncation so a terminal
// that's too short doesn't get garbled full-screen output.
func clipHeight(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		if len(lines) < maxLines {
			lines = append(lines, make([]string, maxLines-len(lines))...)
		}
		return strings.Join(lines, "\n")
	}
	lines = lines[:maxLines-1]
	lines = append(lines, "…")
	return strings.Join(lines, "\n")
}
