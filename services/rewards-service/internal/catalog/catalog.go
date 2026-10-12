// Package catalog is rewards-service's business logic: Reward
// registration and redemption. This is pkg/domain's old
// Engine.RedeemReward, restructured for a world where the player's
// balance isn't a local repository call anymore but a network call to
// another service — see PlayerBalance below.
package catalog

import (
	"context"
	"errors"
	"fmt"

	"reward-platform/pkg/domain"
)

// PlayerBalance is the one capability RewardCatalog needs from
// player-service — just enough to describe what it uses, not a
// dependency on gRPC itself. internal/grpcclient implements this by
// wrapping the generated PlayerLedgerClient; a test can implement it
// with an in-memory fake instead, with no gRPC involved at all.
type PlayerBalance interface {
	GetBalance(ctx context.Context, playerID string) (int, error)
	DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (newBalance int, err error)
}

type RewardCatalog struct {
	clock         domain.Clock
	ids           *domain.IDGenerator
	rewards       domain.RewardRepository
	redemptions   domain.RedemptionRepository
	playerBalance PlayerBalance
}

func New(clock domain.Clock, rewards domain.RewardRepository, redemptions domain.RedemptionRepository, playerBalance PlayerBalance) *RewardCatalog {
	return &RewardCatalog{
		clock:         clock,
		ids:           domain.NewIDGenerator(),
		rewards:       rewards,
		redemptions:   redemptions,
		playerBalance: playerBalance,
	}
}

func (c *RewardCatalog) RegisterReward(ctx context.Context, name string, costPoints, stock int) (domain.Reward, error) {
	if name == "" {
		return domain.Reward{}, &domain.ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if costPoints <= 0 {
		return domain.Reward{}, &domain.ValidationError{Field: "costPoints", Msg: "must be positive"}
	}
	if stock < 0 {
		return domain.Reward{}, &domain.ValidationError{Field: "stock", Msg: "must not be negative"}
	}
	r := domain.Reward{
		Model:      domain.Model{ID: c.ids.Next("reward"), CreatedAt: c.clock.Now()},
		Name:       name,
		CostPoints: costPoints,
		Stock:      stock,
	}
	if err := c.rewards.Put(ctx, r); err != nil {
		return domain.Reward{}, fmt.Errorf("register reward: %w", err)
	}
	return r, nil
}

func (c *RewardCatalog) Rewards(ctx context.Context) ([]domain.Reward, error) {
	rewards, err := c.rewards.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rewards: %w", err)
	}
	return rewards, nil
}

// RedeemReward mirrors the shape of pkg/domain's original
// Engine.RedeemReward closely (idempotency check first, fast-path
// stock/balance pre-checks, atomic authoritative operations last) — the
// one structural difference is that GetBalance/DebitPoints are now gRPC
// calls to player-service via c.playerBalance instead of local
// repository calls. Everything said in Engine's doc comment about what
// is and isn't closed by that atomicity still applies here.
func (c *RewardCatalog) RedeemReward(ctx context.Context, playerID, rewardID, idempotencyKey string) (domain.Redemption, error) {
	if idempotencyKey == "" {
		idempotencyKey = c.ids.Next("redemption")
	}

	if existing, err := c.redemptions.Get(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return domain.Redemption{}, fmt.Errorf("redeem: idempotency check: %w", err)
	}

	reward, err := c.rewards.Get(ctx, rewardID)
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if reward.Stock <= 0 {
		return domain.Redemption{}, fmt.Errorf("redeem: reward %q: %w", reward.Name, domain.ErrOutOfStock)
	}

	balance, err := c.playerBalance.GetBalance(ctx, playerID)
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if balance < reward.CostPoints {
		return domain.Redemption{}, fmt.Errorf("redeem: have %d, need %d: %w", balance, reward.CostPoints, domain.ErrInsufficientBalance)
	}

	redemption := domain.Redemption{
		Model:       domain.Model{ID: idempotencyKey, CreatedAt: c.clock.Now()},
		PlayerID:    playerID,
		RewardID:    rewardID,
		PointsSpent: reward.CostPoints,
	}

	created, err := c.redemptions.Create(ctx, redemption)
	if err != nil {
		return domain.Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if !created {
		existing, err := c.redemptions.Get(ctx, idempotencyKey)
		if err != nil {
			return domain.Redemption{}, fmt.Errorf("redeem: idempotent replay: %w", err)
		}
		return existing, nil
	}

	if err := c.rewards.DecrementStock(ctx, rewardID); err != nil {
		return domain.Redemption{}, fmt.Errorf("redeem: %w", err)
	}

	// Same idempotencyKey as the redemption record itself — player-
	// service's DebitPoints uses it the same way: a retried gRPC call
	// (this process crashed and got redeployed mid-call, say) reuses
	// the key and player-service recognizes the replay instead of
	// debiting twice.
	reason := fmt.Sprintf("redeemed %s", reward.Name)
	if _, err := c.playerBalance.DebitPoints(ctx, playerID, reward.CostPoints, reason, idempotencyKey); err != nil {
		return domain.Redemption{}, fmt.Errorf("redeem: debit: %w", err)
	}

	return redemption, nil
}

func (c *RewardCatalog) ListRedemptions(ctx context.Context, playerID string) ([]domain.Redemption, error) {
	redemptions, err := c.redemptions.ListByPlayer(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}
	return redemptions, nil
}
