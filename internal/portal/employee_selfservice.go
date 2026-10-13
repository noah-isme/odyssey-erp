package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	hremployees "github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	hrexpenses "github.com/odyssey-erp/odyssey-erp/internal/hr/expenses"
	hrleave "github.com/odyssey-erp/odyssey-erp/internal/hr/leave"
	hrovertime "github.com/odyssey-erp/odyssey-erp/internal/hr/overtime"
	hrprofilechanges "github.com/odyssey-erp/odyssey-erp/internal/hr/profilechanges"
	hrschedule "github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// LeaveService is the part of *hrleave.Service the portal uses.
type LeaveService interface {
	Types(ctx context.Context, companyID int64) ([]hrleave.LeaveType, error)
	ListOwn(ctx context.Context, userID int64) ([]hrleave.Request, error)
	GetOwnBalances(ctx context.Context, companyID, userID int64, year int) ([]hrleave.Balance, error)
	Submit(ctx context.Context, in hrleave.CreateInput) (hrleave.Request, error)
	Cancel(ctx context.Context, companyID, requestID, userID int64) error
}

// OvertimeService is the part of *hrovertime.Service the portal uses.
type OvertimeService interface {
	ListOwn(ctx context.Context, companyID, userID int64) ([]hrovertime.Request, error)
	Submit(ctx context.Context, in hrovertime.SubmitInput) (hrovertime.Request, error)
	Cancel(ctx context.Context, companyID, requestID, userID int64) error
}

// ExpenseService is the part of *hrexpenses.Service the portal uses.
type ExpenseService interface {
	ListOwn(ctx context.Context, companyID, userID int64) ([]hrexpenses.Claim, error)
	Submit(ctx context.Context, in hrexpenses.SubmitInput) (hrexpenses.Claim, error)
	Cancel(ctx context.Context, companyID, claimID, userID int64) error
}

// ProfileChangeService is the part of *hrprofilechanges.Service the portal uses.
type ProfileChangeService interface {
	ListOwn(ctx context.Context, companyID, userID int64) ([]hrprofilechanges.Request, error)
	Submit(ctx context.Context, in hrprofilechanges.SubmitInput) (hrprofilechanges.Request, error)
	Cancel(ctx context.Context, companyID, requestID, userID int64) error
}

// HolidayService is the part of *hrschedule.Service the portal uses.
type HolidayService interface {
	ListHolidays(ctx context.Context, companyID int64, year int) ([]hrschedule.Holiday, error)
}

// EmployeeProfileService is the part of *hremployees.Service the portal uses
// for the employee's own (masked) profile.
type EmployeeProfileService interface {
	GetByID(ctx context.Context, companyID, id int64) (hremployees.Employee, error)
	ListContacts(ctx context.Context, companyID, employeeID int64) ([]hremployees.EmergencyContact, error)
}

// SetHRServices wires the HR services behind the employee self-service
// endpoints. Endpoints whose service is nil answer 503.
func (h *Handler) SetHRServices(leave LeaveService, overtime OvertimeService, expenses ExpenseService, profileChanges ProfileChangeService, holidays HolidayService) {
	h.leave = leave
	h.overtime = overtime
	h.expenses = expenses
	h.profileChanges = profileChanges
	h.holidays = holidays
}

// SetEmployeeProfileService wires the employee directory used by
// GET /portal/employee/profile.
func (h *Handler) SetEmployeeProfileService(s EmployeeProfileService) {
	h.employeeProfile = s
}

const (
	dateLayout       = "2006-01-02"
	maxJSONBodyBytes = 1 << 20
)

var errSelfServiceUnavailable = errors.New("employee self-service is not configured")

// --- shared helpers ---------------------------------------------------------

func writeSelfServiceJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeSelfServiceMessage(w http.ResponseWriter, status int, msg string) {
	writeSelfServiceJSON(w, status, map[string]string{"error": msg})
}

// hrDomainMessage returns the user-facing text of an "hr: ..." domain error.
// Every error in the wrap chain must carry the prefix, so wrapped database or
// driver errors ("hr: loading holidays: <driver error>") are never exposed.
func hrDomainMessage(err error) (string, bool) {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if !strings.HasPrefix(e.Error(), "hr: ") {
			return "", false
		}
	}
	msg := strings.TrimSpace(strings.TrimPrefix(err.Error(), "hr: "))
	if msg == "" {
		return "", false
	}
	return strings.ToUpper(msg[:1]) + msg[1:], true
}

func writeSelfServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shared.ErrUnauthorized):
		writeSelfServiceMessage(w, http.StatusUnauthorized, "unauthorized")
		return
	case errors.Is(err, errSelfServiceUnavailable):
		writeSelfServiceMessage(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if msg, ok := hrDomainMessage(err); ok {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(msg), "not found") {
			status = http.StatusNotFound
		}
		writeSelfServiceMessage(w, status, msg)
		return
	}
	writeSelfServiceMessage(w, http.StatusInternalServerError, shared.UserSafeMessage(err))
}

// employeeAccess resolves the session user's active employee link. It writes
// a 401 and returns ok=false when there is none.
func (h *Handler) employeeAccess(w http.ResponseWriter, r *http.Request) (uid, cid, employeeID int64, ok bool) {
	uid, cid, employeeID, err := h.access(r, "EMPLOYEE")
	if err != nil {
		writeSelfServiceMessage(w, http.StatusUnauthorized, "unauthorized")
		return 0, 0, 0, false
	}
	return uid, cid, employeeID, true
}

func decodeSelfServiceBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeSelfServiceMessage(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeSelfServiceMessage(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func parseDate(w http.ResponseWriter, field, value string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(value))
	if err != nil {
		writeSelfServiceMessage(w, http.StatusBadRequest, field+" must be a date (YYYY-MM-DD)")
		return time.Time{}, false
	}
	return t, true
}

func queryYear(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("year"))
	if raw == "" {
		return time.Now().Year(), true
	}
	year, err := strconv.Atoi(raw)
	if err != nil || year < 1900 || year > 9999 {
		writeSelfServiceMessage(w, http.StatusBadRequest, "year must be between 1900 and 9999")
		return 0, false
	}
	return year, true
}

func formatDatePtr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func cancelledJSON(w http.ResponseWriter, id int64) {
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "id": id, "status": "CANCELLED"})
}

// --- leave -------------------------------------------------------------------

type leaveRequestJSON struct {
	ID                int64   `json:"id"`
	LeaveTypeID       int64   `json:"leave_type_id"`
	TypeName          string  `json:"type_name"`
	StartDate         string  `json:"start_date"`
	EndDate           string  `json:"end_date"`
	Days              float64 `json:"days"`
	StartHalfDay      bool    `json:"start_half_day"`
	EndHalfDay        bool    `json:"end_half_day"`
	Reason            string  `json:"reason"`
	Status            string  `json:"status"`
	ApprovalRequestID *int64  `json:"approval_request_id"`
}

type leaveBalanceJSON struct {
	LeaveTypeID int64   `json:"leave_type_id"`
	TypeCode    string  `json:"type_code"`
	TypeName    string  `json:"type_name"`
	Year        int     `json:"year"`
	Entitled    float64 `json:"entitled"`
	Used        float64 `json:"used"`
	Pending     float64 `json:"pending"`
	Available   float64 `json:"available"`
}

