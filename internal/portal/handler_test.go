package portal_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/app"
	hremployees "github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	hrexpenses "github.com/odyssey-erp/odyssey-erp/internal/hr/expenses"
	hrleave "github.com/odyssey-erp/odyssey-erp/internal/hr/leave"
	hrovertime "github.com/odyssey-erp/odyssey-erp/internal/hr/overtime"
	hrprofilechanges "github.com/odyssey-erp/odyssey-erp/internal/hr/profilechanges"
	hrschedule "github.com/odyssey-erp/odyssey-erp/internal/hr/schedule"
	"github.com/odyssey-erp/odyssey-erp/internal/portal"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func TestInvitationTokenReturnsHashOfPlainToken(t *testing.T) {
	plain, hash, err := portal.InvitationTokenForTest()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != 64 || len(hash) != 64 {
		t.Fatalf("token lengths plain=%d hash=%d", len(plain), len(hash))
	}
	sum := sha256.Sum256([]byte(plain))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash does not match token: %q", hash)
	}
}

func TestPortalUserReadsOnlyValidSessionIdentity(t *testing.T) {
	if _, _, ok := portal.PortalUserForTest(httptest.NewRequest(http.MethodGet, "/", nil)); ok {
		t.Fatal("request without session must not authenticate")
	}

	req := portalRequestWithSession(t, "12", "7")
	userID, companyID, ok := portal.PortalUserForTest(req)
	if !ok || userID != 12 || companyID != 7 {
		t.Fatalf("identity user=%d company=%d ok=%v", userID, companyID, ok)
	}
}

func TestPortalRoutesRejectUnavailableOrUnauthorizedRequests(t *testing.T) {
	router := chi.NewRouter()
	portal.NewHandler(nil, nil, nil, rbac.Middleware{}).MountRoutes(router)

	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/portal/admin/invitations", http.StatusForbidden},
		{http.MethodPost, "/portal/invitations/accept", http.StatusServiceUnavailable},
		{http.MethodPut, "/portal/profile", http.StatusUnauthorized},
		{http.MethodPost, "/portal/chat", http.StatusUnauthorized},
		{http.MethodGet, "/portal/employee/overtime", http.StatusUnauthorized},
		{http.MethodPost, "/portal/employee/leave", http.StatusUnauthorized},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func portalRequestWithSession(t *testing.T, userID, companyID string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser(userID)
	session.Set("company_id", companyID)
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

// --- employee self-service ---------------------------------------------------

const (
	testUserID     = int64(12)
	testCompanyID  = int64(7)
	testEmployeeID = int64(42)
)

type cancelCall struct{ CompanyID, ID, UserID int64 }

type fakeLeave struct {
	mu        sync.Mutex
	submits   []hrleave.CreateInput
	cancels   []cancelCall
	listUsers []int64
	balances  []struct {
		CompanyID, UserID int64
		Year              int
	}
	types []int64
}

func (f *fakeLeave) Types(_ context.Context, companyID int64) ([]hrleave.LeaveType, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.types = append(f.types, companyID)
	return []hrleave.LeaveType{{ID: 3, Code: "AL", Name: "Annual", DefaultDays: 12}}, nil
}

func (f *fakeLeave) ListOwn(_ context.Context, userID int64) ([]hrleave.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listUsers = append(f.listUsers, userID)
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return []hrleave.Request{
		{ID: 1, EmployeeID: testEmployeeID, LeaveTypeID: 3, TypeName: "Annual", StartDate: day, EndDate: day, Days: 0.5, StartHalfDay: true, Status: "PENDING"},
		{ID: 2, EmployeeID: 999, LeaveTypeID: 3, TypeName: "Other company", StartDate: day, EndDate: day, Days: 1, Status: "PENDING"},
	}, nil
}

func (f *fakeLeave) GetOwnBalances(_ context.Context, companyID, userID int64, year int) ([]hrleave.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balances = append(f.balances, struct {
		CompanyID, UserID int64
		Year              int
	}{companyID, userID, year})
	return []hrleave.Balance{{LeaveTypeID: 3, TypeCode: "AL", TypeName: "Annual", Year: year, Entitled: 12, Used: 2, Pending: 0.5, Available: 9.5}}, nil
}

func (f *fakeLeave) Submit(_ context.Context, in hrleave.CreateInput) (hrleave.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, in)
	return hrleave.Request{ID: 10, EmployeeID: testEmployeeID, LeaveTypeID: in.LeaveTypeID, StartDate: in.StartDate, EndDate: in.EndDate, Days: 1.5, StartHalfDay: in.StartHalfDay, EndHalfDay: in.EndHalfDay, Status: "PENDING"}, nil
}

func (f *fakeLeave) Cancel(_ context.Context, companyID, requestID, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, cancelCall{companyID, requestID, userID})
	return nil
}

