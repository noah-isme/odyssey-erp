package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDBScenarioQueriesRun executes every dynamic-value SELECT and every
// scenario snapshot query against the migrated local database (ISO004_PG_DSN)
// over the read-only pool, so column drift fails here rather than on staging.
// It also checks that each column an assertion names is returned.
func TestDBScenarioQueriesRun(t *testing.T) {
	dsn := iso004DSN(t)
	ctx := context.Background()
	fx, _, err := loadFixtures(iso004FixturesFile(), os.Getenv)
	require.NoError(t, err)
	p := pool(t, dsn)

	names := make([]string, 0, len(dynamicSpecs))
	for _, d := range dynamicSpecs {
		names = append(names, d.Name)
	}
	dynamic, err := resolveDynamics(ctx, p, names)
	require.NoError(t, err)
	require.Len(t, dynamic, len(dynamicSpecs))
	for name, v := range dynamic {
		if n, ok := v.Value.(int64); ok {
			assert.GreaterOrEqual(t, n, int64(100000), name)
		}
	}

	env := renderEnv{Cfg: &Config{RunID: fx.Key, MaxRetry: 2}, Fixtures: fx, Dynamic: dynamic}
	for _, s := range scenarioRegistry {
		t.Run(s.ID, func(t *testing.T) {
			sp, err := env.renderScenario(s)
			require.NoError(t, err)
			columns := map[string]map[string]bool{}
			for _, q := range sp.Queries {
				rows, err := p.Query(ctx, q.SQL, q.Args...)
				require.NoError(t, err, q.Name)
				cols := map[string]bool{}
				for _, fd := range rows.FieldDescriptions() {
					cols[fd.Name] = true
				}
				for rows.Next() {
					_, err := rows.Values()
					require.NoError(t, err, q.Name)
				}
				require.NoError(t, rows.Err(), q.Name)
				rows.Close()
				columns[q.Name] = cols
			}
			for _, a := range sp.Assertions {
				if a.Column == "" {
					continue
				}
				assert.True(t, columns[a.Query][a.Column], "%s: query %s returns no column %q", a.ID, a.Query, a.Column)
			}
		})
	}
}
