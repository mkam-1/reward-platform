// Package grpcclient implements catalog.PlayerBalance by calling
// player-service's PlayerLedger gRPC service — the client-side mirror of
// player-service's internal/grpcapi.
package grpcclient

import (
	"context"
	"errors"
	"fmt"

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
	resp, err := c.client.DebitPoints(ctx, &playerv1.DebitPointsRequest{
		PlayerId:       playerID,
		Amount:         int32(amount),
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return 0, domainError(err)
	}
	return int(resp.GetNewBalance()), nil
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