type fakeOvertime struct {
	mu        sync.Mutex
	submits   []hrovertime.SubmitInput
	cancels   []cancelCall
	lists     []cancelCall
	submitErr error
}

func (f *fakeOvertime) ListOwn(_ context.Context, companyID, userID int64) ([]hrovertime.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, cancelCall{CompanyID: companyID, UserID: userID})
	return []hrovertime.Request{{ID: 5, WorkedOn: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), Minutes: 90, Status: "PENDING"}}, nil
}

func (f *fakeOvertime) Submit(_ context.Context, in hrovertime.SubmitInput) (hrovertime.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, in)
	if f.submitErr != nil {
		return hrovertime.Request{}, f.submitErr
	}
	return hrovertime.Request{ID: 6, WorkedOn: in.WorkedOn, Minutes: in.Minutes, Reason: in.Reason, Status: "PENDING"}, nil
}

func (f *fakeOvertime) Cancel(_ context.Context, companyID, requestID, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, cancelCall{companyID, requestID, userID})
	return nil
}

type fakeExpenses struct {
	mu      sync.Mutex
	submits []hrexpenses.SubmitInput
	cancels []cancelCall
	lists   []cancelCall
}

func (f *fakeExpenses) ListOwn(_ context.Context, companyID, userID int64) ([]hrexpenses.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, cancelCall{CompanyID: companyID, UserID: userID})
	return []hrexpenses.Claim{{ID: 8, ClaimDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Category: "Travel", Amount: "125000.50", Currency: "IDR", Status: "PENDING"}}, nil
}

func (f *fakeExpenses) Submit(_ context.Context, in hrexpenses.SubmitInput) (hrexpenses.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, in)
	return hrexpenses.Claim{ID: 9, ClaimDate: in.ClaimDate, Category: in.Category, Amount: in.Amount, Currency: "IDR", Status: "PENDING", DocumentID: in.DocumentID}, nil
}

func (f *fakeExpenses) Cancel(_ context.Context, companyID, claimID, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, cancelCall{companyID, claimID, userID})
	return nil
}

type fakeProfileChanges struct {
	mu      sync.Mutex
	submits []hrprofilechanges.SubmitInput
	cancels []cancelCall
	lists   []cancelCall
}

func (f *fakeProfileChanges) ListOwn(_ context.Context, companyID, userID int64) ([]hrprofilechanges.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, cancelCall{CompanyID: companyID, UserID: userID})
	return []hrprofilechanges.Request{{ID: 11, ChangeType: "ADDRESS", Payload: map[string]string{"city": "Bandung"}, Status: "PENDING"}}, nil
}

func (f *fakeProfileChanges) Submit(_ context.Context, in hrprofilechanges.SubmitInput) (hrprofilechanges.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, in)
	return hrprofilechanges.Request{ID: 12, ChangeType: in.ChangeType, Payload: in.Payload, Status: "PENDING"}, nil
}

func (f *fakeProfileChanges) Cancel(_ context.Context, companyID, requestID, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, cancelCall{companyID, requestID, userID})
	return nil
}

type fakeHolidays struct {
	mu    sync.Mutex
	calls []struct {
		CompanyID int64
		Year      int
	}
}

func (f *fakeHolidays) ListHolidays(_ context.Context, companyID int64, year int) ([]hrschedule.Holiday, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		CompanyID int64
		Year      int
	}{companyID, year})
	return []hrschedule.Holiday{{ID: 1, Date: time.Date(year, 8, 17, 0, 0, 0, 0, time.UTC), Name: "Independence Day"}}, nil
}

