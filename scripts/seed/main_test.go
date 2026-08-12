package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingSeedExecutor struct {
	queries []string
	err     error
}

func (e *recordingSeedExecutor) Exec(_ context.Context, query string, _ ...any) (pgconn.CommandTag, error) {
	e.queries = append(e.queries, query)
	return pgconn.NewCommandTag("INSERT 0 6"), e.err
}

func TestSyncGlobalRoleAssignmentsUsesIdempotentCompanyWideCompatibilityRows(t *testing.T) {
	executor := &recordingSeedExecutor{}

	if err := syncGlobalRoleAssignments(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	if err := syncGlobalRoleAssignments(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	if len(executor.queries) != 2 || executor.queries[0] != executor.queries[1] {
		t.Fatalf("queries = %#v, want the same synchronization statement on every run", executor.queries)
	}

	query := executor.queries[0]
	for _, fragment := range []string{
		"INSERT INTO rbac_user_role_assignments",
		"FROM companies c",
		"CROSS JOIN user_roles ur",
		"TIMESTAMPTZ '1970-01-01 00:00:00+00'",
		"ON CONFLICT (company_id, user_id, role_id, valid_from)",
		"WHERE branch_id IS NULL",
		"DO NOTHING",
	} {
		if !strings.Contains(query, fragment) {
			t.Errorf("synchronization query missing %q", fragment)
		}
	}
}

func TestSyncGlobalRoleAssignmentsReturnsExecutorError(t *testing.T) {
	want := errors.New("database unavailable")
	err := syncGlobalRoleAssignments(context.Background(), &recordingSeedExecutor{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped %v", err, want)
	}
}
