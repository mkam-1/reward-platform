package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers compare against these with errors.Is, even when
// the error returned by an Engine method has been wrapped with extra
// context via fmt.Errorf's %w verb.
var (
	ErrNotFound            = errors.New("not found")
	ErrInsufficientBalance = errors.New("insufficient points balance")
	ErrOutOfStock          = errors.New("reward out of stock")
)

// ValidationError reports that a caller-supplied field failed validation.
// Unlike the sentinel errors above, callers recover the offending field
// with errors.As rather than errors.Is, since the field name varies.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Msg)
}
