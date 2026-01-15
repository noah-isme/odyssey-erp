package search

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SearchResult struct {
	Type     string `json:"type"`
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	URL      string `json:"url"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 {
		limit = 20
	}

	query = strings.TrimSpace(query)
	searchPattern := "%" + strings.ToLower(query) + "%"

	var results []SearchResult

	rows, err := s.pool.Query(ctx, `
		SELECT 'customer' AS type, id, name AS title, COALESCE(code, '') AS subtitle
		FROM customers
		WHERE LOWER(name) LIKE $1 OR LOWER(code) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = subtitle
				r.URL = "/sales/customers/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	rows, err = s.pool.Query(ctx, `
		SELECT 'sales_order' AS type, id, doc_number AS title, COALESCE(status, '') AS subtitle
		FROM sales_orders
		WHERE LOWER(doc_number) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = "Status: " + subtitle
				r.URL = "/sales/orders/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	rows, err = s.pool.Query(ctx, `
		SELECT 'quotation' AS type, id, doc_number AS title, COALESCE(status, '') AS subtitle
		FROM quotations
		WHERE LOWER(doc_number) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = "Status: " + subtitle
				r.URL = "/sales/quotations/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	rows, err = s.pool.Query(ctx, `
		SELECT 'purchase_order' AS type, id, number AS title, COALESCE(status, '') AS subtitle
		FROM pos
		WHERE LOWER(number) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = "Status: " + subtitle
				r.URL = "/procurement/pos/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	rows, err = s.pool.Query(ctx, `
		SELECT 'product' AS type, id, name AS title, COALESCE(sku, '') AS subtitle
		FROM products
		WHERE LOWER(name) LIKE $1 OR LOWER(sku) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = "SKU: " + subtitle
				r.URL = "/masterdata/products/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	rows, err = s.pool.Query(ctx, `
		SELECT 'supplier' AS type, id, name AS title, COALESCE(code, '') AS subtitle
		FROM suppliers
		WHERE LOWER(name) LIKE $1 OR LOWER(code) LIKE $1
		LIMIT 5
	`, searchPattern)
	if err == nil {
		for rows.Next() {
			var r SearchResult
			var subtitle string
			if err := rows.Scan(&r.Type, &r.ID, &r.Title, &subtitle); err == nil {
				r.Subtitle = subtitle
				r.URL = "/masterdata/suppliers/" + itoa(r.ID)
				results = append(results, r)
			}
		}
		rows.Close()
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func itoa(i int64) string {
	return fmt.Sprintf("%d", i)
}

type Handler struct {
	logger  *slog.Logger
	service *Service
}

func NewHandler(logger *slog.Logger, service *Service) *Handler {
	return &Handler{logger: logger, service: service}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/api/search", h.handleSearch)
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	results, err := h.service.Search(r.Context(), query, 20)
	if err != nil {
		h.logger.Error("search failed", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
