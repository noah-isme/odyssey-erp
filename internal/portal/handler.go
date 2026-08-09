package portal

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	pool *pgxpool.Pool
	rbac rbac.Middleware
	docs interface {
		Create(ctx context.Context, req documents.CreateDocumentRequest) (documents.Document, error)
		UploadAndCreateVersion(ctx context.Context, file io.Reader, size int64, mimeType string, req documents.CreateVersionRequest) (documents.DocumentVersion, error)
	}
	templates *view.Engine
}

// =============================================================================
// Advanced Portal Features (Profile Updates, RFQ Negotiation, Chat, Analytics)
// =============================================================================

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	uid, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		DisplayName string `json:"display_name"`
		Phone       string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	_, err := h.pool.Exec(r.Context(), `UPDATE users SET display_name=$1 WHERE id=$2 AND company_id=$3`, in.DisplayName, uid, cid)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) sendChatMessage(w http.ResponseWriter, r *http.Request) {
	uid, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		Context string `json:"context"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	_, err := h.pool.Exec(r.Context(), `INSERT INTO portal_chat_messages(company_id, sender_id, context, message) VALUES($1,$2,$3,$4)`, cid, uid, in.Context, in.Message)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) getChatMessages(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctxStr := r.URL.Query().Get("context")
	rows, err := h.pool.Query(r.Context(), `SELECT sender_id, message, created_at FROM portal_chat_messages WHERE company_id=$1 AND context=$2 ORDER BY created_at ASC`, cid, ctxStr)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	defer rows.Close()
	type msg struct {
		SenderID  int64     `json:"sender_id"`
		Message   string    `json:"message"`
		CreatedAt time.Time `json:"created_at"`
	}
	var out []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.SenderID, &m.Message, &m.CreatedAt); err != nil {
			shared.WriteErrorStatus(w, 500, err)
			return
		}
		out = append(out, m)
	}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		return
	}
}

func (h *Handler) negotiateRFQ(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		RFQID         int64   `json:"rfq_id"`
		SupplierID    int64   `json:"supplier_id"`
		ProposedPrice float64 `json:"proposed_price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	_, err := h.pool.Exec(r.Context(), `INSERT INTO portal_rfq_negotiations(company_id, rfq_id, supplier_id, proposed_price) VALUES($1,$2,$3,$4)`, cid, in.RFQID, in.SupplierID, in.ProposedPrice)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) recordAnalyticsEvent(w http.ResponseWriter, r *http.Request) {
	uid, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		EventType string         `json:"event_type"`
		Metadata  map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	meta, _ := json.Marshal(in.Metadata)
	_, err := h.pool.Exec(r.Context(), `INSERT INTO portal_analytics_events(company_id, user_id, event_type, metadata) VALUES($1,$2,$3,$4)`, cid, uid, in.EventType, meta)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func NewHandler(pool *pgxpool.Pool, templates *view.Engine, middleware ...rbac.Middleware) *Handler {
	var m rbac.Middleware
	if len(middleware) > 0 {
		m = middleware[0]
	}
	return &Handler{pool: pool, templates: templates, rbac: m}
}

func (h *Handler) SetDocumentsService(docs interface {
	Create(ctx context.Context, req documents.CreateDocumentRequest) (documents.Document, error)
	UploadAndCreateVersion(ctx context.Context, file io.Reader, size int64, mimeType string, req documents.CreateVersionRequest) (documents.DocumentVersion, error)
}) {
	h.docs = docs
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/portal", func(r chi.Router) {
		r.Get("/app", func(w http.ResponseWriter, r *http.Request) {
			_ = h.templates.Render(w, "pages/portal_app.html", view.TemplateData{Title: "Portal"})
		})

		r.Group(func(r chi.Router) {
			r.Use(h.rbac.RequireAny("portal.manage"))
			r.Post("/admin/invitations", h.createInvitation)
		})
		r.Post("/invitations/accept", h.acceptInvitation)
		r.Get("/customer", h.customer)
		r.Get("/customer/orders", h.customerOrders)
		r.Get("/customer/payments", h.customerPayments)
		r.Get("/customer/credit-notes", h.customerCreditNotes)
		r.Post("/customer/documents", h.uploadCustomerDocument)
		r.Get("/supplier", h.supplier)
		r.Get("/supplier/orders", h.supplierOrders)
		r.Get("/supplier/deliveries", h.supplierDeliveries)
		r.Post("/supplier/documents", h.uploadSupplierDocument)
		r.Get("/supplier/debit-notes", h.supplierDebitNotes)
		r.Get("/employee", h.employee)
		r.Get("/employee/payslips", h.employeePayslips)
		r.Get("/employee/leave", h.employeeLeave)
		r.Get("/employee/attendance", h.employeeAttendance)

		// Advanced Portal Features
		r.Put("/profile", h.updateProfile)
		r.Post("/chat", h.sendChatMessage)
		r.Get("/chat", h.getChatMessages)
		r.Post("/rfq/negotiate", h.negotiateRFQ)
		r.Post("/analytics/event", h.recordAnalyticsEvent)
	})
}

func invitationToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain := hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(sum[:]), nil
}
func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "portal database is unavailable", http.StatusServiceUnavailable)
		return
	}
	uid, cid, ok := portalUser(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		Email        string `json:"email"`
		DisplayName  string `json:"display_name"`
		PortalType   string `json:"portal_type"`
		CustomerID   *int64 `json:"customer_id"`
		SupplierID   *int64 `json:"supplier_id"`
		ExpiresHours int    `json:"expires_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.ExpiresHours <= 0 || in.ExpiresHours > 168 {
		in.ExpiresHours = 72
	}
	if in.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}
	plain, hash, err := invitationToken()
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	var id int64
	err = h.pool.QueryRow(r.Context(), `INSERT INTO portal_invitations(company_id,email,display_name,portal_type,customer_id,supplier_id,token_hash,expires_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,NOW()+($8||' hours')::interval,$9) RETURNING id`, cid, in.Email, in.DisplayName, in.PortalType, in.CustomerID, in.SupplierID, hash, in.ExpiresHours, uid).Scan(&id)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	portalJSON(w, map[string]any{"id": id, "email": in.Email, "portal_type": in.PortalType, "token": plain, "expires_in_hours": in.ExpiresHours})
}
func (h *Handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "portal database is unavailable", http.StatusServiceUnavailable)
		return
	}
	var in struct{ Token, Password string }
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Password) < 12 {
		http.Error(w, "token and password of at least 12 characters are required", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(in.Token))
	hash := hex.EncodeToString(sum[:])
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var inv struct {
		ID, CompanyID                  int64
		Email, DisplayName, PortalType string
		CustomerID, SupplierID         *int64
	}
	err = tx.QueryRow(r.Context(), `SELECT id,company_id,email,display_name,portal_type,customer_id,supplier_id FROM portal_invitations WHERE token_hash=$1 AND accepted_at IS NULL AND expires_at>NOW()`, hash).Scan(&inv.ID, &inv.CompanyID, &inv.Email, &inv.DisplayName, &inv.PortalType, &inv.CustomerID, &inv.SupplierID)
	if err != nil {
		http.Error(w, "invalid or expired invitation", http.StatusBadRequest)
		return
	}
	pw, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	var uid int64
	err = tx.QueryRow(r.Context(), `INSERT INTO users(email,password_hash,is_active) VALUES($1,$2,TRUE) ON CONFLICT(email) DO UPDATE SET password_hash=EXCLUDED.password_hash,is_active=TRUE RETURNING id`, inv.Email, string(pw)).Scan(&uid)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO portal_users(company_id,user_id,portal_type,customer_id,supplier_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,portal_type) DO UPDATE SET company_id=EXCLUDED.company_id,customer_id=EXCLUDED.customer_id,supplier_id=EXCLUDED.supplier_id,active=TRUE`, inv.CompanyID, uid, inv.PortalType, inv.CustomerID, inv.SupplierID)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE portal_invitations SET accepted_at=NOW() WHERE id=$1`, inv.ID); err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	portalJSON(w, map[string]any{"user_id": uid, "email": inv.Email, "portal_type": inv.PortalType})
}

func portalUser(r *http.Request) (int64, int64, bool) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0, 0, false
	}
	uid, err := strconv.ParseInt(sess.User(), 10, 64)
	if err != nil || uid <= 0 {
		return 0, 0, false
	}
	cid, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || cid <= 0 {
		return 0, 0, false
	}
	return uid, cid, true
}

func (h *Handler) access(r *http.Request, kind string) (int64, int64, int64, error) {
	uid, cid, ok := portalUser(r)
	if !ok {
		return 0, 0, 0, shared.ErrUnauthorized
	}
	var id int64
	query := `SELECT COALESCE(pu.customer_id,0) FROM portal_users pu JOIN customers c ON c.id=pu.customer_id AND c.company_id=pu.company_id WHERE pu.user_id=$1 AND pu.company_id=$2 AND pu.portal_type=$3 AND pu.active`
	if kind == "SUPPLIER" {
		query = `SELECT COALESCE(pu.supplier_id,0) FROM portal_users pu JOIN suppliers s ON s.id=pu.supplier_id AND s.company_id=pu.company_id WHERE pu.user_id=$1 AND pu.company_id=$2 AND pu.portal_type=$3 AND pu.active`
	}
	if kind == "EMPLOYEE" {
		query = `SELECT id FROM hr_employees WHERE user_id=$1 AND company_id=$2 AND status='ACTIVE'`
	}
	var row pgx.Row
	if kind == "EMPLOYEE" {
		row = h.pool.QueryRow(r.Context(), query, uid, cid)
	} else {
		row = h.pool.QueryRow(r.Context(), query, uid, cid, kind)
	}
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, 0, shared.ErrUnauthorized
		}
		return 0, 0, 0, err
	}
	return uid, cid, id, nil
}

func portalJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) customer(w http.ResponseWriter, r *http.Request) {
	_, cid, id, err := h.access(r, "CUSTOMER")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT i.number,i.total::float8,i.status FROM ar_invoices i JOIN customers c ON c.id=i.customer_id WHERE i.customer_id=$1 AND c.company_id=$2 ORDER BY i.id DESC LIMIT 100`, id, cid)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type invoice struct {
		Number string  `json:"number"`
		Total  float64 `json:"total"`
		Status string  `json:"status"`
	}
	invoices := []invoice{}
	for rows.Next() {
		var x invoice
		if err := rows.Scan(&x.Number, &x.Total, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		invoices = append(invoices, x)
	}
	portalJSON(w, map[string]any{"portal": "customer", "company_id": cid, "customer_id": id, "invoices": invoices, "internal_routes": "unavailable"})
}
func (h *Handler) supplier(w http.ResponseWriter, r *http.Request) {
	_, cid, id, err := h.access(r, "SUPPLIER")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT i.number,i.total::float8,i.status FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id WHERE i.supplier_id=$1 AND s.company_id=$2 ORDER BY i.id DESC LIMIT 100`, id, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type invoice struct {
		Number string  `json:"number"`
		Total  float64 `json:"total"`
		Status string  `json:"status"`
	}
	invoices := []invoice{}
	for rows.Next() {
		var x invoice
		if err := rows.Scan(&x.Number, &x.Total, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		invoices = append(invoices, x)
	}
	portalJSON(w, map[string]any{"portal": "supplier", "company_id": cid, "supplier_id": id, "invoices": invoices, "internal_routes": "unavailable"})
}
func (h *Handler) employee(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, err := h.access(r, "EMPLOYEE")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id,project_id,task_id,work_date::text,hours::float8,status FROM timesheets WHERE company_id=$1 AND employee_id=$2 ORDER BY work_date DESC LIMIT 100`, cid, uid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type sheet struct {
		ID, ProjectID, TaskID int64
		WorkDate              string `json:"work_date"`
		Hours                 float64
		Status                string
	}
	sheets := []sheet{}
	for rows.Next() {
		var x sheet
		if err := rows.Scan(&x.ID, &x.ProjectID, &x.TaskID, &x.WorkDate, &x.Hours, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		sheets = append(sheets, x)
	}
	portalJSON(w, map[string]any{"portal": "employee", "company_id": cid, "employee_id": uid, "timesheets": sheets, "internal_routes": "unavailable"})
}

func (h *Handler) customerOrders(w http.ResponseWriter, r *http.Request) {
	_, cid, customerID, err := h.access(r, "CUSTOMER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT o.id,o.doc_number,o.order_date::text,o.status::text,o.currency,o.total_amount,COALESCE(string_agg(d.status::text,',' ORDER BY d.id),'') FROM sales_orders o LEFT JOIN delivery_orders d ON d.sales_order_id=o.id WHERE o.company_id=$1 AND o.customer_id=$2 GROUP BY o.id ORDER BY o.id DESC LIMIT 100`, cid, customerID)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type order struct {
		ID                             int64 `json:"id"`
		Number, Date, Status, Currency string
		Total                          float64
		DeliveryStatus                 string `json:"delivery_status"`
	}
	items := []order{}
	for rows.Next() {
		var x order
		if err := rows.Scan(&x.ID, &x.Number, &x.Date, &x.Status, &x.Currency, &x.Total, &x.DeliveryStatus); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "customer", "orders": items, "internal_routes": "unavailable"})
}

