package procurement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func procurementTenantContext(companyID int64) context.Context {
	session := &shared.Session{}
	session.SetUser("42")
	if companyID > 0 {
		session.Set("company_id", strconv.FormatInt(companyID, 10))
	}
	return shared.ContextWithSession(context.Background(), session)
}

func TestEnforceRequestCompanyPreservesInternalContract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ctx       context.Context
		companyID int64
		wantErr   error
	}{
		{name: "context-free worker", ctx: context.Background(), companyID: 99},
		{name: "matching tenant", ctx: procurementTenantContext(7), companyID: 7},
		{name: "missing active tenant", ctx: procurementTenantContext(0), companyID: 7, wantErr: ErrCompanyScopeRequired},
		{name: "foreign tenant", ctx: procurementTenantContext(7), companyID: 8, wantErr: ErrCompanyScopeMismatch},
		{name: "legacy document without tenant", ctx: procurementTenantContext(7), companyID: 0, wantErr: ErrCompanyScopeRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceRequestCompany(tc.ctx, tc.companyID)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestProcurementDocumentReadsAndTransitionsRejectForeignTenant(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.prs[1] = PurchaseRequest{ID: 1, CompanyID: 8, Number: "PR-FOREIGN", Status: PRStatusDraft}
	repo.prs[2] = PurchaseRequest{ID: 2, CompanyID: 8, Number: "PR-FOREIGN-SUBMITTED", Status: PRStatusSubmitted}
	repo.pos[3] = PurchaseOrder{ID: 3, CompanyID: 8, Number: "PO-FOREIGN-DRAFT", Status: POStatusDraft}
	repo.pos[4] = PurchaseOrder{ID: 4, CompanyID: 8, Number: "PO-FOREIGN-APPROVAL", Status: POStatusApproval}
	repo.grns[5] = GoodsReceipt{ID: 5, CompanyID: 8, Number: "GRN-FOREIGN", Status: GRNStatusDraft}
	repo.grnLines[5] = []GRNLine{{ID: 6, GRNID: 5, ProductID: 11, Qty: 1}}

	svc := NewService(nil, repo, nil, nil, nil, nil, nil)
	ctx := procurementTenantContext(7)

	checks := []struct {
		name string
		call func() error
	}{
		{name: "submit PR", call: func() error { return svc.SubmitPurchaseRequest(ctx, 1, 42) }},
		{name: "create PO from PR", call: func() error {
			_, err := svc.CreatePOFromPR(ctx, CreatePOInput{PRID: 2, ExpectedWarehouseID: 1})
			return err
		}},
		{name: "submit PO", call: func() error { return svc.SubmitPurchaseOrder(ctx, 3, 42) }},
		{name: "approve PO", call: func() error { return svc.ApprovePurchaseOrder(ctx, 4, 42) }},
		{name: "post GRN", call: func() error { return svc.PostGoodsReceipt(ctx, 5) }},
		{name: "read PO detail", call: func() error { _, _, err := svc.GetPOWithLines(ctx, 3); return err }},
		{name: "read GRN detail", call: func() error { _, _, err := svc.GetGRNWithLines(ctx, 5); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			require.ErrorIs(t, check.call(), ErrCompanyScopeMismatch)
		})
	}

	require.Equal(t, PRStatusDraft, repo.prs[1].Status)
	require.Equal(t, POStatusDraft, repo.pos[3].Status)
	require.Equal(t, POStatusApproval, repo.pos[4].Status)
	require.Equal(t, GRNStatusDraft, repo.grns[5].Status)
	require.Empty(t, repo.grns[0], "foreign GRN post must not create records")
}

func TestProcurementHandlersRequireAuthenticatedCompanyForDetailAndTransitions(t *testing.T) {
	h := &Handler{}
	handlers := []func(*Handler, http.ResponseWriter, *http.Request){
		(*Handler).showPRForm,
		(*Handler).showPOForm,
		(*Handler).showGRNForm,
		(*Handler).handleListPOs,
		(*Handler).handleListGRNs,
		(*Handler).createPR,
		(*Handler).submitPR,
		(*Handler).createPO,
		(*Handler).submitPO,
		(*Handler).approvePO,
		(*Handler).emailPO,
		(*Handler).createGRN,
		(*Handler).postGRN,
	}
	for _, handle := range handlers {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		handle(h, recorder, req)
		require.Equal(t, http.StatusForbidden, recorder.Code)
	}
}

func TestProcurementServiceAcceptsMatchingTenantIdentity(t *testing.T) {
	repo := newMemoryProcRepo()
	svc := NewService(nil, repo, nil, nil, nil, nil, nil)

	pr, err := svc.CreatePurchaseRequest(procurementTenantContext(7), CreatePRInput{
		CompanyID:  7,
		SupplierID: 1,
		RequestBy:  42,
		Lines:      []PRLineInput{{ProductID: 11, Qty: 1}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), pr.CompanyID)
}

func TestGoodsReturnBindsToPostedGRNTenantAndWarehouse(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.grns[1] = GoodsReceipt{
		ID: 1, CompanyID: 7, SupplierID: 11, WarehouseID: 12, Status: GRNStatusPosted,
	}
	repo.grnLines[1] = []GRNLine{{ID: 9, GRNID: 1, ProductID: 100, Qty: 2, UnitCost: 12}}
	svc := NewService(nil, repo, nil, nil, nil, nil, nil)

	_, err := svc.CreateGoodsReturnGRN(context.Background(), CreateGoodsReturnGRNInput{
		GRNID: 1, CompanyID: 8, SupplierID: 11, WarehouseID: 12,
		Lines: []GoodsReturnGRNLineInput{{GRNLineID: 9, ProductID: 100, QuantityReturned: 1, UnitCost: 12}},
	})
	require.ErrorIs(t, err, ErrCompanyScopeMismatch)
	require.Empty(t, repo.returns)

	_, err = svc.CreateGoodsReturnGRN(context.Background(), CreateGoodsReturnGRNInput{
		GRNID: 1, CompanyID: 7, SupplierID: 11, WarehouseID: 99,
		Lines: []GoodsReturnGRNLineInput{{GRNLineID: 9, ProductID: 100, QuantityReturned: 1, UnitCost: 12}},
	})
	require.ErrorContains(t, err, "return warehouse does not match GRN warehouse")
	require.Empty(t, repo.returns)

	ret, err := svc.CreateGoodsReturnGRN(context.Background(), CreateGoodsReturnGRNInput{
		GRNID: 1, SupplierID: 0, WarehouseID: 0,
		Lines: []GoodsReturnGRNLineInput{{GRNLineID: 9, ProductID: 100, QuantityReturned: 1, UnitCost: 12}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), ret.CompanyID)
	require.Equal(t, int64(11), ret.SupplierID)
	require.Equal(t, int64(12), ret.WarehouseID)
}

func TestGoodsReturnReadsAreTenantScoped(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.returns[1] = GoodsReturnGRN{ID: 1, CompanyID: 8, Number: "RET-FOREIGN", Status: GoodsReturnStatusDraft}
	repo.returns[2] = GoodsReturnGRN{ID: 2, CompanyID: 7, Number: "RET-OWN", Status: GoodsReturnStatusDraft}
	svc := NewService(nil, repo, nil, nil, nil, nil, nil)
	ctx := procurementTenantContext(7)

	_, err := svc.GetGoodsReturnGRN(ctx, 1)
	require.ErrorIs(t, err, ErrCompanyScopeMismatch)
	items, err := svc.ListGoodsReturnGRNs(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(2), items[0].ID)
}

func TestGoodsReturnRejectsLegacyUntenantedGRNForBrowserRequests(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.grns[1] = GoodsReceipt{ID: 1, SupplierID: 11, WarehouseID: 12, Status: GRNStatusPosted}
	repo.grnLines[1] = []GRNLine{{ID: 9, GRNID: 1, ProductID: 100, Qty: 2, UnitCost: 12}}
	svc := NewService(nil, repo, nil, nil, nil, nil, nil)
	_, err := svc.CreateGoodsReturnGRN(procurementTenantContext(7), CreateGoodsReturnGRNInput{
		GRNID: 1, CompanyID: 7, SupplierID: 11, WarehouseID: 12,
		Lines: []GoodsReturnGRNLineInput{{GRNLineID: 9, ProductID: 100, QuantityReturned: 1, UnitCost: 12}},
	})
	require.ErrorIs(t, err, ErrCompanyScopeRequired)
	require.Empty(t, repo.returns)
}