type leaveTypeJSON struct {
	ID          int64   `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	DefaultDays float64 `json:"default_days"`
}

func toLeaveRequestJSON(x hrleave.Request) leaveRequestJSON {
	return leaveRequestJSON{
		ID: x.ID, LeaveTypeID: x.LeaveTypeID, TypeName: x.TypeName,
		StartDate: x.StartDate.Format(dateLayout), EndDate: x.EndDate.Format(dateLayout),
		Days: x.Days, StartHalfDay: x.StartHalfDay, EndHalfDay: x.EndHalfDay,
		Reason: x.Reason, Status: x.Status, ApprovalRequestID: x.ApprovalRequestID,
	}
}

// employeeLeaveList serves GET /portal/employee/leave when the leave service
// is wired: own requests (this company's employee only), balances for ?year=
// and the selectable leave types.
func (h *Handler) employeeLeaveList(w http.ResponseWriter, r *http.Request) {
	uid, cid, employeeID, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	year, ok := queryYear(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	reqs, err := h.leave.ListOwn(ctx, uid)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	balances, err := h.leave.GetOwnBalances(ctx, cid, uid, year)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	types, err := h.leave.Types(ctx, cid)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []leaveRequestJSON{}
	for _, x := range reqs {
		// ListOwn is keyed by user only; keep this company's employee rows.
		if x.EmployeeID != employeeID {
			continue
		}
		items = append(items, toLeaveRequestJSON(x))
	}
	bal := []leaveBalanceJSON{}
	for _, b := range balances {
		bal = append(bal, leaveBalanceJSON{LeaveTypeID: b.LeaveTypeID, TypeCode: b.TypeCode, TypeName: b.TypeName, Year: b.Year, Entitled: b.Entitled, Used: b.Used, Pending: b.Pending, Available: b.Available})
	}
	lt := []leaveTypeJSON{}
	for _, t := range types {
		lt = append(lt, leaveTypeJSON{ID: t.ID, Code: t.Code, Name: t.Name, DefaultDays: t.DefaultDays})
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "year": year, "leave": items, "balances": bal, "leave_types": lt})
}

func (h *Handler) employeeLeaveSubmit(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.leave == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	var in struct {
		LeaveTypeID  int64  `json:"leave_type_id"`
		StartDate    string `json:"start_date"`
		EndDate      string `json:"end_date"`
		Reason       string `json:"reason"`
		StartHalfDay bool   `json:"start_half_day"`
		EndHalfDay   bool   `json:"end_half_day"`
	}
	if !decodeSelfServiceBody(w, r, &in) {
		return
	}
	start, ok := parseDate(w, "start_date", in.StartDate)
	if !ok {
		return
	}
	end, ok := parseDate(w, "end_date", in.EndDate)
	if !ok {
		return
	}
	req, err := h.leave.Submit(r.Context(), hrleave.CreateInput{
		CompanyID: cid, UserID: uid, LeaveTypeID: in.LeaveTypeID,
		StartDate: start, EndDate: end, Reason: strings.TrimSpace(in.Reason),
		StartHalfDay: in.StartHalfDay, EndHalfDay: in.EndHalfDay,
	})
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "leave_request": toLeaveRequestJSON(req)})
}

func (h *Handler) employeeLeaveCancel(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.leave == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.leave.Cancel(r.Context(), cid, id, uid); err != nil {
		writeSelfServiceError(w, err)
		return
	}
	cancelledJSON(w, id)
}

// --- overtime ----------------------------------------------------------------

type overtimeJSON struct {
	ID        int64      `json:"id"`
	WorkedOn  string     `json:"worked_on"`
	Minutes   int        `json:"minutes"`
	Hours     float64    `json:"hours"`
	Reason    string     `json:"reason"`
	Status    string     `json:"status"`
	DecidedAt *time.Time `json:"decided_at"`
	CreatedAt *time.Time `json:"created_at"`
}

func toOvertimeJSON(x hrovertime.Request) overtimeJSON {
	return overtimeJSON{ID: x.ID, WorkedOn: x.WorkedOn.Format(dateLayout), Minutes: x.Minutes, Hours: x.Hours(), Reason: x.Reason, Status: x.Status, DecidedAt: x.DecidedAt, CreatedAt: timePtr(x.CreatedAt)}
}

func (h *Handler) employeeOvertimeList(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.overtime == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	reqs, err := h.overtime.ListOwn(r.Context(), cid, uid)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []overtimeJSON{}
	for _, x := range reqs {
		items = append(items, toOvertimeJSON(x))
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "overtime": items, "max_minutes": hrovertime.MaxMinutes})
}

func (h *Handler) employeeOvertimeSubmit(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.overtime == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	var in struct {
		WorkedOn string `json:"worked_on"`
		Minutes  int    `json:"minutes"`
		Reason   string `json:"reason"`
	}
	if !decodeSelfServiceBody(w, r, &in) {
		return
	}
	day, ok := parseDate(w, "worked_on", in.WorkedOn)
	if !ok {
		return
	}
	req, err := h.overtime.Submit(r.Context(), hrovertime.SubmitInput{UserID: uid, CompanyID: cid, WorkedOn: day, Minutes: in.Minutes, Reason: strings.TrimSpace(in.Reason)})
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "overtime_request": toOvertimeJSON(req)})
}

func (h *Handler) employeeOvertimeCancel(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.overtime == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.overtime.Cancel(r.Context(), cid, id, uid); err != nil {
		writeSelfServiceError(w, err)
		return
	}
	cancelledJSON(w, id)
}

// --- expenses ----------------------------------------------------------------

type expenseJSON struct {
	ID          int64      `json:"id"`
	ClaimDate   string     `json:"claim_date"`
	Category    string     `json:"category"`
	Description string     `json:"description"`
	Amount      string     `json:"amount"`
	Currency    string     `json:"currency"`
	DocumentID  *int64     `json:"document_id"`
	Status      string     `json:"status"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   *time.Time `json:"created_at"`
}

