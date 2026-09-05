package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
	rbac    rbac.Middleware
	pool    *pgxpool.Pool
	stock   *inventory.Service
	ledger  AccountingPort
}

type SalePostedEvent struct {
	TicketID, CompanyID, ActorID int64
	Amount, BaseAmount           float64
	Currency, BaseCurrency       string
}

type AccountingPort interface {
	HandlePOSSalePosted(context.Context, SalePostedEvent) error
	HandlePOSRefunded(context.Context, SalePostedEvent) error
}

func (h *Handler) SetInventoryService(stock *inventory.Service) { h.stock = stock }
func (h *Handler) SetAccountingService(ledger AccountingPort)   { h.ledger = ledger }

func NewHandler(service *Service, middleware rbac.Middleware, pools ...*pgxpool.Pool) *Handler {
	var pool *pgxpool.Pool
	if len(pools) > 0 {
		pool = pools[0]
	}
	return &Handler{service: service, rbac: middleware, pool: pool}
}
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("pos.manage", "pos.view"))
		r.Get("/catalog", h.catalog)
		r.Get("/tickets/recent", h.recentTickets)

		r.Post("/terminals", h.createTerminal)
		r.Post("/terminals/{id}/open", h.openSession)
		r.Post("/sessions/{id}/close", h.closeSession)

		// Advanced POS
		r.Post("/hardware", h.createHardware)
		r.Post("/loyalty", h.createLoyaltyMember)
		r.Post("/giftcards", h.createGiftCard)

		r.Post("/tickets", h.createTicket)
		r.Post("/tickets/{id}/payments", h.payment)
		r.Post("/tickets/{id}/refund", h.refund)
	})
}

func (h *Handler) createTerminal(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "POS database is unavailable", 503)
		return
	}
	_, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Code, Name  string
		WarehouseID *int64 `json:"warehouse_id"`
	}
	if !body(w, r, &in) || in.Code == "" || in.Name == "" {
		if in.Code == "" {
			http.Error(w, "code and name are required", 400)
		}
		return
	}
	var response struct {
		ID         int64 `json:"id"`
		Code, Name string
		Active     bool
	}
	err := h.pool.QueryRow(r.Context(), `INSERT INTO pos_terminals(company_id,code,name,warehouse_id) SELECT $1,$2,$3,w.id FROM warehouses w WHERE ($4::bigint IS NULL OR w.id=$4) AND ($4::bigint IS NULL OR w.company_id=$1) RETURNING id,code,name,active`, companyID, in.Code, in.Name, in.WarehouseID).Scan(&response.ID, &response.Code, &response.Name, &response.Active)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	out(w, 201, response)
}

func (h *Handler) openSession(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "POS database is unavailable", 503)
		return
	}
	uid, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	terminalID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid terminal id", 400)
		return
	}
	var in struct {
		OpeningFloat float64 `json:"opening_float"`
	}
	if !body(w, r, &in) {
		return
	}
	var response struct {
		ID, TerminalID, CashierID int64
		OpeningFloat              float64
		Status                    string
	}
	err = h.pool.QueryRow(r.Context(), `INSERT INTO pos_sessions(company_id,terminal_id,cashier_id,opening_float) SELECT $1,id,$2,$3 FROM pos_terminals WHERE id=$4 AND company_id=$1 AND active RETURNING id,terminal_id,cashier_id,opening_float,status`, companyID, uid, in.OpeningFloat, terminalID).Scan(&response.ID, &response.TerminalID, &response.CashierID, &response.OpeningFloat, &response.Status)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	out(w, 201, response)
}

