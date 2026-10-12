package grpcapi_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/grpcapi"
	"reward-platform/services/player-service/internal/ledger"

	playerv1 "reward-platform/proto/player/v1"
)

var ctx = context.Background()

// newTestServer returns both the gRPC-facing Server and the
// ledger.PlayerLedger underneath it — Server only exposes GetBalance and
// DebitPoints (CreditPoints has no gRPC endpoint; it only happens via
// the SQS consumer in real usage, see main.go), so tests that need to
// set up a starting balance call CreditPoints directly on the ledger,
// then exercise the server built on top of that same instance.
func newTestServer() (*grpcapi.Server, *ledger.PlayerLedger, domain.Player) {
	playerLedger := ledger.New(domain.SystemClock{}, domain.NewMemoryPlayerRepository(), domain.NewMemoryLedgerRepository())
	player, err := playerLedger.RegisterPlayer(ctx, "alice")
	if err != nil {
		panic(err)
	}
	return grpcapi.NewServer(playerLedger), playerLedger, player
}

func TestGetBalance(t *testing.T) {
	server, _, player := newTestServer()

	resp, err := server.GetBalance(ctx, &playerv1.GetBalanceRequest{PlayerId: player.ID})
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if resp.GetBalance() != 0 {
		t.Fatalf("balance = %d, want 0", resp.GetBalance())
	}
}

func TestGetBalanceUnknownPlayerMapsToNotFound(t *testing.T) {
	server, _, _ := newTestServer()

	_, err := server.GetBalance(ctx, &playerv1.GetBalanceRequest{PlayerId: "does-not-exist"})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", st.Code())
	}
}

func TestDebitPointsSufficientBalance(t *testing.T) {
	server, playerLedger, player := newTestServer()

	if _, err := playerLedger.CreditPoints(ctx, player.ID, 50, "played dice-run", ""); err != nil {
		t.Fatalf("CreditPoints: %v", err)
	}

	resp, err := server.DebitPoints(ctx, &playerv1.DebitPointsRequest{
		PlayerId: player.ID,
		Amount:   30,
		Reason:   "redeemed sticker-pack",
	})
	if err != nil {
		t.Fatalf("DebitPoints: %v", err)
	}
	if resp.GetNewBalance() != 20 {
		t.Fatalf("new balance = %d, want 20", resp.GetNewBalance())
	}
}

func TestDebitPointsInsufficientBalanceMapsToFailedPrecondition(t *testing.T) {
	server, _, player := newTestServer()

	_, err := server.DebitPoints(ctx, &playerv1.DebitPointsRequest{
		PlayerId: player.ID,
		Amount:   30,
		Reason:   "redeemed sticker-pack",
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", st.Code())
	}
}
