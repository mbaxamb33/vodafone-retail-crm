// Package postgres implements the CRM and authentication stores on PostgreSQL.
package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"vodafone/store/internal/apperr"
	"vodafone/store/internal/crm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// queries holds every read and write; it runs against the pool or inside a transaction.
type queries struct{ q querier }

// DB is the application's PostgreSQL store.
type DB struct {
	queries
	Pool *pgxpool.Pool
}

// Open connects using a libpq-style URL. Extra runtime parameters (such as search_path in
// tests) may be provided.
func Open(ctx context.Context, url string, runtimeParams map[string]string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	for k, v := range runtimeParams {
		cfg.ConnConfig.RuntimeParams[k] = v
	}
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		// Return timestamps in UTC rather than the server process's local zone.
		conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return &DB{queries: queries{pool}, Pool: pool}, nil
}

func (db *DB) Close() { db.Pool.Close() }

func (db *DB) Ping(ctx context.Context) error { return db.Pool.Ping(ctx) }

// InTx runs fn inside a READ COMMITTED transaction. Business operations lock the rows they
// change with SELECT ... FOR UPDATE, which serializes concurrent edits of the same record.
func (db *DB) InTx(ctx context.Context, fn func(tx crm.Tx) error) error {
	return pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		return fn(&queries{tx})
	})
}

// Migrate applies pending migrations in lexical order, each in its own transaction. An advisory
// lock makes concurrent server starts safe.
func (db *DB) Migrate(ctx context.Context) error {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	const lockKey = 7_404_202_609
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sql, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", version, err)
		}
	}
	return nil
}

// mapErr converts driver errors into application errors where the meaning is clear.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "22P02": // invalid text representation, e.g. a malformed UUID
			return apperr.ErrNotFound
		case "23505":
			if pg.ConstraintName == "users_email_key" {
				return apperr.ErrEmailTaken
			}
			return apperr.ErrConflict
		}
	}
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
