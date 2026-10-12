package catalog_test

import (
	"testing"

	"go.uber.org/mock/gomock"

	"reward-platform/pkg/domain"
	"reward-platform/services/rewards-service/internal/catalog"
	"reward-platform/services/rewards-service/internal/catalog/mocks"
)

// catalog_test.go's fakePlayerBalance (a hand-written struct simulating
// real balance behavior) is what the rest of this package's tests use,
// deliberately — for "does RedeemReward compute the right outcome,"
// a fake that actually behaves like a balance is simpler to read than a
// wall of .EXPECT() calls. A generated mock earns its keep for a
// different question: not "what does it return" but "what did we call,
// with what arguments, in what order" — gomock.InOrder below is the
// concrete case a hand-written fake would need its own extra bookkeeping
// (a recorded call log, manually compared) to express at all.
func TestRedeemRewardCallsGetBalanceBeforeDebitPoints(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBalance := mocks.NewMockPlayerBalance(ctrl)

	rewards := domain.NewMemoryRewardRepository()
	redemptions := domain.NewMemoryRedemptionRepository()
	c := catalog.New(domain.SystemClock{}, rewards, redemptions, mockBalance)

	reward, err := c.RegisterReward(ctx, "sticker-pack", 30, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	// gomock.InOrder pins the exact sequence: a GetBalance call that
	// arrives after DebitPoints (e.g. a future refactor that
	// accidentally reorders the checks) fails this test immediately,
	// with gomock naming exactly which expectation was violated — not
	// something a return-value-only fake would catch at all, since both
	// orders would still produce the same final answer.
	gomock.InOrder(
		mockBalance.EXPECT().
			GetBalance(gomock.Any(), "player_1").
			Return(50, nil),
		mockBalance.EXPECT().
			DebitPoints(gomock.Any(), "player_1", 30, gomock.Any(), gomock.Any()).
			Return(20, nil),
	)

	redemption, err := c.RedeemReward(ctx, "player_1", reward.ID, "")
	if err != nil {
		t.Fatalf("RedeemReward: %v", err)
	}
	if redemption.PointsSpent != 30 {
		t.Fatalf("PointsSpent = %d, want 30", redemption.PointsSpent)
	}
	// ctrl.Finish() runs automatically via t.Cleanup (gomock.NewController
	// registers it) — it's what actually fails the test if GetBalance or
	// DebitPoints was expected but never called.
}

func TestRedeemRewardDoesNotDebitWhenBalanceCheckFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBalance := mocks.NewMockPlayerBalance(ctrl)

	rewards := domain.NewMemoryRewardRepository()
	redemptions := domain.NewMemoryRedemptionRepository()
	c := catalog.New(domain.SystemClock{}, rewards, redemptions, mockBalance)

	reward, err := c.RegisterReward(ctx, "console", 1000, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	// DebitPoints has no .EXPECT() at all here — if RedeemReward called
	// it anyway (a real bug: debiting despite an insufficient-balance
	// verdict), gomock fails the test on that unexpected call, not just
	// on the final returned error.
	mockBalance.EXPECT().
		GetBalance(gomock.Any(), "player_1").
		Return(10, nil)

	_, err = c.RedeemReward(ctx, "player_1", reward.ID, "")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}
