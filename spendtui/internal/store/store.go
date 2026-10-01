// Package store persists spend lines in DuckDB so history survives restarts,
// the UI can paint from cache before the network answers, and the data can be
// queried directly with the duckdb CLI.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

const schema = `
CREATE TABLE IF NOT EXISTS spend (
	provider      VARCHAR     NOT NULL,
	day           DATE        NOT NULL,
	model         VARCHAR     NOT NULL,
	usd           DOUBLE      NOT NULL,
	input_tokens  BIGINT      NOT NULL,
	output_tokens BIGINT      NOT NULL,
	fetched_at    TIMESTAMPTZ NOT NULL,
	PRIMARY KEY (provider, day, model)
);
-- Added after the first release; existing files gain them in place.
ALTER TABLE spend ADD COLUMN IF NOT EXISTS cache_read_tokens  BIGINT DEFAULT 0;
ALTER TABLE spend ADD COLUMN IF NOT EXISTS cache_write_tokens BIGINT DEFAULT 0;`

type Store struct{ db *sql.DB }

// DefaultPath is $XDG_DATA_HOME/spendtui/spend.duckdb, falling back to
// ~/.local/share.
func DefaultPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "spendtui", "spend.duckdb")
}

// Open opens (creating if needed) the database at path. An empty path gives
// an in-memory database.
func Open(path string) (*Store, error) {
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	// DuckDB allows one writer per process; serialize through one connection.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Replace swaps every stored line for provider in [start, end) for lines.
// Providers restate open days, and a model can vanish from a restated day, so
// the whole window is replaced rather than upserted.
func (s *Store) Replace(ctx context.Context, provider string, start, end time.Time, lines []spend.Line) error {
	type key struct {
		day   time.Time
		model string
	}
	merged := map[key]*spend.Line{}
	for _, l := range lines {
		d := spend.Day(l.Day)
		if d.Before(start) || !d.Before(end) {
			continue
		}
		k := key{d, l.Model}
		if m := merged[k]; m != nil {
			m.USD += l.USD
			m.Tokens.Add(l.Tokens)
		} else {
			c := l
			c.Day = d
			merged[k] = &c
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM spend WHERE provider = ? AND day >= CAST(? AS DATE) AND day < CAST(? AS DATE)`,
		provider, start.Format(time.DateOnly), end.Format(time.DateOnly)); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO spend (provider, day, model, usd, input_tokens, output_tokens,
		                    cache_read_tokens, cache_write_tokens, fetched_at)
		 VALUES (?, CAST(? AS DATE), ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC()
	for _, l := range merged {
		if _, err := stmt.ExecContext(ctx, provider, l.Day.Format(time.DateOnly), l.Model,
			l.USD, l.Input, l.Output, l.CacheRead, l.CacheWrite, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Load returns provider's lines on or after since, oldest first.
func (s *Store) Load(ctx context.Context, provider string, since time.Time) ([]spend.Line, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT strftime(day, '%Y-%m-%d'), model, usd, input_tokens, output_tokens,
		        coalesce(cache_read_tokens, 0), coalesce(cache_write_tokens, 0)
		 FROM spend WHERE provider = ? AND day >= CAST(? AS DATE) ORDER BY day, model`,
		provider, since.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []spend.Line
	for rows.Next() {
		var day string
		var l spend.Line
		if err := rows.Scan(&day, &l.Model, &l.USD, &l.Input, &l.Output, &l.CacheRead, &l.CacheWrite); err != nil {
			return nil, err
		}
		if l.Day, err = time.Parse(time.DateOnly, day); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LastSync reports the newest stored day and when provider was last fetched.
// ok is false when nothing is stored yet.
func (s *Store) LastSync(ctx context.Context, provider string) (lastDay, fetchedAt time.Time, ok bool, err error) {
	var day sql.NullString
	var at sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT strftime(max(day), '%Y-%m-%d'), max(fetched_at) FROM spend WHERE provider = ?`, provider).
		Scan(&day, &at)
	if err != nil || !day.Valid {
		return time.Time{}, time.Time{}, false, err
	}
	lastDay, err = time.Parse(time.DateOnly, day.String)
	return lastDay, at.Time, err == nil, err
}

// Syncer keeps one provider's rows in the store current.
type Syncer struct {
	Store    *Store
	Provider spend.Provider
	// Backfill is how far back the first sync reaches when nothing is stored.
	Backfill time.Duration
	// Restate is how many trailing stored days are refetched on every sync,
	// since providers keep revising recent buckets.
	Restate int
	// Keep is how much history Load hands back to the UI.
	Keep time.Duration
}

// Cached returns what's already stored, without touching the network.
func (s *Syncer) Cached(ctx context.Context, now time.Time) ([]spend.Line, time.Time, error) {
	_, at, _, err := s.Store.LastSync(ctx, s.Provider.Name())
	if err != nil {
		return nil, time.Time{}, err
	}
	lines, err := s.Store.Load(ctx, s.Provider.Name(), spend.Day(now).Add(-s.Keep))
	return lines, at, err
}

// Sync fetches whatever is missing or still open, stores it, and returns the
// stored history.
func (s *Syncer) Sync(ctx context.Context, now time.Time) ([]spend.Line, error) {
	name := s.Provider.Name()
	end := spend.Day(now).AddDate(0, 0, 1)
	start := spend.Day(now).Add(-s.Backfill)
	if last, _, ok, err := s.Store.LastSync(ctx, name); err != nil {
		return nil, err
	} else if ok {
		if from := last.AddDate(0, 0, -s.Restate); from.After(start) {
			start = from
		}
	}
	lines, err := s.Provider.Fetch(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if err := s.Store.Replace(ctx, name, start, end, lines); err != nil {
		return nil, fmt.Errorf("saving %s spend: %w", name, err)
	}
	out, err := s.Store.Load(ctx, name, spend.Day(now).Add(-s.Keep))
	sort.SliceStable(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, err
}