type fakeProfiles struct {
	mu    sync.Mutex
	gets  []cancelCall
	lists []cancelCall
}

func (f *fakeProfiles) GetByID(_ context.Context, companyID, id int64) (hremployees.Employee, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, cancelCall{CompanyID: companyID, ID: id})
	dob := time.Date(1990, 5, 6, 0, 0, 0, 0, time.UTC)
	e := hremployees.Employee{ID: id, CompanyID: companyID, EmployeeNumber: "E-042", Name: "Grace Hopper", Status: "ACTIVE", HireDate: time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)}
	e.DateOfBirth = &dob
	e.NationalID = "3171000000000001"
	e.TaxID = "09.254.294.3-407.000"
	e.AddressLine1 = "Jl. Sudirman 1"
	e.City = "Jakarta"
	return e, nil
}

func (f *fakeProfiles) ListContacts(_ context.Context, companyID, employeeID int64) ([]hremployees.EmergencyContact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, cancelCall{CompanyID: companyID, ID: employeeID})
	return []hremployees.EmergencyContact{{ID: 4, EmployeeID: employeeID, Name: "Ada", Relationship: "Sister", IsPrimary: true}}, nil
}

type fakes struct {
	leave    *fakeLeave
	overtime *fakeOvertime
	expenses *fakeExpenses
	changes  *fakeProfileChanges
	holidays *fakeHolidays
	profiles *fakeProfiles
}

func (f fakes) totalCalls() int {
	return len(f.leave.submits) + len(f.leave.cancels) + len(f.leave.listUsers) + len(f.leave.balances) + len(f.leave.types) +
		len(f.overtime.submits) + len(f.overtime.cancels) + len(f.overtime.lists) +
		len(f.expenses.submits) + len(f.expenses.cancels) + len(f.expenses.lists) +
		len(f.changes.submits) + len(f.changes.cancels) + len(f.changes.lists) +
		len(f.holidays.calls) + len(f.profiles.gets) + len(f.profiles.lists)
}

// selfServiceEnv is the portal router behind the real internal/app
// middleware stack (session + CSRF) with a miniredis-backed session store.
type selfServiceEnv struct {
	handler  http.Handler
	sessions *shared.SessionManager
	csrf     *shared.CSRFManager
	fakes    fakes
}

func newSelfServiceEnv(t *testing.T, linked bool, templates *view.Engine) *selfServiceEnv {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	sessions := shared.NewSessionManager(client, "session", "secret", time.Hour, false)
	csrf := shared.NewCSRFManager("csrfsecret")

	f := fakes{&fakeLeave{}, &fakeOvertime{}, &fakeExpenses{}, &fakeProfileChanges{}, &fakeHolidays{}, &fakeProfiles{}}
	h := portal.NewHandler(nil, templates, csrf, rbac.Middleware{})
	h.SetHRServices(f.leave, f.overtime, f.expenses, f.changes, f.holidays)
	h.SetEmployeeProfileService(f.profiles)
	h.SetEmployeeResolverForTest(func(_ context.Context, userID, companyID int64) (int64, error) {
		if !linked || userID != testUserID || companyID != testCompanyID {
			return 0, shared.ErrUnauthorized
		}
		return testEmployeeID, nil
	})
	router := chi.NewRouter()
	h.MountRoutes(router)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var handler http.Handler = router
	stack := app.MiddlewareStack(app.MiddlewareConfig{Logger: logger, SessionManager: sessions, CSRFManager: csrf})
	for i := len(stack) - 1; i >= 0; i-- {
		handler = stack[i](handler)
	}
	return &selfServiceEnv{handler: handler, sessions: sessions, csrf: csrf, fakes: f}
}

// login stores a session for the test user and returns its id and CSRF token.
func (e *selfServiceEnv) login(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, err := e.sessions.Load(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	sess.SetUser(fmt.Sprint(testUserID))
	sess.Set("company_id", fmt.Sprint(testCompanyID))
	token, err := e.csrf.EnsureToken(ctx, sess)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.sessions.Commit(ctx, httptest.NewRecorder(), req, sess); err != nil {
		t.Fatal(err)
	}
	return sess.ID, token
}

func (e *selfServiceEnv) do(t *testing.T, method, path, body, sessionID, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: e.sessions.CookieName(), Value: sessionID})
	}
	if token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