func (h *Handler) closeSession(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "POS database is unavailable", 503)
		return
	}
	uid, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid session id", 400)
		return
	}
	var in struct {
		ClosingAmount float64 `json:"closing_amount"`
	}
	if !body(w, r, &in) {
		return
	}
	var response struct {
		ID                      int64 `json:"id"`
		ClosingAmount, Variance float64
		Status                  string
	}
	err = h.pool.QueryRow(r.Context(), `WITH expected AS (SELECT s.id,s.opening_float+COALESCE(SUM(CASE WHEN p.method='CASH' THEN p.amount ELSE 0 END),0) expected_amount FROM pos_sessions s LEFT JOIN pos_tickets t ON t.session_id=s.id AND t.company_id=$1 LEFT JOIN pos_payments p ON p.ticket_id=t.id WHERE s.id=$2 AND s.company_id=$1 AND s.cashier_id=$3 AND s.status='OPEN' GROUP BY s.id) UPDATE pos_sessions s SET closing_amount=$4,variance=$4-e.expected_amount,status='CLOSED',closed_at=NOW() FROM expected e WHERE s.id=e.id RETURNING s.id,s.closing_amount,s.variance,s.status`, companyID, sessionID, uid, in.ClosingAmount).Scan(&response.ID, &response.ClosingAmount, &response.Variance, &response.Status)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	out(w, 200, response)
}
func ids(r *http.Request) (int64, int64, bool) {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0, 0, false
	}
	u, e := strconv.ParseInt(s.User(), 10, 64)
	if e != nil || u < 1 {
		return 0, 0, false
	}
	c, e := strconv.ParseInt(s.Get("company_id"), 10, 64)
	if e != nil || c < 1 {
		c = 1
	}
	return u, c, true
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if e := json.NewDecoder(r.Body).Decode(v); e != nil {
		http.Error(w, "invalid JSON", 400)
		return false
	}
	return true
}
func out(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in Ticket
	if !body(w, r, &in) {
		return
	}
	in.CompanyID = c
	in.ID = u
	t, e := h.service.CreateTicket(r.Context(), in)
	if e != nil {
		shared.WriteErrorStatus(w, 400, e)
		return
	}
	out(w, 201, t)
}
func (h *Handler) payment(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid ticket id", 400)
		return
	}
	var p Payment
	if !body(w, r, &p) {
		return
	}
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	p.ID = u
	p.TicketID = id
	p2, e := h.service.AddPayment(r.Context(), c, id, p)
	if e != nil {
		shared.WriteErrorStatus(w, 400, e)
		return
	}
	if h.stock != nil && h.pool != nil {
		var status string
		if err := h.pool.QueryRow(r.Context(), `SELECT status FROM pos_tickets WHERE id=$1 AND company_id=$2`, id, c).Scan(&status); err == nil && status == "COMPLETED" {
			var warehouse int64
			_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(t.warehouse_id,0) FROM pos_sessions s JOIN pos_terminals t ON t.id=s.terminal_id WHERE s.id=(SELECT session_id FROM pos_tickets WHERE id=$1)`, id).Scan(&warehouse)
			if warehouse > 0 {
				rows, qerr := h.pool.Query(r.Context(), `SELECT id,product_id,quantity::float8 FROM pos_ticket_lines WHERE ticket_id=$1 ORDER BY id`, id)
				if qerr == nil {
					defer rows.Close()
					for rows.Next() {
						var lineID, productID int64
						var qty float64
						if rows.Scan(&lineID, &productID, &qty) == nil {
							if _, qerr = h.stock.PostAdjustment(r.Context(), inventory.AdjustmentInput{Code: fmt.Sprintf("POS-SALE-%d-%d", id, lineID), WarehouseID: warehouse, ProductID: productID, Qty: -qty, Note: "POS sale", ActorID: u, RefModule: "POS"}); qerr != nil {
								shared.WriteErrorStatus(w, 400, qerr)
								return
							}
						}
					}
				}
			}
		}
	}
	if h.ledger != nil && h.pool != nil {
		var amount, baseAmount, rate float64
		var currency, baseCurrency, status string
		err := h.pool.QueryRow(r.Context(), `SELECT t.total::float8,t.currency,c.base_currency,CASE WHEN t.currency=c.base_currency THEN 1 ELSE fx.rate END,t.status FROM pos_tickets t JOIN companies c ON c.id=t.company_id LEFT JOIN LATERAL (SELECT rate FROM fx_daily_rates WHERE base_currency=c.base_currency AND quote_currency=t.currency AND rate_date<=CURRENT_DATE ORDER BY rate_date DESC LIMIT 1) fx ON TRUE WHERE t.id=$1 AND t.company_id=$2`, id, c).Scan(&amount, &currency, &baseCurrency, &rate, &status)
		if err != nil {
			shared.WriteErrorStatus(w, 400, err)
			return
		}
		if status != "COMPLETED" {
			out(w, 200, p2)
			return
		}
		if rate <= 0 {
			http.Error(w, "POS currency has no approved FX rate", 400)
			return
		}
		baseAmount = amount * rate
		if err := h.ledger.HandlePOSSalePosted(r.Context(), SalePostedEvent{TicketID: id, CompanyID: c, ActorID: u, Amount: amount, BaseAmount: baseAmount, Currency: currency, BaseCurrency: baseCurrency}); err != nil {
			shared.WriteErrorStatus(w, 400, err)
			return
		}
	}
	out(w, 200, p2)
}
func (h *Handler) refund(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid ticket id", 400)
		return
	}
	t, e := h.service.Refund(r.Context(), c, id)
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", 404)
		return
	}
	if e != nil {
		shared.WriteErrorStatus(w, 400, e)
		return
	}
	if h.stock != nil && h.pool != nil {
		var warehouse int64
		_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(t.warehouse_id,0) FROM pos_sessions s JOIN pos_terminals t ON t.id=s.terminal_id WHERE s.id=(SELECT session_id FROM pos_tickets WHERE id=$1)`, id).Scan(&warehouse)
		if warehouse > 0 {
			rows, qerr := h.pool.Query(r.Context(), `SELECT id,product_id,quantity::float8 FROM pos_ticket_lines WHERE ticket_id=$1 ORDER BY id`, id)
			if qerr == nil {
				defer rows.Close()
				for rows.Next() {
					var lineID, productID int64
					var qty float64
					if rows.Scan(&lineID, &productID, &qty) == nil {
						if qerr = func() error {
							_, err := h.stock.PostAdjustment(r.Context(), inventory.AdjustmentInput{Code: fmt.Sprintf("POS-REFUND-%d-%d", id, lineID), WarehouseID: warehouse, ProductID: productID, Qty: qty, Note: "POS refund", ActorID: u, RefModule: "POS"})
							return err
						}(); qerr != nil {
							shared.WriteErrorStatus(w, 400, qerr)
							return
						}
					}
				}
			}
		}
	}
	if h.ledger != nil && h.pool != nil {
		var amount, rate float64
		var currency, baseCurrency string
		if err := h.pool.QueryRow(r.Context(), `SELECT t.total::float8,t.currency,c.base_currency,CASE WHEN t.currency=c.base_currency THEN 1 ELSE fx.rate END FROM pos_tickets t JOIN companies c ON c.id=t.company_id LEFT JOIN LATERAL (SELECT rate FROM fx_daily_rates WHERE base_currency=c.base_currency AND quote_currency=t.currency AND rate_date<=CURRENT_DATE ORDER BY rate_date DESC LIMIT 1) fx ON TRUE WHERE t.id=$1 AND t.company_id=$2`, id, c).Scan(&amount, &currency, &baseCurrency, &rate); err != nil {
			shared.WriteErrorStatus(w, 400, err)
			return
		}
		if rate <= 0 {
			http.Error(w, "POS currency has no approved FX rate", 400)
			return
		}
		if err := h.ledger.HandlePOSRefunded(r.Context(), SalePostedEvent{TicketID: id, CompanyID: c, ActorID: u, Amount: amount, BaseAmount: amount * rate, Currency: currency, BaseCurrency: baseCurrency}); err != nil {
			shared.WriteErrorStatus(w, 400, err)
			return
		}
	}
	out(w, 200, t)
}

