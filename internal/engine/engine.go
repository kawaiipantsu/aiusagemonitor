// Package engine wires collectors to storage and to the UI. It owns the
// in-memory rolling aggregates that back the live dashboard, persists every
// observation to the store, and broadcasts an immutable DashboardState
// snapshot to subscribers on a timer.
package engine

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/collector"
	"github.com/kawaiipantsu/aiusagemonitor/internal/config"
	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	"github.com/kawaiipantsu/aiusagemonitor/internal/pricing"
	"github.com/kawaiipantsu/aiusagemonitor/internal/store"
)

// Engine is safe for concurrent use.
type Engine struct {
	store   *store.Store
	pricing *pricing.Table

	mu         sync.RWMutex
	cfg        *config.Config
	events     []model.Event            // rolling, bounded by horizon
	seen       map[string]struct{}      // dedup keys
	seenOrder  []string                 // FIFO eviction for seen
	limits     map[limitKey]model.Limit // latest per provider+kind+window
	accounts   map[model.Provider]model.AccountStatus
	notes      map[string]string // collector -> status
	errs       []CollectorError
	collNames  []string
	startedAt  time.Time
	eventsSeen int64

	subs   map[chan *DashboardState]struct{}
	subsMu sync.Mutex

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// horizon is how far back in-memory events are retained.
	horizon time.Duration
}

type limitKey struct {
	p model.Provider
	k model.LimitKind
	w string
}

// New builds an engine bound to a store. cfg may be swapped later via
// Reconfigure.
func New(st *store.Store, cfg *config.Config) *Engine {
	return &Engine{
		store:     st,
		pricing:   cfg.PricingTable(),
		cfg:       cfg,
		seen:      map[string]struct{}{},
		limits:    map[limitKey]model.Limit{},
		accounts:  map[model.Provider]model.AccountStatus{},
		notes:     map[string]string{},
		subs:      map[chan *DashboardState]struct{}{},
		startedAt: time.Now(),
		horizon:   24 * time.Hour,
	}
}

// Pricing exposes the resolver so the settings UI can add overrides.
func (e *Engine) Pricing() *pricing.Table { return e.pricing }

// Subscribe returns a channel that receives a fresh snapshot on every refresh.
// The caller must Unsubscribe when done.
func (e *Engine) Subscribe() chan *DashboardState {
	ch := make(chan *DashboardState, 4)
	e.subsMu.Lock()
	e.subs[ch] = struct{}{}
	e.subsMu.Unlock()
	// Prime immediately so the UI isn't blank.
	go func() { ch <- e.Snapshot() }()
	return ch
}

// Unsubscribe removes and closes a subscription channel.
func (e *Engine) Unsubscribe(ch chan *DashboardState) {
	e.subsMu.Lock()
	if _, ok := e.subs[ch]; ok {
		delete(e.subs, ch)
		close(ch)
	}
	e.subsMu.Unlock()
}

func (e *Engine) broadcast() {
	snap := e.Snapshot()
	e.subsMu.Lock()
	for ch := range e.subs {
		select {
		case ch <- snap:
		default: // drop for slow consumers; next tick catches up
		}
	}
	e.subsMu.Unlock()
}

// Start launches collectors and the background loops. It returns immediately;
// call Stop (or cancel the parent context) to shut down.
func (e *Engine) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	e.mu.Lock()
	e.cancel = cancel
	cfg := e.cfg
	e.mu.Unlock()

	out := make(chan collector.Emission, 256)
	cols := collector.Build(cfg)

	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, c.Name())
	}
	e.mu.Lock()
	e.collNames = names
	e.mu.Unlock()

	for _, c := range cols {
		c := c
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			if err := c.Run(ctx, out); err != nil && ctx.Err() == nil {
				e.recordErr(c.Name(), err.Error())
			}
		}()
	}

	// Ingest loop.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case em := <-out:
				e.ingest(em)
			}
		}
	}()

	// Refresh + flush + prune loop.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		refresh := time.NewTicker(clampDur(cfg.UI.RefreshRate, 250*time.Millisecond, 10*time.Second))
		flush := time.NewTicker(clampDur(cfg.Storage.FlushInterval, time.Second, time.Minute))
		prune := time.NewTicker(6 * time.Hour)
		defer refresh.Stop()
		defer flush.Stop()
		defer prune.Stop()
		for {
			select {
			case <-ctx.Done():
				if err := e.store.Flush(context.Background()); err != nil {
					e.recordErr("store", "final flush: "+err.Error())
				}
				return
			case <-refresh.C:
				e.trimHorizon()
				e.broadcast()
			case <-flush.C:
				if err := e.store.Flush(ctx); err != nil {
					e.recordErr("store", "flush: "+err.Error())
				}
			case <-prune.C:
				if _, err := e.store.Prune(cfg.Storage.RetentionDays); err != nil {
					e.recordErr("store", "prune: "+err.Error())
				}
			}
		}
	}()
}

