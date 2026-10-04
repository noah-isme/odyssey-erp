//go:build integration

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// outboxBarrierAdapter blocks the first ExecuteCommand for the watched command
// until release is closed; later calls return immediately. It counts calls.
type outboxBarrierAdapter struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func newOutboxBarrierAdapter() *outboxBarrierAdapter {
	return &outboxBarrierAdapter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (a *outboxBarrierAdapter) ValidateConnection(context.Context, *connectors.Connection) error {
	return nil
}

func (a *outboxBarrierAdapter) CheckHealth(context.Context, *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

func (a *outboxBarrierAdapter) RefreshToken(context.Context, *connectors.Connection) error {
	return nil
}

func (a *outboxBarrierAdapter) VerifyCallbackSignature(context.Context, *connectors.Connection, map[string]string, []byte) error {
	return nil
}

func (a *outboxBarrierAdapter) ExecuteCommand(context.Context, *connectors.Connection, *connectors.OutboxCommand) error {
	a.mu.Lock()
	a.calls++
	first := a.calls == 1
	a.mu.Unlock()
	if first {
		close(a.entered)
		select {
		case <-a.release:
		case <-time.After(30 * time.Second):
			return errors.New("outbox barrier adapter: release timeout")
		}
	}
	return nil
}

func (a *outboxBarrierAdapter) TranslateWebhook(context.Context, *connectors.Connection, map[string]string, []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, nil
}

func (a *outboxBarrierAdapter) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

// scopedOutboxRepo restricts the pending selection to one command so the test
// never touches rows owned by other suites, and holds every selection until
// `parties` callers have selected: both workers then race on the claim with
// the same stale view of the row.
type scopedOutboxRepo struct {
	*connectors.PGRepository
	commandID int64
	selected  sync.WaitGroup
}

func (r *scopedOutboxRepo) GetPendingOutboxCommands(ctx context.Context, limit int32) ([]connectors.OutboxCommand, error) {
	rows, err := r.PGRepository.GetPendingOutboxCommands(ctx, 10000)
	if err != nil {
		return nil, err
	}
	var scoped []connectors.OutboxCommand
	for _, row := range rows {
		if row.ID == r.commandID {
			scoped = append(scoped, row)
		}
	}
	r.selected.Done()
	waited := make(chan struct{})
	go func() { r.selected.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(15 * time.Second):
		return nil, errors.New("scoped outbox repo: selection barrier timeout")
	}
	return scoped, nil
}

type singleAdapterRegistry struct{ adapter connectors.ProviderAdapter }

func (r singleAdapterRegistry) GetAdapter(string) (connectors.ProviderAdapter, error) {
	return r.adapter, nil
}

func openConnectorOutboxPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for connector outbox claim integration suite")
	}
	applyAllMigrations(t, dsn)
	p, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func seedOutboxCommand(t *testing.T, p *pgxpool.Pool, suffix string, attempts int) int64 {
	t.Helper()
	ctx := context.Background()
	var companyID, connectionID, commandID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'Outbox integration','IDR') RETURNING id`, "OB-"+suffix).Scan(&companyID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO connector_connections(company_id,provider,type,name,secret_ref,status) VALUES($1,'integration-barrier','payment','Outbox integration','vault:none','healthy') RETURNING id`, companyID).Scan(&connectionID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO connector_outbox_commands(company_id,connection_id,command_type,correlation_id,payload,attempts,next_attempt)
		VALUES($1,$2,'payment.charge',$3,'{}'::jsonb,$4,NOW() - INTERVAL '1 minute') RETURNING id`, companyID, connectionID, "corr-"+suffix, attempts).Scan(&commandID))
	return commandID
}

func outboxCommandState(t *testing.T, p *pgxpool.Pool, id int64) (string, int) {
	t.Helper()
	var state string
	var attempts int
	require.NoError(t, p.QueryRow(context.Background(), `SELECT state, attempts FROM connector_outbox_commands WHERE id=$1`, id).Scan(&state, &attempts))
	return state, attempts
}

func TestConnectorOutboxClaimConcurrentWorkersExecuteOnce(t *testing.T) {
	p := openConnectorOutboxPool(t)
	ctx := context.Background()
	commandID := seedOutboxCommand(t, p, fmt.Sprintf("dup-%d", time.Now().UnixNano()), 0)

	repo := &scopedOutboxRepo{PGRepository: connectors.NewRepository(p), commandID: commandID}
	repo.selected.Add(2)
	adapter := newOutboxBarrierAdapter()
	registry := singleAdapterRegistry{adapter: adapter}
	workerA := connectors.NewOutboxWorker(repo, registry)
	workerB := connectors.NewOutboxWorker(repo, registry)

	type result struct {
		name string
		err  error
	}
	done := make(chan result, 2)
	go func() { done <- result{"A", workerA.ProcessPending(ctx, 100)} }()
	go func() { done <- result{"B", workerB.ProcessPending(ctx, 100)} }()

	// Exactly one worker reaches the adapter and blocks there; the other must
	// return (claim lost) while the first execution is still in flight. The
	// two events can happen in either order.
	entered := false
	var loser *result
	deadline := time.After(15 * time.Second)
	for !entered || loser == nil {
		select {
		case <-adapter.entered:
			entered = true
			adapter.entered = nil // a nil channel never fires again
		case r := <-done:
			require.Nil(t, loser, "both workers returned while an execution was blocked")
			loser = &r
		case <-deadline:
			close(adapter.release)
			t.Fatalf("timed out: execution entered=%v, losing worker returned=%v", entered, loser != nil)
		}
	}
	require.NoError(t, loser.err, "losing worker %s", loser.name)
	require.Equal(t, 1, adapter.Calls(), "the losing worker must not execute the command")

	close(adapter.release)
	r := <-done
	require.NoError(t, r.err, "winning worker %s", r.name)

	require.Equal(t, 1, adapter.Calls(), "command executed exactly once")
	state, attempts := outboxCommandState(t, p, commandID)
	require.Equal(t, "completed", state)
	require.Equal(t, 1, attempts)
}

func TestConnectorOutboxClaimExhaustedCommandDeadLetters(t *testing.T) {
	p := openConnectorOutboxPool(t)
	ctx := context.Background()
	commandID := seedOutboxCommand(t, p, fmt.Sprintf("exh-%d", time.Now().UnixNano()), 5)

	repo := &scopedOutboxRepo{PGRepository: connectors.NewRepository(p), commandID: commandID}
	repo.selected.Add(1)
	adapter := newOutboxBarrierAdapter()
	close(adapter.release)

	require.NoError(t, connectors.NewOutboxWorker(repo, singleAdapterRegistry{adapter: adapter}).ProcessPending(ctx, 100))
	require.Equal(t, 0, adapter.Calls(), "exhausted command must not execute")
	state, attempts := outboxCommandState(t, p, commandID)
	require.Equal(t, "dead_letter", state)
	require.Equal(t, 5, attempts)
}