func (h *Handler) customerPayments(w http.ResponseWriter, r *http.Request) {
	_, cid, customerID, err := h.access(r, "CUSTOMER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.number,p.amount,p.paid_at::text,p.method FROM ar_payments p JOIN ar_invoices i ON i.id=p.ar_invoice_id JOIN customers c ON c.id=i.customer_id WHERE i.customer_id=$1 AND c.company_id=$2 ORDER BY p.paid_at DESC LIMIT 100`, customerID, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type payment struct {
		Number, Date, Method string
		Amount               float64
	}
	items := []payment{}
	for rows.Next() {
		var x payment
		if err := rows.Scan(&x.Number, &x.Amount, &x.Date, &x.Method); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "customer", "payments": items, "internal_routes": "unavailable"})
}

func (h *Handler) employeePayslips(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, err := h.access(r, "EMPLOYEE")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.id,p.document_key,p.generated_at::text,l.net_pay FROM payroll_payslips p JOIN payroll_run_lines l ON l.id=p.run_line_id JOIN hr_employees e ON e.id=l.employee_id WHERE e.user_id=$1 AND e.company_id=$2 ORDER BY p.generated_at DESC LIMIT 100`, uid, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type slip struct {
		ID                       int64 `json:"id"`
		DocumentKey, GeneratedAt string
		NetPay                   float64
	}
	items := []slip{}
	for rows.Next() {
		var x slip
		if err := rows.Scan(&x.ID, &x.DocumentKey, &x.GeneratedAt, &x.NetPay); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "employee", "payslips": items, "internal_routes": "unavailable"})
}

func (h *Handler) employeeLeave(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, err := h.access(r, "EMPLOYEE")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT l.id,t.code,l.start_date::text,l.end_date::text,l.days,l.status FROM hr_leave_requests l JOIN hr_employees e ON e.id=l.employee_id JOIN hr_leave_types t ON t.id=l.leave_type_id WHERE e.user_id=$1 AND e.company_id=$2 ORDER BY l.start_date DESC LIMIT 100`, uid, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type leave struct {
		ID                       int64 `json:"id"`
		Type, Start, End, Status string
		Days                     float64
	}
	items := []leave{}
	for rows.Next() {
		var x leave
		if err := rows.Scan(&x.ID, &x.Type, &x.Start, &x.End, &x.Days, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "employee", "leave": items, "internal_routes": "unavailable"})
}