type selfServiceRoute struct {
	method, path, body string
}

var selfServiceRoutes = []selfServiceRoute{
	{http.MethodGet, "/portal/employee/leave?year=2026", ""},
	{http.MethodPost, "/portal/employee/leave", `{"leave_type_id":3,"start_date":"2026-11-02","end_date":"2026-11-03","reason":"family","start_half_day":true,"end_half_day":false}`},
	{http.MethodPost, "/portal/employee/leave/10/cancel", ""},
	{http.MethodGet, "/portal/employee/overtime", ""},
	{http.MethodPost, "/portal/employee/overtime", `{"worked_on":"2026-10-03","minutes":90,"reason":"release"}`},
	{http.MethodPost, "/portal/employee/overtime/6/cancel", ""},
	{http.MethodGet, "/portal/employee/expenses", ""},
	{http.MethodPost, "/portal/employee/expenses", `{"claim_date":"2026-10-02","category":"Travel","description":"Taxi","amount":"125000.50","document_id":77}`},
	{http.MethodPost, "/portal/employee/expenses/9/cancel", ""},
	{http.MethodGet, "/portal/employee/profile", ""},
	{http.MethodGet, "/portal/employee/profile-changes", ""},
	{http.MethodPost, "/portal/employee/profile-changes", `{"change_type":"address","payload":{"city":"Bandung"}}`},
	{http.MethodPost, "/portal/employee/profile-changes/12/cancel", ""},
	{http.MethodGet, "/portal/employee/holidays?year=2026", ""},
}

func TestEmployeeSelfServiceRequiresEmployeeLink(t *testing.T) {
	env := newSelfServiceEnv(t, false, nil)
	sessionID, token := env.login(t)
	for _, tc := range selfServiceRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := env.do(t, tc.method, tc.path, tc.body, sessionID, token)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	if n := env.fakes.totalCalls(); n != 0 {
		t.Fatalf("services called %d times without an employee link", n)
	}
}

func TestEmployeeSelfServiceWritesRequireCSRFToken(t *testing.T) {
	env := newSelfServiceEnv(t, true, nil)
	sessionID, _ := env.login(t)
	for _, tc := range selfServiceRoutes {
		if tc.method == http.MethodGet {
			continue
		}
		for _, token := range []string{"", "wrong-token"} {
			t.Run(tc.path+" token="+token, func(t *testing.T) {
				rec := env.do(t, tc.method, tc.path, tc.body, sessionID, token)
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status=%d want 403 body=%s", rec.Code, rec.Body.String())
				}
			})
		}
	}
	if n := env.fakes.totalCalls(); n != 0 {
		t.Fatalf("services called %d times on CSRF-rejected requests", n)
	}
}

