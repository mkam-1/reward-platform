// Package grpcapi implements the generated PlayerLedgerServer interface
// on top of internal/ledger — the gRPC equivalent of what internal/api
// is for HTTP. Same separation of concerns: this package translates
// between the wire format (protobuf messages, gRPC status codes) and
// domain types/errors; it contains no business logic of its own.
package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/ledger"

	playerv1 "reward-platform/proto/player/v1"
)

type Server struct {
	playerv1.UnimplementedPlayerLedgerServer
	ledger *ledger.PlayerLedger
}

func NewServer(l *ledger.PlayerLedger) *Server {
	return &Server{ledger: l}
}

func (s *Server) GetBalance(ctx context.Context, req *playerv1.GetBalanceRequest) (*playerv1.GetBalanceResponse, error) {
	balance, err := s.ledger.Balance(ctx, req.GetPlayerId())
	if err != nil {
		return nil, grpcError(err)
	}
	balance32, err := toInt32(balance)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &playerv1.GetBalanceResponse{Balance: balance32}, nil
}

func (s *Server) DebitPoints(ctx context.Context, req *playerv1.DebitPointsRequest) (*playerv1.DebitPointsResponse, error) {
	newBalance, err := s.ledger.DebitPoints(ctx, req.GetPlayerId(), int(req.GetAmount()), req.GetReason(), req.GetIdempotencyKey())
	if err != nil {
		return nil, grpcError(err)
	}
	newBalance32, err := toInt32(newBalance)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &playerv1.DebitPointsResponse{NewBalance: newBalance32}, nil
}

// toInt32 guards the int -> int32 narrowing the proto wire format
// requires (balances are plain Go int, 64-bit on this platform). A
// balance north of ~2.1 billion points is nowhere near realistic for
// this domain today, but silently wrapping to a negative or nonsense
// number if it ever happened would be a far worse bug than returning an
// honest error — gosec's G115 flagged the bare conversion this replaces.
func toInt32(n int) (int32, error) {
	if n > math.MaxInt32 || n < math.MinInt32 {
		return 0, fmt.Errorf("value %d out of int32 range", n)
	}
	return int32(n), nil
}

// grpcError maps a domain error to the gRPC status code that best
// represents it — the gRPC-side equivalent of internal/api/errors.go's
// statusFor, same reasoning: pkg/domain has no notion of any transport,
// HTTP or gRPC, so each transport layer does its own translation.
func grpcError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
