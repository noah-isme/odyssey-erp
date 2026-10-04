package db

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolMaxConnsParam is the pgx DSN parameter that sets the pool size.
const PoolMaxConnsParam = "pool_max_conns"

var keywordPoolMaxConns = regexp.MustCompile(`(^|\s)` + PoolMaxConnsParam + `\s*=`)

// New creates a new PostgreSQL connection pool.
func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("platform/db: parse config: %w", err)
	}
	return newWithConfig(ctx, config)
}

// NewWithMaxConns creates a pool like New, but sizes it explicitly. The pool
// size is resolved in this order: a positive override (for example the
// PG_MAX_CONNS environment setting), then pool_max_conns in the DSN, then
// defaultMaxConns, then pgx's own default of max(4, NumCPU).
func NewWithMaxConns(ctx context.Context, dsn string, override, defaultMaxConns int32) (*pgxpool.Pool, error) {
	config, err := ConfigWithMaxConns(dsn, override, defaultMaxConns)
	if err != nil {
		return nil, err
	}
	return newWithConfig(ctx, config)
}

// ConfigWithMaxConns parses dsn and resolves the pool size. Precedence:
//
//  1. override, when positive;
//  2. pool_max_conns in the DSN;
//  3. defaultMaxConns, when positive;
//  4. pgx's default.
//
// pool_max_conns is a pgx-only parameter. A DSN that is shared with
// golang-migrate must not carry it, because the migrate postgres driver
// (lib/pq) forwards unknown keys to the server as run-time settings and
// PostgreSQL rejects them. The override exists so the pool size can be
// configured without touching that shared DSN.
func ConfigWithMaxConns(dsn string, override, defaultMaxConns int32) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("platform/db: parse config: %w", err)
	}
	switch {
	case override > 0:
		config.MaxConns = override
	case dsnSetsPoolMaxConns(dsn):
		// pgx already applied the DSN value.
	case defaultMaxConns > 0:
		config.MaxConns = defaultMaxConns
	}
	return config, nil
}

// dsnSetsPoolMaxConns reports whether a URL or keyword/value DSN carries the
// pool_max_conns parameter.
func dsnSetsPoolMaxConns(dsn string) bool {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return false
		}
		return u.Query().Has(PoolMaxConnsParam)
	}
	return keywordPoolMaxConns.MatchString(dsn)
}

func newWithConfig(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("platform/db: new pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("platform/db: ping: %w", err)
	}

	return pool, nil
}