func TestEmployeeSelfServiceWithValidTokenCallsServices(t *testing.T) {
	env := newSelfServiceEnv(t, true, nil)
	sessionID, token := env.login(t)
	bodies := map[string]map[string]any{}
	for _, tc := range selfServiceRoutes {
		rec := env.do(t, tc.method, tc.path, tc.body, sessionID, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d want 200 body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s %s content-type=%q", tc.method, tc.path, ct)
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s invalid JSON: %v", tc.method, tc.path, err)
		}
		bodies[tc.method+" "+tc.path] = out
	}
	f := env.fakes

	// Leave.
	if len(f.leave.submits) != 1 {
		t.Fatalf("leave submits=%d", len(f.leave.submits))
	}
	ls := f.leave.submits[0]
	if ls.CompanyID != testCompanyID || ls.UserID != testUserID || ls.LeaveTypeID != 3 || !ls.StartHalfDay || ls.EndHalfDay || ls.Reason != "family" ||
		ls.StartDate.Format("2006-01-02") != "2026-11-02" || ls.EndDate.Format("2006-01-02") != "2026-11-03" {
		t.Fatalf("leave submit input=%+v", ls)
	}
	if want := (cancelCall{testCompanyID, 10, testUserID}); len(f.leave.cancels) != 1 || f.leave.cancels[0] != want {
		t.Fatalf("leave cancels=%+v", f.leave.cancels)
	}
	if len(f.leave.balances) != 1 || f.leave.balances[0].CompanyID != testCompanyID || f.leave.balances[0].UserID != testUserID || f.leave.balances[0].Year != 2026 {
		t.Fatalf("leave balances calls=%+v", f.leave.balances)
	}
	leaveItems := bodies["GET /portal/employee/leave?year=2026"]["leave"].([]any)
	if len(leaveItems) != 1 || leaveItems[0].(map[string]any)["id"].(float64) != 1 {
		t.Fatalf("leave list must keep only the resolved employee's rows: %+v", leaveItems)
	}

	// Overtime.
	if len(f.overtime.submits) != 1 {
		t.Fatalf("overtime submits=%d", len(f.overtime.submits))
	}
	os1 := f.overtime.submits[0]
	if os1.CompanyID != testCompanyID || os1.UserID != testUserID || os1.Minutes != 90 || os1.Reason != "release" || os1.WorkedOn.Format("2006-01-02") != "2026-10-03" {
		t.Fatalf("overtime submit input=%+v", os1)
	}
	if want := (cancelCall{testCompanyID, 6, testUserID}); len(f.overtime.cancels) != 1 || f.overtime.cancels[0] != want {
		t.Fatalf("overtime cancels=%+v", f.overtime.cancels)
	}
	if want := (cancelCall{CompanyID: testCompanyID, UserID: testUserID}); len(f.overtime.lists) != 1 || f.overtime.lists[0] != want {
		t.Fatalf("overtime lists=%+v", f.overtime.lists)
	}

	// Expenses: amount passes through as exact text.
	if len(f.expenses.submits) != 1 {
		t.Fatalf("expense submits=%d", len(f.expenses.submits))
	}
	es := f.expenses.submits[0]
	if es.CompanyID != testCompanyID || es.UserID != testUserID || es.Amount != "125000.50" || es.Category != "Travel" || es.DocumentID == nil || *es.DocumentID != 77 {
		t.Fatalf("expense submit input=%+v", es)
	}
	if want := (cancelCall{testCompanyID, 9, testUserID}); len(f.expenses.cancels) != 1 || f.expenses.cancels[0] != want {
		t.Fatalf("expense cancels=%+v", f.expenses.cancels)
	}
	claim := bodies["POST /portal/employee/expenses"]["expense_claim"].(map[string]any)
	if claim["amount"] != "125000.50" {
		t.Fatalf("expense amount in response=%v", claim["amount"])
	}

	// Profile changes: change type normalised to upper case.
	if len(f.changes.submits) != 1 {
		t.Fatalf("profile change submits=%d", len(f.changes.submits))
	}
	ps := f.changes.submits[0]
	if ps.CompanyID != testCompanyID || ps.UserID != testUserID || ps.ChangeType != "ADDRESS" || ps.Payload["city"] != "Bandung" {
		t.Fatalf("profile change submit input=%+v", ps)
	}
	if want := (cancelCall{testCompanyID, 12, testUserID}); len(f.changes.cancels) != 1 || f.changes.cancels[0] != want {
		t.Fatalf("profile change cancels=%+v", f.changes.cancels)
	}

	// Profile uses the resolved employee id and omits national/tax ids.
	if want := (cancelCall{CompanyID: testCompanyID, ID: testEmployeeID}); len(f.profiles.gets) != 1 || f.profiles.gets[0] != want {
		t.Fatalf("profile gets=%+v", f.profiles.gets)
	}
	profile := bodies["GET /portal/employee/profile"]["profile"].(map[string]any)
	for _, key := range []string{"national_id", "tax_id", "NationalID", "TaxID"} {
		if _, ok := profile[key]; ok {
			t.Fatalf("profile exposes %s: %+v", key, profile)
		}
	}
	if profile["date_of_birth"] != "1990-05-06" || profile["address_line1"] != "Jl. Sudirman 1" || profile["city"] != "Jakarta" {
		t.Fatalf("profile must show own DOB and address: %+v", profile)
	}

	// Holidays.
	if len(f.holidays.calls) != 1 || f.holidays.calls[0].CompanyID != testCompanyID || f.holidays.calls[0].Year != 2026 {
		t.Fatalf("holiday calls=%+v", f.holidays.calls)
	}
}

