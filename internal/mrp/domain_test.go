package mrp

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type repo struct {
	bom   BOM
	order WorkOrder
}

func (r *repo) CreateBOM(context.Context, BOM) (BOM, error) { return r.bom, nil }
func (r *repo) CreateBOMRevision(_ context.Context, _ int64, _ int64, _ int64, in BOMRevisionInput) (BOM, error) {
	r.bom.Version, r.bom.RevisionStatus, r.bom.EffectiveFrom, r.bom.ChangeReason = in.Version, BOMRevisionDraft, in.EffectiveFrom, in.ChangeReason
	return r.bom, nil
}
func (r *repo) ApproveBOM(_ context.Context, _ int64, _ int64, actorID int64, reason string) (BOM, error) {
	r.bom.RevisionStatus, r.bom.ChangeReason = BOMRevisionApproved, reason
	r.bom.ApprovedBy = &actorID
	return r.bom, nil
}
func (r *repo) ListBOMRevisions(context.Context, int64, int64) ([]BOM, error) {
	return []BOM{r.bom}, nil
}
func (r *repo) CreateWorkOrder(_ context.Context, o WorkOrder) (WorkOrder, error) {
	r.order = o
	r.order.ID = 8
	return r.order, nil
}

func (r *repo) GetBOM(context.Context, int64, int64) (BOM, error)             { return r.bom, nil }
func (r *repo) GetWorkOrder(context.Context, int64, int64) (WorkOrder, error) { return r.order, nil }
func (r *repo) UpdateWorkOrder(_ context.Context, o WorkOrder) error          { r.order = o; return nil }
func (r *repo) GenerateWorkOrderOperations(context.Context, WorkOrder) error  { return nil }
func (r *repo) CreateWIPLocation(_ context.Context, location WIPLocation) (WIPLocation, error) {
	return location, nil
}
func (r *repo) ListWIPLocations(context.Context, int64) ([]WIPLocation, error) { return nil, nil }
func (r *repo) DeactivateWIPLocation(context.Context, int64, int64) error      { return nil }
func (r *repo) ResolveWIPLocation(context.Context, int64, int64, int64) (WIPLocation, error) {
	return WIPLocation{}, nil
}
func (r *repo) CreateWorkCenter(_ context.Context, c WorkCenter) (WorkCenter, error) {
	c.ID = 11
	return c, nil
}
func (r *repo) CreateRouting(_ context.Context, routing Routing) (Routing, error) {
	routing.ID = 12
	return routing, nil
}
func TestWorkOrderLifecycle(t *testing.T) {
	r := &repo{bom: approvedBOM(), order: WorkOrder{ID: 8, ProductID: 9, BOMID: 4, PlannedQty: 2, Status: "DRAFT"}}
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

func TestCreateRoutingRequiresOrderedViableOperations(t *testing.T) {
	routing := Routing{CompanyID: 1, ProductID: 2, CreatedBy: 3, Code: "ASSEMBLY", Version: "v1", Operations: []RoutingOperation{{WorkCenterID: 4, Sequence: 10, Code: "CUT", Name: "Cut material", SetupMinutes: 15, RunMinutes: 3, YieldPct: 98}}}
	out, err := NewService(&repo{}).CreateRouting(context.Background(), routing)
	require.NoError(t, err)
	require.True(t, out.Active)

	routing.Operations = append(routing.Operations, RoutingOperation{WorkCenterID: 4, Sequence: 10, Code: "PACK", Name: "Pack", YieldPct: 100})
	_, err = NewService(&repo{}).CreateRouting(context.Background(), routing)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestCreateBOMRejectsSelfReferentialAndInvalidScrap(t *testing.T) {
	valid := BOM{CompanyID: 1, ProductID: 2, CreatedBy: 3, Version: "v1", Lines: []BOMLine{{ProductID: 4, Quantity: 1}}}
	_, err := NewService(&repo{}).CreateBOM(context.Background(), valid)
	require.NoError(t, err)

	valid.Lines[0].ProductID = valid.ProductID
	_, err = NewService(&repo{}).CreateBOM(context.Background(), valid)
	require.ErrorIs(t, err, ErrInvalidState)

	valid.Lines[0].ProductID = 4
	valid.ScrapPct = 100.01
	_, err = NewService(&repo{}).CreateBOM(context.Background(), valid)
	require.ErrorIs(t, err, ErrInvalidState)
}

func TestCreateWorkOrderRequiresMatchingActiveBOMAndWarehouse(t *testing.T) {
	order := WorkOrder{CompanyID: 1, ProductID: 9, BOMID: 4, WarehouseID: 2, CreatedBy: 7, PlannedQty: 2}

	t.Run("accepts matching active BOM", func(t *testing.T) {
		r := &repo{bom: approvedBOM()}
		out, err := NewService(r).CreateWorkOrder(context.Background(), order)
		require.NoError(t, err)
		require.Equal(t, "DRAFT", out.Status)
	})

	for _, tc := range []struct {
		name string
		bom  BOM
		in   WorkOrder
	}{
		{name: "draft BOM", bom: BOM{ID: 4, ProductID: 9, Active: true, RevisionStatus: BOMRevisionDraft, EffectiveFrom: time.Now()}, in: order},
		{name: "BOM product mismatch", bom: BOM{ID: 4, ProductID: 10, Active: true, RevisionStatus: BOMRevisionApproved, EffectiveFrom: time.Now()}, in: order},
		{name: "missing warehouse", bom: approvedBOM(), in: WorkOrder{CompanyID: 1, ProductID: 9, BOMID: 4, CreatedBy: 7, PlannedQty: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &repo{bom: tc.bom}
			_, err := NewService(r).CreateWorkOrder(context.Background(), tc.in)
			require.ErrorIs(t, err, ErrInvalidState)
		})
	}
}

func TestBOMRevisionRequiresReasonAndStartsDraft(t *testing.T) {
	s := NewService(&repo{bom: approvedBOM()})
	_, err := s.CreateBOMRevision(context.Background(), 1, 4, 3, BOMRevisionInput{Version: "v2", EffectiveFrom: time.Now()})
	require.ErrorIs(t, err, ErrInvalidState)
	out, err := s.CreateBOMRevision(context.Background(), 1, 4, 3, BOMRevisionInput{Version: "v2", EffectiveFrom: time.Now(), ChangeReason: "Correct component quantity"})
	require.NoError(t, err)
	require.Equal(t, BOMRevisionDraft, out.RevisionStatus)
	_, err = s.ApproveBOM(context.Background(), 1, 4, 3, "")
	require.ErrorIs(t, err, ErrInvalidState)
}

func approvedBOM() BOM {
	return BOM{ID: 4, ProductID: 9, Active: true, RevisionStatus: BOMRevisionApproved, EffectiveFrom: time.Now()}
}
