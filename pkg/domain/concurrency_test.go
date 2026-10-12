package domain_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"reward-platform/pkg/domain"
)

// TestRedeemRewardConcurrentIsSafe fires many goroutines at RedeemReward
// for the same single-unit reward at once, each with its own empty
// idempotency key (so each is a genuinely distinct attempt, not a replay
// of another). Safety now comes from RewardRepository.DecrementStock
// being atomic — the in-memory implementation gets that from a mutex
// scoped around its own Get-check-Put; the DynamoDB-backed one gets it
// from a conditional UpdateItem. Either way, this test is what would
// catch it if that atomicity broke: without it, two or more goroutines
// could each see Stock == 1 before either had decremented anything, and
// more than one redemption would succeed against one unit of stock.
//
// Run with: go test ./pkg/domain/... -race -run TestRedeemRewardConcurrent
// -race instruments every memory access and reports a failure the moment
// two goroutines touch the same memory unsynchronized, even on a run
// where the logic itself happens to still come out correct.
func TestRedeemRewardConcurrentIsSafe(t *testing.T) {
	e := domain.NewEngine(domain.SystemClock{})

	player, err := e.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	game, err := e.RegisterGame(ctx, "dice-run", 1000)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}
	if _, err := e.RecordSession(ctx, player.ID, game.ID); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}
	reward, err := e.RegisterReward(ctx, "limited-edition-skin", 100, 1) // only 1 in stock
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	const attempts = 50
	var successes atomic.Int64
	var outOfStock atomic.Int64

	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			_, err := e.RedeemReward(ctx, player.ID, reward.ID, "")
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, domain.ErrOutOfStock):
				outOfStock.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful redemptions = %d, want exactly 1", got)
	}
	if got := outOfStock.Load(); got != attempts-1 {
		t.Fatalf("ErrOutOfStock count = %d, want %d", got, attempts-1)
	}
}
