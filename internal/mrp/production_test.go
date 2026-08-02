package mrp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaterialQuantityIncludesBOMAndComponentScrap(t *testing.T) {
	got := materialQuantity(10, 2, 5, 10)
	require.InDelta(t, 23.1, got, 0.000001)
}

func TestValidateCompletionInput(t *testing.T) {
	valid := CompletionInput{CompanyID: 1, ActorID: 2, WorkOrderID: 3, Quantity: 1, IdempotencyKey: "receipt-1"}
	require.NoError(t, validateCompletionInput(valid))

	valid.IdempotencyKey = "  "
	require.ErrorIs(t, validateCompletionInput(valid), ErrIdempotencyKeyRequired)

	valid.IdempotencyKey = "receipt-1"
	valid.Quantity = 0
	require.ErrorIs(t, validateCompletionInput(valid), ErrInvalidState)
}
