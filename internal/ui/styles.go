// Package ui contains the Bubble Tea root model, view router, and the
// per-view sub-models under ui/views. Rendering primitives live in
// ui/components; palettes live in the theme package.
package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// Styles is a bundle of lipgloss styles derived from one Theme. Rebuild it
// whenever the theme changes.
type Styles struct {
	Theme theme.Theme

	App         lipgloss.Style
	TabBar      lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style
	StatusVal   lipgloss.Style

	Panel      lipgloss.Style
	PanelTitle lipgloss.Style
	PanelFocus lipgloss.Style

	Text    lipgloss.Style
	Muted   lipgloss.Style
	Subtle  lipgloss.Style
	Bold    lipgloss.Style
	Accent  lipgloss.Style
	Success lipgloss.Style
	Warning lipgloss.Style
	Danger  lipgloss.Style

	StatValue lipgloss.Style
	StatLabel lipgloss.Style

	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableSel    lipgloss.Style

	Help lipgloss.Style
}

// NewStyles builds a fresh style bundle for t.
func NewStyles(t theme.Theme) Styles {
	base := lipgloss.NewStyle().Background(lipgloss.Color(t.Bg)).Foreground(lipgloss.Color(t.Text))
	s := Styles{
		Theme: t,
		App:   base,

		TabBar:      lipgloss.NewStyle().Background(lipgloss.Color(t.Surface)).Foreground(lipgloss.Color(t.Muted)).Padding(0, 1),
		TabActive:   lipgloss.NewStyle().Background(lipgloss.Color(t.Primary)).Foreground(lipgloss.Color(t.Bg)).Bold(true).Padding(0, 2),
		TabInactive: lipgloss.NewStyle().Background(lipgloss.Color(t.Surface)).Foreground(lipgloss.Color(t.Muted)).Padding(0, 2),

		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color(t.Surface)).Foreground(lipgloss.Color(t.Muted)).Padding(0, 1),
		StatusKey: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Accent)).Bold(true),
		StatusVal: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted)),

		Panel: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Border)).
			Background(lipgloss.Color(t.Surface)).Padding(0, 1),
		PanelTitle: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Primary)).Bold(true),
		PanelFocus: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(t.Accent)).
			Background(lipgloss.Color(t.Surface)).Padding(0, 1),

		Text:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.Text)),
		Muted:   lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted)),
		Subtle:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.Subtle)),
		Bold:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.Text)).Bold(true),
		Accent:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.Accent)),
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Success)),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Warning)),
		Danger:  lipgloss.NewStyle().Foreground(lipgloss.Color(t.Danger)),

		StatValue: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Text)).Bold(true),
		StatLabel: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Subtle)),

		TableHeader: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Bg)).Background(lipgloss.Color(t.Primary)).Bold(true).Padding(0, 1),
		TableRow:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.Text)).Padding(0, 1),
		TableSel:    lipgloss.NewStyle().Foreground(lipgloss.Color(t.Bg)).Background(lipgloss.Color(t.Accent)).Padding(0, 1),

		Help: lipgloss.NewStyle().Foreground(lipgloss.Color(t.Subtle)),
	}
	return s
}

// ProviderStyle returns a style foregrounded with a provider's signature colour.
func (s Styles) ProviderStyle(key string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(s.Theme.ProviderColor(key)))
}
