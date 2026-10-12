package resilience_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"reward-platform/pkg/domain"
	"reward-platform/services/rewards-service/internal/resilience"
)

var ctx = context.Background()

// fakeCaller fails the first `failCount` calls (with err) then succeeds,
// returning balance. Safe for concurrent use since CircuitBreaker guards
// its own state with a mutex but calls through to inner without one.
type fakeCaller struct {
	mu        sync.Mutex
	failCount int
	err       error
	calls     int
	balance   int
}

func (f *fakeCaller) GetBalance(_ context.Context, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failCount {
		return 0, f.err
	}
	return f.balance, nil
}

func (f *fakeCaller) DebitPoints(_ context.Context, _ string, _ int, _, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failCount {
		return 0, f.err
	}
	return f.balance, nil
}

func (f *fakeCaller) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

var errTransient = errors.New("connection reset")

func TestRetryingRetriesTransientErrors(t *testing.T) {
	inner := &fakeCaller{failCount: 2, err: errTransient, balance: 42}
	r := resilience.NewRetrying(inner, resilience.WithMaxAttempts(5), resilience.WithBaseDelay(time.Millisecond), resilience.WithMaxDelay(time.Millisecond))

	balance, err := r.GetBalance(ctx, "player_1")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance != 42 {
		t.Fatalf("balance = %d, want 42", balance)
	}
	if inner.callCount() != 3 {
		t.Fatalf("calls = %d, want 3 (2 failures + 1 success)", inner.callCount())
	}
}

func TestRetryingDoesNotRetryBusinessErrors(t *testing.T) {
	inner := &fakeCaller{failCount: 100, err: domain.ErrInsufficientBalance}
	r := resilience.NewRetrying(inner, resilience.WithMaxAttempts(5), resilience.WithBaseDelay(time.Millisecond))

	_, err := r.GetBalance(ctx, "player_1")
	if !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("error = %v, want ErrInsufficientBalance", err)
	}
	if inner.callCount() != 1 {
		t.Fatalf("calls = %d, want 1 (no retry for a business error)", inner.callCount())
	}
}

func TestRetryingExhaustsAttempts(t *testing.T) {
	inner := &fakeCaller{failCount: 100, err: errTransient}
	r := resilience.NewRetrying(inner, resilience.WithMaxAttempts(3), resilience.WithBaseDelay(time.Millisecond), resilience.WithMaxDelay(time.Millisecond))

	_, err := r.GetBalance(ctx, "player_1")
	if !errors.Is(err, errTransient) {
		t.Fatalf("error = %v, want errTransient", err)
	}
	if inner.callCount() != 3 {
		t.Fatalf("calls = %d, want 3 (maxAttempts)", inner.callCount())
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	inner := &fakeCaller{failCount: 100, err: errTransient}
	clock := newFakeClock()
	cb := resilience.NewCircuitBreaker(inner, resilience.WithFailureThreshold(3), resilience.WithCooldown(time.Minute), resilience.WithClock(clock))

	for i := 0; i < 3; i++ {
		if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, errTransient) {
			t.Fatalf("call %d: error = %v, want errTransient", i, err)
		}
	}
	if inner.callCount() != 3 {
		t.Fatalf("calls = %d, want 3", inner.callCount())
	}

	// The breaker should now be open: the 4th call must fail fast
	// (ErrCircuitOpen) without reaching the inner caller at all.
	if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("error = %v, want ErrCircuitOpen", err)
	}
	if inner.callCount() != 3 {
		t.Fatalf("calls = %d, want still 3 (breaker should short-circuit)", inner.callCount())
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	inner := &fakeCaller{failCount: 3, err: errTransient, balance: 10}
	clock := newFakeClock()
	cb := resilience.NewCircuitBreaker(inner, resilience.WithFailureThreshold(3), resilience.WithCooldown(time.Minute), resilience.WithClock(clock))

	for i := 0; i < 3; i++ {
		cb.GetBalance(ctx, "player_1")
	}
	// Confirm it's open.
	if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("error = %v, want ErrCircuitOpen", err)
	}

	clock.Advance(2 * time.Minute) // past cooldown

	// Half-open trial: inner's 4th call succeeds (failCount=3), so this
	// should close the breaker.
	balance, err := cb.GetBalance(ctx, "player_1")
	if err != nil {
		t.Fatalf("half-open trial: %v", err)
	}
	if balance != 10 {
		t.Fatalf("balance = %d, want 10", balance)
	}

	// Breaker should be closed again now: a subsequent failure needs the
	// full threshold again, not just one, to reopen.
	inner.mu.Lock()
	inner.failCount = inner.calls + 1 // fail exactly the next call only
	inner.mu.Unlock()
	if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, errTransient) {
		t.Fatalf("error = %v, want errTransient", err)
	}
	if _, err := cb.GetBalance(ctx, "player_1"); errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatal("breaker reopened after a single failure post-recovery, want it to still be closed")
	}
}

func TestCircuitBreakerHalfOpenFailureReopensImmediately(t *testing.T) {
	inner := &fakeCaller{failCount: 100, err: errTransient}
	clock := newFakeClock()
	cb := resilience.NewCircuitBreaker(inner, resilience.WithFailureThreshold(3), resilience.WithCooldown(time.Minute), resilience.WithClock(clock))

	for i := 0; i < 3; i++ {
		cb.GetBalance(ctx, "player_1")
	}
	clock.Advance(2 * time.Minute)

	// Half-open trial fails (inner still always fails) — must reopen
	// immediately, not wait for the full threshold again.
	if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, errTransient) {
		t.Fatalf("trial error = %v, want errTransient", err)
	}
	if _, err := cb.GetBalance(ctx, "player_1"); !errors.Is(err, resilience.ErrCircuitOpen) {
		t.Fatalf("error = %v, want ErrCircuitOpen (should have reopened after one half-open failure)", err)
	}
}
