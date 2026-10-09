package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// The workspace company scope reads rbac_user_role_assignments through the
// sqlc queries in sql/queries/rbac.sql. These tests pin which statements run
// for each case and with which arguments.

const (
	sqlListUserCompanies         = `-- name: ListUserCompanies :many`
	sqlUserHasActiveAssignment   = `-- name: UserHasActiveCompanyAssignment :one`
	sqlUserGlobalRoleNames       = `-- name: UserGlobalRoleNames :many`
	sqlAllCompaniesOrderedByName = `SELECT id, name FROM companies ORDER BY name, id`
)

func newWorkspaceMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return mock
}

func TestLoadWorkspaceCompaniesReadsActiveAssignments(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(int64(3), "Alpha").AddRow(int64(9), "Beta"))

	companies, err := loadWorkspaceCompanies(context.Background(), mock, 7, nil)
	require.NoError(t, err)
	require.Equal(t, []workspaceCompany{{ID: 3, Name: "Alpha"}, {ID: 9, Name: "Beta"}}, companies)
	require.NoError(t, mock.ExpectationsWereMet(), "no global-role lookup when RBAC_GLOBAL_ROLES is empty")
}

func TestLoadWorkspaceCompaniesEmptyWithoutAssignments(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}))

	companies, err := loadWorkspaceCompanies(context.Background(), mock, 7, nil)
	require.NoError(t, err)
	require.NotNil(t, companies, "an empty list must encode as [] in /api/me")
	require.Empty(t, companies)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadWorkspaceCompaniesGlobalRoleSeesEveryCompany(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlUserGlobalRoleNames).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("viewer").AddRow("Group Operator"))
	mock.ExpectQuery(sqlAllCompaniesOrderedByName).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Alpha").AddRow(int64(2), "Beta"))

	companies, err := loadWorkspaceCompanies(context.Background(), mock, 7, []string{" group operator "})
	require.NoError(t, err)
	require.Equal(t, []workspaceCompany{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}, companies)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadWorkspaceCompaniesUnlistedRoleIsScoped(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlUserGlobalRoleNames).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("admin"))
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(int64(3), "Alpha"))

	companies, err := loadWorkspaceCompanies(context.Background(), mock, 7, []string{"group-operator"})
	require.NoError(t, err)
	require.Equal(t, []workspaceCompany{{ID: 3, Name: "Alpha"}}, companies)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveWorkspaceCompanyKeepsPermittedCompany(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlUserHasActiveAssignment).
		WithArgs(int64(7), int64(3)).
		WillReturnRows(pgxmock.NewRows([]string{"has_assignment"}).AddRow(true))

	got, err := resolveWorkspaceCompany(context.Background(), mock, 7, 3, nil)
	require.NoError(t, err)
	require.Equal(t, int64(3), got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveWorkspaceCompanyReplacesForeignCompany(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlUserHasActiveAssignment).
		WithArgs(int64(7), int64(4)).
		WillReturnRows(pgxmock.NewRows([]string{"has_assignment"}).AddRow(false))
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(int64(9), "Alpha").AddRow(int64(3), "Beta"))

	got, err := resolveWorkspaceCompany(context.Background(), mock, 7, 4, nil)
	require.NoError(t, err)
	require.Equal(t, int64(9), got, "first permitted company by name, then id")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveWorkspaceCompanyUnsetPicksFirstPermitted(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}).AddRow(int64(9), "Alpha"))

	got, err := resolveWorkspaceCompany(context.Background(), mock, 7, 0, nil)
	require.NoError(t, err)
	require.Equal(t, int64(9), got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveWorkspaceCompanyFailsClosedWithoutAssignments(t *testing.T) {
	mock := newWorkspaceMock(t)
	mock.ExpectQuery(sqlListUserCompanies).
		WithArgs(int64(7)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name"}))

	got, err := resolveWorkspaceCompany(context.Background(), mock, 7, 0, nil)
	require.NoError(t, err)
	require.Zero(t, got, "no implicit lowest-id company")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequireActiveCompany(t *testing.T) {
	sessions := shared.NewSessionManager(nil, "s", "secret", time.Hour, false)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	cases := []struct {
		name    string
		user    string
		company string
		want    int
	}{
		{"signed in without company", "7", "", http.StatusForbidden},
		{"signed in with company 0", "7", "0", http.StatusForbidden},
		{"signed in with company", "7", "3", http.StatusNoContent},
		{"anonymous passes to the route's own checks", "", "", http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/finance/ar/invoices", nil)
			sess, err := sessions.Load(context.Background(), req)
			require.NoError(t, err)
			if tc.user != "" {
				sess.SetUser(tc.user)
			}
			if tc.company != "" {
				sess.Set("company_id", tc.company)
			}
			rec := httptest.NewRecorder()
			requireActiveCompany(ok).ServeHTTP(rec, req.WithContext(shared.ContextWithSession(req.Context(), sess)))
			require.Equal(t, tc.want, rec.Code)
		})
	}
}
