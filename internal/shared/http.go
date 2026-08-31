package shared

import (
	"encoding/json"
	"net/http"
)

// WriteHTTPError emits the standard plain-text response used by non-template endpoints.
// Template handlers should surface the same safe message through their form error state.
func WriteHTTPError(w http.ResponseWriter, status int, message string) {
	if message == "" {
		message = http.StatusText(status)
	}
	http.Error(w, message, status)
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
	_ = json.NewEncoder(w).Encode(data)
}

// JSONError writes a JSON error response with the given status code.
func JSONError(w http.ResponseWriter, status int, message string) {
	JSONResponse(w, status, map[string]string{"error": message})
}
