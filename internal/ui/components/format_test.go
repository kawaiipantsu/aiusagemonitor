package components

import (
	"testing"
	"time"
)

func TestCompact(t *testing.T) {
	cases := map[float64]string{
		0:             "0",
		999:           "999",
		1500:          "1.5K",
		1_200_000:     "1.2M",
		2_500_000_000: "2.5B",
	}
	for in, want := range cases {
		if got := Compact(in); got != want {
			t.Errorf("Compact(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestMoney(t *testing.T) {
	if got := Money(0); got != "$0.00" {
		t.Errorf("Money(0) = %q", got)
	}
	if got := Money(0.001234); got != "$0.0012" {
		t.Errorf("Money(0.001234) = %q", got)
	}
	if got := Money(12.5); got != "$12.50" {
		t.Errorf("Money(12.5) = %q", got)
	}
}

func TestDuration(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "0s",
		45 * time.Second: "45s",
		90 * time.Second: "1m30s",
		75 * time.Minute: "1h15m",
		50 * time.Hour:   "2d2h",
	}
	for in, want := range cases {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPct(t *testing.T) {
	if got := Pct(0.5); got != "50%" {
		t.Errorf("Pct(0.5) = %q", got)
	}
	if got := Pct(1); got != "100%" {
		t.Errorf("Pct(1) = %q", got)
	}
}

func TestPadTruncatesWithEllipsis(t *testing.T) {
	if got := Pad("hello", 10); got != "hello     " {
		t.Errorf("Pad short string = %q", got)
	}
	if got := Pad("hello world", 5); got != "hell…" {
		t.Errorf("Pad long string = %q", got)
	}
}

func TestResampleShrinkAndGrow(t *testing.T) {
	// Shrinking averages chunks.
	in := []float64{0, 10, 0, 10, 0, 10}
	out := resample(in, 3)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}

	// Growing must interpolate across the full width, not bunch real values
	// at one edge and zero-pad the rest (regression: this used to leave a
	// long dead run of zeros before any real data appeared).
	small := []float64{0, 100}
	grown := resample(small, 10)
	if len(grown) != 10 {
		t.Fatalf("len = %d, want 10", len(grown))
	}
	if grown[0] != 0 {
		t.Fatalf("grown[0] = %v, want 0 (first real sample)", grown[0])
	}
	if grown[len(grown)-1] != 100 {
		t.Fatalf("grown[last] = %v, want 100 (last real sample)", grown[len(grown)-1])
	}
	// The midpoint should be interpolated, not a hold-over zero.
	mid := grown[len(grown)/2]
	if mid <= 0 {
		t.Fatalf("grown midpoint = %v, want a positive interpolated value", mid)
	}
}
