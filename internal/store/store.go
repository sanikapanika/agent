// Package store persists monitors, their history and settings in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver: no CGO, so static builds and distroless images work
)

// ErrNotFound is returned when a row doesn't exist.
var ErrNotFound = errors.New("not found")

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (and creates, if needed) the database at path and migrates it.
func Open(path string) (*Store, error) {
	// The database holds password hashes, webhook URLs and keys: keep the
	// directory from other users on the machine.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection serializes writes and avoids SQLITE_BUSY; the
	// workload is small enough that this is never the bottleneck.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrations are applied in order, each once; the database's user_version
// records how many have run. Append new ones; never edit a released one.
var migrations = []string{
	// 1: initial schema.
	`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL,
		must_change_password INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		last_login_at INTEGER
	);
	CREATE TABLE sessions (
		token_hash TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at INTEGER NOT NULL
	);
	CREATE INDEX sessions_user ON sessions(user_id);

	CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE notifiers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		config TEXT NOT NULL DEFAULT '{}',
		created_at INTEGER NOT NULL
	);

	-- What's common to healthchecks and heartbeats; their settings live in
	-- the table for their kind. status_* is the status page presentation,
	-- owned by the status page editor (the monitors file never writes it).
	CREATE TABLE monitors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL CHECK (kind IN ('healthcheck', 'heartbeat')),
		name TEXT NOT NULL,
		paused INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT 'ui',
		public INTEGER NOT NULL DEFAULT 1,
		status_label TEXT NOT NULL DEFAULT '',
		status_order INTEGER NOT NULL DEFAULT 0,
		status_section TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE healthchecks (
		monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		target TEXT NOT NULL,
		interval_seconds INTEGER NOT NULL,
		timeout_seconds INTEGER NOT NULL,
		failure_threshold INTEGER NOT NULL,
		config TEXT NOT NULL DEFAULT '{}'
	);
	CREATE TABLE heartbeats (
		monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
		token TEXT NOT NULL UNIQUE,
		every_seconds INTEGER NOT NULL DEFAULT 0,
		cron TEXT NOT NULL DEFAULT '',
		timezone TEXT NOT NULL DEFAULT '',
		grace_seconds INTEGER NOT NULL
	);

	-- Healthcheck probe results.
	CREATE TABLE results (
		monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
		ts INTEGER NOT NULL,
		ok INTEGER NOT NULL,
		latency_ms INTEGER NOT NULL,
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX results_monitor_ts ON results(monitor_id, ts);

	-- Heartbeat runs. ts orders them: when the run finished, started (still
	-- running) or was due (missed).
	CREATE TABLE runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
		ts INTEGER NOT NULL,
		due_at INTEGER,
		started_at INTEGER,
		finished_at INTEGER,
		outcome TEXT NOT NULL,
		on_time INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER,
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX runs_monitor_ts ON runs(monitor_id, ts);

	-- Status changes, for both kinds.
	CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
		ts INTEGER NOT NULL,
		status TEXT NOT NULL,
		message TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX events_monitor_ts ON events(monitor_id, ts);`,

	// 2: alert routing, maintenance windows and API tokens.
	`-- A notifier alerts for every monitor, or only for those listed.
	ALTER TABLE notifiers ADD COLUMN all_monitors INTEGER NOT NULL DEFAULT 1;
	CREATE TABLE notifier_monitors (
		notifier_id INTEGER NOT NULL REFERENCES notifiers(id) ON DELETE CASCADE,
		monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
		PRIMARY KEY (notifier_id, monitor_id)
	);
	CREATE INDEX notifier_monitors_monitor ON notifier_monitors(monitor_id);

	-- Planned work: no down alerts for the monitors it covers, and a notice
	-- on the status page when public.
	CREATE TABLE maintenance (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		starts_at INTEGER NOT NULL,
		ends_at INTEGER NOT NULL,
		all_monitors INTEGER NOT NULL DEFAULT 1,
		public INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL
	);
	CREATE INDEX maintenance_ends ON maintenance(ends_at);
	CREATE TABLE maintenance_monitors (
		maintenance_id INTEGER NOT NULL REFERENCES maintenance(id) ON DELETE CASCADE,
		monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
		PRIMARY KEY (maintenance_id, monitor_id)
	);

	-- Personal API tokens. Only a hash is kept; hint is the last characters,
	-- to tell tokens apart.
	CREATE TABLE api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		hint TEXT NOT NULL,
		read_only INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		last_used_at INTEGER
	);
	CREATE INDEX api_tokens_user ON api_tokens(user_id);`,
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("the database is from a newer version of Uptimy Agent (schema %d, this version knows %d); upgrade the agent", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Prune deletes history (results, runs, events) older than before.
func (s *Store) Prune(ctx context.Context, before time.Time) error {
	ms := before.UnixMilli()
	for _, table := range []string{"results", "runs", "events"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE ts < ?", ms); err != nil { //nolint:gosec // G202: table names are constants
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM maintenance WHERE ends_at < ?", ms); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now().UnixMilli())
	return err
}

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
