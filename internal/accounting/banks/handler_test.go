package banks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type statementsRepository struct {
	accountID     int64
	accountErr    error
	listCalled    bool
	companyID     int64
	statements    []BankStatement
	statementsErr error
}

func (r *statementsRepository) ImportStatement(context.Context, int64, time.Time, []ParsedStatementLine) (int64, error) {
	return 0, nil
}
func (r *statementsRepository) ConfirmStatement(context.Context, int64) error          { return nil }
func (r *statementsRepository) PerformTransfer(context.Context, TransferRequest) error { return nil }
func (r *statementsRepository) BankAccountIDForCompany(_ context.Context, companyID int64) (int64, error) {
	r.companyID = companyID
	return r.accountID, r.accountErr
}
func (r *statementsRepository) ListStatements(context.Context, int64, int32, int32) ([]BankStatement, error) {
	r.listCalled = true
	return r.statements, r.statementsErr
}
func (r *statementsRepository) GetStatement(context.Context, int64) (BankStatement, error) {
	return BankStatement{}, nil
}
func (r *statementsRepository) ListStatementLines(context.Context, int64) ([]BankStatementLine, error) {
	return nil, nil
}

type statementsRenderer struct {
	called bool
	name   string
	data   view.TemplateData
}

func (r *statementsRenderer) Render(w http.ResponseWriter, name string, data view.TemplateData) error {
	r.called = true
	r.name = name
	r.data = data
	w.WriteHeader(http.StatusOK)
	return nil
}

func statementsRequest(t *testing.T, companyID string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, "/accounting/banks/statements", nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("9")
	session.Set("company_id", companyID)
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func TestListStatementsWithoutBankAccountRendersEmptyState(t *testing.T) {
	repo := &statementsRepository{accountErr: ErrNoBankAccount}
	renderer := &statementsRenderer{}
	handler := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), NewService(repo), renderer, nil)
	recorder := httptest.NewRecorder()

	handler.listStatements(recorder, statementsRequest(t, "2"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !renderer.called || renderer.name != "pages/accounting/bank_statements.html" {
		t.Fatalf("renderer call = %v %q", renderer.called, renderer.name)
	}
	if repo.companyID != 2 {
		t.Fatalf("company id = %d, want 2", repo.companyID)
	}
	if repo.listCalled {
		t.Fatal("ListStatements called without a bank account")
	}
	data := renderer.data.Data.(map[string]any)
	statements, ok := data["Statements"].([]BankStatement)
	if !ok || len(statements) != 0 {
		t.Fatalf("statements = %#v, want empty []BankStatement", data["Statements"])
	}
}

func TestListStatementsStillFailsForRepositoryError(t *testing.T) {
	repo := &statementsRepository{accountErr: errors.New("database unavailable")}
	renderer := &statementsRenderer{}
	handler := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), NewService(repo), renderer, nil)
	recorder := httptest.NewRecorder()

	handler.listStatements(recorder, statementsRequest(t, "2"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if renderer.called {
		t.Fatal("renderer called for unexpected repository error")
	}
}