func (h *Handler) employeeAttendance(w http.ResponseWriter, r *http.Request) {
	uid, cid, _, err := h.access(r, "EMPLOYEE")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT a.attendance_date::text,a.check_in,a.check_out,a.status FROM hr_attendance a JOIN hr_employees e ON e.id=a.employee_id WHERE e.user_id=$1 AND e.company_id=$2 ORDER BY a.attendance_date DESC LIMIT 100`, uid, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type attendance struct {
		Date, Status      string
		CheckIn, CheckOut *time.Time
	}
	items := []attendance{}
	for rows.Next() {
		var x attendance
		if err := rows.Scan(&x.Date, &x.CheckIn, &x.CheckOut, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "employee", "attendance": items, "internal_routes": "unavailable"})
}

func (h *Handler) customerCreditNotes(w http.ResponseWriter, r *http.Request) {
	_, cid, customerID, err := h.access(r, "CUSTOMER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT n.number,n.total::float8,n.status FROM ar_credit_notes n JOIN customers c ON c.id=n.customer_id WHERE n.customer_id=$1 AND c.company_id=$2 ORDER BY n.id DESC LIMIT 100`, customerID, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type note struct {
		Number string  `json:"number"`
		Total  float64 `json:"total"`
		Status string  `json:"status"`
	}
	items := []note{}
	for rows.Next() {
		var x note
		if err := rows.Scan(&x.Number, &x.Total, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "customer", "credit_notes": items, "internal_routes": "unavailable"})
}

func (h *Handler) supplierOrders(w http.ResponseWriter, r *http.Request) {
	_, cid, supplierID, err := h.access(r, "SUPPLIER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT p.number,p.status,p.currency FROM pos p JOIN suppliers s ON s.id=p.supplier_id WHERE p.supplier_id=$1 AND s.company_id=$2 ORDER BY p.id DESC LIMIT 100`, supplierID, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type order struct{ Number, Status, Currency string }
	items := []order{}
	for rows.Next() {
		var x order
		if err := rows.Scan(&x.Number, &x.Status, &x.Currency); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "supplier", "orders": items, "internal_routes": "unavailable"})
}

