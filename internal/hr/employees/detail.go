package employees

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

var (
	errCompensationForbidden = errors.New("hr: compensation requires the hr.compensation.admin permission")
	errInvalidDependents     = errors.New("hr: dependents count must be a whole number")
	errInvalidDate           = errors.New("hr: invalid date")
	errEffectiveDateRequired = errors.New("hr: effective date required")
)

var (
	employmentTypes = []string{EmploymentPermanent, EmploymentContract, EmploymentProbation, EmploymentIntern}
	ptkpCodes       = []string{"TK/0", "TK/1", "TK/2", "TK/3", "K/0", "K/1", "K/2", "K/3"}
	detailTabs      = map[string]bool{"profile": true, "contacts": true, "job": true, "compensation": true, "assets": true}
)

// ChecklistAsset is a fixed asset shown on the detail page's Assets tab.
type ChecklistAsset struct {
	ID     int64
	Number string
	Name   string
	Status string
}

// ChecklistTemplateOption is an active checklist template offered by the
// start onboarding/offboarding dialogs.
type ChecklistTemplateOption struct {
	ID   int64
	Code string
	Name string
	Kind string // ONBOARDING or OFFBOARDING
}

// EmployeeChecklist summarises one of the employee's checklists.
type EmployeeChecklist struct {
	ID              int64
	Kind            string
	Status          string
	TemplateName    string
	StartedOn       time.Time
	TerminationDate *time.Time
	OpenTasks       int
}

// ChecklistSource is the checklist/asset read surface used by the detail
// page. internal/hr/checklists provides the implementation (it imports this
// package, so the types live here).
type ChecklistSource interface {
	AssetsHeldBy(ctx context.Context, companyID, employeeID int64) ([]ChecklistAsset, error)
	AssignableAssets(ctx context.Context, companyID int64) ([]ChecklistAsset, error)
	ActiveTemplates(ctx context.Context, companyID int64) ([]ChecklistTemplateOption, error)
	EmployeeChecklists(ctx context.Context, companyID, employeeID int64) ([]EmployeeChecklist, error)
}

// accessFlags are the caller's detail-page capabilities, computed once per
// request from rbac EffectivePermissions (same pattern as the leave
// handler's hasLeaveAdmin).
type accessFlags struct {
	employeeAdmin     bool
	sensitive         bool
	compensationView  bool
	compensationAdmin bool
	checklistAdmin    bool
}

func (h *Handler) permissions(r *http.Request, userID int64) accessFlags {
	var f accessFlags
	if h.rbac.Service == nil {
		return f
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		h.logger.Error("hr employee permissions", slog.Any("error", err))
		return f
	}
	for _, p := range perms {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "admin", "*":
			return accessFlags{true, true, true, true, true}
		case shared.PermHREmployeeAdmin:
			f.employeeAdmin = true
		case shared.PermHREmployeeSensitive:
			f.sensitive = true
		case shared.PermHRCompensationView:
			f.compensationView = true
		case shared.PermHRCompensationAdmin:
			f.compensationAdmin = true
		case shared.PermHRChecklistAdmin:
			f.checklistAdmin = true
		}
	}
	return f
}

func (f accessFlags) canSeeCompensation() bool { return f.compensationView || f.compensationAdmin }

func optDate(v string) *time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return nil
	}
	return &t
}

// parseOptDate is optDate that reports a malformed (non-empty) value.
func parseOptDate(v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, errInvalidDate
	}
	return &t, nil
}

func dateInput(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func formBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.FormValue(key))) {
	case "on", "true", "1", "yes":
		return true
	}
	return false
}

func compensationForm(r *http.Request, actorID int64) compensation.ChangeInput {
	return compensation.ChangeInput{
		EffectiveFrom:     derefTime(optDate(r.FormValue("effective_from"))),
		BaseSalary:        strings.TrimSpace(r.FormValue("base_salary")),
		PTKPCode:          strings.TrimSpace(r.FormValue("ptkp_code")),
		BPJSHealth:        formBool(r, "bpjs_health"),
		BPJSEmployment:    formBool(r, "bpjs_employment"),
		BankCode:          strings.TrimSpace(r.FormValue("bank_code")),
		BankAccountNumber: strings.TrimSpace(r.FormValue("bank_account_number")),
		BankAccountName:   strings.TrimSpace(r.FormValue("bank_account_name")),
		Reason:            strings.TrimSpace(r.FormValue("reason")),
		ActorID:           actorID,
	}
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func pathID(r *http.Request, key string) int64 {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func detailURL(id int64, tab string) string {
	return "/hr/employees/" + strconv.FormatInt(id, 10) + "?tab=" + tab
}

func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, ErrNotFound)
}

