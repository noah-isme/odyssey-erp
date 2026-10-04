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

// NewWithDefaults creates a pool like New, but when the DSN does not carry
// pool_max_conns the pool size is defaultMaxConns instead of pgx's default
// of max(4, NumCPU). An explicit pool_max_conns in the DSN is honored as is.
func NewWithDefaults(ctx context.Context, dsn string, defaultMaxConns int32) (*pgxpool.Pool, error) {
	config, err := ConfigWithDefaults(dsn, defaultMaxConns)
	if err != nil {
		return nil, err
	}
	return newWithConfig(ctx, config)
}

// ConfigWithDefaults parses dsn and applies defaultMaxConns when the DSN does
// not set pool_max_conns.
func ConfigWithDefaults(dsn string, defaultMaxConns int32) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("platform/db: parse config: %w", err)
	}
	if defaultMaxConns > 0 && !dsnSetsPoolMaxConns(dsn) {
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
