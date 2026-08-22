package treasury

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

type operationsReaderFake struct {
	operation PaymentOperation
	byResult  PaymentOperation
}

func (f operationsReaderFake) ListOperations(context.Context, int64, OperationsFilter) ([]PaymentOperation, error) {
	return []PaymentOperation{f.operation}, nil
}

func (f operationsReaderFake) GetOperation(context.Context, int64, string) (PaymentOperation, error) {
	return f.operation, nil
}

func (f operationsReaderFake) GetOperationByResult(context.Context, int64, string) (PaymentOperation, error) {
	return f.byResult, nil
}

type operationsReplayerFake struct {
	id      int64
	company int64
	actor   int64
	key     string
	calls   int
}

func (f *operationsReplayerFake) Replay(_ context.Context, companyID, id int64, key string, actorID int64) (automation.OutboxMessage, error) {
	f.calls++
	f.company, f.id, f.key, f.actor = companyID, id, key, actorID
	return automation.OutboxMessage{ID: id, CompanyID: companyID}, nil
}

func TestOperationsServiceRecoveryReplaysOnlyAmbiguousExecutionDeadLetter(t *testing.T) {
	reader := operationsReaderFake{operation: PaymentOperation{
		InstructionID: "instruction-1",
		State:         string(payments.StateAmbiguous),
		Outbox: []PaymentOperationOutbox{{
			ID:        17,
			Operation: automation.OperationPaymentExecute,
			Status:    string(automation.OutboxDeadLettered),
		}},
	}}
	replayer := &operationsReplayerFake{}
	service := NewOperationsService(reader, replayer)
	require.NoError(t, service.Recover(context.Background(), 7, "instruction-1", 303))
	require.Equal(t, 1, replayer.calls)
	require.Equal(t, int64(7), replayer.company)
	require.Equal(t, int64(17), replayer.id)
	require.Equal(t, int64(303), replayer.actor)
	require.NotEmpty(t, replayer.key)
}

func TestOperationsServiceRejectsRecoveryForSettledInstruction(t *testing.T) {
	service := NewOperationsService(operationsReaderFake{operation: PaymentOperation{State: string(payments.StateSettled)}}, &operationsReplayerFake{})
	require.ErrorIs(t, service.Recover(context.Background(), 7, "instruction-1", 303), ErrRecoveryNotAllowed)
}

func TestOperationsServiceRetryEffectsReplaysResultImportDeadLetter(t *testing.T) {
	reader := operationsReaderFake{byResult: PaymentOperation{
		Results: []PaymentOperationResult{{ResultID: "result-1", State: string(payments.StateSettled)}},
		Outbox: []PaymentOperationOutbox{{
			ID:        22,
			Operation: automation.OperationPaymentResultImport,
			ResultID:  "result-1",
			Status:    string(automation.OutboxDeadLettered),
		}},
	}}
	replayer := &operationsReplayerFake{}
	service := NewOperationsService(reader, replayer)
	require.NoError(t, service.RetryEffects(context.Background(), 7, "result-1", 303))
	require.Equal(t, int64(22), replayer.id)
}

func TestOperationsMaskHelpers(t *testing.T) {
	require.Equal(t, "********1234", MaskSensitive("account-1234"))
	require.Equal(t, "S*******", MaskName("Supplier"))
	require.Equal(t, "", MaskSensitive(""))
}

func TestOperationsRepositoryMasksExecutionPayloadAndLoadsDetails(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	repo := NewOperationsRepository(db)

	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: "iris"}
	instruction := payments.Instruction{
		Reference:         automation.ExternalReference{Connection: connection, ObjectType: "treasury_payment_item", ObjectID: "instruction-1"},
		Correlation:       automation.Correlation{ID: "corr-1"},
		BeneficiaryRef:    "account-1234",
		BeneficiaryName:   "Supplier",
		Amount:            automation.MustParseExact("125.50"),
		EndToEndReference: "e2e-1234",
	}
	payload, err := json.Marshal(payments.PaymentExecution{Instruction: instruction, State: payments.StateAmbiguous})
	require.NoError(t, err)
	now := time.Date(2026, time.August, 20, 10, 0, 0, 0, time.UTC)
	db.ExpectQuery("SELECT id, company_id, connection_id").WithArgs(int64(7), "instruction-1").WillReturnRows(
		pgxmock.NewRows([]string{"id", "company_id", "connection_id", "provider", "object_type", "object_id", "state", "payload", "created_at", "updated_at"}).AddRow(int64(1), int64(7), int64(11), "iris", "treasury_payment_item", "instruction-1", "AMBIGUOUS", payload, now, now),
	)
	db.ExpectQuery("SELECT result_id, status, state").WithArgs(int64(7), int64(11), "treasury_payment_item", "instruction-1").WillReturnRows(pgxmock.NewRows([]string{"result_id", "status", "state", "effect_applied", "coalesce", "currency", "recorded_at"}))
	db.ExpectQuery("SELECT e.effect_key, e.result_id").WithArgs(int64(7), int64(11), "treasury_payment_item", "instruction-1").WillReturnRows(pgxmock.NewRows([]string{"effect_key", "result_id", "state", "applied_at", "count"}))
	db.ExpectQuery("SELECT id, operation, status").WithArgs(int64(7), "instruction-1").WillReturnRows(pgxmock.NewRows([]string{"id", "operation", "status", "attempts", "max_attempts", "last_error", "dead_lettered_at", "replayed_from_id", "created_at", "payload"}))
	db.ExpectQuery("SELECT EXISTS").WithArgs(int64(7), "e2e-1234").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	operation, err := repo.GetOperation(context.Background(), 7, "instruction-1")
	require.NoError(t, err)
	require.Equal(t, "********1234", operation.BeneficiaryRef)
	require.Equal(t, "S*******", operation.BeneficiaryName)
	require.Equal(t, "****1234", operation.EndToEndReference)
	require.Equal(t, "125.50", operation.Amount)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestOperationsServiceRequiresResultReaderExtension(t *testing.T) {
	reader := operationsReaderOnlyFake{}
	service := NewOperationsService(reader, &operationsReplayerFake{})
	require.ErrorIs(t, service.RetryEffects(context.Background(), 7, "result-1", 303), ErrEffectsRetryNotAvailable)
}

type operationsReaderOnlyFake struct{}

func (operationsReaderOnlyFake) ListOperations(context.Context, int64, OperationsFilter) ([]PaymentOperation, error) {
	return nil, nil
}

func (operationsReaderOnlyFake) GetOperation(context.Context, int64, string) (PaymentOperation, error) {
	return PaymentOperation{}, errors.New("not implemented")
}
