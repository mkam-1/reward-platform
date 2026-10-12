package domain_test

import (
	"errors"
	"testing"

	"reward-platform/pkg/domain"
)

// These are the one place in this codebase where genuinely arbitrary,
// untrusted strings reach domain logic directly — a username or a game
// name is whatever an HTTP client's JSON body said it was, unicode,
// empty, enormous, or otherwise. Go's built-in fuzzer explores that
// input space far more thoroughly than anyone would hand-write table
// cases for, looking specifically for panics and invariant violations —
// run it for real with:
//
//	go test -fuzz=FuzzRegisterPlayer -fuzztime=30s

func FuzzRegisterPlayer(f *testing.F) {
	for _, seed := range []string{"", "alice", " ", "\x00", "a very long username that goes on and on and on", "名前", "'; DROP TABLE players; --"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, username string) {
		e := newTestEngine()
		player, err := e.RegisterPlayer(ctx, username)

		if username == "" {
			var verr *domain.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("username = %q: error = %v, want *ValidationError", username, err)
			}
			return
		}

		if err != nil {
			t.Fatalf("username = %q: unexpected error: %v", username, err)
		}
		if player.ID == "" {
			t.Fatalf("username = %q: player.ID is empty", username)
		}
		if player.Username != username {
			t.Fatalf("username = %q: player.Username = %q, want exact match", username, player.Username)
		}
	})
}

func FuzzRegisterGame(f *testing.F) {
	f.Add("dice-run", 15)
	f.Add("", 15)
	f.Add("dice-run", 0)
	f.Add("dice-run", -1)
	f.Add("dice-run", 1<<30)

	f.Fuzz(func(t *testing.T, name string, pointsPerSession int) {
		e := newTestEngine()
		game, err := e.RegisterGame(ctx, name, pointsPerSession)

		wantErr := name == "" || pointsPerSession <= 0
		var verr *domain.ValidationError
		if wantErr {
			if !errors.As(err, &verr) {
				t.Fatalf("name=%q points=%d: error = %v, want *ValidationError", name, pointsPerSession, err)
			}
			return
		}

		if err != nil {
			t.Fatalf("name=%q points=%d: unexpected error: %v", name, pointsPerSession, err)
		}
		if game.Name != name || game.PointsPerSession != pointsPerSession {
			t.Fatalf("name=%q points=%d: got %+v, want fields to match input exactly", name, pointsPerSession, game)
		}
	})
}

func FuzzRegisterReward(f *testing.F) {
	f.Add("sticker-pack", 30, 5)
	f.Add("", 30, 5)
	f.Add("sticker-pack", 0, 5)
	f.Add("sticker-pack", 30, -1)
	f.Add("sticker-pack", -1, -1)

	f.Fuzz(func(t *testing.T, name string, costPoints, stock int) {
		e := newTestEngine()
		reward, err := e.RegisterReward(ctx, name, costPoints, stock)

		wantErr := name == "" || costPoints <= 0 || stock < 0
		var verr *domain.ValidationError
		if wantErr {
			if !errors.As(err, &verr) {
				t.Fatalf("name=%q cost=%d stock=%d: error = %v, want *ValidationError", name, costPoints, stock, err)
			}
			return
		}

		if err != nil {
			t.Fatalf("name=%q cost=%d stock=%d: unexpected error: %v", name, costPoints, stock, err)
		}
		if reward.Name != name || reward.CostPoints != costPoints || reward.Stock != stock {
			t.Fatalf("name=%q cost=%d stock=%d: got %+v, want fields to match input exactly", name, costPoints, stock, reward)
		}
	})
}
