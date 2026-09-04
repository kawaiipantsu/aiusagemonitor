// Package theme defines colour palettes for the TUI and a registry of
// built-in themes.
package theme

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a named palette. Every field is a hex colour string.
type Theme struct {
	Name    string
	Display string
	Dark    bool

	Bg      string // app background
	Surface string // panel background
	Overlay string // nested panel / selected row
	Text    string // primary foreground
	Muted   string // secondary foreground
	Subtle  string // tertiary / hints
	Border  string // panel borders

	Primary   string // brand / active tab
	Secondary string
	Accent    string

	Success string
	Warning string
	Danger  string

	// Gradient is a low→high ramp used by sparklines and heatmaps.
	Gradient []string
	// Providers maps a provider key to a signature colour for charts/legends.
	Providers map[string]string
}

// C converts a palette hex into a lipgloss colour.
func (t Theme) C(hex string) lipgloss.Color { return lipgloss.Color(hex) }

// GradientAt samples the gradient at f in [0,1].
func (t Theme) GradientAt(f float64) string {
	if len(t.Gradient) == 0 {
		return t.Primary
	}
	if f <= 0 {
		return t.Gradient[0]
	}
	if f >= 1 {
		return t.Gradient[len(t.Gradient)-1]
	}
	idx := int(f * float64(len(t.Gradient)-1))
	return t.Gradient[idx]
}

// ProviderColor returns the signature colour for a provider key, falling back
// to the accent colour.
func (t Theme) ProviderColor(key string) string {
	if c, ok := t.Providers[strings.ToLower(key)]; ok {
		return c
	}
	return t.Accent
}

// GaugeColor picks success/warning/danger based on a 0..1 fill fraction.
func (t Theme) GaugeColor(frac float64) string {
	switch {
	case frac >= 0.9:
		return t.Danger
	case frac >= 0.7:
		return t.Warning
	default:
		return t.Success
	}
}

var registry = map[string]Theme{}

func register(t Theme) { registry[t.Name] = t }

// Get returns the named theme, or the default ("midnight") if unknown.
func Get(name string) Theme {
	if t, ok := registry[strings.ToLower(strings.TrimSpace(name))]; ok {
		return t
	}
	return registry["midnight"]
}

// Names returns all registered theme names, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Next returns the theme name after the given one (wrapping), for hotkey
// cycling.
func Next(current string) string {
	names := Names()
	for i, n := range names {
		if n == current {
			return names[(i+1)%len(names)]
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "midnight"
}

// Prev returns the theme name before the given one (wrapping).
func Prev(current string) string {
	names := Names()
	for i, n := range names {
		if n == current {
			return names[(i-1+len(names))%len(names)]
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "midnight"
}