// Stop cancels collectors and waits for goroutines to drain.
func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	e.wg.Wait()
}

// Reconfigure applies a new config: stops everything, rebuilds collectors and
// restarts. In-memory event history is preserved.
func (e *Engine) Reconfigure(parent context.Context, cfg *config.Config) {
	e.Stop()
	e.mu.Lock()
	e.cfg = cfg
	e.pricing = cfg.PricingTable()
	e.horizon = 24 * time.Hour
	e.mu.Unlock()
	e.Start(parent)
}

func (e *Engine) recordErr(source, msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.errs = append(e.errs, CollectorError{Source: source, Err: msg, Time: time.Now()})
	if len(e.errs) > 50 {
		e.errs = e.errs[len(e.errs)-50:]
	}
}

func (e *Engine) ingest(em collector.Emission) {
	if em.Err != nil {
		e.recordErr(em.Source, em.Err.Error())
		return
	}
	if em.Note != "" {
		e.mu.Lock()
		e.notes[em.Source] = em.Note
		e.mu.Unlock()
	}
	if em.Limit != nil {
		l := *em.Limit
		if l.Observed.IsZero() {
			l.Observed = time.Now()
		}
		e.mu.Lock()
		e.limits[limitKey{l.Provider, l.Kind, l.Window}] = l
		e.mu.Unlock()
		e.store.BufferLimit(l)
	}
	if em.Account != nil {
		a := *em.Account
		if a.Observed.IsZero() {
			a.Observed = time.Now()
		}
		e.mu.Lock()
		e.accounts[a.Provider] = a
		e.mu.Unlock()
		// Account status is a live login/plan snapshot, not a usage event —
		// it isn't persisted; History/Profile are about token usage over time.
	}
	if em.Event != nil {
		ev := *em.Event
		if ev.Time.IsZero() {
			ev.Time = time.Now()
		}
		if ev.Dedup != "" {
			e.mu.Lock()
			if _, dup := e.seen[ev.Dedup]; dup {
				e.mu.Unlock()
				return
			}
			e.seen[ev.Dedup] = struct{}{}
			e.seenOrder = append(e.seenOrder, ev.Dedup)
			if len(e.seenOrder) > 200000 {
				drop := e.seenOrder[0]
				e.seenOrder = e.seenOrder[1:]
				delete(e.seen, drop)
			}
			e.mu.Unlock()
		}
		if ev.CostUSD == 0 {
			ev.CostUSD = e.pricing.Cost(ev.Model, ev.Usage)
		}
		e.mu.Lock()
		e.events = append(e.events, ev)
		e.eventsSeen++
		e.mu.Unlock()
		e.store.Buffer(ev)
	}
}

func (e *Engine) trimHorizon() {
	cut := time.Now().Add(-e.horizon)
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.events) == 0 || !e.events[0].Time.Before(cut) {
		return
	}
	i := sort.Search(len(e.events), func(i int) bool { return !e.events[i].Time.Before(cut) })
	e.events = append([]model.Event(nil), e.events[i:]...)
}

func clampDur(d, lo, hi time.Duration) time.Duration {
	if d < lo {
		return lo
	}
	if d > hi {
		return hi
	}
	return d
}
