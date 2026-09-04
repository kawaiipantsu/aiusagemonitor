package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestFlushPersistsEvents guards against a regression where every insert
// silently failed: the events table's dedup column is protected by a PARTIAL
// unique index, and an upsert's ON CONFLICT target must repeat that index's
// WHERE clause verbatim or SQLite rejects the statement. Events were being
// buffered and "flushed" without error, yet never actually landing in the
// database, because the flush error was discarded by the caller.
func TestFlushPersistsEvents(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	now := time.Now()

	st.Buffer(model.Event{
		Provider: model.ProviderAnthropic, Source: "test", Time: now,
		SessionID: "s1", Model: "claude-sonnet-5", Kind: "message",
		Usage:   model.TokenUsage{InputTokens: 100, OutputTokens: 50, Requests: 1},
		CostUSD: 0.01,
		// No Dedup set — this is the common case (proxy/poll/demo events) and
		// is exactly what triggered the silent-failure bug.
	})
	if err := st.Flush(ctx); err != nil {
		t.Fatalf("Flush returned an error: %v", err)
	}

	totals, err := st.RangeTotals(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RangeTotals: %v", err)
	}
	if totals.Events != 1 {
		t.Fatalf("Events = %d, want 1 (event was buffered+flushed but not persisted?)", totals.Events)
	}
	if got := totals.Usage.Total(); got != 150 {
		t.Fatalf("total tokens = %d, want 150", got)
	}
}

func TestFlushDedupDropsDuplicates(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	now := time.Now()

	ev := model.Event{
		Provider: model.ProviderOpenAI, Source: "claude-code", Time: now,
		SessionID: "s1", Model: "gpt-5", Kind: "message",
		Usage: model.TokenUsage{InputTokens: 10, OutputTokens: 5, Requests: 1},
		Dedup: "cc:s1:req-1",
	}
	// A log tailer that re-reads the same line (e.g. after a restart) must
	// not double-count it.
	st.Buffer(ev)
	st.Buffer(ev)
	if err := st.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	totals, err := st.RangeTotals(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RangeTotals: %v", err)
	}
	if totals.Events != 1 {
		t.Fatalf("Events = %d, want 1 (duplicate Dedup should be dropped)", totals.Events)
	}
}

func TestRecentSessionsAndSeries(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 3; i++ {
		st.Buffer(model.Event{
			Provider: model.ProviderGoogle, Source: "test", Time: now.Add(time.Duration(i) * time.Minute),
			SessionID: "sess-a", SessionLabel: "my-project", Model: "gemini-2.5-pro", Kind: "message",
			Usage: model.TokenUsage{InputTokens: 100, OutputTokens: 20, Requests: 1},
		})
	}
	if err := st.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	sessions, err := st.RecentSessions(ctx, 10)
	if err != nil {
		t.Fatalf("RecentSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if sessions[0].Events != 3 {
		t.Fatalf("session Events = %d, want 3", sessions[0].Events)
	}
	if sessions[0].Label != "my-project" {
		t.Fatalf("session Label = %q, want my-project", sessions[0].Label)
	}

	series, err := st.SessionSeries(ctx, "sess-a")
	if err != nil {
		t.Fatalf("SessionSeries: %v", err)
	}
	if len(series) == 0 {
		t.Fatalf("expected at least one bucket in session series")
	}
}

func TestLimitsRoundTrip(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	now := time.Now()

	st.BufferLimit(model.Limit{
		Provider: model.ProviderXAI, Kind: model.LimitTokens, Window: "1m",
		Limit: 1000, Remaining: 400, ResetAt: now.Add(time.Minute), Observed: now,
	})
	if err := st.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	limits, err := st.LatestLimits(ctx, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("LatestLimits: %v", err)
	}
	if len(limits) != 1 {
		t.Fatalf("got %d limits, want 1", len(limits))
	}
	if limits[0].Remaining != 400 {
		t.Fatalf("Remaining = %v, want 400", limits[0].Remaining)
	}
}

func TestPruneRemovesOldEvents(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	old := time.Now().AddDate(0, 0, -10)

	st.Buffer(model.Event{Provider: model.ProviderOpenAI, Source: "test", Time: old, Kind: "message"})
	if err := st.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	n, err := st.Prune(1) // retain only the last day
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune removed %d rows, want 1", n)
	}
	totals, err := st.RangeTotals(ctx, old.Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("RangeTotals: %v", err)
	}
	if totals.Events != 0 {
		t.Fatalf("expected the pruned event to be gone, still see %d", totals.Events)
	}
}
