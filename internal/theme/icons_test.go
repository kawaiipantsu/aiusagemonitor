package theme

import "testing"

// TestNerdIconCodepoints guards against the Nerd Font glyphs silently
// drifting to the wrong Private Use Area codepoint on a future edit — a
// wrong PUA byte renders as some unrelated glyph rather than an obvious
// error, so pin the exact runes here.
func TestNerdIconCodepoints(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want rune
	}{
		{"Dashboard (fa-tachometer)", Nerd.Dashboard, 0xF0E4},
		{"Sessions (fa-list-ul)", Nerd.Sessions, 0xF0CA},
		{"History (fa-history)", Nerd.History, 0xF1DA},
		{"Profile (fa-area-chart)", Nerd.Profile, 0xF1FE},
		{"Waterfall (fa-random)", Nerd.Waterfall, 0xF074},
		{"Settings (fa-cog)", Nerd.Settings, 0xF013},
		{"Help (fa-question-circle)", Nerd.Help, 0xF059},
		{"Live (fa-circle)", Nerd.Live, 0xF111},
		{"Paused (fa-pause)", Nerd.Paused, 0xF04C},
	}
	for _, c := range cases {
		r := []rune(c.got)
		if len(r) != 1 {
			t.Errorf("%s: got %d runes, want exactly 1", c.name, len(r))
			continue
		}
		if r[0] != c.want {
			t.Errorf("%s: got U+%04X, want U+%04X", c.name, r[0], c.want)
		}
	}
}

func TestIconsForToggles(t *testing.T) {
	if IconsFor(false) != Plain {
		t.Errorf("IconsFor(false) should return Plain")
	}
	if IconsFor(true) != Nerd {
		t.Errorf("IconsFor(true) should return Nerd")
	}
}
