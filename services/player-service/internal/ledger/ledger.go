// Package ledger is player-service's business logic: everything to do
// with Players and the points ledger. This is the narrower slice of what
// used to be pkg/domain's monolithic Engine — Game, Reward and
// Redemption logic now live in game-service and rewards-service
// instead, each with their own equivalent package. pkg/domain itself
// still has the original Engine (unchanged) for cmd/rewardctl's local,
// single-process demo/simulate commands; it's a deliberately-kept
// reference implementation, not what production runs.
package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	"reward-platform/mq"
	"reward-platform/pkg/domain"
)

// outboxAppender is satisfied by a LedgerRepository that can also write
// an OutboxEvent in the same transaction as the ledger entry — see
// domain.OutboxEvent's doc comment. The Postgres-backed implementation
// satisfies this (it has a real transaction to anchor the pattern to);
// the in-memory one doesn't, so New falls back to a thin adapter that
// just appends and drops the event — there's no relay process reading
// from memory anyway, so nothing would ever consume it.
type outboxAppender interface {
	AppendWithOutbox(ctx context.Context, entry domain.LedgerEntry, event domain.OutboxEvent) (created bool, err error)
}

type fallbackOutboxAppender struct{ domain.LedgerRepository }

func (f fallbackOutboxAppender) AppendWithOutbox(ctx context.Context, entry domain.LedgerEntry, _ domain.OutboxEvent) (bool, error) {
	return f.Append(ctx, entry)
}

// PlayerLedger owns player registration and every points movement.
type PlayerLedger struct {
	clock   domain.Clock
	ids     *domain.IDGenerator
	players domain.PlayerRepository
	ledger  domain.LedgerRepository
	outbox  outboxAppender
}

func New(clock domain.Clock, players domain.PlayerRepository, ledgerRepo domain.LedgerRepository) *PlayerLedger {
	outbox, ok := ledgerRepo.(outboxAppender)
	if !ok {
		outbox = fallbackOutboxAppender{ledgerRepo}
	}
	return &PlayerLedger{
		clock:   clock,
		ids:     domain.NewIDGenerator(),
		players: players,
		ledger:  ledgerRepo,
		outbox:  outbox,
	}
}

func (l *PlayerLedger) RegisterPlayer(ctx context.Context, username string) (domain.Player, error) {
	if username == "" {
		return domain.Player{}, &domain.ValidationError{Field: "username", Msg: "must not be empty"}
	}
	p := domain.Player{
		Model:    domain.Model{ID: l.ids.Next("player"), CreatedAt: l.clock.Now()},
		Username: username,
	}
	if err := l.players.Put(ctx, p); err != nil {
		return domain.Player{}, fmt.Errorf("register player: %w", err)
	}
	return p, nil
}

func (l *PlayerLedger) GetPlayer(ctx context.Context, id string) (domain.Player, error) {
	player, err := l.players.Get(ctx, id)
	if err != nil {
		return domain.Player{}, fmt.Errorf("get player: %w", err)
	}
	return player, nil
}

func (l *PlayerLedger) Players(ctx context.Context) ([]domain.Player, error) {
	players, err := l.players.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	return players, nil
}

// PlayersPage is the paginated, production-safe way to list players —
// see domain.PlayerRepository.ListPage's doc comment for why Players
// above (an unbounded List) isn't safe to expose over HTTP at any real
// scale. limit is clamped to a sane range here rather than trusted
// from the caller: an API consumer passing limit=0 or a huge number
// shouldn't be able to turn "paginated" back into "unbounded" by
// accident or on purpose.
func (l *PlayerLedger) PlayersPage(ctx context.Context, limit int, after *domain.PlayerPageCursor) ([]domain.Player, error) {
	const (
		defaultLimit = 50
		maxLimit     = 200
	)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	players, err := l.players.ListPage(ctx, limit, after)
	if err != nil {
		return nil, fmt.Errorf("list players page: %w", err)
	}
	return players, nil
}

func (l *PlayerLedger) Balance(ctx context.Context, playerID string) (int, error) {
	if _, err := l.players.Get(ctx, playerID); err != nil {
		return 0, fmt.Errorf("balance: %w", err)
	}
	balance, err := l.ledger.Balance(ctx, playerID)
	if err != nil {
		return 0, fmt.Errorf("balance: %w", err)
	}
	return balance, nil
}

// CreditPoints adds points to a player's balance, keyed by idempotencyKey
// so a redelivered SQS message (game-service's SessionCompleted event,
// in production) credits exactly once no matter how many times it's
// redelivered. A positive delta can never overdraw a balance, so unlike
// DebitPoints below, there's no sufficiency check.
//
// The credit and a PointsAwarded outbox event are written together via
// AppendWithOutbox — see domain.OutboxEvent's doc comment. A separate
// relay process (services/player-service/internal/outbox) is what
// actually publishes outbox rows to SQS afterward; this method's job
// ends at committing both writes together.
func (l *PlayerLedger) CreditPoints(ctx context.Context, playerID string, delta int, reason, idempotencyKey string) (domain.LedgerEntry, error) {
	if _, err := l.players.Get(ctx, playerID); err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("credit points: %w", err)
	}
	if idempotencyKey == "" {
		idempotencyKey = l.ids.Next("entry")
	}

	entry := domain.LedgerEntry{
		Model:    domain.Model{ID: idempotencyKey, CreatedAt: l.clock.Now()},
		PlayerID: playerID,
		Delta:    delta,
		Reason:   reason,
	}

	payload, err := json.Marshal(mq.PointsAwarded{PlayerID: playerID, Points: delta})
	if err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("credit points: marshal outbox event: %w", err)
	}
	event := domain.OutboxEvent{Type: "PointsAwarded", Payload: string(payload)}

	if _, err := l.outbox.AppendWithOutbox(ctx, entry, event); err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("credit points: %w", err)
	}
	return entry, nil
}

// DebitPoints removes points from a player's balance, failing with
// ErrInsufficientBalance if they don't have enough. It's the server side
// of the PlayerLedger gRPC service rewards-service calls during a
// redemption — idempotencyKey here is the same value rewards-service
// uses for the redemption itself, so a retried gRPC call (the caller
// timed out waiting, say) can't double-debit.
func (l *PlayerLedger) DebitPoints(ctx context.Context, playerID string, amount int, reason, idempotencyKey string) (int, error) {
	if _, err := l.players.Get(ctx, playerID); err != nil {
		return 0, fmt.Errorf("debit points: %w", err)
	}
	if idempotencyKey == "" {
		idempotencyKey = l.ids.Next("entry")
	}

	entry := domain.LedgerEntry{
		Model:    domain.Model{ID: idempotencyKey, CreatedAt: l.clock.Now()},
		PlayerID: playerID,
		Delta:    -amount,
		Reason:   reason,
	}
	newBalance, _, err := l.ledger.DebitIfSufficient(ctx, entry, amount)
	if err != nil {
		return 0, fmt.Errorf("debit points: %w", err)
	}
	return newBalance, nil
}
