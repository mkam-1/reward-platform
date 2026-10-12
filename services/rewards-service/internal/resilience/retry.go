// Package resilience wraps a flaky network dependency with the two
// patterns that make it safe to depend on anyway: retry-with-backoff for
// individual transient failures, and a circuit breaker for when the
// dependency is down hard enough that retrying is just wasted time.
//
// Both are implemented as decorators — wrapping something that already
// satisfies the caller interface, adding behavior around it, and still
// satisfying the same interface themselves — exactly the same pattern
// services/*/internal/api/middleware.go already used for the HTTP
// request/logging/recovery chain back in phase 3, just applied to an
// outbound call instead of an inbound one. They compose:
// NewCircuitBreaker(NewRetrying(raw)) wraps twice, in that specific
// order — see circuitbreaker.go's doc comment for why the order matters.
package resilience

import (
	"context"
	"errors"
	"time"

	"reward-platform/pkg/domain"
)

// caller is the shape this package wraps — defined locally rather than
// importing rewards-service's catalog.PlayerBalance, so this package
// stays reusable for any similarly-shaped dependency, not coupled to
// this one caller.
type caller interface {
	GetBalance(ctx context.Context, playerID string) (int, error)
	DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (int, error)
}

// isRetryable distinguishes "the call failed because of something
// transient" from "the call failed because the answer is genuinely no."
// Retrying ErrInsufficientBalance or ErrNotFound wouldn't change the
// outcome — it's not a flaky network, it's a correct, final answer — so
// only unrecognized (infrastructure-shaped) errors get retried.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInsufficientBalance) {
		return false
	}
	return true
}

type retryOptions struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

type RetryOption func(*retryOptions)

func WithMaxAttempts(n int) RetryOption { return func(o *retryOptions) { o.maxAttempts = n } }
func WithBaseDelay(d time.Duration) RetryOption { return func(o *retryOptions) { o.baseDelay = d } }
func WithMaxDelay(d time.Duration) RetryOption   { return func(o *retryOptions) { o.maxDelay = d } }

// Retrying retries a transient failure with exponential backoff
// (baseDelay, doubling, capped at maxDelay) up to maxAttempts times.
type Retrying struct {
	inner caller
	opts  retryOptions
}

func NewRetrying(inner caller, opts ...RetryOption) *Retrying {
	o := retryOptions{maxAttempts: 3, baseDelay: 100 * time.Millisecond, maxDelay: 2 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}
	return &Retrying{inner: inner, opts: o}
}

func (r *Retrying) call(ctx context.Context, attempt func() (int, error)) (int, error) {
	var lastErr error
	delay := r.opts.baseDelay

	for i := 0; i < r.opts.maxAttempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			if delay > r.opts.maxDelay {
				delay = r.opts.maxDelay
			}
		}

		result, err := attempt()
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return 0, err
		}
	}
	return 0, lastErr
}

func (r *Retrying) GetBalance(ctx context.Context, playerID string) (int, error) {
	return r.call(ctx, func() (int, error) { return r.inner.GetBalance(ctx, playerID) })
}

func (r *Retrying) DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (int, error) {
	return r.call(ctx, func() (int, error) {
		return r.inner.DebitPoints(ctx, playerID, amount, reason, idempotencyKey)
	})
}
