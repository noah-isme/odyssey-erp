package shared

import (
	"encoding/json"
	"errors"
	"net/http"
)

// HTTPStatus classifies a domain error at the HTTP boundary. The mapping is
// kept here so handlers do not each implement a slightly different switch.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrInvalidCredentials):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrCSRFTokenMissing), errors.Is(err, ErrCSRFTokenMismatch):
		return http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrDuplicate), errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrValidation), errors.Is(err, ErrInvalidInput):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// WriteHTTPError emits the standard plain-text response used by non-template endpoints.
// Template handlers should surface the same safe message through their form error state.
func WriteHTTPError(w http.ResponseWriter, status int, message string) {
	if message == "" {
		message = http.StatusText(status)
	}
	http.Error(w, message, status)
}

// WriteError writes a safe plain-text response with a status inferred from
// the domain error. Unknown errors never expose their internal message.
func WriteError(w http.ResponseWriter, err error) {
	WriteHTTPError(w, HTTPStatus(err), UserSafeMessage(err))
}

// WriteErrorStatus preserves a handler's explicit status while centralizing
// safe error-message translation.
func WriteErrorStatus(w http.ResponseWriter, status int, err error) {
	WriteHTTPError(w, status, UserSafeMessage(err))
}

// DecodeJSON decodes the JSON body of the request into v.
func DecodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// JSONResponse writes a JSON response with the given status code.
func JSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Log error in a real app
	}
}

// JSONError writes a JSON error response with the given status code.
func JSONError(w http.ResponseWriter, status int, message string) {
	JSONResponse(w, status, map[string]string{"error": message})
}

// JSONErrorFrom writes a JSON error using the shared safe-message policy.
func JSONErrorFrom(w http.ResponseWriter, status int, err error) {
	JSONError(w, status, UserSafeMessage(err))
}

// RespondJSONError infers the HTTP status and safe response message from a
// domain error. Use JSONErrorFrom when the endpoint has an explicit status
// contract for a particular operation.
func RespondJSONError(w http.ResponseWriter, err error) {
	JSONErrorFrom(w, HTTPStatus(err), err)
}
