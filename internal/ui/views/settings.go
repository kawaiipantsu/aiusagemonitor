package views

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
	"github.com/kawaiipantsu/aiusagemonitor/internal/ui/components"
)

// fieldKind selects how a settings row is edited.
type fieldKind int

const (
	kindEnum fieldKind = iota
	kindText
	kindSecret
	kindHeader
	kindAction
)

// field is one row of the settings form. It closes over the live config, so
// edits apply immediately in memory; only disk persistence + collector
// restart require the explicit Save action.
type field struct {
	kind    fieldKind
	label   string
	hint    string
	get     func() string  // display value (secrets are masked)
	raw     func() string  // unmasked value, used to seed the edit box
	options []string       // kindEnum
	setIdx  func(i int)    // kindEnum
	setText func(s string) // kindText/kindSecret
	action  func() string  // kindAction: returns a sentinel
}

// Settings is the configuration tab.
type Settings struct {
	cursor   int
	scroll   int
	editing  bool
	input    textinput.Model
	status   string
	statusAt time.Time
}

func NewSettings() *Settings {
	ti := textinput.New()
	ti.CharLimit = 300
	return &Settings{input: ti}
}

func boolOptions() []string { return []string{"off", "on"} }
func boolIdx(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (v *Settings) buildFields(c Ctx) []field {
	cfg := c.Cfg
	var fs []field
	fs = append(fs, field{kind: kindHeader, label: "Appearance"})
	fs = append(fs, field{
		kind: kindEnum, label: "Theme", hint: "cycles the whole UI palette live",
		get:     func() string { return theme.Get(cfg.UI.Theme).Display },
		options: themeDisplayNames(),
		setIdx:  func(i int) { cfg.UI.Theme = theme.Names()[i] },
	})
	fs = append(fs, field{
		kind: kindEnum, label: "Graph style", hint: "dashboard main chart renderer",
		get:     func() string { return cfg.UI.GraphStyle },
		options: []string{"braille", "block"},
		setIdx:  func(i int) { cfg.UI.GraphStyle = []string{"braille", "block"}[i] },
	})
	fs = append(fs, field{
		kind: kindEnum, label: "Clock", hint: "",
		get: func() string {
			if cfg.UI.Use24Hour {
				return "24h"
			}
			return "12h"
		},
		options: []string{"12h", "24h"},
		setIdx:  func(i int) { cfg.UI.Use24Hour = i == 1 },
	})
	fs = append(fs, field{
		kind: kindText, label: "Refresh rate", hint: "e.g. 1s, 500ms",
		get: func() string { return cfg.UI.RefreshRate.String() },
		setText: func(s string) {
			if d, err := time.ParseDuration(s); err == nil {
				cfg.UI.RefreshRate = d
			}
		},
	})
	fs = append(fs, field{
		kind: kindText, label: "Window (minutes)", hint: "dashboard rolling graph span",
		get: func() string { return fmt.Sprintf("%d", cfg.UI.WindowMin) },
		setText: func(s string) {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				cfg.UI.WindowMin = n
			}
		},
	})
	fs = append(fs, field{
		kind: kindEnum, label: "Demo data", hint: "adds synthetic traffic on top of your real collectors",
		get:     func() string { return boolOptions()[boolIdx(cfg.Demo)] },
		options: boolOptions(),
		setIdx:  func(i int) { cfg.Demo = i == 1 },
	})

	for _, p := range model.AllProviders {
		pc := cfg.Providers[p]
		fs = append(fs, field{kind: kindHeader, label: p.Title()})
		fs = append(fs, field{
			kind: kindEnum, label: "Enabled",
			get:     func() string { return boolOptions()[boolIdx(pc.Enabled)] },
			options: boolOptions(),
			setIdx:  func(i int) { pc.Enabled = i == 1 },
		})
		fs = append(fs, field{
			kind: kindEnum, label: "Collector", hint: collectorHint(p),
			get:     func() string { return string(pc.Collector) },
			options: collectorOptions(),
			setIdx:  func(i int) { pc.Collector = config.AllCollectorKinds[i] },
		})
		fs = append(fs, field{
			kind: kindSecret, label: "API key", hint: "supports ${ENV_VAR}",
			get:     func() string { return maskSecret(pc.APIKey) },
			raw:     func() string { return pc.APIKey },
			setText: func(s string) { pc.APIKey = s },
		})
		fs = append(fs, field{
			kind: kindSecret, label: "Admin key", hint: "needed for usage-API polling",
			get:     func() string { return maskSecret(pc.AdminKey) },
			raw:     func() string { return pc.AdminKey },
			setText: func(s string) { pc.AdminKey = s },
		})
		fs = append(fs, field{
			kind: kindText, label: "Base URL", hint: "override upstream (self-hosted / Azure)",
			get:     func() string { return pc.BaseURL },
			setText: func(s string) { pc.BaseURL = s },
		})
	}

	fs = append(fs, field{kind: kindHeader, label: "Local proxy"})
	fs = append(fs, field{
		kind: kindEnum, label: "Always on", hint: "run the capture proxy even if no provider selects it",
		get:     func() string { return boolOptions()[boolIdx(cfg.Proxy.Enabled)] },
		options: boolOptions(),
		setIdx:  func(i int) { cfg.Proxy.Enabled = i == 1 },
	})
	fs = append(fs, field{
		kind: kindText, label: "Listen address", hint: "point your SDK base_url at http://<addr>/<provider>",
		get:     func() string { return cfg.Proxy.Listen },
		setText: func(s string) { cfg.Proxy.Listen = s },
	})

	fs = append(fs, field{kind: kindHeader, label: "Polling & storage"})
	fs = append(fs, field{
		kind: kindText, label: "Poll interval", hint: "e.g. 5m",
		get: func() string { return cfg.Poll.Interval.String() },
		setText: func(s string) {
			if d, err := time.ParseDuration(s); err == nil && d >= time.Minute {
				cfg.Poll.Interval = d
			}
		},
	})
	fs = append(fs, field{
		kind: kindText, label: "History DB path",
		get:     func() string { return cfg.Storage.Path },
		setText: func(s string) { cfg.Storage.Path = s },
	})
	fs = append(fs, field{
		kind: kindText, label: "Retention (days)", hint: "0 = keep forever",
		get: func() string { return fmt.Sprintf("%d", cfg.Storage.RetentionDays) },
		setText: func(s string) {
			if n, err := strconv.Atoi(s); err == nil && n >= 0 {
				cfg.Storage.RetentionDays = n
			}
		},
	})

	fs = append(fs, field{kind: kindHeader, label: "Actions"})
	fs = append(fs, field{
		kind: kindAction, label: "Save & apply (restarts collectors)",
		action: func() string { return "__save__" },
	})
	fs = append(fs, field{
		kind: kindAction, label: "Reload from disk (discard changes)",
		action: func() string { return "__reload__" },
	})
	return fs
}

