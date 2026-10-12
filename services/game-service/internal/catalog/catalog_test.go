package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"reward-platform/pkg/domain"
	"reward-platform/services/game-service/internal/catalog"
)

var ctx = context.Background()

type fakePublisher struct {
	mu        sync.Mutex
	published []string
	err       error
}

func (p *fakePublisher) Publish(_ context.Context, _ string, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, body)
	return nil
}

func newTestCatalog() (*catalog.GameCatalog, *fakePublisher) {
	pub := &fakePublisher{}
	return catalog.New(domain.SystemClock{}, domain.NewStore[domain.Game](), pub, "queue-url"), pub
}

func TestRegisterGameValidation(t *testing.T) {
	tests := []struct {
		name             string
		gameName         string
		pointsPerSession int
		wantErr          bool
	}{
		{name: "valid", gameName: "dice-run", pointsPerSession: 15, wantErr: false},
		{name: "empty name", gameName: "", pointsPerSession: 15, wantErr: true},
		{name: "zero points", gameName: "dice-run", pointsPerSession: 0, wantErr: true},
		{name: "negative points", gameName: "dice-run", pointsPerSession: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestCatalog()
			_, err := c.RegisterGame(ctx, tt.gameName, tt.pointsPerSession)
			if tt.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetAndListGames(t *testing.T) {
	c, _ := newTestCatalog()

	game, err := c.RegisterGame(ctx, "dice-run", 15)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	got, err := c.GetGame(ctx, game.ID)
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if got.Name != "dice-run" {
		t.Fatalf("Name = %q, want dice-run", got.Name)
	}

	if _, err := c.GetGame(ctx, "does-not-exist"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}

	if games := c.Games(ctx); len(games) != 1 {
		t.Fatalf("len(Games()) = %d, want 1", len(games))
	}
}

func TestRecordSessionPublishesSessionCompleted(t *testing.T) {
	c, pub := newTestCatalog()
	game, err := c.RegisterGame(ctx, "dice-run", 15)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	if err := c.RecordSession(ctx, "player_1", game.ID, "session-key-1"); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(pub.published))
	}

	var event struct {
		PlayerID       string `json:"playerId"`
		GameID         string `json:"gameId"`
		Points         int    `json:"points"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if err := json.Unmarshal([]byte(pub.published[0]), &event); err != nil {
		t.Fatalf("unmarshal published event: %v", err)
	}
	if event.PlayerID != "player_1" || event.GameID != game.ID || event.Points != 15 {
		t.Fatalf("event = %+v, want PlayerID=player_1 GameID=%s Points=15", event, game.ID)
	}
	if event.IdempotencyKey != "session-key-1" {
		t.Fatalf("IdempotencyKey = %q, want the caller-supplied key, not a generated one", event.IdempotencyKey)
	}
}

func TestRecordSessionUnknownGame(t *testing.T) {
	c, _ := newTestCatalog()

	err := c.RecordSession(ctx, "player_1", "does-not-exist", "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRecordSessionPublishFailure(t *testing.T) {
	c, pub := newTestCatalog()
	game, err := c.RegisterGame(ctx, "dice-run", 15)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}
	pub.err = errors.New("sqs unavailable")

	if err := c.RecordSession(ctx, "player_1", game.ID, ""); err == nil {
		t.Fatal("expected an error when publishing fails, got nil")
	}
}
