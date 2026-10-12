// Package catalog is game-service's business logic: Game registration
// and recording play sessions. Recording a session no longer credits
// points directly (that would mean reaching into player-service's own
// database, exactly the kind of cross-service coupling splitting into
// services is supposed to eliminate) — instead it publishes a
// SessionCompleted event for player-service to consume asynchronously.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"reward-platform/mq"
	"reward-platform/pkg/domain"
)

// Publisher is the one capability GameCatalog needs from messaging —
// just enough to describe what it uses, not a dependency on the mq
// package's full Client type. *mq.Client satisfies this interface
// structurally, with no explicit declaration linking the two.
type Publisher interface {
	Publish(ctx context.Context, queueURL, body string) error
}

type GameCatalog struct {
	clock     domain.Clock
	ids       *domain.IDGenerator
	games     *domain.Store[domain.Game]
	publisher Publisher
	queueURL  string
}

func New(clock domain.Clock, games *domain.Store[domain.Game], publisher Publisher, sessionCompletedQueueURL string) *GameCatalog {
	return &GameCatalog{
		clock:     clock,
		ids:       domain.NewIDGenerator(),
		games:     games,
		publisher: publisher,
		queueURL:  sessionCompletedQueueURL,
	}
}

func (c *GameCatalog) RegisterGame(ctx context.Context, name string, pointsPerSession int) (domain.Game, error) {
	if name == "" {
		return domain.Game{}, &domain.ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if pointsPerSession <= 0 {
		return domain.Game{}, &domain.ValidationError{Field: "pointsPerSession", Msg: "must be positive"}
	}
	g := domain.Game{
		Model:            domain.Model{ID: c.ids.Next("game"), CreatedAt: c.clock.Now()},
		Name:             name,
		PointsPerSession: pointsPerSession,
	}
	c.games.Put(ctx, g.ID, g)
	return g, nil
}

func (c *GameCatalog) GetGame(ctx context.Context, id string) (domain.Game, error) {
	return c.games.Get(ctx, id)
}

func (c *GameCatalog) Games(ctx context.Context) []domain.Game {
	return c.games.List(ctx)
}

// RecordSession looks up the game, computes the points it's worth, and
// publishes a SessionCompleted event — it does not touch a player's
// balance at all; that happens asynchronously once player-service
// consumes the event. idempotencyKey ties this call to the resulting
// ledger entry player-service eventually writes; if empty, one is
// generated (fine for a one-shot call, not retry-safe).
func (c *GameCatalog) RecordSession(ctx context.Context, playerID, gameID, idempotencyKey string) error {
	game, err := c.games.Get(ctx, gameID)
	if err != nil {
		return fmt.Errorf("record session: %w", err)
	}
	if idempotencyKey == "" {
		idempotencyKey = c.ids.Next("session")
	}

	event := mq.SessionCompleted{
		PlayerID:       playerID,
		GameID:         gameID,
		Points:         game.PointsPerSession,
		Reason:         fmt.Sprintf("played %s", game.Name),
		IdempotencyKey: idempotencyKey,
	}
	bodyBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("record session: marshal event: %w", err)
	}
	body := string(bodyBytes)

	if err := c.publisher.Publish(ctx, c.queueURL, body); err != nil {
		return fmt.Errorf("record session: publish: %w", err)
	}
	return nil
}
