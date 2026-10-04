// Package store contains all SQL. Handlers never build queries themselves.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{db: pool} }

// Pool exposes the connection pool for tools such as the importer.
func (s *Store) Pool() *pgxpool.Pool { return s.db }

var (
	ErrNotFound  = pgx.ErrNoRows
	ErrTooMany   = errors.New("too many items")
	ErrDuplicate = errors.New("duplicate")
	ErrInUse     = errors.New("in use")
)

// classify maps Postgres constraint errors to store errors.
func classify(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrDuplicate
		case "23503", "23001": // foreign_key_violation, restrict_violation
			return ErrInUse
		}
	}
	return err
}

func collect[T any](ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string, args ...any) ([]T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByNameLax[T])
	if items == nil {
		items = []T{}
	}
	return items, err
}

func one[T any](ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string, args ...any) (T, error) {
	var zero T
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return zero, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[T])
}
