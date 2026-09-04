package engine

import (
	"sort"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Snapshot builds an immutable DashboardState from the current in-memory
// events and limits. Safe to call from any goroutine.
func (e *Engine) Snapshot() *DashboardState {
	e.mu.RLock()
	events := e.events
	cfg := e.cfg
	startedAt := e.startedAt
	eventsSeen := e.eventsSeen
	limitsCopy := make([]model.Limit, 0, len(e.limits))
	for _, l := range e.limits {
		limitsCopy = append(limitsCopy, l)
	}
	notes := make(map[string]string, len(e.notes))
	for k, v := range e.notes {
		notes[k] = v
	}
	errs := append([]CollectorError(nil), e.errs...)
	colls := append([]string(nil), e.collNames...)
	accounts := make(map[model.Provider]model.AccountStatus, len(e.accounts))
	for k, v := range e.accounts {
		accounts[k] = v
	}
	e.mu.RUnlock()

	now := time.Now()
	windowMin := cfg.UI.WindowMin
	if windowMin <= 0 {
		windowMin = 60
	}
	winStart := now.Add(-time.Duration(windowMin) * time.Minute).Truncate(time.Minute)

	ds := &DashboardState{
		Now:        now,
		StartedAt:  startedAt,
		WindowMin:  windowMin,
		Providers:  map[model.Provider]*ProviderState{},
		Notes:      notes,
		Errors:     errs,
		Collectors: colls,
		EventsSeen: eventsSeen,
		Accounts:   accounts,
	}

	// Per-provider minute-bucketed series over the window.
	nBuckets := windowMin
	bucketIndex := func(t time.Time) int {
		if t.Before(winStart) {
			return -1
		}
		idx := int(t.Sub(winStart) / time.Minute)
		if idx < 0 || idx >= nBuckets {
			return -1
		}
		return idx
	}

	ensure := func(p model.Provider) *ProviderState {
		ps := ds.Providers[p]
		if ps == nil {
			ps = &ProviderState{
				Provider:  p,
				Models:    map[string]model.TokenUsage{},
				ModelCost: map[string]float64{},
				Series:    make([]Point, nBuckets),
			}
			for i := range ps.Series {
				ps.Series[i].T = winStart.Add(time.Duration(i) * time.Minute)
			}
			ds.Providers[p] = ps
		}
		return ps
	}

	type sessAcc struct {
		s      model.Session
		series map[int64]*Point
		recent model.TokenUsage
	}
	sessions := map[string]*sessAcc{}
	fiveAgo := now.Add(-5 * time.Minute)

	modelSeries := map[wfKey][]float64{}
	agentSeries := make([]float64, nBuckets)
	backgroundSeries := make([]float64, nBuckets)

	for i := range events {
		ev := events[i]
		p := ev.Provider
		if !p.Valid() {
			continue
		}
		ps := ensure(p)

		// Session-lifetime totals (since app start / backfilled history).
		ps.Session = ps.Session.Add(ev.Usage)
		ps.SessionCost += ev.CostUSD
		if ev.Time.After(ps.LastEvent) {
			ps.LastEvent = ev.Time
		}
		ds.SessionTotals = ds.SessionTotals.Add(ev.Usage)
		ds.SessionCost += ev.CostUSD

		if ev.Time.After(fiveAgo) {
			ps.RatePerMin += float64(ev.Usage.Total())
		}

		if bi := bucketIndex(ev.Time); bi >= 0 {
			ps.Window = ps.Window.Add(ev.Usage)
			ps.Cost += ev.CostUSD
			ps.Series[bi].Usage = ps.Series[bi].Usage.Add(ev.Usage)
			ps.Series[bi].Cost += ev.CostUSD
			ps.Models[ev.Model] = ps.Models[ev.Model].Add(ev.Usage)
			ps.ModelCost[ev.Model] += ev.CostUSD
			ds.Totals = ds.Totals.Add(ev.Usage)
			ds.TotalCost += ev.CostUSD

			total := float64(ev.Usage.Total())
			if ev.Model != "" {
				key := wfKey{p, ev.Model}
				ser := modelSeries[key]
				if ser == nil {
					ser = make([]float64, nBuckets)
					modelSeries[key] = ser
				}
				ser[bi] += total
			}
			if ev.IsAgent {
				agentSeries[bi] += total
			}
			if ev.Kind == "poll" {
				backgroundSeries[bi] += total
			}
		}

		if ev.SessionID != "" {
			sa := sessions[ev.SessionID]
			if sa == nil {
				sa = &sessAcc{
					s:      model.Session{ID: ev.SessionID, Provider: p, Label: ev.SessionLabel, Source: ev.Source, FirstSeen: ev.Time, LastSeen: ev.Time},
					series: map[int64]*Point{},
				}
				sessions[ev.SessionID] = sa
			}
			if ev.Time.Before(sa.s.FirstSeen) {
				sa.s.FirstSeen = ev.Time
			}
			if ev.Time.After(sa.s.LastSeen) {
				sa.s.LastSeen = ev.Time
			}
			if ev.SessionLabel != "" {
				sa.s.Label = ev.SessionLabel
			}
			sa.s.Usage = sa.s.Usage.Add(ev.Usage)
			sa.s.CostUSD += ev.CostUSD
			sa.s.Events++
			mb := ev.Time.Truncate(time.Minute).UnixMilli()
			pt := sa.series[mb]
			if pt == nil {
				pt = &Point{T: time.UnixMilli(mb)}
				sa.series[mb] = pt
			}
			pt.Usage = pt.Usage.Add(ev.Usage)
			pt.Cost += ev.CostUSD
			if ev.Time.After(fiveAgo) {
				sa.recent = sa.recent.Add(ev.Usage)
			}
		}
	}

	ds.RatePerMin = 0
	for _, ps := range ds.Providers {
		ps.RatePerMin /= 5
		ds.RatePerMin += ps.RatePerMin
		ps.Limits = limitsForProvider(limitsCopy, ps.Provider)
	}

	// Order providers: active first, then by window tokens.
	for p := range ds.Providers {
		ds.Order = append(ds.Order, p)
	}
	sort.Slice(ds.Order, func(i, j int) bool {
		a, b := ds.Providers[ds.Order[i]], ds.Providers[ds.Order[j]]
		ai, bi := a.Active(now), b.Active(now)
		if ai != bi {
			return ai
		}
		if a.Window.Total() != b.Window.Total() {
			return a.Window.Total() > b.Window.Total()
		}
		return string(a.Provider) < string(b.Provider)
	})

	// Finalise sessions.
	for _, sa := range sessions {
		pts := make([]Point, 0, len(sa.series))
		for _, p := range sa.series {
			pts = append(pts, *p)
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })
		rate := float64(sa.recent.Total()) / 5
		ds.Sessions = append(ds.Sessions, SessionAgg{Session: sa.s, Series: pts, RatePerMin: rate})
	}
	sort.Slice(ds.Sessions, func(i, j int) bool { return ds.Sessions[i].LastSeen.After(ds.Sessions[j].LastSeen) })
	if len(ds.Sessions) > 60 {
		ds.Sessions = ds.Sessions[:60]
	}

	ds.Waterfall = buildWaterfall(modelSeries, agentSeries, backgroundSeries)

	return ds
}