func themeDisplayNames() []string {
	names := theme.Names()
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = theme.Get(n).Display
	}
	return out
}

func collectorOptions() []string {
	out := make([]string, len(config.AllCollectorKinds))
	for i, k := range config.AllCollectorKinds {
		out[i] = string(k)
	}
	return out
}

func collectorHint(p model.Provider) string {
	switch p {
	case model.ProviderAnthropic:
		return "logs = tail Claude Code transcripts"
	case model.ProviderOpenAI:
		return "logs = tail Codex CLI rollouts"
	case model.ProviderGoogle:
		return "logs = tail Gemini CLI (experimental)"
	case model.ProviderXAI:
		return "no local CLI transcripts; use proxy or poll"
	}
	return ""
}

func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(not set)"
	}
	if strings.HasPrefix(s, "$") {
		return s // env var reference, safe to show
	}
	if len(s) <= 8 {
		return "••••••••"
	}
	return s[:3] + strings.Repeat("•", 6) + s[len(s)-4:]
}

// SaveResult is emitted (via a tea.Cmd) when the user asks to persist config.
type SaveResult struct {
	Applied bool
	Err     error
}

// Action is returned by Update when the user triggers save/reload; app.go
// performs the actual side effect (it owns the engine + config path).
type Action int

const (
	ActionNone Action = iota
	ActionSave
	ActionReload
)

func (v *Settings) Update(msg tea.Msg, c Ctx) (Action, tea.Cmd) {
	fs := v.buildFields(c)
	if v.editing {
		switch m := msg.(type) {
		case tea.KeyMsg:
			switch m.String() {
			case "enter":
				fs[v.cursor].setText(v.input.Value())
				v.editing = false
				v.flash("saved locally — press Save & apply to persist")
				return ActionNone, nil
			case "esc":
				v.editing = false
				return ActionNone, nil
			}
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(msg)
		return ActionNone, cmd
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return ActionNone, nil
	}
	switch key.String() {
	case "up", "k":
		v.move(fs, -1)
	case "down", "j":
		v.move(fs, 1)
	case "left", "h":
		v.cycle(fs, -1)
	case "right", "l":
		v.cycle(fs, 1)
	case "enter", " ":
		return v.activate(fs, c)
	}
	return ActionNone, nil
}

func (v *Settings) move(fs []field, d int) {
	n := len(fs)
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		v.cursor = ((v.cursor+d)%n + n) % n
		if fs[v.cursor].kind != kindHeader {
			return
		}
	}
}

