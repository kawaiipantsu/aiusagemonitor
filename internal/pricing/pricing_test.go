package pricing

import (
	"testing"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

func TestRateResolutionPrefersLongestPrefix(t *testing.T) {
	tbl := NewTable(nil)
	r, ok := tbl.Rate("claude-3-5-sonnet-20241022")
	if !ok {
		t.Fatalf("expected a match for claude-3-5-sonnet-20241022")
	}
	if r.Input != 3.00 || r.Output != 15.00 {
		t.Fatalf("got rate %+v, want the claude-3-5-sonnet rate", r)
	}
}

func TestRateUnknownModel(t *testing.T) {
	tbl := NewTable(nil)
	if _, ok := tbl.Rate("some-made-up-model-xyz"); ok {
		t.Fatalf("expected no match for an unknown model")
	}
	if _, ok := tbl.Rate(""); ok {
		t.Fatalf("expected no match for an empty model id")
	}
}

func TestOverrideWinsOverDefault(t *testing.T) {
	tbl := NewTable(map[string]Rate{"gpt-4o": {Input: 1, Output: 2}})
	r, ok := tbl.Rate("gpt-4o-2024-05-13")
	if !ok {
		t.Fatalf("expected a match")
	}
	if r.Input != 1 || r.Output != 2 {
		t.Fatalf("override was not applied, got %+v", r)
	}
}

func TestSetOverrideAtRuntime(t *testing.T) {
	tbl := NewTable(nil)
	tbl.SetOverride("my-custom-model", Rate{Input: 5, Output: 10})
	r, ok := tbl.Rate("my-custom-model")
	if !ok || r.Input != 5 {
		t.Fatalf("runtime override not applied: %+v ok=%v", r, ok)
	}
}

func TestCostComputation(t *testing.T) {
	tbl := NewTable(nil)
	u := model.TokenUsage{InputTokens: 1_000_000, OutputTokens: 500_000, CacheReadTokens: 2_000_000}
	cost := tbl.Cost("claude-sonnet-4", u)
	// input: 1M * 3.00/M = 3.00 ; output: 0.5M * 15.00/M = 7.50 ; cache-read: 2M * 0.30/M = 0.60
	want := 3.00 + 7.50 + 0.60
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestCostUnknownModelIsZero(t *testing.T) {
	tbl := NewTable(nil)
	if c := tbl.Cost("totally-unknown", model.TokenUsage{InputTokens: 1000}); c != 0 {
		t.Fatalf("expected 0 cost for unknown model, got %v", c)
	}
}