func (h *Handler) supplierDeliveries(w http.ResponseWriter, r *http.Request) {
	_, cid, supplierID, err := h.access(r, "SUPPLIER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT g.number,g.status,g.received_at::text,g.warehouse_id FROM grns g JOIN suppliers s ON s.id=g.supplier_id WHERE g.supplier_id=$1 AND s.company_id=$2 ORDER BY g.id DESC LIMIT 100`, supplierID, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type delivery struct {
		Number, Status, ReceivedAt string
		WarehouseID                int64 `json:"warehouse_id"`
	}
	items := []delivery{}
	for rows.Next() {
		var x delivery
		if err := rows.Scan(&x.Number, &x.Status, &x.ReceivedAt, &x.WarehouseID); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "supplier", "deliveries": items, "internal_routes": "unavailable"})
}

func (h *Handler) uploadCustomerDocument(w http.ResponseWriter, r *http.Request) {
	h.uploadDocument(w, r, "CUSTOMER")
}
func (h *Handler) uploadSupplierDocument(w http.ResponseWriter, r *http.Request) {
	h.uploadDocument(w, r, "SUPPLIER")
}
func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request, kind string) {
	uid, cid, _, err := h.access(r, kind)
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "multipart form must be <= 10 MB", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if err != nil || len(content) > 10<<20 {
		http.Error(w, "file must be <= 10 MB", http.StatusBadRequest)
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if h.docs != nil {
		var classID int64
		_ = h.pool.QueryRow(r.Context(), `SELECT id FROM document_classifications WHERE company_id = $1 AND code = 'INTERNAL'`, cid).Scan(&classID)

		var catID int64
		err := h.pool.QueryRow(r.Context(), `SELECT id FROM document_categories WHERE company_id = $1 AND code = 'PORTAL'`, cid).Scan(&catID)
		if err != nil {
			_ = h.pool.QueryRow(r.Context(), `INSERT INTO document_categories (company_id, code, name, created_by) VALUES ($1, 'PORTAL', 'Portal Uploads', $2) ON CONFLICT DO NOTHING RETURNING id`, cid, uid).Scan(&catID)
			if catID == 0 {
				_ = h.pool.QueryRow(r.Context(), `SELECT id FROM document_categories WHERE company_id = $1 AND code = 'PORTAL'`, cid).Scan(&catID)
			}
		}

		docReq := documents.CreateDocumentRequest{
			CompanyID:        cid,
			Title:            header.Filename,
			Description:      "Uploaded via portal: " + kind,
			CategoryID:       catID,
			ClassificationID: classID,
			OwnerID:          uid,
			ActorID:          uid,
		}
		doc, err := h.docs.Create(r.Context(), docReq)
		if err != nil {
			shared.WriteErrorStatus(w, 500, err)
			return
		}

		verReq := documents.CreateVersionRequest{
			CompanyID:        cid,
			DocumentID:       doc.ID,
			ClassificationID: classID,
			Description:      "Initial upload",
			ActorID:          uid,
		}

		_, err = h.docs.UploadAndCreateVersion(r.Context(), bytes.NewReader(content), int64(len(content)), contentType, verReq)
		if err != nil {
			shared.WriteErrorStatus(w, 500, err)
			return
		}

		portalJSON(w, map[string]any{"id": doc.ID, "filename": header.Filename, "content_type": contentType})
		return
	}

	var id int64
	if err := h.pool.QueryRow(r.Context(), `INSERT INTO portal_documents(company_id,user_id,portal_type,filename,content_type,content) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, cid, uid, kind, header.Filename, contentType, content).Scan(&id); err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	portalJSON(w, map[string]any{"id": id, "filename": header.Filename, "content_type": contentType})
}

func (h *Handler) supplierDebitNotes(w http.ResponseWriter, r *http.Request) {
	_, cid, supplierID, err := h.access(r, "SUPPLIER")
	if err != nil {
		http.Error(w, http.StatusText(401), http.StatusUnauthorized)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT n.number,n.total::float8,n.status FROM ap_debit_notes n JOIN suppliers s ON s.id=n.supplier_id WHERE n.supplier_id=$1 AND s.company_id=$2 ORDER BY n.id DESC LIMIT 100`, supplierID, cid)
	if err != nil {
		http.Error(w, http.StatusText(500), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type note struct {
		Number string  `json:"number"`
		Total  float64 `json:"total"`
		Status string  `json:"status"`
	}
	items := []note{}
	for rows.Next() {
		var x note
		if err := rows.Scan(&x.Number, &x.Total, &x.Status); err != nil {
			http.Error(w, http.StatusText(500), http.StatusInternalServerError)
			return
		}
		items = append(items, x)
	}
	portalJSON(w, map[string]any{"portal": "supplier", "debit_notes": items, "internal_routes": "unavailable"})
}
