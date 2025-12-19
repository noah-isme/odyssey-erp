// Package httpx provides HTTP response utilities following RFC7807 problem details.
package httpx

import (
	"encoding/json"
	"net/http"
)

// ProblemDetail represents RFC7807 problem details.
type ProblemDetail struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// JSON sends a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Problem sends an RFC7807 problem details response.
func Problem(w http.ResponseWriter, status int, title, detail string) {
	JSON(w, status, ProblemDetail{
		Title:  title,
		Status: status,
		Detail: detail,
	})
}

// DecodeJSON decodes JSON request body into the target struct.
func DecodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(r.Body).Decode(target)
}
