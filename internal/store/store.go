// Package store persists usage events and rate-limit observations in a local
// SQLite database (pure-Go driver, no cgo) and answers the aggregate queries
// the History and Profile views need.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kawaiipantsu/aiusagemonitor/internal/model"
	_ "modernc.org/sqlite"
)

// Store wraps the database handle and a write buffer.
type Store struct {
	db   *sql.DB
	path string

	mu     sync.Mutex
	buf    []model.Event
	limbuf []model.Limit
}

const schema = `
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	ts            INTEGER NOT NULL,          -- unix millis
	provider      TEXT    NOT NULL,
	source        TEXT    NOT NULL,
	session_id    TEXT    NOT NULL,
	model         TEXT    NOT NULL,
	kind          TEXT    NOT NULL,
	input_tokens  INTEGER NOT NULL DEFAULT 0,
	output_tokens INTEGER NOT NULL DEFAULT 0,
	cache_read    INTEGER NOT NULL DEFAULT 0,
	cache_write   INTEGER NOT NULL DEFAULT 0,
	requests      INTEGER NOT NULL DEFAULT 0,
	cost_usd      REAL    NOT NULL DEFAULT 0,
	dedup         TEXT
);
CREATE INDEX IF NOT EXISTS idx_events_ts       ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_session  ON events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_provider ON events(provider, ts);
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_dedup ON events(dedup) WHERE dedup IS NOT NULL AND dedup <> '';

CREATE TABLE IF NOT EXISTS limits (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	ts        INTEGER NOT NULL,
	provider  TEXT    NOT NULL,
	model     TEXT    NOT NULL DEFAULT '',
	kind      TEXT    NOT NULL,
	window    TEXT    NOT NULL DEFAULT '',
	lim       REAL    NOT NULL,
	remaining REAL    NOT NULL,
	reset_at  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_limits_ts ON limits(provider, ts);

CREATE TABLE IF NOT EXISTS sessions (
	session_id TEXT PRIMARY KEY,
	provider   TEXT NOT NULL,
	label      TEXT NOT NULL DEFAULT '',
	source     TEXT NOT NULL DEFAULT '',
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL
);
`

// Open creates (or opens) the database at path and applies the schema.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer; keeps WAL simple
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: schema: %w", err)
	}
	_, _ = db.Exec(`INSERT INTO meta(key,value) VALUES('schema_version','1') ON CONFLICT(key) DO NOTHING`)
	return &Store{db: db, path: path}, nil
}

// Path returns the on-disk location.
func (s *Store) Path() string { return s.path }

// Close flushes and closes the database.
func (s *Store) Close() error {
	_ = s.Flush(context.Background())
	return s.db.Close()
}

// Buffer queues an event for the next Flush.
func (s *Store) Buffer(e model.Event) {
	s.mu.Lock()
	s.buf = append(s.buf, e)
	s.mu.Unlock()
}

// BufferLimit queues a rate-limit observation for the next Flush.
func (s *Store) BufferLimit(l model.Limit) {
	s.mu.Lock()
	s.limbuf = append(s.limbuf, l)
	s.mu.Unlock()
}

// Pending reports how many events are waiting to be written.
func (s *Store) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buf) + len(s.limbuf)
}

// Flush writes all buffered rows in a single transaction.
func (s *Store) Flush(ctx context.Context) error {
	s.mu.Lock()
	events := s.buf
	limits := s.limbuf
	s.buf = nil
	s.limbuf = nil
	s.mu.Unlock()
	if len(events) == 0 && len(limits) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if len(events) > 0 {
		evStmt, err := tx.PrepareContext(ctx, `
			INSERT INTO events(ts,provider,source,session_id,model,kind,input_tokens,output_tokens,cache_read,cache_write,requests,cost_usd,dedup)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(dedup) WHERE dedup IS NOT NULL AND dedup <> '' DO NOTHING`)
		if err != nil {
			return err
		}
		sessStmt, err := tx.PrepareContext(ctx, `
			INSERT INTO sessions(session_id,provider,label,source,first_seen,last_seen)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(session_id) DO UPDATE SET
				last_seen=MAX(last_seen, excluded.last_seen),
				label=CASE WHEN excluded.label <> '' THEN excluded.label ELSE label END`)
		if err != nil {
			return err
		}
		for _, e := range events {
			ms := e.Time.UnixMilli()
			var dedup any
			if e.Dedup != "" {
				dedup = e.Dedup
			}
			if _, err := evStmt.ExecContext(ctx, ms, string(e.Provider), e.Source, e.SessionID, e.Model, e.Kind,
				e.Usage.InputTokens, e.Usage.OutputTokens, e.Usage.CacheReadTokens, e.Usage.CacheWriteTokens,
				e.Usage.Requests, e.CostUSD, dedup); err != nil {
				return err
			}
			if e.SessionID != "" {
				if _, err := sessStmt.ExecContext(ctx, e.SessionID, string(e.Provider), e.SessionLabel, e.Source, ms, ms); err != nil {
					return err
				}
			}
		}
	}

	if len(limits) > 0 {
		limStmt, err := tx.PrepareContext(ctx, `
			INSERT INTO limits(ts,provider,model,kind,window,lim,remaining,reset_at)
			VALUES(?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		for _, l := range limits {
			var reset int64
			if !l.ResetAt.IsZero() {
				reset = l.ResetAt.UnixMilli()
			}
			if _, err := limStmt.ExecContext(ctx, l.Observed.UnixMilli(), string(l.Provider), l.Model,
				string(l.Kind), l.Window, l.Limit, l.Remaining, reset); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// Prune deletes events and limit rows older than the retention window.
func (s *Store) Prune(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cut := time.Now().AddDate(0, 0, -retentionDays).UnixMilli()
	res, err := s.db.Exec(`DELETE FROM events WHERE ts < ?`, cut)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`DELETE FROM limits WHERE ts < ?`, cut); err != nil {
		return n, err
	}
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE last_seen < ?`, cut)
	return n, nil
}

// Vacuum reclaims disk space (call sparingly).
func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}
