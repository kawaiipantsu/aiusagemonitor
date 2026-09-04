package components

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kawaiipantsu/aiusagemonitor/internal/theme"
)

// Series is one plotted line for a Braille chart.
type Series struct {
	Values []float64
	Color  string
	Fill   bool // shade the area under the line
}

// dot bit for (x in 0..1, y in 0..3) inside a braille cell.
var brailleDot = [2][4]byte{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// Braille renders one or more series into a w×h grid of braille characters
// (2×4 sub-pixels per cell). Later series draw on top, so pass the most
// important one last. A shared max across all series keeps them comparable.
func Braille(series []Series, w, h int, th theme.Theme) string {
	if w < 2 || h < 1 {
		return ""
	}
	pixW, pixH := w*2, h*4
	cells := make([][]byte, h)
	colors := make([][]string, h)
	for i := range cells {
		cells[i] = make([]byte, w)
		colors[i] = make([]string, w)
	}

	maxV := 0.0
	for _, s := range series {
		for _, v := range s.Values {
			if v > maxV {
				maxV = v
			}
		}
	}
	if maxV <= 0 {
		maxV = 1
	}

	set := func(px, py int, col string) {
		if px < 0 || px >= pixW || py < 0 || py >= pixH {
			return
		}
		cx, cy := px/2, py/4
		cells[cy][cx] |= brailleDot[px%2][py%4]
		colors[cy][cx] = col
	}

	for _, s := range series {
		if len(s.Values) == 0 {
			continue
		}
		col := s.Color
		if col == "" {
			col = th.Primary
		}
		pts := resample(s.Values, pixW)
		prevY := math.NaN()
		for x := 0; x < pixW; x++ {
			v := pts[x]
			y := int(math.Round((1 - v/maxV) * float64(pixH-1)))
			if y < 0 {
				y = 0
			}
			if y >= pixH {
				y = pixH - 1
			}
			if s.Fill {
				for yy := y; yy < pixH; yy++ {
					set(x, yy, col)
				}
			} else {
				set(x, y, col)
				if !math.IsNaN(prevY) {
					lo, hi := y, int(prevY)
					if lo > hi {
						lo, hi = hi, lo
					}
					for yy := lo; yy <= hi; yy++ {
						set(x, yy, col)
					}
				}
			}
			prevY = float64(y)
		}
	}

	var b strings.Builder
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			bits := cells[cy][cx]
			if bits == 0 {
				b.WriteByte(' ')
				continue
			}
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(colors[cy][cx]))
			b.WriteString(st.Render(string(rune(0x2800 + int(bits)))))
		}
		if cy < h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// AxisLabels returns min/mid/max labels for a chart's left gutter, top→bottom,
// sized to h lines.
func AxisLabels(maxV float64, h int) []string {
	if h <= 0 {
		return nil
	}
	out := make([]string, h)
	for i := 0; i < h; i++ {
		frac := 1 - float64(i)/float64(maxInt(1, h-1))
		out[i] = Compact(maxV * frac)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
