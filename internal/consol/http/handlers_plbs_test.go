package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

func init() {
	if err := SetupCacheMetrics(prometheus.NewRegistry()); err != nil {
		panic(err)
	}
}

type member struct {
	id     int64
	name   string
	amount float64
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestTemplates(t *testing.T) *view.Engine {
	t.Helper()
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("failed to init templates: %v", err)
	}
	return templates
}

func makeMembersJSON(t *testing.T, members ...member) []byte {
	t.Helper()
	payload := make([]map[string]interface{}, len(members))
	for i, m := range members {
		payload[i] = map[string]interface{}{
			"company_id":    m.id,
			"company_name":  m.name,
			"local_ccy_amt": m.amount,
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal members: %v", err)
	}
	return data
}

type stubPLRepo struct {
	mu               sync.Mutex
	calls            int
	rows             []consol.ConsolBalanceByTypeQueryRow
	groupCurrency    string
	memberCurrencies map[int64]string
	quotes           map[string]fx.Quote
}

func (r *stubPLRepo) ConsolBalancesByType(ctx context.Context, groupID int64, period string, entities []int64) ([]consol.ConsolBalanceByTypeQueryRow, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	out := make([]consol.ConsolBalanceByTypeQueryRow, len(r.rows))
	copy(out, r.rows)
	return out, nil
}

func (r *stubPLRepo) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	if r.groupCurrency != "" {
		return r.groupCurrency, nil
	}
	return "USD", nil
}

func (r *stubPLRepo) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	out := make(map[int64]string, len(r.memberCurrencies))
	for id, cur := range r.memberCurrencies {
		out[id] = cur
	}
	return out, nil
}

func (r *stubPLRepo) FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error) {
	if quote, ok := r.quotes[pair]; ok {
		return quote, nil
	}
	return fx.Quote{}, fmt.Errorf("rate missing")
}

type stubBSRepo struct {
	mu               sync.Mutex
	calls            int
	rows             []consol.ConsolBalanceByTypeQueryRow
	groupCurrency    string
	memberCurrencies map[int64]string
	quotes           map[string]fx.Quote
}

func (r *stubBSRepo) ConsolBalancesByType(ctx context.Context, groupID int64, period string, entities []int64) ([]consol.ConsolBalanceByTypeQueryRow, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	out := make([]consol.ConsolBalanceByTypeQueryRow, len(r.rows))
	copy(out, r.rows)
	return out, nil
}

func (r *stubBSRepo) GroupReportingCurrency(ctx context.Context, groupID int64) (string, error) {
	if r.groupCurrency != "" {
		return r.groupCurrency, nil
	}
	return "USD", nil
}

func (r *stubBSRepo) MemberCurrencies(ctx context.Context, groupID int64) (map[int64]string, error) {
	out := make(map[int64]string, len(r.memberCurrencies))
	for id, cur := range r.memberCurrencies {
		out[id] = cur
	}
	return out, nil
}

func (r *stubBSRepo) FxRateForPeriod(ctx context.Context, asOf time.Time, pair string) (fx.Quote, error) {
	if quote, ok := r.quotes[pair]; ok {
		return quote, nil
	}
	return fx.Quote{}, fmt.Errorf("rate missing")
}

type slowPLRepo struct {
	*stubPLRepo
	delay time.Duration
}

func (r *slowPLRepo) ConsolBalancesByType(ctx context.Context, groupID int64, period string, entities []int64) ([]consol.ConsolBalanceByTypeQueryRow, error) {
	time.Sleep(r.delay)
	return r.stubPLRepo.ConsolBalancesByType(ctx, groupID, period, entities)
}

func resetCacheMetrics(t *testing.T) {
	t.Helper()
	if cacheHitCounter != nil {
		cacheHitCounter.Reset()
	}
	if cacheMissCounter != nil {
		cacheMissCounter.Reset()
	}
	if vmBuildHistogram != nil {
		vmBuildHistogram.Reset()
	}
}