// ============================================================================
// Advanced POS
// ============================================================================

func (h *Handler) createHardware(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in POSHardware
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	_ = cid // POSHardware doesn't have CompanyID currently
	created, err := h.service.CreatePOSHardware(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	shared.JSONResponse(w, 201, created)
}

func (h *Handler) createLoyaltyMember(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in LoyaltyMember
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	in.CompanyID = cid
	created, err := h.service.CreateLoyaltyMember(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	shared.JSONResponse(w, 201, created)
}

func (h *Handler) createGiftCard(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in GiftCard
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	in.CompanyID = cid
	created, err := h.service.CreateGiftCard(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, 500, err)
		return
	}
	shared.JSONResponse(w, 201, created)
}

type CatalogProduct struct {
	ID           int64   `json:"id"`
	SKU          string  `json:"sku"`
	Name         string  `json:"name"`
	CategoryID   int64   `json:"category_id"`
	CategoryName string  `json:"category_name"`
	PriceCents   int64   `json:"price_cents"`
	Price        float64 `json:"price"`
	Unit         string  `json:"unit"`
	Barcode      string  `json:"barcode"`
}

type CatalogCategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TerminalInfo struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type SessionInfo struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type CashierInfo struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CompanyInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type CatalogResponse struct {
	Terminal   TerminalInfo      `json:"terminal"`
	Session    SessionInfo       `json:"session"`
	Cashier    CashierInfo       `json:"cashier"`
	Company    CompanyInfo       `json:"company"`
	Currency   string            `json:"currency"`
	Categories []CatalogCategory `json:"categories"`
	Products   []CatalogProduct  `json:"products"`
}

type RecentTicket struct {
	ID            int64     `json:"id"`
	Number        string    `json:"number"`
	Currency      string    `json:"currency"`
	SubtotalCents int64     `json:"subtotal_cents"`
	TaxCents      int64     `json:"tax_cents"`
	TotalCents    int64     `json:"total_cents"`
	PaidCents     int64     `json:"paid_cents"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

func (h *Handler) catalog(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "POS database unavailable", http.StatusServiceUnavailable)
		return
	}
	uid, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var companyName, currency string
	err := h.pool.QueryRow(r.Context(), `SELECT name, COALESCE(base_currency, 'IDR') FROM companies WHERE id = $1`, companyID).Scan(&companyName, &currency)
	if err != nil {
		companyName = "Odyssey ERP"
		currency = "IDR"
	}

	var cashierName, cashierEmail string
	_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(NULLIF(name, ''), email), email FROM users WHERE id = $1`, uid).Scan(&cashierName, &cashierEmail)
	if cashierName == "" {
		cashierName = "Cashier"
	}

	var terminalID int64
	var terminalCode, terminalName string
	err = h.pool.QueryRow(r.Context(), `SELECT id, code, name FROM pos_terminals WHERE company_id = $1 AND active = TRUE ORDER BY id ASC LIMIT 1`, companyID).Scan(&terminalID, &terminalCode, &terminalName)
	if err != nil {
		_ = h.pool.QueryRow(r.Context(), `INSERT INTO pos_terminals(company_id, code, name, active) VALUES($1, 'POS-01', 'Kasir Utama', TRUE) RETURNING id, code, name`, companyID).Scan(&terminalID, &terminalCode, &terminalName)
	}

	var sessionID int64
	var sessionStatus string
	err = h.pool.QueryRow(r.Context(), `SELECT id, status FROM pos_sessions WHERE company_id = $1 AND terminal_id = $2 AND status = 'OPEN' ORDER BY id DESC LIMIT 1`, companyID, terminalID).Scan(&sessionID, &sessionStatus)
	if err != nil && terminalID > 0 {
		_ = h.pool.QueryRow(r.Context(), `INSERT INTO pos_sessions(company_id, terminal_id, cashier_id, opening_float, status) VALUES($1, $2, $3, 0, 'OPEN') RETURNING id, status`, companyID, terminalID, uid).Scan(&sessionID, &sessionStatus)
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT p.id, p.sku, p.name, COALESCE(p.category_id, 0), COALESCE(c.name, 'General'),
		       (p.price * 100)::bigint, p.price::float8, COALESCE(u.name, 'pcs'),
		       COALESCE((SELECT barcode FROM wms_barcode_aliases WHERE product_id = p.id AND company_id = $1 LIMIT 1), p.sku)
		FROM products p
		LEFT JOIN categories c ON c.id = p.category_id
		LEFT JOIN units u ON u.id = p.unit_id
		WHERE p.is_active = TRUE AND p.deleted_at IS NULL
		ORDER BY p.name ASC
	`, companyID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	products := make([]CatalogProduct, 0)
	catMap := make(map[int64]string)

	for rows.Next() {
		var prod CatalogProduct
		if err := rows.Scan(&prod.ID, &prod.SKU, &prod.Name, &prod.CategoryID, &prod.CategoryName, &prod.PriceCents, &prod.Price, &prod.Unit, &prod.Barcode); err == nil {
			products = append(products, prod)
			if prod.CategoryID > 0 {
				catMap[prod.CategoryID] = prod.CategoryName
			}
		}
	}

	categories := make([]CatalogCategory, 0, len(catMap))
	for id, name := range catMap {
		categories = append(categories, CatalogCategory{ID: id, Name: name})
	}

	resp := CatalogResponse{
		Terminal:   TerminalInfo{ID: terminalID, Code: terminalCode, Name: terminalName},
		Session:    SessionInfo{ID: sessionID, Status: sessionStatus},
		Cashier:    CashierInfo{ID: uid, Name: cashierName, Email: cashierEmail},
		Company:    CompanyInfo{ID: companyID, Name: companyName},
		Currency:   currency,
		Categories: categories,
		Products:   products,
	}
	out(w, http.StatusOK, resp)
}

func (h *Handler) recentTickets(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "POS database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT t.id, t.number, t.currency, (t.subtotal * 100)::bigint, (t.tax_amount * 100)::bigint,
		       (t.total * 100)::bigint, COALESCE((SELECT SUM(amount*100) FROM pos_payments WHERE ticket_id = t.id), 0)::bigint,
		       t.status, t.created_at
		FROM pos_tickets t
		WHERE t.company_id = $1
		ORDER BY t.id DESC
		LIMIT 10
	`, companyID)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()

	tickets := make([]RecentTicket, 0)
	for rows.Next() {
		var t RecentTicket
		if err := rows.Scan(&t.ID, &t.Number, &t.Currency, &t.SubtotalCents, &t.TaxCents, &t.TotalCents, &t.PaidCents, &t.Status, &t.CreatedAt); err == nil {
			tickets = append(tickets, t)
		}
	}
	out(w, http.StatusOK, tickets)
}
