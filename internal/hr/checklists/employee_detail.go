package checklists

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

// EmployeeDetailSource adapts Service to employees.ChecklistSource for the
// employee detail page's Assets tab and start dialogs.
type EmployeeDetailSource struct{ svc *Service }

var _ employees.ChecklistSource = EmployeeDetailSource{}

// NewEmployeeDetailSource wraps s for the employees handler.
func NewEmployeeDetailSource(s *Service) EmployeeDetailSource { return EmployeeDetailSource{svc: s} }

func toEmployeeAssets(in []Asset) []employees.ChecklistAsset {
	out := make([]employees.ChecklistAsset, 0, len(in))
	for _, a := range in {
		out = append(out, employees.ChecklistAsset{ID: a.ID, Number: a.Number, Name: a.Name, Status: a.Status})
	}
	return out
}

// AssetsHeldBy returns the employee's assets.
func (d EmployeeDetailSource) AssetsHeldBy(ctx context.Context, companyID, employeeID int64) ([]employees.ChecklistAsset, error) {
	assets, err := d.svc.AssetsHeldBy(ctx, companyID, employeeID)
	if err != nil {
		return nil, err
	}
	return toEmployeeAssets(assets), nil
}

// AssignableAssets returns the company's unassigned, non-disposed assets.
func (d EmployeeDetailSource) AssignableAssets(ctx context.Context, companyID int64) ([]employees.ChecklistAsset, error) {
	assets, err := d.svc.AssignableAssets(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return toEmployeeAssets(assets), nil
}

// ActiveTemplates returns the company's active templates (items omitted).
func (d EmployeeDetailSource) ActiveTemplates(ctx context.Context, companyID int64) ([]employees.ChecklistTemplateOption, error) {
	templates, err := d.svc.ListTemplates(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]employees.ChecklistTemplateOption, 0, len(templates))
	for _, t := range templates {
		if t.IsActive {
			out = append(out, employees.ChecklistTemplateOption{ID: t.ID, Code: t.Code, Name: t.Name, Kind: t.Kind})
		}
	}
	return out, nil
}

// EmployeeChecklists returns the employee's checklists, newest first.
func (d EmployeeDetailSource) EmployeeChecklists(ctx context.Context, companyID, employeeID int64) ([]employees.EmployeeChecklist, error) {
	lists, err := d.svc.List(ctx, companyID, Filter{EmployeeID: &employeeID})
	if err != nil {
		return nil, err
	}
	out := make([]employees.EmployeeChecklist, 0, len(lists))
	for _, c := range lists {
		out = append(out, employees.EmployeeChecklist{ID: c.ID, Kind: c.Kind, Status: c.Status, TemplateName: c.TemplateName,
			StartedOn: c.StartedOn, TerminationDate: c.TerminationDate, OpenTasks: c.OpenTasks})
	}
	return out, nil
}
