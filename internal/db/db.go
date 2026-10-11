// Package db opens the Postgres pool and applies embedded migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 3 // keep warm connections; each new TLS handshake to the DB costs a round trip
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return pool, nil
}

// Migrate applies every migration that has not run yet. Safe to call on every
// start. Pending migrations run in one transaction holding a transaction-level
// advisory lock, so concurrent starts are serialised and the lock is always
// released when the transaction ends (session locks can leak through a
// connection pooler such as Neon's PgBouncer).
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, err
	}

	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	// Fast path: nothing to do (the usual case) needs no lock.
	done := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		done[v] = true
	}
	rows.Close()
	pending := false
	for _, f := range files {
		if !done[migrationVersion(f)] {
			pending = true
			break
		}
	}
	if !pending {
		return nil, nil
	}

	const lockID = 734001 // arbitrary, unique to this app
	var applied []string
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
			return err
		}
		for _, f := range files {
			version := migrationVersion(f)
			// Re-check under the lock: another start may have applied it.
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
				return err
			}
			if exists {
				continue
			}
			sql, err := migrations.ReadFile(f)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("migration %s: %w", version, err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
				return err
			}
			applied = append(applied, version)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return applied, nil
}

func migrationVersion(file string) string {
	return strings.TrimSuffix(strings.TrimPrefix(file, "migrations/"), ".sql")
}
