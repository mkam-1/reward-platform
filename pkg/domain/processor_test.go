package domain_test

import (
	"context"
	"testing"
	"time"

	"reward-platform/pkg/domain"
)

func TestProcessorRunProcessesEveryEvent(t *testing.T) {
	e := domain.NewEngine(domain.SystemClock{})
	player, err := e.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	game, err := e.RegisterGame(ctx, "dice-run", 10)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	const eventCount = 200
	events := make(chan domain.SessionEvent)
	go func() {
		defer close(events)
		for i := 0; i < eventCount; i++ {
			events <- domain.SessionEvent{PlayerID: player.ID, GameID: game.ID}
		}
	}()

	proc := domain.NewProcessor(e, 8) // 8 workers racing for the same player's ledger entries
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	results := proc.Run(runCtx, events)

	got := 0
	for res := range results { // closes automatically once every worker exits
		if res.Err != nil {
			t.Fatalf("unexpected error processing event: %v", res.Err)
		}
		if res.Entry.Delta != 10 {
			t.Fatalf("Delta = %d, want 10", res.Entry.Delta)
		}
		got++
	}

	if got != eventCount {
		t.Fatalf("processed %d results, want %d", got, eventCount)
	}
	if proc.Processed() != int64(eventCount) {
		t.Fatalf("Processed() = %d, want %d", proc.Processed(), eventCount)
	}

	balance, err := e.Balance(ctx, player.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if want := eventCount * 10; balance != want {
		t.Fatalf("balance = %d, want %d", balance, want)
	}
}

func TestProcessorRunStopsOnCancel(t *testing.T) {
	e := domain.NewEngine(domain.SystemClock{})
	player, err := e.RegisterPlayer(ctx, "alice")
	if err != nil {
		t.Fatalf("RegisterPlayer: %v", err)
	}
	game, err := e.RegisterGame(ctx, "dice-run", 10)
	if err != nil {
		t.Fatalf("RegisterGame: %v", err)
	}

	// An events producer that never stops — the only way out is ctx
	// cancellation, which is exactly what this test is checking.
	events := make(chan domain.SessionEvent)
	go func() {
		for {
			select {
			case events <- domain.SessionEvent{PlayerID: player.ID, GameID: game.ID}:
			case <-time.After(time.Second):
				return // safety net so a test failure doesn't leak this goroutine forever
			}
		}
	}()

	proc := domain.NewProcessor(e, 4)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel() // belt-and-suspenders: also cancel explicitly below once 5 events are in

	results := proc.Run(runCtx, events)

	// Let a handful of events through, then cancel.
	drained := 0
	for range results {
		drained++
		if drained == 5 {
			cancel()
		}
	}

	// The channel is only closed once every worker has actually returned,
	// so reaching here already proves cancellation was honored. A sanity
	// bound on Processed() confirms work actually stopped rather than the
	// producer simply running dry.
	if proc.Processed() > 1000 {
		t.Fatalf("Processed() = %d, cancellation does not appear to have stopped the workers", proc.Processed())
	}
}
