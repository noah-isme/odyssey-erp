package shared

import (
	"math"
	"net/http"
	"strconv"
)

// ParseLimitOffset reads limit/offset pagination parameters from the query
// string, falling back to defaultLimit and clamping to maxLimit. Listings whose
// pagination links emit these parameters must parse them, or every page beyond
// the first silently returns the same rows.
func ParseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return limit, offset
}

// Pagination contains metadata for paginated listings.
type Pagination struct {
	Page       int
	PerPage    int
	Total      int
	TotalPages int
}

// NewPagination computes pagination metadata.
func NewPagination(page, perPage, total int) Pagination {
	if perPage <= 0 {
		perPage = 20
	}
	if page <= 0 {
		page = 1
	}
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	return Pagination{Page: page, PerPage: perPage, Total: total, TotalPages: totalPages}
}
