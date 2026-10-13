package checklists

import (
	"context"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

func TestAssignableAssetsScopedAndFiltered(t *testing.T) {
	db, svc, _, _, _ := newService(t)
	db.ExpectQuery(q(`FROM fixed_assets fa WHERE fa.company_id=$1 AND fa.custodian_employee_id IS NULL AND fa.status<>'DISPOSED'`)).WithArgs(companyA).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "number", "name", "status", "custodian_employee_id"}).
			AddRow(assetID, companyA, "FA-1", "Laptop", "ACTIVE", nil))
	got, err := NewEmployeeDetailSource(svc).AssignableAssets(context.Background(), companyA)
	require.NoError(t, err)
	require.Equal(t, []employees.ChecklistAsset{{ID: assetID, Number: "FA-1", Name: "Laptop", Status: "ACTIVE"}}, got)
	require.NoError(t, db.ExpectationsWereMet())

	_, err = svc.AssignableAssets(context.Background(), 0)
	require.ErrorIs(t, err, ErrCompanyRequired)
}

func TestEmployeeDetailSourceActiveTemplatesOnly(t *testing.T) {
	db, svc, _, _, _ := newService(t)
	db.ExpectQuery(q(`FROM hr_checklist_templates t WHERE t.company_id=$1 ORDER BY t.kind, t.code`)).WithArgs(companyA).
		WillReturnRows(pgxmock.NewRows(templateCls).
			AddRow(templateID, companyA, "OFF-STD", "Standard offboarding", KindOffboarding, true, createdAt).
			AddRow(templateID+1, companyA, "OFF-OLD", "Old offboarding", KindOffboarding, false, createdAt))
	db.ExpectQuery(q(`ORDER BY i.template_id, i.seq`)).WithArgs(companyA).WillReturnRows(pgxmock.NewRows(itemCls))
	got, err := NewEmployeeDetailSource(svc).ActiveTemplates(context.Background(), companyA)
	require.NoError(t, err)
	require.Equal(t, []employees.ChecklistTemplateOption{{ID: templateID, Code: "OFF-STD", Name: "Standard offboarding", Kind: KindOffboarding}}, got)
	require.NoError(t, db.ExpectationsWereMet())
}