func toExpenseJSON(x hrexpenses.Claim) expenseJSON {
	return expenseJSON{ID: x.ID, ClaimDate: x.ClaimDate.Format(dateLayout), Category: x.Category, Description: x.Description, Amount: x.Amount, Currency: x.Currency, DocumentID: x.DocumentID, Status: x.Status, DecidedAt: x.DecidedAt, CreatedAt: timePtr(x.CreatedAt)}
}

func (h *Handler) employeeExpenseList(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.expenses == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	claims, err := h.expenses.ListOwn(r.Context(), cid, uid)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []expenseJSON{}
	for _, x := range claims {
		items = append(items, toExpenseJSON(x))
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "expenses": items})
}

func (h *Handler) employeeExpenseSubmit(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.expenses == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	var in struct {
		ClaimDate   string `json:"claim_date"`
		Category    string `json:"category"`
		Description string `json:"description"`
		// Amount accepts a JSON string ("125000.50") or number; it is passed
		// to the service as text so it is never rounded through float.
		Amount     json.Number `json:"amount"`
		DocumentID *int64      `json:"document_id"`
	}
	if !decodeSelfServiceBody(w, r, &in) {
		return
	}
	day, ok := parseDate(w, "claim_date", in.ClaimDate)
	if !ok {
		return
	}
	claim, err := h.expenses.Submit(r.Context(), hrexpenses.SubmitInput{UserID: uid, CompanyID: cid, ClaimDate: day, Category: in.Category, Description: in.Description, Amount: in.Amount.String(), DocumentID: in.DocumentID})
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "expense_claim": toExpenseJSON(claim)})
}

func (h *Handler) employeeExpenseCancel(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.expenses == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.expenses.Cancel(r.Context(), cid, id, uid); err != nil {
		writeSelfServiceError(w, err)
		return
	}
	cancelledJSON(w, id)
}

// --- profile -----------------------------------------------------------------

// employeeProfileJSON is the employee's own profile. national_id and tax_id
// are deliberately absent; the employee sees their own address and DOB.
type employeeProfileJSON struct {
	ID                int64   `json:"id"`
	EmployeeNumber    string  `json:"employee_number"`
	Name              string  `json:"name"`
	Email             string  `json:"email"`
	Department        string  `json:"department"`
	Position          string  `json:"position"`
	Manager           string  `json:"manager"`
	Status            string  `json:"status"`
	EmploymentType    string  `json:"employment_type"`
	HireDate          *string `json:"hire_date"`
	DateOfBirth       *string `json:"date_of_birth"`
	Gender            string  `json:"gender"`
	MaritalStatus     string  `json:"marital_status"`
	DependentsCount   int     `json:"dependents_count"`
	Phone             string  `json:"phone"`
	AddressLine1      string  `json:"address_line1"`
	AddressLine2      string  `json:"address_line2"`
	City              string  `json:"city"`
	Province          string  `json:"province"`
	PostalCode        string  `json:"postal_code"`
	Country           string  `json:"country"`
	ContractStartDate *string `json:"contract_start_date"`
	ContractEndDate   *string `json:"contract_end_date"`
	ProbationEndDate  *string `json:"probation_end_date"`
}

type emergencyContactJSON struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
	Email        string `json:"email"`
	IsPrimary    bool   `json:"is_primary"`
}

