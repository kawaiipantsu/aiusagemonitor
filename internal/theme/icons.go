package theme

// Icons is the small glyph set used in chrome — the tab bar and status bar.
// The plain set is pure Unicode and safe everywhere; the Nerd Font set only
// renders correctly when the terminal's font is a Nerd Font variant, so it
// is opt-in (Settings ▸ Appearance ▸ Nerd Font icons, off by default).
type Icons struct {
	Dashboard string
	Sessions  string
	History   string
	Profile   string
	Waterfall string
	Settings  string
	Help      string
	Live      string
	Paused    string
}

// Plain is the default, font-independent icon set.
var Plain = Icons{
	Dashboard: "▦",
	Sessions:  "☰",
	History:   "◷",
	Profile:   "▤",
	Waterfall: "≈",
	Settings:  "⚙",
	Help:      "?",
	Live:      "●",
	Paused:    "⏸",
}

// Nerd is the Nerd Font icon set. These are classic Font Awesome 4
// codepoints (U+F000-F2FF) that Nerd Fonts preserves verbatim — the same
// set long used by powerline/vim-airline-style status lines. Written as
// \u escapes rather than literal glyphs, so the exact codepoint is
// verifiable in source rather than depending on an editor round-tripping a
// private-use character correctly.
var Nerd = Icons{
	Dashboard: "", // fa-tachometer
	Sessions:  "", // fa-list-ul
	History:   "", // fa-history
	Profile:   "", // fa-area-chart
	Waterfall: "", // fa-random
	Settings:  "", // fa-cog
	Help:      "", // fa-question-circle
	Live:      "", // fa-circle
	Paused:    "", // fa-pause
}

// IconsFor returns the Nerd or Plain set.
func IconsFor(nerd bool) Icons {
	if nerd {
		return Nerd
	}
	return Plain
}
