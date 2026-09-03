// Package httpx provides HTTP response utilities.
package httpx

import (
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RespondError maps domain errors to RFC7807 using the shared classification
// and safe-message policy. This keeps platform and SSR/API handlers aligned.
func RespondError(w http.ResponseWriter, err error) {
	status := shared.HTTPStatus(err)
	detail := ""
	if status < http.StatusInternalServerError {
		detail = shared.UserSafeMessage(err)
	}
	Problem(w, status, http.StatusText(status), detail)
}
