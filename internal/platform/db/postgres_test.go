package db

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestConfigWithMaxConns(t *testing.T) {
	plain, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db?sslmode=disable")
	require.NoError(t, err)
	pgxDefault := plain.MaxConns

	const plainURL = "postgres://u:p@localhost:5432/db?sslmode=disable"
	tests := []struct {
		name         string
		dsn          string
		override     int32
		defaultConns int32
		want         int32
	}{
		{name: "nothing set uses default", dsn: plainURL, defaultConns: 16, want: 16},
		{name: "postgresql scheme without param uses default", dsn: "postgresql://u:p@localhost/db", defaultConns: 16, want: 16},
		{name: "dsn param beats default", dsn: plainURL + "&pool_max_conns=7", defaultConns: 16, want: 7},
		{name: "dsn param larger than default beats default", dsn: "postgres://u:p@localhost/db?pool_max_conns=40", defaultConns: 16, want: 40},
		{name: "override beats default", dsn: plainURL, override: 24, defaultConns: 16, want: 24},
		{name: "override beats dsn param", dsn: plainURL + "&pool_max_conns=7", override: 24, defaultConns: 16, want: 24},
		{name: "smaller override beats larger dsn param", dsn: "postgres://u:p@localhost/db?pool_max_conns=40", override: 6, defaultConns: 16, want: 6},
		{name: "override beats keyword dsn param", dsn: "host=localhost user=u dbname=db pool_max_conns=5", override: 12, defaultConns: 16, want: 12},
		{name: "zero override falls through to dsn param", dsn: plainURL + "&pool_max_conns=7", override: 0, defaultConns: 16, want: 7},
		{name: "negative override is ignored", dsn: plainURL + "&pool_max_conns=7", override: -3, defaultConns: 16, want: 7},
		{name: "negative override falls through to default", dsn: plainURL, override: -3, defaultConns: 16, want: 16},
		{name: "keyword dsn without param uses default", dsn: "host=localhost user=u dbname=db sslmode=disable", defaultConns: 16, want: 16},
		{name: "keyword dsn with param honored", dsn: "host=localhost user=u dbname=db pool_max_conns=5", defaultConns: 16, want: 5},
		{name: "keyword dsn with spaced param honored", dsn: "host=localhost pool_max_conns = 9 user=u", defaultConns: 16, want: 9},
		{name: "zero default keeps pgx default", dsn: "postgres://u:p@localhost/db", defaultConns: 0, want: pgxDefault},
		{name: "override applies even without a default", dsn: "postgres://u:p@localhost/db", override: 11, defaultConns: 0, want: 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ConfigWithMaxConns(tc.dsn, tc.override, tc.defaultConns)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.MaxConns)
		})
	}
}

func TestConfigWithMaxConnsRejectsInvalidDSN(t *testing.T) {
	_, err := ConfigWithMaxConns("postgres://u:p@localhost/db?pool_max_conns=abc", 0, 16)
	require.Error(t, err)

	// An override does not paper over an unparsable DSN.
	_, err = ConfigWithMaxConns("postgres://u:p@localhost/db?pool_max_conns=abc", 8, 16)
	require.Error(t, err)
}
