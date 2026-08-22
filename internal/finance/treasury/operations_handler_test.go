package treasury

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/stretchr/testify/require"
)

func operationsRequestContext() context.Context {
	sess := &shared.Session{}
	sess.SetUser("303")
	sess.Set("company_id", "7")
	return shared.ContextWithSession(context.Background(), sess)
}

func TestOperationsHandlerListRequiresTenantIdentity(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)
	h := NewOperationsHandler(nil, NewOperationsService(operationsReaderFake{}, &operationsReplayerFake{}), engine, nil, rbac.Middleware{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	h.list(resp, req)
	require.Equal(t, http.StatusUnauthorized, resp.Code)
}

func TestOperationsHandlerListRendersMaskedOperation(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)
	reader := operationsReaderFake{operation: PaymentOperation{
		InstructionID:  "instruction-1",
		Provider:       "iris",
		State:          string(payments.StateAmbiguous),
		BeneficiaryRef: "********1234",
		Amount:         "125.50",
		Currency:       "USD",
	}}
	h := NewOperationsHandler(nil, NewOperationsService(reader, &operationsReplayerFake{}), engine, nil, rbac.Middleware{})
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(operationsRequestContext())
	resp := httptest.NewRecorder()
	h.list(resp, req)
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), "instruction-1")
	require.Contains(t, resp.Body.String(), "********1234")
	require.NotContains(t, resp.Body.String(), "account-1234")
}

func TestOperationsHandlerRecoveryUsesPRG(t *testing.T) {
	engine, err := view.NewEngine()
	require.NoError(t, err)
	reader := operationsReaderFake{operation: PaymentOperation{
		InstructionID: "instruction-1",
		State:         string(payments.StateAmbiguous),
		Outbox: []PaymentOperationOutbox{{
			ID:        9,
			Operation: automation.OperationPaymentExecute,
			Status:    string(automation.OutboxDeadLettered),
		}},
	}}
	replayer := &operationsReplayerFake{}
	h := NewOperationsHandler(nil, NewOperationsService(reader, replayer), engine, nil, rbac.Middleware{})
	req := httptest.NewRequest(http.MethodPost, "/instruction-1/recover", strings.NewReader(""))
	req = req.WithContext(operationsRequestContext())
	req = withOperationsParams(req, map[string]string{"instruction_id": "instruction-1"})
	resp := httptest.NewRecorder()
	h.recover(resp, req)
	require.Equal(t, http.StatusSeeOther, resp.Code)
	require.Equal(t, "/finance/treasury/operations/instruction-1", resp.Header().Get("Location"))
	require.Equal(t, 1, replayer.calls)
}

func withOperationsParams(request *http.Request, params map[string]string) *http.Request {
	routeContext := chi.NewRouteContext()
	for key, value := range params {
		routeContext.URLParams.Add(key, value)
	}
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