func (v *Settings) cycle(fs []field, d int) {
	if v.cursor < 0 || v.cursor >= len(fs) {
		return
	}
	f := fs[v.cursor]
	if f.kind != kindEnum {
		return
	}
	cur := indexOf(f.options, f.get())
	n := len(f.options)
	cur = ((cur+d)%n + n) % n
	f.setIdx(cur)
}

func (v *Settings) activate(fs []field, c Ctx) (Action, tea.Cmd) {
	if v.cursor < 0 || v.cursor >= len(fs) {
		return ActionNone, nil
	}
	f := fs[v.cursor]
	switch f.kind {
	case kindEnum:
		v.cycle(fs, 1)
	case kindText, kindSecret:
		v.editing = true
		if f.raw != nil {
			v.input.SetValue(f.raw())
		} else {
			v.input.SetValue(f.get())
		}
		v.input.Focus()
		v.input.CursorEnd()
	case kindAction:
		switch f.action() {
		case "__save__":
			return ActionSave, nil
		case "__reload__":
			return ActionReload, nil
		}
	}
	return ActionNone, nil
}

func (v *Settings) flash(msg string) {
	v.status = msg
	v.statusAt = time.Now()
}

// Editing reports whether a text field is currently capturing raw keystrokes
// (the app must not intercept global shortcuts while this is true).
func (v *Settings) Editing() bool { return v.editing }

// Notify sets the status line from outside (used by app after Save/Reload).
func (v *Settings) Notify(msg string) { v.flash(msg) }

func (v *Settings) View(c Ctx) string {
	s := c.Styles
	fs := v.buildFields(c)
	if v.cursor >= 0 && v.cursor < len(fs) && fs[v.cursor].kind == kindHeader {
		v.move(fs, 1) // self-heal off the initial/zero-value cursor position
	}
	labelW := 26
	valueW := clampInt(c.Width-labelW-20, 16, 60)

	// Render every field to a line, remembering which field index (if any)
	// produced it, so the viewport below can scroll while keeping the cursor
	// visible without splitting a field's own rendering logic in two.
	var lines []string
	cursorLine := 0
	for i, f := range fs {
		if f.kind == kindHeader {
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, s.PanelTitle.Render("── "+f.label+" "))
			continue
		}
		cursor := "  "
		lineStyle := s.Text
		if i == v.cursor {
			cursor = s.Accent.Render("▸ ")
			lineStyle = s.Bold
			cursorLine = len(lines)
		}
		label := lineStyle.Render(components.Pad(f.label, labelW))
		var value string
		if v.editing && i == v.cursor {
			value = v.input.View()
		} else {
			switch f.kind {
			case kindAction:
				value = s.Accent.Render("[ " + f.label + " ]")
				label = ""
			default:
				value = s.StatValue.Render(components.Pad(f.get(), valueW))
			}
		}
		hint := ""
		if f.hint != "" {
			hint = "  " + s.Subtle.Render(f.hint)
		}
		lines = append(lines, cursor+label+" "+value+hint)
	}

	visible := clampInt(c.ContentHeight()-4, 5, len(lines))
	if visible > len(lines) {
		visible = len(lines)
	}
	if cursorLine < v.scroll {
		v.scroll = cursorLine
	} else if cursorLine >= v.scroll+visible {
		v.scroll = cursorLine - visible + 1
	}
	if v.scroll > len(lines)-visible {
		v.scroll = len(lines) - visible
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
	end := v.scroll + visible
	if end > len(lines) {
		end = len(lines)
	}
	shown := lines[v.scroll:end]
	scrollHint := ""
	if len(lines) > visible {
		scrollHint = fmt.Sprintf("  (%d/%d)", cursorLine+1, len(lines))
	}

	body := s.Panel.Width(c.Width - 2).Render(strings.Join(shown, "\n"))
	help := s.Help.Render("↑/↓ move · ←/→ change · enter edit/activate · esc cancel edit" + scrollHint)
	status := ""
	if v.status != "" && time.Since(v.statusAt) < 6*time.Second {
		status = s.Success.Render(v.status)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, help, status)
}

func indexOf(opts []string, val string) int {
	for i, o := range opts {
		if o == val {
			return i
		}
	}
	return 0
}
