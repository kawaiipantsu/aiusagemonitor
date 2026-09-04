package model

import (
	"testing"
	"time"
)

func TestTokenUsageAddAndTotal(t *testing.T) {
	a := TokenUsage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 2, CacheWriteTokens: 1}
	b := TokenUsage{InputTokens: 1, OutputTokens: 1, Requests: 1}
	sum := a.Add(b)
	if sum.Total() != 20 {
		t.Fatalf("Total() = %d, want 20", sum.Total())
	}
	if sum.Billable() != 17 {
		t.Fatalf("Billable() = %d, want 17", sum.Billable())
	}
	if sum.Requests != 1 {
		t.Fatalf("Requests = %d, want 1", sum.Requests)
	}
}

func TestTokenUsageIsZero(t *testing.T) {
	if !(TokenUsage{}).IsZero() {
		t.Fatalf("zero-value TokenUsage should be IsZero")
	}
	if (TokenUsage{InputTokens: 1}).IsZero() {
		t.Fatalf("non-zero TokenUsage should not be IsZero")
	}
}

func TestCacheHitRatio(t *testing.T) {
	u := TokenUsage{InputTokens: 25, CacheReadTokens: 75}
	if got := u.CacheHitRatio(); got != 0.75 {
		t.Fatalf("CacheHitRatio() = %v, want 0.75", got)
	}
	if got := (TokenUsage{}).CacheHitRatio(); got != 0 {
		t.Fatalf("CacheHitRatio() on empty usage should be 0, got %v", got)
	}
}

func TestLimitFractionAndUsed(t *testing.T) {
	l := Limit{Limit: 100, Remaining: 40}
	if l.Used() != 60 {
		t.Fatalf("Used() = %v, want 60", l.Used())
	}
	if l.Fraction() != 0.6 {
		t.Fatalf("Fraction() = %v, want 0.6", l.Fraction())
	}
	// A limit of 0 must not divide by zero.
	if (Limit{}).Fraction() != 0 {
		t.Fatalf("Fraction() on zero-limit should be 0")
	}
	// Remaining > Limit (clock skew / stale data) must clamp, not go negative.
	over := Limit{Limit: 10, Remaining: 50}
	if over.Used() != 0 {
		t.Fatalf("Used() should clamp to 0 when remaining > limit, got %v", over.Used())
	}
}

func TestLimitResetIn(t *testing.T) {
	now := time.Now()
	future := Limit{ResetAt: now.Add(30 * time.Second)}
	if d := future.ResetIn(now); d <= 0 || d > 30*time.Second {
		t.Fatalf("ResetIn() = %v, want ~30s", d)
	}
	past := Limit{ResetAt: now.Add(-time.Minute)}
	if d := past.ResetIn(now); d != 0 {
		t.Fatalf("ResetIn() for a past reset should clamp to 0, got %v", d)
	}
	unset := Limit{}
	if d := unset.ResetIn(now); d != 0 {
		t.Fatalf("ResetIn() with no ResetAt should be 0, got %v", d)
	}
}

func TestParseProvider(t *testing.T) {
	cases := map[string]Provider{
		"claude":    ProviderAnthropic,
		"Anthropic": ProviderAnthropic,
		"gpt":       ProviderOpenAI,
		"gemini":    ProviderGoogle,
		"grok":      ProviderXAI,
	}
	for in, want := range cases {
		got, ok := ParseProvider(in)
		if !ok || got != want {
			t.Errorf("ParseProvider(%q) = %q,%v want %q,true", in, got, ok, want)
		}
	}
	if _, ok := ParseProvider("nonsense"); ok {
		t.Errorf("ParseProvider(nonsense) should fail")
	}
}