// detail renders the employee profile page. Sensitive profile fields are
// masked server-side without hr.employee.sensitive, and compensation data is
// neither loaded nor rendered without hr.compensation.view/admin.
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	s := shared.SessionFromContext(r.Context())
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	emp, err := h.service.GetByID(r.Context(), ident.CompanyID, id)
	if isNotFound(err) {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	if err != nil {
		h.logger.Error("hr employee detail", slog.Int64("employee_id", id), slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
		return
	}

	data := h.detailData(r, ident.CompanyID, h.permissions(r, ident.UserID), emp)

	var token string
	var flash *shared.FlashMessage
	if s != nil {
		token, _ = h.csrf.EnsureToken(r.Context(), s)
		flash = s.PopFlash()
	}
	if err := h.templates.Render(w, "pages/hr/employee_detail.html", view.TemplateData{
		Title:       emp.Name + " - Employee",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       flash,
		Data:        data,
	}); err != nil {
		h.logger.Error("render employee detail", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// detailData builds the detail-page view model. It is the server-side PII
// and compensation boundary: sensitive fields are masked unless perms
// include hr.employee.sensitive, and compensation is neither loaded nor
// placed in the model without hr.compensation.view/admin.
func (h *Handler) detailData(r *http.Request, companyID int64, perms accessFlags, emp Employee) map[string]any {
	id := emp.ID
	canSeeSensitive := perms.sensitive
	canSeeCompensation := perms.canSeeCompensation() && h.compensation != nil
	if !canSeeSensitive {
		MaskSensitive(&emp)
	}

	contacts, err := h.service.ListContacts(r.Context(), companyID, id)
	if err != nil {
		h.logger.Error("hr employee contacts", slog.Int64("employee_id", id), slog.Any("error", err))
	}
	history, err := h.service.ListJobHistory(r.Context(), companyID, id)
	if err != nil {
		h.logger.Error("hr employee job history", slog.Int64("employee_id", id), slog.Any("error", err))
	}
	var assignments []compensation.Assignment
	if canSeeCompensation {
		assignments, err = h.compensation.ListForEmployee(r.Context(), companyID, id)
		if err != nil {
			h.logger.Error("hr employee compensation", slog.Int64("employee_id", id), slog.Any("error", err))
		}
	}

	data := map[string]any{
		"Employee":             emp,
		"Contacts":             contacts,
		"JobHistory":           history,
		"CanAdmin":             perms.employeeAdmin,
		"CanSeeSensitive":      canSeeSensitive,
		"CanSeeCompensation":   canSeeCompensation,
		"CanAdminCompensation": perms.compensationAdmin && h.compensation != nil,
		"EmploymentTypes":      employmentTypes,
		"PTKPCodes":            ptkpCodes,
		"Genders":              []string{"MALE", "FEMALE", "OTHER"},
		"MaritalStatuses":      []string{"SINGLE", "MARRIED", "DIVORCED", "WIDOWED"},
		"Statuses":             []string{StatusActive, StatusInactive},
		"Today":                businessToday().Format("2006-01-02"),
		"Dates": map[string]string{
			"DateOfBirth":       dateInput(emp.DateOfBirth),
			"ContractStartDate": dateInput(emp.ContractStartDate),
			"ContractEndDate":   dateInput(emp.ContractEndDate),
			"ProbationEndDate":  dateInput(emp.ProbationEndDate),
			"HireDate":          dateInput(&emp.HireDate),
		},
	}
	if canSeeCompensation {
		data["Compensation"] = assignments
	}
	if perms.employeeAdmin {
		depts, _ := h.service.ListDepartments(r.Context(), companyID)
		positions, _ := h.service.ListPositions(r.Context(), companyID)
		staff, _ := h.service.List(r.Context(), companyID)
		users, _ := h.service.ListAvailableUsers(r.Context())
		managers := make([]Employee, 0, len(staff))
		for _, e := range staff {
			if e.ID == emp.ID {
				continue
			}
			// Only id/name/number are rendered; never carry other profile data.
			managers = append(managers, Employee{ID: e.ID, Name: e.Name, EmployeeNumber: e.EmployeeNumber})
		}
		data["Departments"] = depts
		data["Positions"] = positions
		data["Managers"] = managers
		data["Users"] = users
	}

	canAdminChecklists := perms.checklistAdmin && h.checklists != nil
	data["CanAdminChecklists"] = canAdminChecklists
	if canAdminChecklists {
		h.addChecklistData(r, companyID, id, data)
	}

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if !detailTabs[tab] || (tab == "compensation" && !canSeeCompensation) || (tab == "assets" && !canAdminChecklists) {
		tab = "profile"
	}
	data["Tab"] = tab

	return data
}

// addChecklistData loads the Assets tab and start-dialog data. Callers
// must have checked hr.checklist.admin.
func (h *Handler) addChecklistData(r *http.Request, companyID, employeeID int64, data map[string]any) {
	ctx := r.Context()
	held, err := h.checklists.AssetsHeldBy(ctx, companyID, employeeID)
	if err != nil {
		h.logger.Error("hr employee assets", slog.Int64("employee_id", employeeID), slog.Any("error", err))
	}
	available, err := h.checklists.AssignableAssets(ctx, companyID)
	if err != nil {
		h.logger.Error("hr assignable assets", slog.Any("error", err))
	}
	templates, err := h.checklists.ActiveTemplates(ctx, companyID)
	if err != nil {
		h.logger.Error("hr checklist templates", slog.Any("error", err))
	}
	lists, err := h.checklists.EmployeeChecklists(ctx, companyID, employeeID)
	if err != nil {
		h.logger.Error("hr employee checklists", slog.Int64("employee_id", employeeID), slog.Any("error", err))
	}
	var onboarding, offboarding []ChecklistTemplateOption
	for _, t := range templates {
		switch t.Kind {
		case "ONBOARDING":
			onboarding = append(onboarding, t)
		case "OFFBOARDING":
			offboarding = append(offboarding, t)
		}
	}
	openKinds := map[string]bool{}
	for _, c := range lists {
		if c.Status == "OPEN" {
			openKinds[c.Kind] = true
		}
	}
	data["Assets"] = held
	data["AssignableAssets"] = available
	data["OnboardingTemplates"] = onboarding
	data["OffboardingTemplates"] = offboarding
	data["Checklists"] = lists
	data["HasOpenOnboarding"] = openKinds["ONBOARDING"]
	data["HasOpenOffboarding"] = openKinds["OFFBOARDING"]
}

// finish adds a success or error flash and redirects back to the detail tab.
func (h *Handler) finish(w http.ResponseWriter, r *http.Request, id int64, tab, success string, err error) {
	if s := shared.SessionFromContext(r.Context()); s != nil {
		if err != nil {
			s.AddFlash(shared.FlashMessage{Kind: "error", Message: hrFlashError(err)})
		} else {
			s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
		}
	}
	http.Redirect(w, r, detailURL(id, tab), http.StatusSeeOther)
}

// updateProfile saves identity and profile fields. Sensitive fields are
// written only when the caller holds hr.employee.sensitive and the form
// carries them; otherwise the stored values are kept.
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	in, err := profileForm(r)
	if err == nil {
		in.ActorID = ident.UserID
		in.IncludeSensitive = h.permissions(r, ident.UserID).sensitive && r.PostForm.Has("national_id")
		_, err = h.service.Update(r.Context(), ident.CompanyID, id, in)
	}
	h.finish(w, r, id, "profile", "Profile updated", err)
}

func profileForm(r *http.Request) (UpdateInput, error) {
	if err := r.ParseForm(); err != nil {
		return UpdateInput{}, err
	}
	hire, err := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("hire_date")))
	if err != nil {
		return UpdateInput{}, ErrInvalidEmployee
	}
	dependents := 0
	if v := strings.TrimSpace(r.FormValue("dependents_count")); v != "" {
		dependents, err = strconv.Atoi(v)
		if err != nil {
			return UpdateInput{}, errInvalidDependents
		}
	}
	dates := map[string]**time.Time{}
	p := Profile{
		Gender:          r.FormValue("gender"),
		NationalID:      r.FormValue("national_id"),
		TaxID:           r.FormValue("tax_id"),
		MaritalStatus:   r.FormValue("marital_status"),
		DependentsCount: dependents,
		Phone:           r.FormValue("phone"),
		AddressLine1:    r.FormValue("address_line1"),
		AddressLine2:    r.FormValue("address_line2"),
		City:            r.FormValue("city"),
		Province:        r.FormValue("province"),
		PostalCode:      r.FormValue("postal_code"),
		Country:         r.FormValue("country"),
	}
	dates["date_of_birth"] = &p.DateOfBirth
	dates["contract_start_date"] = &p.ContractStartDate
	dates["contract_end_date"] = &p.ContractEndDate
	dates["probation_end_date"] = &p.ProbationEndDate
	for key, dst := range dates {
		d, err := parseOptDate(r.FormValue(key))
		if err != nil {
			return UpdateInput{}, err
		}
		*dst = d
	}
	return UpdateInput{
		UserID:         opt(r.FormValue("user_id")),
		EmployeeNumber: r.FormValue("employee_number"),
		Name:           r.FormValue("name"),
		Email:          r.FormValue("email"),
		HireDate:       hire,
		Profile:        p,
	}, nil
}

func (h *Handler) upsertContact(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	contactID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("contact_id")), 10, 64)
	_, err := h.service.UpsertContact(r.Context(), ident.CompanyID, id, ContactInput{
		ID:           contactID,
		Name:         r.FormValue("name"),
		Relationship: r.FormValue("relationship"),
		Phone:        r.FormValue("phone"),
		Email:        r.FormValue("email"),
		IsPrimary:    formBool(r, "is_primary"),
		ActorID:      ident.UserID,
	})
	h.finish(w, r, id, "contacts", "Emergency contact saved", err)
}

