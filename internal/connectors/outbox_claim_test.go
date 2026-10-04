package connectors_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// fakeOutboxRepo emulates the SQL semantics of the connector outbox queries
// (GetPendingOutboxCommands, ClaimOutboxCommand, DeadLetterExhaustedOutboxCommands,
// UpdateOutboxCommandState) over an in-memory table.
type fakeOutboxRepo struct {
	mu       sync.Mutex
	now      func() time.Time
	commands map[int64]*connectors.OutboxCommand
	updates  []connectors.OutboxCommandStateUpdate
	claimErr error
	// staleSelect makes GetPendingOutboxCommands return every non-terminal
	// command regardless of lease, simulating a selection taken before a
	// concurrent run claimed the command.
	staleSelect bool
}

func newFakeOutboxRepo(cmds ...connectors.OutboxCommand) *fakeOutboxRepo {
	r := &fakeOutboxRepo{now: time.Now, commands: map[int64]*connectors.OutboxCommand{}}
	for i := range cmds {
		c := cmds[i]
		r.commands[c.ID] = &c
	}
	return r
}

func (r *fakeOutboxRepo) GetConnection(_ context.Context, companyID, connectionID int64) (connectors.Connection, error) {
	return connectors.Connection{ID: connectionID, CompanyID: companyID, Provider: "fake", Status: connectors.StatusHealthy}, nil
}

func (r *fakeOutboxRepo) GetPendingOutboxCommands(_ context.Context, limit int32) ([]connectors.OutboxCommand, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []connectors.OutboxCommand
	for _, c := range r.commands {
		if c.State != "pending" && c.State != "processing" {
			continue
		}
		if !r.staleSelect && c.NextAttempt.After(r.now()) {
			continue
		}
		out = append(out, *c)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeOutboxRepo) ClaimOutboxCommand(_ context.Context, id int64) (connectors.OutboxCommand, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return connectors.OutboxCommand{}, r.claimErr
	}
	c, ok := r.commands[id]
	if !ok || (c.State != "pending" && c.State != "processing") || c.NextAttempt.After(r.now()) || c.Attempts >= 5 {
		return connectors.OutboxCommand{}, pgx.ErrNoRows
	}
	c.State = "processing"
	c.Attempts++
	c.NextAttempt = r.now().Add(10 * time.Minute)
	return *c, nil
}

func (r *fakeOutboxRepo) DeadLetterExhaustedOutboxCommands(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, c := range r.commands {
		if (c.State == "pending" || c.State == "processing") && c.Attempts >= 5 && !c.NextAttempt.After(r.now()) {
			c.State = "dead_letter"
			n++
		}
	}
	return n, nil
}

func (r *fakeOutboxRepo) UpdateOutboxCommandState(_ context.Context, u connectors.OutboxCommandStateUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, u)
	c, ok := r.commands[u.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	// attempts is intentionally not incremented: the claim owns that counter.
	c.State = u.State
	c.NextAttempt = u.NextAttempt
	return nil
}

func (r *fakeOutboxRepo) get(id int64) connectors.OutboxCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.commands[id]
}

// recordingAdapter counts executions and can run a hook during execution.
type recordingAdapter struct {
	mu       sync.Mutex
	executed []int64
	attempts []int
	err      error
	onExec   func()
}

func (a *recordingAdapter) ValidateConnection(context.Context, *connectors.Connection) error {
	return nil
}

func (a *recordingAdapter) CheckHealth(context.Context, *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

func (a *recordingAdapter) RefreshToken(context.Context, *connectors.Connection) error { return nil }

func (a *recordingAdapter) VerifyCallbackSignature(context.Context, *connectors.Connection, map[string]string, []byte) error {
	return nil
}

func (a *recordingAdapter) ExecuteCommand(_ context.Context, _ *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.mu.Lock()
	a.executed = append(a.executed, cmd.ID)
	a.attempts = append(a.attempts, cmd.Attempts)
	hook := a.onExec
	a.onExec = nil
	a.mu.Unlock()
	if hook != nil {
		hook()
	}
	return a.err
}

func (a *recordingAdapter) TranslateWebhook(context.Context, *connectors.Connection, map[string]string, []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, nil
}

func (a *recordingAdapter) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.executed)
}

func pendingCommand(id int64, attempts int, nextAttempt time.Time) connectors.OutboxCommand {
	return connectors.OutboxCommand{
		ID: id, CompanyID: 1, ConnectionID: 1, CommandType: "payment.charge",
		CorrelationID: "corr", State: "pending", Attempts: attempts, NextAttempt: nextAttempt,
	}
}