func TestEmployeeSelfServiceProfileResponseHasNoSensitiveIDs(t *testing.T) {
	env := newSelfServiceEnv(t, true, nil)
	sessionID, token := env.login(t)
	rec := env.do(t, http.MethodGet, "/portal/employee/profile", "", sessionID, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{"3171000000000001", "09.254.294.3-407.000", "national_id", "tax_id"} {
		if strings.Contains(body, secret) {
			t.Fatalf("profile response leaks %q: %s", secret, body)
		}
	}
}

func TestEmployeeSelfServiceErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		status     int
		wantMsg    string
		notContain string
	}{
		{"domain error is shown", hrovertime.ErrAlreadyRequested, http.StatusBadRequest, "Overtime already requested for that day", ""},
		{"not found maps to 404", hrovertime.ErrEmployeeNotFound, http.StatusNotFound, "Not found: employee", ""},
		{"wrapped driver error is hidden", fmt.Errorf("hr: payroll lock check: %w", errors.New("pq: relation secret_table")), http.StatusInternalServerError, "", "secret_table"},
		{"plain error is hidden", errors.New("connection refused"), http.StatusInternalServerError, "", "connection refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newSelfServiceEnv(t, true, nil)
			env.fakes.overtime.submitErr = tc.err
			sessionID, token := env.login(t)
			rec := env.do(t, http.MethodPost, "/portal/employee/overtime", `{"worked_on":"2026-10-03","minutes":90}`, sessionID, token)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			var out struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("error body is not JSON: %s", rec.Body.String())
			}
			if tc.wantMsg != "" && out.Error != tc.wantMsg {
				t.Fatalf("error=%q want %q", out.Error, tc.wantMsg)
			}
			if tc.notContain != "" && strings.Contains(rec.Body.String(), tc.notContain) {
				t.Fatalf("error body leaks %q: %s", tc.notContain, rec.Body.String())
			}
		})
	}
}

