package shared

import "math"

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