func TestOutboxWorkerClaimBeforeExecute(t *testing.T) {
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)

	tests := []struct {
		name         string
		cmd          connectors.OutboxCommand
		adapterErr   error
		staleSelect  bool
		overlap      bool // a second ProcessPending runs while the first is executing
		wantExecuted int
		wantState    string
		wantAttempts int
		wantBackoff  time.Duration // expected next_attempt - now for pending retries
	}{
		{
			name:         "success completes with one attempt",
			cmd:          pendingCommand(1, 0, past),
			wantExecuted: 1, wantState: "completed", wantAttempts: 1,
		},
		{
			name:         "overlapping run with stale selection cannot claim and does not execute",
			cmd:          pendingCommand(2, 0, past),
			staleSelect:  true,
			overlap:      true,
			wantExecuted: 1, wantState: "completed", wantAttempts: 1,
		},
		{
			name:         "failure records backoff using the claimed attempt count",
			cmd:          pendingCommand(3, 1, past),
			adapterErr:   errors.New("provider down"),
			wantExecuted: 1, wantState: "pending", wantAttempts: 2, wantBackoff: 4 * time.Minute,
		},
		{
			name:         "failure on the last attempt dead-letters",
			cmd:          pendingCommand(4, 4, past),
			adapterErr:   errors.New("provider down"),
			wantExecuted: 1, wantState: "dead_letter", wantAttempts: 5,
		},
		{
			name:         "exhausted command dead-letters without execution",
			cmd:          pendingCommand(5, 5, past),
			wantExecuted: 0, wantState: "dead_letter", wantAttempts: 5,
		},
		{
			name:         "exhausted processing command whose lease expired dead-letters",
			cmd:          func() connectors.OutboxCommand { c := pendingCommand(6, 5, past); c.State = "processing"; return c }(),
			wantExecuted: 0, wantState: "dead_letter", wantAttempts: 5,
		},
		{
			name:         "leased command is not claimed even from a stale selection",
			cmd:          func() connectors.OutboxCommand { c := pendingCommand(7, 1, time.Now().Add(5*time.Minute)); c.State = "processing"; return c }(),
			staleSelect:  true,
			wantExecuted: 0, wantState: "processing", wantAttempts: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeOutboxRepo(tc.cmd)
			repo.staleSelect = tc.staleSelect
			adapter := &recordingAdapter{err: tc.adapterErr}
			registry := connectors.NewRegistry()
			registry.Register("fake", adapter)
			worker := connectors.NewOutboxWorker(repo, registry)

			if tc.overlap {
				adapter.onExec = func() {
					if err := worker.ProcessPending(ctx, 100); err != nil {
						t.Errorf("overlapping ProcessPending: %v", err)
					}
				}
			}

			start := time.Now()
			if err := worker.ProcessPending(ctx, 100); err != nil {
				t.Fatalf("ProcessPending: %v", err)
			}
			if got := adapter.count(); got != tc.wantExecuted {
				t.Fatalf("executed %d times, want %d", got, tc.wantExecuted)
			}
			got := repo.get(tc.cmd.ID)
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Attempts != tc.wantAttempts {
				t.Errorf("attempts = %d, want %d", got.Attempts, tc.wantAttempts)
			}
			if tc.wantExecuted > 0 && adapter.attempts[0] != tc.wantAttempts {
				t.Errorf("adapter saw attempts = %d, want claimed value %d", adapter.attempts[0], tc.wantAttempts)
			}
			if tc.wantBackoff > 0 {
				delay := got.NextAttempt.Sub(start)
				if delay < tc.wantBackoff || delay > tc.wantBackoff+time.Minute {
					t.Errorf("backoff = %s, want about %s", delay, tc.wantBackoff)
				}
			}
		})
	}
}

func TestOutboxWorkerSecondSweepAfterCompletionDoesNotExecute(t *testing.T) {
	ctx := context.Background()
	repo := newFakeOutboxRepo(pendingCommand(1, 0, time.Now().Add(-time.Minute)))
	repo.staleSelect = true
	adapter := &recordingAdapter{}
	registry := connectors.NewRegistry()
	registry.Register("fake", adapter)
	worker := connectors.NewOutboxWorker(repo, registry)

	for i := 0; i < 2; i++ {
		if err := worker.ProcessPending(ctx, 100); err != nil {
			t.Fatalf("ProcessPending #%d: %v", i+1, err)
		}
	}
	if got := adapter.count(); got != 1 {
		t.Fatalf("executed %d times, want 1", got)
	}
}

func TestOutboxWorkerClaimErrorSkipsExecutionAndIsReported(t *testing.T) {
	ctx := context.Background()
	repo := newFakeOutboxRepo(pendingCommand(1, 0, time.Now().Add(-time.Minute)))
	repo.claimErr = errors.New("connection reset")
	adapter := &recordingAdapter{}
	registry := connectors.NewRegistry()
	registry.Register("fake", adapter)

	err := connectors.NewOutboxWorker(repo, registry).ProcessPending(ctx, 100)
	if err == nil || !errors.Is(err, repo.claimErr) {
		t.Fatalf("ProcessPending error = %v, want wrapped claim error", err)
	}
	if adapter.count() != 0 {
		t.Fatal("command executed without a claim")
	}
	if len(repo.updates) != 0 {
		t.Fatalf("unexpected state updates: %+v", repo.updates)
	}
}
