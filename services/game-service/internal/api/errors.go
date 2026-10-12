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
	default:
		return http.StatusInternalServerError
	}
}
