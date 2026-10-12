package resilience

import (
	"context"
	"errors"
	"sync"
	"time"

	"reward-platform/pkg/domain"
)

// ErrCircuitOpen is returned immediately, without even attempting the
// call, while the breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker open: dependency considered unhealthy")

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

type breakerOptions struct {
	failureThreshold int
	cooldown         time.Duration
	clock            domain.Clock
}

type CircuitBreakerOption func(*breakerOptions)

func WithFailureThreshold(n int) CircuitBreakerOption {
	return func(o *breakerOptions) { o.failureThreshold = n }
}
func WithCooldown(d time.Duration) CircuitBreakerOption {
	return func(o *breakerOptions) { o.cooldown = d }
}

// WithClock overrides the real clock — tests use this to control cooldown
// expiry deterministically instead of actually sleeping.
func WithClock(c domain.Clock) CircuitBreakerOption {
	return func(o *breakerOptions) { o.clock = c }
}

// CircuitBreaker wraps a caller with the classic three-state breaker:
//
//   - Closed: calls pass through normally. Each failure is counted;
//     failureThreshold consecutive failures opens the circuit.
//   - Open: calls fail immediately with ErrCircuitOpen, without even
//     attempting the inner call — the whole point being to stop sending
//     load at a dependency that's already shown it's down, rather than
//     queuing up slow failures behind it. After cooldown elapses, the
//     next call transitions to Half-Open instead of failing fast.
//   - Half-Open: exactly one trial call is let through. Success closes
//     the circuit (back to normal); failure reopens it (cooldown restarts).
//
// This wraps Retrying, not the other way around — CircuitBreaker(
// Retrying(raw)) — so the breaker's failure count reflects "did this
// logical call ultimately fail," one increment per call, not one per
// retry attempt inside it. Counting every retry attempt as a separate
// failure would trip the breaker open from a single slow-but-recovering
// call's own retries, which isn't the failure mode this is meant to
// catch.
type CircuitBreaker struct {
	inner caller
	opts  breakerOptions

	mu               sync.Mutex
	state            circuitState
	consecutiveFails int
	openedAt         time.Time
}

func NewCircuitBreaker(inner caller, opts ...CircuitBreakerOption) *CircuitBreaker {
	o := breakerOptions{failureThreshold: 5, cooldown: 30 * time.Second, clock: domain.SystemClock{}}
	for _, opt := range opts {
		opt(&o)
	}
	return &CircuitBreaker{inner: inner, opts: o, state: stateClosed}
}

// allow decides whether a call may proceed, transitioning Open->Half-Open
// as a side effect once cooldown has elapsed.
func (cb *CircuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state != stateOpen {
		return nil
	}
	if cb.opts.clock.Now().Sub(cb.openedAt) < cb.opts.cooldown {
		return ErrCircuitOpen
	}
	cb.state = stateHalfOpen
	return nil
}

func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.consecutiveFails = 0
		cb.state = stateClosed
		return
	}

	cb.consecutiveFails++
	// A Half-Open trial failing reopens immediately, regardless of the
	// configured threshold — one failed trial is enough evidence the
	// dependency isn't actually recovered yet.
	if cb.state == stateHalfOpen || cb.consecutiveFails >= cb.opts.failureThreshold {
		cb.state = stateOpen
		cb.openedAt = cb.opts.clock.Now()
	}
}

func (cb *CircuitBreaker) GetBalance(ctx context.Context, playerID string) (int, error) {
	if err := cb.allow(); err != nil {
		return 0, err
	}
	result, err := cb.inner.GetBalance(ctx, playerID)
	cb.recordResult(err)
	return result, err
}

func (cb *CircuitBreaker) DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (int, error) {
	if err := cb.allow(); err != nil {
		return 0, err
	}
	result, err := cb.inner.DebitPoints(ctx, playerID, amount, reason, idempotencyKey)
	cb.recordResult(err)
	return result, err
}
