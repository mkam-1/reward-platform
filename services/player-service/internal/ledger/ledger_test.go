package ledger_test

import (
	"context"
	"errors"
	"testing"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/ledger"
)

var ctx = context.Background()

func newTestLedger() *ledger.PlayerLedger {
	return ledger.New(domain.SystemClock{}, domain.NewMemoryPlayerRepository(), domain.NewMemoryLedgerRepository())
}

func TestRegisterPlayer(t *testing.T) {
	l := newTestLedger()

	p, err := l.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	if p.Username != "alice" || p.ID == "" {
		t.Fatalf("player = %+v, want non-empty ID and Username=alice", p)
	}

	var verr *domain.ValidationError
	if _, err := l.RegisterPlayer(ctx, ""); !errors.As(err, &verr) {
		t.Fatalf("empty username error = %v, want *domain.ValidationError", err)
	}
}

func TestCreditPointsIsIdempotent(t *testing.T) {
	l := newTestLedger()
	player, err := l.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}

	const key = "session-event-1"
	if _, err := l.CreditPoints(ctx, player.ID, 15, "played dice-run", key); err != nil {
		t.Fatalf("first CreditPoints: %v", err)
	}
	// Simulates the same SQS message being redelivered.
	if _, err := l.CreditPoints(ctx, player.ID, 15, "played dice-run", key); err != nil {
		t.Fatalf("redelivered CreditPoints: %v", err)
	}

	balance, err := l.Balance(ctx, player.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 15 {
		t.Fatalf("balance = %d, want 15 (credited once, not twice)", balance)
	}
}

func TestDebitPointsInsufficientBalance(t *testing.T) {
	l := newTestLedger()
	player, err := l.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}

	if _, err := l.DebitPoints(ctx, player.ID, 30, "redeemed sticker-pack", ""); !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("error = %v, want ErrInsufficientBalance", err)
	}
}

func TestDebitPointsIsIdempotent(t *testing.T) {
	l := newTestLedger()
	player, err := l.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	if _, err := l.CreditPoints(ctx, player.ID, 50, "played dice-run", ""); err != nil {
		t.Fatalf("CreditPoints: %v", err)
	}

	const key = "redemption-1"
	first, err := l.DebitPoints(ctx, player.ID, 30, "redeemed sticker-pack", key)
	if err != nil {
		t.Fatalf("first DebitPoints: %v", err)
	}
	if first != 20 {
		t.Fatalf("first new balance = %d, want 20", first)
	}

	// Simulates rewards-service retrying a gRPC call it never saw the
	// response for.
	second, err := l.DebitPoints(ctx, player.ID, 30, "redeemed sticker-pack", key)
	if err != nil {
		t.Fatalf("retried DebitPoints: %v", err)
	}
	if second != 20 {
		t.Fatalf("retried new balance = %d, want 20 (debited once, not twice)", second)
	}
}