// wfKey identifies one model-usage series within a snapshot.
type wfKey struct {
	provider model.Provider
	model    string
}

// buildWaterfall picks the busiest models (capped, so the view stays
// legible) and always appends the Agent and Background rows, even at zero,
// so the shape of the view doesn't jump around as those turn on and off.
func buildWaterfall(modelSeries map[wfKey][]float64, agentSeries, backgroundSeries []float64) []WaterfallRow {
	const maxModelRows = 8
	type ranked struct {
		provider model.Provider
		model    string
		series   []float64
		total    float64
	}
	rows := make([]ranked, 0, len(modelSeries))
	for k, ser := range modelSeries {
		var sum float64
		for _, v := range ser {
			sum += v
		}
		rows = append(rows, ranked{k.provider, k.model, ser, sum})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].total != rows[j].total {
			return rows[i].total > rows[j].total
		}
		return rows[i].model < rows[j].model
	})
	if len(rows) > maxModelRows {
		rows = rows[:maxModelRows]
	}

	out := make([]WaterfallRow, 0, len(rows)+2)
	for _, r := range rows {
		out = append(out, WaterfallRow{Label: r.provider.Title() + " " + r.model, Provider: r.provider, Series: r.series})
	}
	out = append(out,
		WaterfallRow{Label: "Agent", Series: agentSeries},
		WaterfallRow{Label: "Background", Series: backgroundSeries},
	)
	return out
}

func limitsForProvider(all []model.Limit, p model.Provider) []model.Limit {
	var out []model.Limit
	for _, l := range all {
		if l.Provider == p {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return limitKindOrder(out[i].Kind) < limitKindOrder(out[j].Kind)
	})
	return out
}

func limitKindOrder(k model.LimitKind) int {
	switch k {
	case model.LimitRequests:
		return 0
	case model.LimitTokens:
		return 1
	case model.LimitInputTokens:
		return 2
	case model.LimitOutputTokens:
		return 3
	case model.LimitCostUSD:
		return 4
	default:
		return 9
	}
}
