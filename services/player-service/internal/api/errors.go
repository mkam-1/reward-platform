package api

import (
	"errors"
	"net/http"

	"reward-platform/pkg/domain"
)

// statusFor maps a domain error to the HTTP status code that best
// represents it. pkg/domain has no notion of HTTP — it just returns Go
// errors — so this is the one place in the whole service that
// translation happens, keeping the domain layer transport-agnostic.
func statusFor(err error) int {
	var verr *domain.ValidationError

	switch {
	case errors.As(err, &verr):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrInsufficientBalance):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