func histogramSampleCount(t *testing.T, vec *prometheus.HistogramVec, labels prometheus.Labels) uint64 {
	t.Helper()
	if vec == nil {
		return 0
	}
	metricsCh := make(chan prometheus.Metric, 16)
	go func() {
		vec.Collect(metricsCh)
		close(metricsCh)
	}()
	for metric := range metricsCh {
		dtoMetric := &dto.Metric{}
		if err := metric.Write(dtoMetric); err != nil {
			t.Fatalf("write histogram metric: %v", err)
		}
		if dtoMetric.GetHistogram() == nil {
			continue
		}
		if labelsMatch(dtoMetric, labels) {
			return dtoMetric.GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func labelsMatch(metric *dto.Metric, labels prometheus.Labels) bool {
	if metric == nil {
		return false
	}
	pairs := metric.GetLabel()
	if len(pairs) != len(labels) {
		return false
	}
	for _, pair := range pairs {
		if pair == nil {
			continue
		}
		if val, ok := labels[pair.GetName()]; !ok || val != pair.GetValue() {
			return false
		}
	}
	return true
}

type stubDB struct {
	perms map[int64][]string
}

func (s *stubDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *stubDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if strings.Contains(sql, "FROM user_roles") {
		var userID int64
		if len(args) > 0 {
			switch v := args[0].(type) {
			case int64:
				userID = v
			case int32:
				userID = int64(v)
			}
		}
		perms := append([]string(nil), s.perms[userID]...)
		return &stubRows{values: perms, index: -1}, nil
	}
	return &stubRows{values: nil, index: -1}, nil
}

func (s *stubDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return &stubRow{err: pgx.ErrNoRows}
}

type stubRows struct {
	values []string
	index  int
}

func (r *stubRows) Close() {
	r.index = len(r.values)
}

func (r *stubRows) Err() error {
	return nil
}

func (r *stubRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *stubRows) Next() bool {
	if r.index+1 >= len(r.values) {
		r.index = len(r.values)
		return false
	}
	r.index++
	return true
}

func (r *stubRows) Scan(dest ...interface{}) error {
	if r.index < 0 || r.index >= len(r.values) {
		return fmt.Errorf("no row available")
	}
	if len(dest) == 0 {
		return fmt.Errorf("no destination provided")
	}
	if s, ok := dest[0].(*string); ok {
		*s = r.values[r.index]
		return nil
	}
	return fmt.Errorf("unsupported destination %T", dest[0])
}

func (r *stubRows) Values() ([]interface{}, error) {
	if r.index < 0 || r.index >= len(r.values) {
		return nil, fmt.Errorf("no row available")
	}
	return []interface{}{r.values[r.index]}, nil
}

func (r *stubRows) RawValues() [][]byte {
	return nil
}

func (r *stubRows) Conn() *pgx.Conn {
	return nil
}

type stubRow struct {
	err error
}

func (r *stubRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	return fmt.Errorf("not implemented")
}

func newRBACMiddleware(perms map[int64][]string) rbac.Middleware {
	svc := &rbac.Service{}
	queries := sqlc.New(&stubDB{perms: perms})
	field := reflect.ValueOf(svc).Elem().FieldByName("queries")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(queries))
	return rbac.Middleware{Service: svc}
}

func TestProfitLossHandleGetCachesViewModel(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -1000,
				GroupAmount:      -1000,
				MembersJSON: makeMembersJSON(t,
					member{id: 1, name: "Alpha", amount: -1000},
				),
			},
			{
				GroupAccountID:   2,
				GroupAccountCode: "5000",
				GroupAccountName: "COGS",
				AccountType:      "EXPENSE",
				LocalAmount:      600,
				GroupAmount:      600,
				MembersJSON: makeMembersJSON(t,
					member{id: 1, name: "Alpha", amount: 600},
				),
			},
		},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl?group=1&period=2024-01&fx=off", nil)
	rr := httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if repo.calls != 1 {
		t.Fatalf("expected 1 build call, got %d", repo.calls)
	}
	rr = httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if repo.calls != 1 {
		t.Fatalf("expected cache hit to avoid new build, got %d calls", repo.calls)
	}
}

