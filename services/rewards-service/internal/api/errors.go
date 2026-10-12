package api

import (
	"errors"
	"net/http"

	"reward-platform/pkg/domain"
)

func statusFor(err error) int {
	var verr *domain.ValidationError

	switch {
	case errors.As(err, &verr):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrInsufficientBalance), errors.Is(err, domain.ErrOutOfStock):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
