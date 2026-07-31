package mrp

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type repo struct {
	bom   BOM
	order WorkOrder
}

func (r *repo) CreateBOM(context.Context, BOM) (BOM, error) { return r.bom, nil }
func (r *repo) CreateWorkOrder(_ context.Context, o WorkOrder) (WorkOrder, error) {
	r.order = o
	r.order.ID = 8
	return r.order, nil
}

func (r *repo) GetBOM(context.Context, int64, int64) (BOM, error)             { return r.bom, nil }
func (r *repo) GetWorkOrder(context.Context, int64, int64) (WorkOrder, error) { return r.order, nil }
func (r *repo) UpdateWorkOrder(_ context.Context, o WorkOrder) error          { r.order = o; return nil }
func TestWorkOrderLifecycle(t *testing.T) {
	r := &repo{bom: BOM{ID: 4}, order: WorkOrder{ID: 8, BOMID: 4, PlannedQty: 2, Status: "DRAFT"}}
	s := NewService(r)
	o, e := s.Release(context.Background(), 1, 8)
	require.NoError(t, e)
	require.Equal(t, "RELEASED", o.Status)
	o, e = s.Start(context.Background(), 1, 8)
	require.NoError(t, e)
	o, e = s.Complete(context.Background(), 1, 8, 2)
	require.NoError(t, e)
	require.Equal(t, "COMPLETED", o.Status)
}