func TestBalanceSheetHandleGetCachesViewModel(t *testing.T) {
	BustConsolViewCache()
	repo := &stubBSRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "1000",
				GroupAccountName: "Cash",
				AccountType:      "ASSET",
				LocalAmount:      1000,
				GroupAmount:      1000,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: 1000}),
			},
			{
				GroupAccountID:   2,
				GroupAccountCode: "2000",
				GroupAccountName: "Accounts Payable",
				AccountType:      "LIABILITY",
				LocalAmount:      -1000,
				GroupAmount:      -1000,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -1000}),
			},
		},
	}
	service := consol.NewBalanceSheetService(repo)
	templates := newTestTemplates(t)
	handler, err := NewBalanceSheetHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/bs?group=1&period=2024-01&fx=off", nil)
	rr := httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if repo.calls != 1 {
		t.Fatalf("expected 1 build call, got %d", repo.calls)
	}
	rr = httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if repo.calls != 1 {
		t.Fatalf("expected cache hit to avoid new build, got %d calls", repo.calls)
	}
}

func TestConsolHandlersRecordCacheMetrics(t *testing.T) {
	BustConsolViewCache()
	resetCacheMetrics(t)
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -1000,
				GroupAmount:      -1000,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -1000}),
			},
			{
				GroupAccountID:   2,
				GroupAccountCode: "5000",
				GroupAccountName: "COGS",
				AccountType:      "EXPENSE",
				LocalAmount:      600,
				GroupAmount:      600,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: 600}),
			},
		},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl?group=1&period=2024-01&fx=off", nil)
	rr := httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if got := testutil.ToFloat64(cacheMissCounter.WithLabelValues("pl", "1", "2024-01")); got != 1 {
		t.Fatalf("expected 1 cache miss metric, got %v", got)
	}
	if got := testutil.ToFloat64(cacheHitCounter.WithLabelValues("pl", "1", "2024-01")); got != 1 {
		t.Fatalf("expected 1 cache hit metric, got %v", got)
	}
	if count := histogramSampleCount(t, vmBuildHistogram, prometheus.Labels{"report": "pl", "group": "1", "period": "2024-01"}); count == 0 {
		t.Fatalf("expected vm build duration metric sample")
	}
}

func TestProfitLossHandleGetSingleflightPreventsStampede(t *testing.T) {
	BustConsolViewCache()
	resetCacheMetrics(t)
	base := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -1000,
				GroupAmount:      -1000,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -1000}),
			},
		},
	}
	repo := &slowPLRepo{stubPLRepo: base, delay: 50 * time.Millisecond}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl?group=1&period=2024-01&fx=off", nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rr := httptest.NewRecorder()
			handler.HandleGet(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("unexpected status %d", rr.Code)
			}
		}()
	}
	close(start)
	wg.Wait()
	if repo.calls != 1 {
		t.Fatalf("expected single build call, got %d", repo.calls)
	}
	if got := testutil.ToFloat64(cacheMissCounter.WithLabelValues("pl", "1", "2024-01")); got != 1 {
		t.Fatalf("expected one cache miss metric, got %v", got)
	}
	if got := testutil.ToFloat64(cacheHitCounter.WithLabelValues("pl", "1", "2024-01")); got != 0 {
		t.Fatalf("expected zero cache hit metrics during stampede, got %v", got)
	}
}

func TestProfitLossHandleGetFXFallbackWarning(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -500,
				GroupAmount:      -500,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -500}),
			},
		},
		groupCurrency: "USD",
		memberCurrencies: map[int64]string{
			1: "IDR",
		},
		quotes: map[string]fx.Quote{},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl?group=1&period=2024-01&fx=on", nil)
	rr := httptest.NewRecorder()
	handler.HandleGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	key := buildCacheKey("pl", 1, "2024-01", nil, true)
	cached, ok := viewModelCache.Get(key)
	if !ok {
		t.Fatalf("expected cached view model")
	}
	vm, ok := cached.(ConsolPLViewModel)
	if !ok {
		t.Fatalf("unexpected cached type %T", cached)
	}
	if vm.Filters.FxOn {
		t.Fatalf("expected FX to be disabled after fallback")
	}
	if len(vm.Warnings) == 0 {
		t.Fatalf("expected warnings when rate missing")
	}
	found := false
	for _, msg := range vm.Warnings {
		if strings.Contains(msg, "FX rate missing") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected FX warning in view model, got %v", vm.Warnings)
	}
}

