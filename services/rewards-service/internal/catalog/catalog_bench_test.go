package catalog_test

import (
	"context"
	"testing"
	"time"

	"reward-platform/pkg/domain"
	"reward-platform/services/rewards-service/internal/catalog"
)

// slowRewardRepository wraps the in-memory repository with an
// artificial delay on List, standing in for a real DynamoDB Scan's
// network round-trip — the in-memory fake alone responds in
// nanoseconds, which would make the cache's benefit invisible in a
// benchmark even though it's real against an actual table.
type slowRewardRepository struct {
	domain.RewardRepository
	listDelay time.Duration
}

func (s *slowRewardRepository) List(ctx context.Context) ([]domain.Reward, error) {
	time.Sleep(s.listDelay)
	return s.RewardRepository.List(ctx)
}

// BenchmarkRewards_Uncached simulates every call missing the cache —
// what Rewards() cost before the TTL cache was added in Phase 14.
func BenchmarkRewards_Uncached(b *testing.B) {
	repo := &slowRewardRepository{RewardRepository: domain.NewMemoryRewardRepository(), listDelay: 2 * time.Millisecond}
	c := catalog.New(domain.SystemClock{}, repo, domain.NewMemoryRedemptionRepository(), newFakePlayerBalance(nil))
	_, _ = c.RegisterReward(ctx, "sticker-pack", 10, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each iteration waits out catalogCacheTTL so every call is a
		// genuine miss — reproducing "no cache" behavior on top of code
		// that now has one, rather than editing the cache out.
		time.Sleep(6 * time.Millisecond)
		if _, err := c.Rewards(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRewards_Cached is the Phase 14 state: repeated calls inside
// the TTL window hit the in-memory cache instead of the repository.
func BenchmarkRewards_Cached(b *testing.B) {
	repo := &slowRewardRepository{RewardRepository: domain.NewMemoryRewardRepository(), listDelay: 2 * time.Millisecond}
	c := catalog.New(domain.SystemClock{}, repo, domain.NewMemoryRedemptionRepository(), newFakePlayerBalance(nil))
	_, _ = c.RegisterReward(ctx, "sticker-pack", 10, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Rewards(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
