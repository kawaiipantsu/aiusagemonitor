// Package model defines the vendor-neutral domain types shared across
// collectors, the aggregation engine, storage and the TUI.
package model

import (
	"strings"
	"time"
)

// Provider identifies an AI vendor.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGoogle    Provider = "google"
	ProviderXAI       Provider = "xai"
)

// AllProviders is the canonical ordered list.
var AllProviders = []Provider{ProviderOpenAI, ProviderAnthropic, ProviderGoogle, ProviderXAI}

// Title returns a display name for the provider.
func (p Provider) Title() string {
	switch p {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Claude"
	case ProviderGoogle:
		return "Google"
	case ProviderXAI:
		return "xAI"
	default:
		return strings.Title(string(p)) //nolint:staticcheck
	}
}

// Valid reports whether p is one of the known providers.
func (p Provider) Valid() bool {
	for _, x := range AllProviders {
		if x == p {
			return true
		}
	}
	return false
}

// ParseProvider is a forgiving parser for user / header input.
func ParseProvider(s string) (Provider, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "openai", "oai", "gpt":
		return ProviderOpenAI, true
	case "anthropic", "claude":
		return ProviderAnthropic, true
	case "google", "gemini", "vertex", "googleai":
		return ProviderGoogle, true
	case "xai", "grok", "x":
		return ProviderXAI, true
	default:
		return "", false
	}
}

// TokenUsage is an additive bundle of token counters. Depending on context it
// may represent a single request delta or a rolled-up total.
type TokenUsage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	Requests         int64 `json:"requests"`
}

// Total returns every token counted once.
func (t TokenUsage) Total() int64 {
	return t.InputTokens + t.OutputTokens + t.CacheReadTokens + t.CacheWriteTokens
}

// Billable returns input+output (cache read is usually discounted, but callers
// that need exact cost should use the pricing package instead).
func (t TokenUsage) Billable() int64 { return t.InputTokens + t.OutputTokens }

// Add returns the element-wise sum.
func (t TokenUsage) Add(o TokenUsage) TokenUsage {
	return TokenUsage{
		InputTokens:      t.InputTokens + o.InputTokens,
		OutputTokens:     t.OutputTokens + o.OutputTokens,
		CacheReadTokens:  t.CacheReadTokens + o.CacheReadTokens,
		CacheWriteTokens: t.CacheWriteTokens + o.CacheWriteTokens,
		Requests:         t.Requests + o.Requests,
	}
}

// IsZero reports whether nothing was counted.
func (t TokenUsage) IsZero() bool { return t == TokenUsage{} }

// CacheHitRatio is cache-read tokens over all input-side tokens (0..1).
func (t TokenUsage) CacheHitRatio() float64 {
	denom := t.InputTokens + t.CacheReadTokens + t.CacheWriteTokens
	if denom == 0 {
		return 0
	}
	return float64(t.CacheReadTokens) / float64(denom)
}

// LimitKind names the resource a Limit constrains.
type LimitKind string

const (
	LimitRequests     LimitKind = "requests"
	LimitTokens       LimitKind = "tokens"
	LimitInputTokens  LimitKind = "input_tokens"
	LimitOutputTokens LimitKind = "output_tokens"
	LimitCostUSD      LimitKind = "cost_usd"
	LimitConcurrent   LimitKind = "concurrent"
)

// Short returns a compact label for gauges.
func (k LimitKind) Short() string {
	switch k {
	case LimitRequests:
		return "req"
	case LimitTokens:
		return "tok"
	case LimitInputTokens:
		return "in"
	case LimitOutputTokens:
		return "out"
	case LimitCostUSD:
		return "usd"
	case LimitConcurrent:
		return "conc"
	default:
		return string(k)
	}
}

// Limit is a single rate-limit / quota bucket reported by a provider.
type Limit struct {
	Provider  Provider  `json:"provider"`
	Kind      LimitKind `json:"kind"`
	Model     string    `json:"model,omitempty"`
	Window    string    `json:"window,omitempty"` // "1m", "1d", "1mo"
	Limit     float64   `json:"limit"`
	Remaining float64   `json:"remaining"`
	ResetAt   time.Time `json:"reset_at,omitempty"`
	Observed  time.Time `json:"observed"`
}

// Used returns how much of the bucket is consumed.
func (l Limit) Used() float64 {
	u := l.Limit - l.Remaining
	if u < 0 {
		return 0
	}
	return u
}

