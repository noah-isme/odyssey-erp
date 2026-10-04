package db

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestConfigWithDefaults(t *testing.T) {
	plain, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)
	pgxDefault := plain.MaxConns

	tests := []struct {
		name         string
		dsn          string
		defaultConns int32
		want         int32
	}{
		{name: "url without pool_max_conns uses default", dsn: "postgres://u:p@localhost:5432/db?sslmode=disable", defaultConns: 16, want: 16},
		{name: "postgresql scheme without param uses default", dsn: "postgresql://u:p@localhost/db", defaultConns: 16, want: 16},
		{name: "url with pool_max_conns honored", dsn: "postgres://u:p@localhost:5432/db?sslmode=disable&pool_max_conns=7", defaultConns: 16, want: 7},
		{name: "url with larger pool_max_conns honored", dsn: "postgres://u:p@localhost/db?pool_max_conns=40", defaultConns: 16, want: 40},
		{name: "keyword dsn without param uses default", dsn: "host=localhost user=u dbname=db sslmode=disable", defaultConns: 16, want: 16},
		{name: "keyword dsn with param honored", dsn: "host=localhost user=u dbname=db pool_max_conns=5", defaultConns: 16, want: 5},
		{name: "keyword dsn with spaced param honored", dsn: "host=localhost pool_max_conns = 9 user=u", defaultConns: 16, want: 9},
		{name: "zero default keeps pgx default", dsn: "postgres://u:p@localhost/db", defaultConns: 0, want: pgxDefault},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ConfigWithDefaults(tc.dsn, tc.defaultConns)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.MaxConns)
		})
	}
}

func TestConfigWithDefaultsRejectsInvalidDSN(t *testing.T) {
	_, err := ConfigWithDefaults("postgres://u:p@localhost/db?pool_max_conns=abc", 16)
	require.Error(t, err)
}