func TestEmployeeSelfServiceRejectsBadInput(t *testing.T) {
	env := newSelfServiceEnv(t, true, nil)
	sessionID, token := env.login(t)
	for _, tc := range []selfServiceRoute{
		{http.MethodPost, "/portal/employee/leave", `{"leave_type_id":3,"start_date":"02/11/2026","end_date":"2026-11-03"}`},
		{http.MethodPost, "/portal/employee/overtime", `not json`},
		{http.MethodPost, "/portal/employee/expenses/abc/cancel", ""},
		{http.MethodGet, "/portal/employee/holidays?year=20x6", ""},
	} {
		rec := env.do(t, tc.method, tc.path, tc.body, sessionID, token)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d want 400 body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
	if n := env.fakes.totalCalls(); n != 0 {
		t.Fatalf("services called %d times on invalid input", n)
	}
}

func TestPortalAppRendersSessionCSRFToken(t *testing.T) {
	engine, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	env := newSelfServiceEnv(t, true, engine)
	sessionID, token := env.login(t)
	rec := env.do(t, http.MethodGet, "/portal/app", "", sessionID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if want := `<meta name="csrf-token" content="` + token + `">`; !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("portal app missing %s", want)
	}
}

func TestPortalAppRendersEmployeeSelfServiceSections(t *testing.T) {
	engine, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	env := newSelfServiceEnv(t, true, engine)
	sessionID, token := env.login(t)
	rec := env.do(t, http.MethodGet, "/portal/app", "", sessionID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if want := `<meta name="csrf-token" content="` + token + `">`; !strings.Contains(body, want) {
		t.Fatalf("portal app missing %s", want)
	}
	for _, id := range []string{
		"section-leave-request", "section-overtime-request", "section-expense-claim", "section-profile-change",
		"leave-list", "overtime-list", "expense-list", "profile-change-list",
	} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("portal app missing element id %q", id)
		}
	}
	for _, want := range []string{
		`id="leaveStartHalf"`, `id="leaveEndHalf"`, `class="checkbox-label"`, `class="checkbox-input"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("portal app missing %q", want)
		}
	}
	// The behavior lives in the static script because the CSP forbids inline
	// scripts; the endpoints it calls are asserted there.
	js := readPortalJS(t)
	for _, want := range []string{
		`/portal/employee/leave`, `/portal/employee/overtime`, `/portal/employee/expenses`,
		`/portal/employee/profile-changes`, `/portal/employee/holidays`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("portal.js missing %q", want)
		}
	}
	// Server data is rendered with DOM APIs only.
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(js, bad) {
			t.Errorf("portal.js must build DOM with createElement/textContent, found %s", bad)
		}
	}
	if !strings.Contains(body, `<script src="/static/js/pages/portal.js" defer></script>`) {
		t.Errorf("portal app must load /static/js/pages/portal.js")
	}
}

func readPortalJS(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../web/static/js/pages/portal.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var (
	scriptTagRE   = regexp.MustCompile(`(?is)<script\b([^>]*)>`)
	inlineEventRE = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
)

// TestPortalAppHasNoInlineExecutableScript guards against the CSP
// (script-src 'self') silently disabling the page: every <script> must load a
// file or be a non-executed application/json data block, and no inline on*=
// event handler attributes may be used.
// TestPortalJSPrefillsAddressFromOwnProfile keeps the ADDRESS change form
// starting from the employee's current values (the profile endpoint already
// returns them); the server also ignores blank fields on approval.
func TestPortalJSPrefillsAddressFromOwnProfile(t *testing.T) {
	js := readPortalJS(t)
	for _, want := range []string{"address_line1", "postal_code", "defaultValue", "'pc-' + f"} {
		if !strings.Contains(js, want) {
			t.Errorf("portal.js loadProfile must prefill the address form (missing %q)", want)
		}
	}
}

func TestPortalAppHasNoInlineExecutableScript(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/pages/portal_app.html")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	tags := scriptTagRE.FindAllStringSubmatch(src, -1)
	if len(tags) == 0 {
		t.Fatal("portal_app.html must load its script via <script src=...>")
	}
	for _, m := range tags {
		attrs := m[1]
		hasSrc := regexp.MustCompile(`(?i)\ssrc\s*=`).MatchString(" " + attrs)
		isJSON := regexp.MustCompile(`(?i)\stype\s*=\s*["']application/json["']`).MatchString(" " + attrs)
		if !hasSrc && !isJSON {
			t.Errorf("inline executable <script%s> is blocked by CSP script-src 'self'", attrs)
		}
	}
	if loc := inlineEventRE.FindStringIndex(src); loc != nil {
		t.Errorf("inline event handler attribute %q is blocked by CSP", strings.TrimSpace(src[loc[0]:loc[1]]))
	}
}

// TestPortalAppFetchOnlyInsidePostJSON keeps every network call behind the
// helper that attaches X-CSRF-Token.
func TestPortalAppFetchOnlyInsidePostJSON(t *testing.T) {
	tmpl, err := os.ReadFile("../../web/templates/pages/portal_app.html")
	if err != nil {
		t.Fatal(err)
	}
	src := readPortalJS(t)
	if n := strings.Count(src, "fetch("); n != 1 {
		t.Fatalf("portal.js has %d fetch( occurrences, want exactly 1 (inside postJSON)", n)
	}
	start := strings.Index(src, "async function postJSON(")
	if start < 0 {
		t.Fatal("postJSON helper not found")
	}
	open := strings.Index(src[start:], "{")
	if open < 0 {
		t.Fatal("postJSON body not found")
	}
	depth, end := 0, -1
	for i := start + open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
		}
		if depth == 0 {
			end = i
			break
		}
	}
	if end < 0 {
		t.Fatal("postJSON body is not closed")
	}
	helper := src[start : end+1]
	if strings.Count(helper, "fetch(") != 1 {
		t.Fatal("the only fetch( must be inside postJSON")
	}
	for _, want := range []string{"X-CSRF-Token", "credentials: 'same-origin'", "'Content-Type'", "application/json", `meta[name="csrf-token"]`} {
		if !strings.Contains(helper, want) {
			t.Fatalf("postJSON helper missing %s", want)
		}
	}
	if !strings.Contains(string(tmpl), `<meta name="csrf-token" content="{{ .CSRFToken }}">`) {
		t.Fatal("portal_app.html must emit the csrf-token meta tag")
	}
}
