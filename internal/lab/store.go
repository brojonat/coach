// Package lab implements the experiment store and task corpus that power
// evidence-driven persona iteration.
package lab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store is a thin wrapper around *sql.DB that owns the lab schema. Callers
// should hold a single Store for the lifetime of the process.
type Store struct {
	db *sql.DB
}

// Open connects to the SQLite database at path (creating it if missing) and
// runs any pending migrations. Pass ":memory:" for an in-process store.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	// SQLite with modernc is fine with concurrent readers but writes serialize;
	// keep MaxOpenConns small to surface contention early.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB for callers that need to run ad-hoc
// queries (reports, replays). Prefer the typed methods where they exist.
func (s *Store) DB() *sql.DB { return s.db }

// migrations are applied in order. Each entry is a single SQL statement (or
// batch of statements separated by ';'). Never edit a committed migration;
// append a new one instead.
var migrations = []string{
	// 1: schema_version + core tables.
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY
	);
	CREATE TABLE IF NOT EXISTS tasks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		slug        TEXT NOT NULL UNIQUE,
		title       TEXT NOT NULL,
		goal        TEXT NOT NULL,
		difficulty  TEXT NOT NULL,
		tags        TEXT NOT NULL DEFAULT '',
		notes       TEXT NOT NULL DEFAULT '',
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS experiments (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id       INTEGER REFERENCES tasks(id) ON DELETE SET NULL,
		persona_name  TEXT NOT NULL,
		persona_text  TEXT NOT NULL,
		config_json   TEXT NOT NULL,
		started_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		ended_at      TIMESTAMP,
		notes         TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS ratings (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		experiment_id  INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
		dimension      TEXT NOT NULL,
		value          INTEGER NOT NULL,
		note           TEXT NOT NULL DEFAULT '',
		created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS events (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		experiment_id  INTEGER NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
		ts             TIMESTAMP NOT NULL,
		level          TEXT NOT NULL DEFAULT '',
		source         TEXT NOT NULL DEFAULT '',
		msg            TEXT NOT NULL DEFAULT '',
		data_json      TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_events_experiment ON events(experiment_id, ts);
	CREATE INDEX IF NOT EXISTS idx_ratings_experiment ON ratings(experiment_id);`,
}

func (s *Store) migrate(ctx context.Context) error {
	// Bootstrap schema_version so reads of the current version always succeed.
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var current int
	row := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	for i, stmt := range migrations {
		v := i + 1
		if v <= current {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version) VALUES (?)`, v); err != nil {
			tx.Rollback()
			return fmt.Errorf("record version %d: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
