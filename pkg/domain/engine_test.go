package domain_test

import (
	"errors"
	"testing"
	"time"

	"reward-platform/pkg/domain"
)

// fakeClock lets tests assert on timestamps without racing the real clock.
type fakeClock struct{ t time.Time }

func (f fakeClock) Now() time.Time { return f.t }

func newTestEngine() *domain.Engine {
	return domain.NewEngine(fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
}

func TestRegisterPlayer(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{name: "valid username", username: "alice", wantErr: false},
		{name: "empty username", username: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine()
			p, err := e.RegisterPlayer(tt.username)

			if tt.wantErr {
				var verr *domain.ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("error = %v (%T), want *domain.ValidationError", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID == "" {
				t.Fatal("expected non-empty player ID")
			}
			if p.Username != tt.username {
				t.Fatalf("Username = %q, want %q", p.Username, tt.username)
			}
			if !p.CreatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("CreatedAt = %v, want fake clock time", p.CreatedAt)
			}
		})
	}
}

func TestRegisterGameValidation(t *testing.T) {
	tests := []struct {
		name             string
		gameName         string
		pointsPerSession int
		wantErr          bool
	}{
		{name: "valid", gameName: "dice-run", pointsPerSession: 10, wantErr: false},
		{name: "empty name", gameName: "", pointsPerSession: 10, wantErr: true},
		{name: "zero points", gameName: "dice-run", pointsPerSession: 0, wantErr: true},
		{name: "negative points", gameName: "dice-run", pointsPerSession: -5, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine()
			_, err := e.RegisterGame(tt.gameName, tt.pointsPerSession)
			if tt.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRecordSessionAndBalance(t *testing.T) {
	e := newTestEngine()
	player, err := e.RegisterPlayer("alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	game, err := e.RegisterGame("dice-run", 10)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := e.RecordSession(player.ID, game.ID); err != nil {
			t.Fatalf("RecordSession: %v", err)
		}
	}

	balance, err := e.Balance(player.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 20 {
		t.Fatalf("balance = %d, want 20", balance)
	}
}

func TestGetPlayer(t *testing.T) {
	e := newTestEngine()
	player, err := e.RegisterPlayer("alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}

	got, err := e.GetPlayer(player.ID)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("Username = %q, want alice", got.Username)
	}

	if _, err := e.GetPlayer("does-not-exist"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRecordSessionUnknownPlayer(t *testing.T) {
	e := newTestEngine()
	game, err := e.RegisterGame("dice-run", 10)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	_, err = e.RecordSession("does-not-exist", game.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestBalanceUnknownPlayer(t *testing.T) {
	e := newTestEngine()
	if _, err := e.Balance("does-not-exist"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestRedeemReward(t *testing.T) {
	e := newTestEngine()
	player, err := e.RegisterPlayer("alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	game, err := e.RegisterGame("dice-run", 50)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}
	reward, err := e.RegisterReward("sticker-pack", 30, 1)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	if _, err := e.RecordSession(player.ID, game.ID); err != nil {
		t.Fatalf("RecordSession: %v", err)
	}

	redemption, err := e.RedeemReward(player.ID, reward.ID)
	if err != nil {
		t.Fatalf("RedeemReward: %v", err)
	}
	if redemption.PointsSpent != 30 {
		t.Fatalf("PointsSpent = %d, want 30", redemption.PointsSpent)
	}

	balance, err := e.Balance(player.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 20 {
		t.Fatalf("balance after redemption = %d, want 20", balance)
	}

	// Stock was 1; a second redemption must fail as out of stock.
	if _, err := e.RedeemReward(player.ID, reward.ID); !errors.Is(err, domain.ErrOutOfStock) {
		t.Fatalf("second redemption error = %v, want ErrOutOfStock", err)
	}
}

func TestRedeemRewardInsufficientBalance(t *testing.T) {
	e := newTestEngine()
	player, err := e.RegisterPlayer("alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	reward, err := e.RegisterReward("console", 1000, 5)
	if err != nil {
		t.Fatalf("RegisterReward: %v", err)
	}

	_, err = e.RedeemReward(player.ID, reward.ID)
	if !errors.Is(err, domain.ErrInsufficientBalance) {
		t.Fatalf("error = %v, want ErrInsufficientBalance", err)
	}
}
