package engine

import (
	"sort"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Point is one per-minute bucket in a time series.
type Point struct {
	T     time.Time
	Usage model.TokenUsage
	Cost  float64
}

// ProviderState is the live roll-up for a single provider.
type ProviderState struct {
	Provider    model.Provider
	Window      model.TokenUsage // totals inside the dashboard window
	Session     model.TokenUsage // totals since the app started
	Cost        float64          // window cost
	SessionCost float64
	Series      []Point // per-minute, oldest→newest, len == WindowMin
	Models      map[string]model.TokenUsage
	ModelCost   map[string]float64
	Limits      []model.Limit
	LastEvent   time.Time
	RatePerMin  float64 // tokens/min, last 5 min
}

// Active reports whether an event landed in the last 2 minutes.
func (p *ProviderState) Active(now time.Time) bool {
	return !p.LastEvent.IsZero() && now.Sub(p.LastEvent) < 2*time.Minute
}

// TopModels returns model usage sorted by total tokens, capped at n.
func (p *ProviderState) TopModels(n int) []ModelUsage {
	out := make([]ModelUsage, 0, len(p.Models))
	for m, u := range p.Models {
		out = append(out, ModelUsage{Model: m, Usage: u, Cost: p.ModelCost[m]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Usage.Total() > out[j].Usage.Total() })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// ModelUsage pairs a model id with its usage.
type ModelUsage struct {
	Model string
	Usage model.TokenUsage
	Cost  float64
}

// SessionAgg is a per-session roll-up with a mini time series.
type SessionAgg struct {
	model.Session
	Series     []Point
	RatePerMin float64
}

// CollectorError is a non-fatal problem reported by a collector.
type CollectorError struct {
	Source string
	Err    string
	Time   time.Time
}

// DashboardState is the immutable snapshot the UI renders. The engine swaps a
// fresh pointer on every refresh; the UI never mutates it.
type DashboardState struct {
	Now           time.Time
	StartedAt     time.Time
	WindowMin     int
	Providers     map[model.Provider]*ProviderState
	Order         []model.Provider // providers that have data, most-active first
	Sessions      []SessionAgg
	Totals        model.TokenUsage
	TotalCost     float64
	SessionTotals model.TokenUsage
	SessionCost   float64
	RatePerMin    float64
	EventsSeen    int64
	Notes         map[string]string
	Errors        []CollectorError
	Collectors    []string
	// Accounts holds each provider's CLI login/plan state (subscription vs.
	// API key, session/weekly usage allowance) where a collector reports it.
	Accounts map[model.Provider]model.AccountStatus
	// Waterfall is the Waterfall view's per-minute matrix: the busiest
	// models plus the synthetic Agent (subagent turns) and Background
	// (non-interactive poll-collector usage) rows, over the same window as
	// each ProviderState.Series.
	Waterfall []WaterfallRow
}

// WaterfallRow is one row of the Waterfall view.
type WaterfallRow struct {
	Label    string
	Provider model.Provider // "" for the synthetic Agent/Background rows
	Series   []float64      // per-minute totals, aligned with ProviderState.Series
}

// ProviderOrDefault returns the state for p, or a zero-valued one.
func (d *DashboardState) ProviderOrDefault(p model.Provider) *ProviderState {
	if d.Providers != nil {
		if ps, ok := d.Providers[p]; ok {
			return ps
		}
	}
	return &ProviderState{Provider: p, Models: map[string]model.TokenUsage{}, ModelCost: map[string]float64{}}
}