func toEmployeeProfileJSON(e hremployees.Employee) employeeProfileJSON {
	hire := e.HireDate
	return employeeProfileJSON{
		ID: e.ID, EmployeeNumber: e.EmployeeNumber, Name: e.Name, Email: e.Email,
		Department: e.Department, Position: e.Position, Manager: e.Manager, Status: e.Status,
		EmploymentType: e.EmploymentType, HireDate: formatDatePtr(&hire),
		DateOfBirth: formatDatePtr(e.DateOfBirth), Gender: e.Gender, MaritalStatus: e.MaritalStatus,
		DependentsCount: e.DependentsCount, Phone: e.Phone,
		AddressLine1: e.AddressLine1, AddressLine2: e.AddressLine2, City: e.City,
		Province: e.Province, PostalCode: e.PostalCode, Country: e.Country,
		ContractStartDate: formatDatePtr(e.ContractStartDate), ContractEndDate: formatDatePtr(e.ContractEndDate),
		ProbationEndDate: formatDatePtr(e.ProbationEndDate),
	}
}

func (h *Handler) employeeProfileGet(w http.ResponseWriter, r *http.Request) {
	_, cid, employeeID, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.employeeProfile == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	emp, err := h.employeeProfile.GetByID(r.Context(), cid, employeeID)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	contacts, err := h.employeeProfile.ListContacts(r.Context(), cid, employeeID)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []emergencyContactJSON{}
	for _, c := range contacts {
		items = append(items, emergencyContactJSON{ID: c.ID, Name: c.Name, Relationship: c.Relationship, Phone: c.Phone, Email: c.Email, IsPrimary: c.IsPrimary})
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "profile": toEmployeeProfileJSON(emp), "emergency_contacts": items})
}

// --- profile changes ---------------------------------------------------------

type profileChangeJSON struct {
	ID         int64             `json:"id"`
	ChangeType string            `json:"change_type"`
	Payload    map[string]string `json:"payload"`
	Status     string            `json:"status"`
	AppliedAt  *time.Time        `json:"applied_at"`
	CreatedAt  *time.Time        `json:"created_at"`
}

func toProfileChangeJSON(x hrprofilechanges.Request) profileChangeJSON {
	payload := x.Payload
	if payload == nil {
		payload = map[string]string{}
	}
	return profileChangeJSON{ID: x.ID, ChangeType: x.ChangeType, Payload: payload, Status: x.Status, AppliedAt: x.AppliedAt, CreatedAt: timePtr(x.CreatedAt)}
}

func (h *Handler) employeeProfileChangeList(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.profileChanges == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	reqs, err := h.profileChanges.ListOwn(r.Context(), cid, uid)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []profileChangeJSON{}
	for _, x := range reqs {
		items = append(items, toProfileChangeJSON(x))
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "profile_changes": items})
}

func (h *Handler) employeeProfileChangeSubmit(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.profileChanges == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	var in struct {
		ChangeType string            `json:"change_type"`
		Payload    map[string]string `json:"payload"`
	}
	if !decodeSelfServiceBody(w, r, &in) {
		return
	}
	req, err := h.profileChanges.Submit(r.Context(), hrprofilechanges.SubmitInput{UserID: uid, CompanyID: cid, ChangeType: strings.ToUpper(strings.TrimSpace(in.ChangeType)), Payload: in.Payload})
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "profile_change": toProfileChangeJSON(req)})
}

func (h *Handler) employeeProfileChangeCancel(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.profileChanges == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.profileChanges.Cancel(r.Context(), cid, id, uid); err != nil {
		writeSelfServiceError(w, err)
		return
	}
	cancelledJSON(w, id)
}

// --- holidays ----------------------------------------------------------------

type holidayJSON struct {
	ID   int64  `json:"id"`
	Date string `json:"date"`
	Name string `json:"name"`
}

func (h *Handler) employeeHolidays(w http.ResponseWriter, r *http.Request) {
	_, cid, _, ok := h.employeeAccess(w, r)
	if !ok {
		return
	}
	if h.holidays == nil {
		writeSelfServiceError(w, errSelfServiceUnavailable)
		return
	}
	year, ok := queryYear(w, r)
	if !ok {
		return
	}
	list, err := h.holidays.ListHolidays(r.Context(), cid, year)
	if err != nil {
		writeSelfServiceError(w, err)
		return
	}
	items := []holidayJSON{}
	for _, x := range list {
		items = append(items, holidayJSON{ID: x.ID, Date: x.Date.Format(dateLayout), Name: x.Name})
	}
	writeSelfServiceJSON(w, http.StatusOK, map[string]any{"portal": "employee", "year": year, "holidays": items})
}
