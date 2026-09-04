package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders values as a single row of block glyphs, coloured along the
// theme gradient by relative height. width is the number of cells; the series
// is resampled (last-N with averaging) to fit.
func Sparkline(values []float64, width int, th theme.Theme) string {
	if width <= 0 {
		return ""
	}
	pts := resample(values, width)
	maxV := 0.0
	for _, v := range pts {
		if v > maxV {
			maxV = v
		}
	}
	var b strings.Builder
	for _, v := range pts {
		frac := 0.0
		if maxV > 0 {
			frac = v / maxV
		}
		idx := int(frac*float64(len(blocks)-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(th.GradientAt(frac)))
		b.WriteString(st.Render(string(blocks[idx])))
	}
	return b.String()
}

// resample reduces or stretches values to exactly n points. Shrinking
// averages each chunk; growing linearly interpolates across the full width
// so a fixed, time-indexed series (e.g. one point per minute) keeps every
// point at its correct relative position rather than bunching real data at
// one edge and zero-padding the rest.
func resample(values []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	if len(values) == 0 {
		return make([]float64, n)
	}
	if len(values) == n {
		return values
	}
	if len(values) == 1 {
		out := make([]float64, n)
		for i := range out {
			out[i] = values[0]
		}
		return out
	}
	if len(values) < n {
		out := make([]float64, n)
		last := float64(len(values) - 1)
		for i := 0; i < n; i++ {
			pos := float64(i) * last / float64(n-1)
			lo := int(pos)
			hi := lo + 1
			if hi > int(last) {
				hi = int(last)
			}
			frac := pos - float64(lo)
			out[i] = values[lo]*(1-frac) + values[hi]*frac
		}
		return out
	}
	out := make([]float64, n)
	ratio := float64(len(values)) / float64(n)
	for i := 0; i < n; i++ {
		start := int(float64(i) * ratio)
		end := int(float64(i+1) * ratio)
		if end <= start {
			end = start + 1
		}
		if end > len(values) {
			end = len(values)
		}
		sum := 0.0
		for _, v := range values[start:end] {
			sum += v
		}
		out[i] = sum / float64(end-start)
	}
	return out
}

// MiniBars renders values as fixed two-cell-wide vertical bars — a chunkier
// alternative to Sparkline for small counts.
func MiniBars(values []float64, th theme.Theme) string {
	maxV := 0.0
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		frac := 0.0
		if maxV > 0 {
			frac = v / maxV
		}
		idx := int(frac*float64(len(blocks)-1) + 0.5)
		if idx < 1 {
			idx = 1
		}
		st := lipgloss.NewStyle().Foreground(lipgloss.Color(th.GradientAt(frac)))
		b.WriteString(st.Render(strings.Repeat(string(blocks[idx]), 2)))
	}
	return b.String()
}