func TestProfitLossCSVExportWarningHeaderAndMime(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -500,
				GroupAmount:      -500,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -500}),
			},
		},
		groupCurrency:    "USD",
		memberCurrencies: map[int64]string{1: "IDR"},
		quotes:           map[string]fx.Quote{},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, rbac.Middleware{}, nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl/export.csv?group=1&period=2024-01&fx=on", nil)
	rr := httptest.NewRecorder()
	handler.HandleExportCSV(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/csv" {
		t.Fatalf("expected text/csv got %s", got)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("expected CSV body")
	}
	if warn := rr.Header().Get("X-Consol-Warning"); !strings.Contains(warn, "FX rate missing") {
		t.Fatalf("expected warning header, got %q", warn)
	}
}

func TestProfitLossExportRequiresPermission(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -500,
				GroupAmount:      -500,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -500}),
			},
		},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)

	tests := []struct {
		name        string
		permissions map[int64][]string
		wantStatus  int
	}{
		{
			name: "denied without export permission",
			permissions: map[int64][]string{
				1: {shared.PermFinanceConsolView},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "allowed with export permission",
			permissions: map[int64][]string{
				1: {shared.PermFinanceConsolView, shared.PermFinanceConsolExport},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, newRBACMiddleware(tc.permissions), nil)
			if err != nil {
				t.Fatalf("failed to init handler: %v", err)
			}
			router := chi.NewRouter()
			handler.MountRoutes(router)
			req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl/export.csv?group=1&period=2024-01&fx=off", nil)
			sess := &shared.Session{ID: "sess"}
			sess.SetUser("1")
			req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, rr.Code)
			}
			if tc.wantStatus == http.StatusOK {
				if got := rr.Header().Get("Content-Type"); got != "text/csv" {
					t.Fatalf("expected text/csv got %s", got)
				}
			}
		})
	}
}

func TestProfitLossExportRateLimit(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -500,
				GroupAmount:      -500,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -500}),
			},
		},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	permissions := map[int64][]string{
		1: {shared.PermFinanceConsolView, shared.PermFinanceConsolExport},
	}
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, newRBACMiddleware(permissions), nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	router := chi.NewRouter()
	handler.MountRoutes(router)

	for i := 0; i < 11; i++ {
		req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl/export.csv?group=1&period=2024-01&fx=off", nil)
		sess := &shared.Session{ID: fmt.Sprintf("sess-%d", i)}
		sess.SetUser("1")
		req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if i < 10 {
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 on request %d got %d", i+1, rr.Code)
			}
		} else {
			if rr.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429 on rate limited request, got %d", rr.Code)
			}
		}
	}
}

func TestProfitLossExportPDFUnavailable(t *testing.T) {
	BustConsolViewCache()
	repo := &stubPLRepo{
		rows: []consol.ConsolBalanceByTypeQueryRow{
			{
				GroupAccountID:   1,
				GroupAccountCode: "4000",
				GroupAccountName: "Revenue",
				AccountType:      "REVENUE",
				LocalAmount:      -500,
				GroupAmount:      -500,
				MembersJSON:      makeMembersJSON(t, member{id: 1, name: "Alpha", amount: -500}),
			},
		},
	}
	service := consol.NewProfitLossService(repo)
	templates := newTestTemplates(t)
	permissions := map[int64][]string{
		1: {shared.PermFinanceConsolView, shared.PermFinanceConsolExport},
	}
	handler, err := NewProfitLossHandler(newTestLogger(), service, templates, shared.NewCSRFManager("secret"), nil, newRBACMiddleware(permissions), nil)
	if err != nil {
		t.Fatalf("failed to init handler: %v", err)
	}
	router := chi.NewRouter()
	handler.MountRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/finance/consol/pl/pdf?group=1&period=2024-01&fx=off", nil)
	sess := &shared.Session{ID: "sess"}
	sess.SetUser("1")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when PDF exporter unavailable, got %d", rr.Code)
	}
}

func TestBustConsolViewCacheClearsEntries(t *testing.T) {
	key := buildCacheKey("pl", 1, "2024-01", nil, false)
	viewModelCache.Set(key, ConsolPLViewModel{})
	if _, ok := viewModelCache.Get(key); !ok {
		t.Fatalf("expected value to be cached")
	}
	BustConsolViewCache()
	if _, ok := viewModelCache.Get(key); ok {
		t.Fatalf("expected cache to be cleared")
	}
}
