package catalog_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"reward-platform/pkg/domain"
	"reward-platform/services/rewards-service/internal/catalog"
)

var ctx = context.Background()

// fakePlayerBalance stands in for a gRPC call to player-service — these
// tests are about RedeemReward's own orchestration logic (idempotency,
// stock, ordering), not about gRPC itself, which grpcclient has no logic
// of its own to test beyond "translate this call shape to that one."
type fakePlayerBalance struct {
	mu       sync.Mutex
	balances map[string]int
	debits   int
}

func newFakePlayerBalance(balances map[string]int) *fakePlayerBalance {
	return &fakePlayerBalance{balances: balances}
}

func (f *fakePlayerBalance) GetBalance(_ context.Context, playerID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balances[playerID], nil
}

func (f *fakePlayerBalance) DebitPoints(_ context.Context, playerID string, amount int, _, _ string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.balances[playerID] < amount {
		return 0, domain.ErrInsufficientBalance
	}
	f.balances[playerID] -= amount
	f.debits++
	return f.balances[playerID], nil
}

func newTestCatalog(balances map[string]int) (*catalog.RewardCatalog, *fakePlayerBalance) {
	pb := newFakePlayerBalance(balances)
	c := catalog.New(domain.SystemClock{}, domain.NewMemoryRewardRepository(), domain.NewMemoryRedemptionRepository(), pb)
	return c, pb
}

func TestRegisterAndListRewards(t *testing.T) {
	c, _ := newTestCatalog(nil)

	reward, err := c.RegisterReward(ctx, "sticker-pack", 30, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	rewards, err := c.Rewards(ctx)
	if err != nil {
		t.Fatalf("Rewards: %v", err)
	}
	if len(rewards) != 1 || rewards[0].ID != reward.ID {
		t.Fatalf("rewards = %+v, want just %+v", rewards, reward)
	}
}

func TestRedeemReward(t *testing.T) {
	c, pb := newTestCatalog(map[string]int{"player_1": 50})
	reward, err := c.RegisterReward(ctx, "sticker-pack", 30, 1)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	redemption, err := c.RedeemReward(ctx, "player_1", reward.ID, "")
	if err != nil {
		t.Fatalf("RedeemReward: %v", err)
	}
	if redemption.PointsSpent != 30 {
		t.Fatalf("PointsSpent = %d, want 30", redemption.PointsSpent)
	}
	if pb.debits != 1 {
		t.Fatalf("debits = %d, want 1", pb.debits)
	}

	// Stock was 1; a second, distinct redemption must fail out of stock.
	if _, err := c.RedeemReward(ctx, "player_1", reward.ID, ""); !errors.Is(err, domain.ErrOutOfStock) {
		t.Fatalf("second redemption error = %v, want ErrOutOfStock", err)
	}
}

func TestRedeemRewardIdempotentReplayDoesNotDebitTwice(t *testing.T) {
	c, pb := newTestCatalog(map[string]int{"player_1": 50})
	reward, err := c.RegisterReward(ctx, "sticker-pack", 30, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	const key = "redeem-key-1"
	first, err := c.RedeemReward(ctx, "player_1", reward.ID, key)
	if err != nil {
		t.Fatalf("first RedeemReward: %v", err)
	}
	second, err := c.RedeemReward(ctx, "player_1", reward.ID, key)
	if err != nil {
		t.Fatalf("replayed RedeemReward: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay ID = %q, want %q", second.ID, first.ID)
	}
	if pb.debits != 1 {
		t.Fatalf("debits = %d, want 1 (replay must not re-debit)", pb.debits)
	}
}

func TestRedeemRewardInsufficientBalance(t *testing.T) {
	c, _ := newTestCatalog(map[string]int{"player_1": 10})
	reward, err := c.RegisterReward(ctx, "console", 1000, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	if _, err := c.RedeemReward(ctx, "player_1", reward.ID, ""); !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("error = %v, want ErrInsufficientBalance", err)
	}
}
