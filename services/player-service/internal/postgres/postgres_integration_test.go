//go:build integration

// Integration tests are excluded from the default `go test ./...` run by
// the build tag above — they need Docker and take real seconds to spin a
// container up, which doesn't belong in the fast inner loop every other
// package's tests run in. Run these explicitly with:
//
//	go test -tags=integration ./services/player-service/internal/postgres/...
package postgres_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"reward-platform/pkg/domain"
	"reward-platform/services/player-service/internal/postgres"
)

// newTestDB starts a real, throwaway Postgres container, runs every
// migration against it, and registers cleanup (container + connection)
// to run when the test finishes — t.Cleanup instead of a manual defer so
// it still runs even if the test fails partway through.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("rewardplatform"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Relative to this package's directory, which is where `go test`
	// sets its working directory.
	if err := postgres.Migrate(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

func TestPlayerRepository(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	players := postgres.NewPlayerRepository(db)
	ledger := postgres.NewLedgerRepository(db)

	player := domain.Player{
		Model:    domain.Model{ID: "player_test_1", CreatedAt: time.Now().UTC().Truncate(time.Microsecond)},
		Username: "alice",
	}
	if err := players.Put(ctx, player); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := players.Get(ctx, player.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Username != "alice" {
		t.Fatalf("Username = %q, want alice", got.Username)
	}

	if _, err := players.Get(ctx, "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown player, got nil")
	}

	list, err := players.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}

	// Ledger: two entries for the same player, Balance should be their sum,
	// computed by Postgres (SUM()), not by Go code iterating entries.
	entries := []domain.LedgerEntry{
		{Model: domain.Model{ID: "entry_1", CreatedAt: time.Now().UTC()}, PlayerID: player.ID, Delta: 15, Reason: "played dice-run"},
		{Model: domain.Model{ID: "entry_2", CreatedAt: time.Now().UTC()}, PlayerID: player.ID, Delta: -10, Reason: "redeemed sticker-pack"},
	}
	for _, e := range entries {
		if err := ledger.Append(ctx, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	balance, err := ledger.Balance(ctx, player.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if balance != 5 {
		t.Fatalf("Balance = %d, want 5", balance)
	}

	// A player with no ledger entries at all must get 0, not an error —
	// COALESCE(SUM(...), 0) in the query is what guarantees this.
	balance, err = ledger.Balance(ctx, "player_with_no_entries")
	if err != nil {
		t.Fatalf("Balance (no entries): %v", err)
	}
	if balance != 0 {
		t.Fatalf("Balance (no entries) = %d, want 0", balance)
	}
}