// Fraction returns consumed / limit clamped to [0,1].
func (l Limit) Fraction() float64 {
	if l.Limit <= 0 {
		return 0
	}
	f := l.Used() / l.Limit
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// ResetIn returns the duration until the bucket resets (0 if unknown/past).
func (l Limit) ResetIn(now time.Time) time.Duration {
	if l.ResetAt.IsZero() {
		return 0
	}
	d := l.ResetAt.Sub(now)
	if d < 0 {
		return 0
	}
	return d
}

// Event is a single usage observation emitted by a collector. Usage is always
// a delta (the cost of one request / one accounting bucket), never a running
// total — the engine is responsible for aggregation.
type Event struct {
	Provider     Provider   `json:"provider"`
	Source       string     `json:"source"` // collector name, e.g. "claude-code"
	Time         time.Time  `json:"time"`
	SessionID    string     `json:"session_id"`
	SessionLabel string     `json:"session_label"` // human hint, e.g. project dir
	Model        string     `json:"model"`
	Usage        TokenUsage `json:"usage"`
	CostUSD      float64    `json:"cost_usd"`
	Kind         string     `json:"kind"` // "message" | "proxy" | "poll" | "demo"
	// IsAgent marks a turn run by a subagent (e.g. Claude Code's Task-tool
	// sidechains) rather than the main conversation thread. Live-only: not
	// persisted, purely for the Waterfall view's Agent row.
	IsAgent bool `json:"-"`
	// Dedup is an optional idempotency key; collectors that may re-read the same
	// record (log tailers) set it so the engine can drop duplicates.
	Dedup string `json:"-"`
}

// Session is a persisted description of a monitored session.
type Session struct {
	ID        string
	Provider  Provider
	Label     string
	Source    string
	FirstSeen time.Time
	LastSeen  time.Time
	Usage     TokenUsage
	CostUSD   float64
	Events    int64
}

// Duration of the session as seen so far.
func (s Session) Duration() time.Duration {
	if s.FirstSeen.IsZero() || s.LastSeen.Before(s.FirstSeen) {
		return 0
	}
	return s.LastSeen.Sub(s.FirstSeen)
}

// LoginKind distinguishes how a CLI is currently authenticated to a vendor.
// This is orthogonal to Limit: a Limit models a per-minute/per-day API rate
// bucket, while LoginKind + UsageWindow model a subscription plan's own
// session/weekly usage allowance (e.g. Claude Pro/Max via Claude Code).
type LoginKind string

const (
	LoginSubscription LoginKind = "subscription" // OAuth: Claude.ai Pro/Max/Team/Enterprise plan
	LoginAPIKey       LoginKind = "api_key"      // pay-as-you-go console API key
	LoginUnknown      LoginKind = "unknown"
)

// UsageWindow is one subscription-plan allowance bucket (e.g. Claude Code's
// rolling 5-hour session window or 7-day weekly window).
type UsageWindow struct {
	Kind     string    `json:"kind"`     // "session", "weekly", ...
	Used     float64   `json:"used"`     // percent used, 0..100
	ResetAt  time.Time `json:"reset_at"` // zero if unknown
	Active   bool      `json:"active"`   // currently the binding/throttling constraint
	Severity string    `json:"severity"` // "critical", "normal", ...
}

// Remaining returns the percent of the window still available.
func (w UsageWindow) Remaining() float64 {
	r := 100 - w.Used
	if r < 0 {
		return 0
	}
	if r > 100 {
		return 100
	}
	return r
}

// ResetIn returns the duration until this window resets (0 if unknown/past).
func (w UsageWindow) ResetIn(now time.Time) time.Duration {
	if w.ResetAt.IsZero() {
		return 0
	}
	d := w.ResetAt.Sub(now)
	if d < 0 {
		return 0
	}
	return d
}

// AccountStatus is a snapshot of a vendor CLI's own login/plan state, as
// opposed to raw API rate limits. It never carries secrets (access tokens,
// refresh tokens, email, or name) — only plan/quota metadata.
type AccountStatus struct {
	Provider    Provider      `json:"provider"`
	Source      string        `json:"source"`
	Login       LoginKind     `json:"login"`
	PlanLabel   string        `json:"plan_label"` // "Pro", "Max", "Team", "API Console", ...
	Windows     []UsageWindow `json:"windows"`
	LowPriority bool          `json:"low_priority"` // currently throttled by a critical, active window
	Observed    time.Time     `json:"observed"`
	// FetchedAt is when the CLI itself last refreshed this data server-side
	// (Claude Code's cachedUsageUtilization.fetchedAtMs) — NOT when this
	// collector last read the file. The CLI may go a long time between
	// refreshes, so Windows can lag well behind what `/status` or claude.ai
	// shows live; surface this age rather than implying a real-time value.
	FetchedAt time.Time `json:"fetched_at"`
}

// WindowByKind returns the window matching kind, if present.
func (a AccountStatus) WindowByKind(kind string) (UsageWindow, bool) {
	for _, w := range a.Windows {
		if w.Kind == kind {
			return w, true
		}
	}
	return UsageWindow{}, false
}
