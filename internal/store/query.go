package store

import (
	"context"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

// Bucket is a time-bucketed usage total.
type Bucket struct {
	Start    time.Time
	Provider model.Provider
	Usage    model.TokenUsage
	CostUSD  float64
}

// SessionRow is a session summary for the Sessions view.
type SessionRow struct {
	model.Session
}

// Totals is an overall roll-up for a time range.
type Totals struct {
	Usage       model.TokenUsage
	CostUSD     float64
	Events      int64
	Sessions    int64
	ByProvider  map[model.Provider]model.TokenUsage
	CostByProv  map[model.Provider]float64
	ByModel     map[string]model.TokenUsage
	CostByModel map[string]float64
}

// RangeTotals aggregates everything between from and to.
func (s *Store) RangeTotals(ctx context.Context, from, to time.Time) (Totals, error) {
	t := Totals{
		ByProvider:  map[model.Provider]model.TokenUsage{},
		CostByProv:  map[model.Provider]float64{},
		ByModel:     map[string]model.TokenUsage{},
		CostByModel: map[string]float64{},
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, model,
		       SUM(input_tokens), SUM(output_tokens), SUM(cache_read), SUM(cache_write),
		       SUM(requests), SUM(cost_usd), COUNT(*)
		FROM events WHERE ts >= ? AND ts < ?
		GROUP BY provider, model`, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return t, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var prov, mdl string
		var in, out, cr, cw, req, cnt int64
		var cost float64
		if err := rows.Scan(&prov, &mdl, &in, &out, &cr, &cw, &req, &cost, &cnt); err != nil {
			return t, err
		}
		u := model.TokenUsage{InputTokens: in, OutputTokens: out, CacheReadTokens: cr, CacheWriteTokens: cw, Requests: req}
		p := model.Provider(prov)
		t.Usage = t.Usage.Add(u)
		t.CostUSD += cost
		t.Events += cnt
		t.ByProvider[p] = t.ByProvider[p].Add(u)
		t.CostByProv[p] += cost
		if mdl != "" {
			t.ByModel[mdl] = t.ByModel[mdl].Add(u)
			t.CostByModel[mdl] += cost
		}
	}
	if err := rows.Err(); err != nil {
		return t, err
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT session_id) FROM events WHERE ts >= ? AND ts < ? AND session_id <> ''`,
		from.UnixMilli(), to.UnixMilli()).Scan(&t.Sessions)
	return t, nil
}

// Series returns usage bucketed by the given interval, one row per
// (bucket, provider). Buckets with no data are omitted.
func (s *Store) Series(ctx context.Context, from, to time.Time, interval time.Duration) ([]Bucket, error) {
	step := interval.Milliseconds()
	if step <= 0 {
		step = int64((time.Hour).Milliseconds())
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT (ts/?)*? AS b, provider,
		       SUM(input_tokens), SUM(output_tokens), SUM(cache_read), SUM(cache_write),
		       SUM(requests), SUM(cost_usd)
		FROM events WHERE ts >= ? AND ts < ?
		GROUP BY b, provider ORDER BY b ASC`,
		step, step, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Bucket
	for rows.Next() {
		var b, in, o, cr, cw, req int64
		var prov string
		var cost float64
		if err := rows.Scan(&b, &prov, &in, &o, &cr, &cw, &req, &cost); err != nil {
			return nil, err
		}
		out = append(out, Bucket{
			Start:    time.UnixMilli(b),
			Provider: model.Provider(prov),
			Usage:    model.TokenUsage{InputTokens: in, OutputTokens: o, CacheReadTokens: cr, CacheWriteTokens: cw, Requests: req},
			CostUSD:  cost,
		})
	}
	return out, rows.Err()
}

// Heatmap returns a 7x24 grid (weekday 0=Sunday, hour 0..23) of total tokens
// in the given range — the raw material for the Profile view's activity map.
func (s *Store) Heatmap(ctx context.Context, from, to time.Time) ([7][24]int64, error) {
	var grid [7][24]int64
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts, input_tokens+output_tokens+cache_read+cache_write FROM events WHERE ts >= ? AND ts < ?`,
		from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return grid, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var ms, tot int64
		if err := rows.Scan(&ms, &tot); err != nil {
			return grid, err
		}
		tm := time.UnixMilli(ms)
		grid[int(tm.Weekday())][tm.Hour()] += tot
	}
	return grid, rows.Err()
}

// RecentSessions returns up to limit sessions ordered by last activity, each
// with its rolled-up usage.
func (s *Store) RecentSessions(ctx context.Context, limit int) ([]model.Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.session_id, s.provider, s.label, s.source, s.first_seen, s.last_seen,
		       COALESCE(SUM(e.input_tokens),0), COALESCE(SUM(e.output_tokens),0),
		       COALESCE(SUM(e.cache_read),0), COALESCE(SUM(e.cache_write),0),
		       COALESCE(SUM(e.requests),0), COALESCE(SUM(e.cost_usd),0), COUNT(e.id)
		FROM sessions s LEFT JOIN events e ON e.session_id = s.session_id
		GROUP BY s.session_id
		ORDER BY s.last_seen DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Session
	for rows.Next() {
		var sess model.Session
		var prov string
		var first, last, in, o, cr, cw, req, cnt int64
		var cost float64
		if err := rows.Scan(&sess.ID, &prov, &sess.Label, &sess.Source, &first, &last,
			&in, &o, &cr, &cw, &req, &cost, &cnt); err != nil {
			return nil, err
		}
		sess.Provider = model.Provider(prov)
		sess.FirstSeen = time.UnixMilli(first)
		sess.LastSeen = time.UnixMilli(last)
		sess.Usage = model.TokenUsage{InputTokens: in, OutputTokens: o, CacheReadTokens: cr, CacheWriteTokens: cw, Requests: req}
		sess.CostUSD = cost
		sess.Events = cnt
		out = append(out, sess)
	}
	return out, rows.Err()
}

// SessionSeries returns per-minute usage for one session (for its detail graph).
func (s *Store) SessionSeries(ctx context.Context, sessionID string) ([]Bucket, error) {
	const step = int64(60_000)
	rows, err := s.db.QueryContext(ctx, `
		SELECT (ts/?)*?, SUM(input_tokens), SUM(output_tokens), SUM(cache_read), SUM(cache_write), SUM(requests), SUM(cost_usd)
		FROM events WHERE session_id = ?
		GROUP BY 1 ORDER BY 1 ASC`, step, step, sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Bucket
	for rows.Next() {
		var b, in, o, cr, cw, req int64
		var cost float64
		if err := rows.Scan(&b, &in, &o, &cr, &cw, &req, &cost); err != nil {
			return nil, err
		}
		out = append(out, Bucket{
			Start:   time.UnixMilli(b),
			Usage:   model.TokenUsage{InputTokens: in, OutputTokens: o, CacheReadTokens: cr, CacheWriteTokens: cw, Requests: req},
			CostUSD: cost,
		})
	}
	return out, rows.Err()
}

// LoadEventsSince returns raw events newer than ts (used to warm the engine's
// in-memory aggregates on startup). Ordered oldest first.
func (s *Store) LoadEventsSince(ctx context.Context, since time.Time, limit int) ([]model.Event, error) {
	if limit <= 0 {
		limit = 20000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts,provider,source,session_id,model,kind,input_tokens,output_tokens,cache_read,cache_write,requests,cost_usd
		FROM events WHERE ts >= ? ORDER BY ts ASC LIMIT ?`, since.UnixMilli(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Event
	for rows.Next() {
		var e model.Event
		var ms int64
		var prov string
		if err := rows.Scan(&ms, &prov, &e.Source, &e.SessionID, &e.Model, &e.Kind,
			&e.Usage.InputTokens, &e.Usage.OutputTokens, &e.Usage.CacheReadTokens, &e.Usage.CacheWriteTokens,
			&e.Usage.Requests, &e.CostUSD); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ms)
		e.Provider = model.Provider(prov)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatestLimits returns the most recent limit row per (provider, kind, window).
func (s *Store) LatestLimits(ctx context.Context, since time.Time) ([]model.Limit, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT provider, model, kind, window, lim, remaining, reset_at, MAX(ts)
		FROM limits WHERE ts >= ?
		GROUP BY provider, kind, window`, since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []model.Limit
	for rows.Next() {
		var l model.Limit
		var prov, kind string
		var reset, ts int64
		if err := rows.Scan(&prov, &l.Model, &kind, &l.Window, &l.Limit, &l.Remaining, &reset, &ts); err != nil {
			return nil, err
		}
		l.Provider = model.Provider(prov)
		l.Kind = model.LimitKind(kind)
		l.Observed = time.UnixMilli(ts)
		if reset > 0 {
			l.ResetAt = time.UnixMilli(reset)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
