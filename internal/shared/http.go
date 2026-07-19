package shared

import "net/http"

// WriteHTTPError emits the standard plain-text response used by non-template endpoints.
// Template handlers should surface the same safe message through their form error state.
func WriteHTTPError(w http.ResponseWriter, status int, message string) {
	if message == "" {
		message = http.StatusText(status)
	}
	http.Error(w, message, status)
}
