package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

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
		r.Use(h.rbac.RequireAny("pos.manage"))
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
		http.Error(w, "POS database is unavailable", http.StatusServiceUnavailable)
		return
	}
	_, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in struct {
		Code, Name  string
		WarehouseID *int64 `json:"warehouse_id"`
	}
	if !body(w, r, &in) || in.Code == "" || in.Name == "" {
		if in.Code == "" {
			http.Error(w, "code and name are required", http.StatusBadRequest)
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
		http.Error(w, "POS database is unavailable", http.StatusServiceUnavailable)
		return
	}
	uid, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	terminalID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid terminal id", http.StatusBadRequest)
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
		http.Error(w, "POS database is unavailable", http.StatusServiceUnavailable)
		return
	}
	uid, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
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
	return u, c, e == nil && c > 0
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if e := json.NewDecoder(r.Body).Decode(v); e != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid ticket id", http.StatusBadRequest)
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
			http.Error(w, "POS currency has no approved FX rate", http.StatusBadRequest)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid ticket id", http.StatusBadRequest)
		return
	}
	t, e := h.service.Refund(r.Context(), c, id)
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
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
			http.Error(w, "POS currency has no approved FX rate", http.StatusBadRequest)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