func (h *Handler) deleteContact(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, cid := pathID(r, "id"), pathID(r, "cid")
	if id == 0 || cid == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "contact not found")
		return
	}
	err := h.service.DeleteContact(r.Context(), ident.CompanyID, id, cid, ident.UserID)
	h.finish(w, r, id, "contacts", "Emergency contact deleted", err)
}

func (h *Handler) recordJobChange(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	var err error
	validFrom := optDate(r.FormValue("valid_from"))
	if validFrom == nil {
		err = errEffectiveDateRequired
	} else {
		_, err = h.service.RecordJobChange(r.Context(), ident.CompanyID, id, JobChangeInput{
			DepartmentID:   opt(r.FormValue("department_id")),
			PositionID:     opt(r.FormValue("position_id")),
			ManagerID:      opt(r.FormValue("manager_id")),
			Status:         r.FormValue("status"),
			EmploymentType: r.FormValue("employment_type"),
			Grade:          r.FormValue("grade"),
			ValidFrom:      *validFrom,
			Reason:         r.FormValue("reason"),
			ActorID:        ident.UserID,
		})
	}
	h.finish(w, r, id, "job", "Job change recorded", err)
}

// changeCompensation applies a salary change (change_type=salary, default)
// or a bank change (change_type=bank). The route is gated by
// hr.compensation.admin in MountRoutes.
func (h *Handler) changeCompensation(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id := pathID(r, "id")
	if id == 0 {
		shared.WriteHTTPError(w, http.StatusNotFound, "employee not found")
		return
	}
	if h.compensation == nil {
		h.finish(w, r, id, "compensation", "", ErrCompensationDisabled)
		return
	}
	in := compensationForm(r, ident.UserID)
	var err error
	success := "Compensation change recorded"
	if strings.EqualFold(strings.TrimSpace(r.FormValue("change_type")), "bank") {
		success = "Bank details updated"
		_, err = h.compensation.ChangeBank(r.Context(), ident.CompanyID, id, compensation.BankDetails{
			BankCode:          in.BankCode,
			BankAccountNumber: in.BankAccountNumber,
			BankAccountName:   in.BankAccountName,
		}, in.EffectiveFrom, in.Reason, ident.UserID)
	} else {
		_, err = h.compensation.Change(r.Context(), ident.CompanyID, id, in)
	}
	h.finish(w, r, id, "compensation", success, err)
}
