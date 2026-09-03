package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPStatusClassifiesWrappedDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "unauthorized", err: fmt.Errorf("login: %w", ErrUnauthorized), want: http.StatusUnauthorized},
		{name: "forbidden", err: fmt.Errorf("check: %w", ErrForbidden), want: http.StatusForbidden},
		{name: "not found", err: ErrNotFound, want: http.StatusNotFound},
		{name: "conflict", err: ErrConflict, want: http.StatusConflict},
		{name: "validation", err: ErrValidation, want: http.StatusBadRequest},
		{name: "unknown", err: errors.New("database password leaked"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPStatus(tt.err); got != tt.want {
				t.Fatalf("HTTPStatus() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRespondJSONErrorDoesNotLeakUnknownError(t *testing.T) {
	w := httptest.NewRecorder()
	RespondJSONError(w, errors.New("pq: password=secret"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] == "" || body["error"] == "pq: password=secret" {
		t.Fatalf("unsafe error body: %#v", body)
	}
}
