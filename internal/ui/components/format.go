// Package components holds reusable TUI rendering primitives: sparklines,
// braille line graphs, bar charts, gauges, stat cards and formatting helpers.
package components

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Compact renders a number with a K/M/B/T suffix (e.g. 12.3K, 4.1M).
func Compact(n float64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var out string
	switch {
	case n < 1000:
		out = trimZeros(fmt.Sprintf("%.0f", n))
	case n < 1_000_000:
		out = trimZeros(fmt.Sprintf("%.1f", n/1000)) + "K"
	case n < 1_000_000_000:
		out = trimZeros(fmt.Sprintf("%.2f", n/1_000_000)) + "M"
	case n < 1_000_000_000_000:
		out = trimZeros(fmt.Sprintf("%.2f", n/1_000_000_000)) + "B"
	default:
		out = trimZeros(fmt.Sprintf("%.2f", n/1_000_000_000_000)) + "T"
	}
	if neg {
		return "-" + out
	}
	return out
}

// CompactInt is Compact for int64.
func CompactInt(n int64) string { return Compact(float64(n)) }

func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// Money formats a USD amount with adaptive precision.
func Money(v float64) string {
	switch {
	case v == 0:
		return "$0.00"
	case v < 0.01:
		return fmt.Sprintf("$%.4f", v)
	case v < 100:
		return fmt.Sprintf("$%.2f", v)
	default:
		return "$" + Compact(v)
	}
}

// Duration renders a compact human duration (2h13m, 45s, 3d4h).
func Duration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// Countdown renders a short "resets in" string.
func Countdown(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return Duration(d)
}

// Clock formats a timestamp as HH:MM or HH:MM:SS depending on withSeconds.
func Clock(t time.Time, use24h, withSeconds bool) string {
	layout := "3:04 PM"
	if use24h {
		layout = "15:04"
		if withSeconds {
			layout = "15:04:05"
		}
	} else if withSeconds {
		layout = "3:04:05 PM"
	}
	return t.Format(layout)
}

// Pct formats a 0..1 fraction as an integer percentage.
func Pct(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", f*100)
}

// Pad right-pads (or truncates) s to width, respecting rune count.
func Pad(s string, width int) string {
	r := []rune(s)
	if len(r) == width {
		return s
	}
	if len(r) > width {
		if width <= 1 {
			return string(r[:max(0, width)])
		}
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

// PadLeft left-pads (or truncates) s to width.
func PadLeft(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return Pad(s, width)
	}
	return strings.Repeat(" ", width-len(r)) + s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
