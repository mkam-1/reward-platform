// Package grpcclient implements catalog.PlayerBalance by calling
// player-service's PlayerLedger gRPC service — the client-side mirror of
// player-service's internal/grpcapi.
package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"reward-platform/pkg/domain"

	playerv1 "reward-platform/proto/player/v1"
)

type PlayerBalanceClient struct {
	client playerv1.PlayerLedgerClient
}

func New(client playerv1.PlayerLedgerClient) *PlayerBalanceClient {
	return &PlayerBalanceClient{client: client}
}

func (c *PlayerBalanceClient) GetBalance(ctx context.Context, playerID string) (int, error) {
	resp, err := c.client.GetBalance(ctx, &playerv1.GetBalanceRequest{PlayerId: playerID})
	if err != nil {
		return 0, domainError(err)
	}
	return int(resp.GetBalance()), nil
}

func (c *PlayerBalanceClient) DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (int, error) {
	amount32, err := toInt32(amount)
	if err != nil {
		return 0, err
	}
	resp, err := c.client.DebitPoints(ctx, &playerv1.DebitPointsRequest{
		PlayerId:       playerID,
		Amount:         amount32,
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return 0, domainError(err)
	}
	return int(resp.GetNewBalance()), nil
}

// toInt32 guards the int -> int32 narrowing the proto wire format
// requires — the client-side mirror of grpcapi's same-named helper. A
// redemption cost north of ~2.1 billion points is nowhere near
// realistic, but silently wrapping to a negative amount if it ever
// happened would be a far worse bug than returning an honest error.
func toInt32(n int) (int32, error) {
	if n > math.MaxInt32 || n < math.MinInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", n)
	}
	return int32(n), nil
}

// domainError translates a gRPC status code back into the domain
// sentinel errors catalog.go's errors.Is checks already understand —
// the inverse of player-service's grpcapi.grpcError. Without this,
// errors.Is(err, domain.ErrInsufficientBalance) would never match a
// gRPC-originated error, since gRPC errors don't wrap domain errors by
// default; they're just status codes with a message string.
func domainError(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	switch st.Code() {
	case codes.NotFound:
		return fmt.Errorf("%s: %w", st.Message(), domain.ErrNotFound)
	case codes.FailedPrecondition:
		return fmt.Errorf("%s: %w", st.Message(), domain.ErrInsufficientBalance)
	default:
		return errors.New(st.Message())
	}
}
