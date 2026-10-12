// Command rewardctl exercises the reward-platform domain engine from the
// command line. It exists so phase 1 has something runnable, not just
// tests: `go run ./cmd/rewardctl demo` plays through the whole domain
// model end to end.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"text/tabwriter"
	"time"

	"reward-platform/pkg/domain"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "demo":
		runDemo()
	case "simulate":
		runSimulate(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`rewardctl - exercise the reward-platform domain engine

    Usage:
      rewardctl demo        run a scripted walkthrough: register players, games
                             and rewards, simulate play sessions, and redeem a
                             reward
      rewardctl simulate    fire a batch of play-session events at a worker
                             pool concurrently (flags: -players -games -events
                             -workers)
      rewardctl help         show this message`)
}

func runDemo() {
	ctx := context.Background()
	engine := domain.NewSystemEngine()

	alice, err := engine.RegisterPlayer(ctx, "alice")
	must(err)
	bob, err := engine.RegisterPlayer(ctx, "bob")
	must(err)

	diceRun, err := engine.RegisterGame(ctx, "dice-run", 15)
	must(err)
	towerClimb, err := engine.RegisterGame(ctx, "tower-climb", 25)
	must(err)

	stickerPack, err := engine.RegisterReward(ctx, "sticker-pack", 30, 5)
	must(err)
	giftCard, err := engine.RegisterReward(ctx, "$5-gift-card", 500, 2)
	must(err)

	sessions := []struct {
		player domain.Player
		game   domain.Game
	}{
		{alice, diceRun},
		{alice, diceRun},
		{alice, towerClimb},
		{bob, towerClimb},
	}
	for _, s := range sessions {
		entry, err := engine.RecordSession(ctx, s.player.ID, s.game.ID)
		must(err)
		fmt.Printf("session: %s earned %d points playing %s\n", s.player.Username, entry.Delta, s.game.Name)
	}

	fmt.Println()
	printBalances(ctx, engine, []domain.Player{alice, bob})

	fmt.Println()
	redemption, err := engine.RedeemReward(ctx, alice.ID, stickerPack.ID, "")
	must(err)
	fmt.Printf("redemption: %s spent %d points on %s\n", alice.Username, redemption.PointsSpent, stickerPack.Name)

	if _, err := engine.RedeemReward(ctx, bob.ID, giftCard.ID, ""); err != nil {
		fmt.Printf("redemption: bob tried %s and failed: %v\n", giftCard.Name, err)
	}

	fmt.Println()
	printBalances(ctx, engine, []domain.Player{alice, bob})
}

func runSimulate(args []string) {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	numPlayers := fs.Int("players", 20, "number of players to register")
	numGames := fs.Int("games", 3, "number of games to register")
	numEvents := fs.Int("events", 500, "number of play-session events to process")
	numWorkers := fs.Int("workers", 8, "number of concurrent workers processing events")
	must(fs.Parse(args))

	engine := domain.NewSystemEngine()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	players := make([]domain.Player, *numPlayers)
	for i := range players {
		p, err := engine.RegisterPlayer(ctx, fmt.Sprintf("player-%d", i+1))
		must(err)
		players[i] = p
	}

	games := make([]domain.Game, *numGames)
	for i := range games {
		//nolint:gosec // math/rand/v2 is fine here — picking a points value for fake test data, not a security context
		g, err := engine.RegisterGame(ctx, fmt.Sprintf("game-%d", i+1), 5+rand.IntN(20))
		must(err)
		games[i] = g
	}

	// Producer goroutine: generates the event stream and closes the
	// channel when done, which is what lets the processor's workers know
	// there's no more work and exit cleanly.
	eventCh := make(chan domain.SessionEvent)
	go func() {
		defer close(eventCh)
		for i := 0; i < *numEvents; i++ {
			//nolint:gosec // math/rand/v2 is fine here — picking which fake player/game to simulate, not a security context
			eventCh <- domain.SessionEvent{
				PlayerID: players[rand.IntN(len(players))].ID,
				GameID:   games[rand.IntN(len(games))].ID,
			}
		}
	}()

	proc := domain.NewProcessor(engine, *numWorkers)

	start := time.Now()
	results := proc.Run(ctx, eventCh)

	succeeded, failed := 0, 0
	for res := range results {
		if res.Err != nil {
			failed++
			continue
		}
		succeeded++
	}
	elapsed := time.Since(start)

	fmt.Printf("simulate: processed %d events (%d ok, %d failed) across %d workers in %s\n",
		proc.Processed(), succeeded, failed, *numWorkers, elapsed)

	fmt.Println()
	printBalances(ctx, engine, players)
}

func printBalances(ctx context.Context, engine *domain.Engine, players []domain.Player) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	// Writes to a stdout-backed tabwriter: an error here would mean
	// stdout itself is broken, which isn't something this CLI can
	// usefully recover from — not worth a must() per line.
	_, _ = fmt.Fprintln(w, "PLAYER\tBALANCE")
	for _, p := range players {
		balance, err := engine.Balance(ctx, p.ID)
		must(err)
		_, _ = fmt.Fprintf(w, "%s\t%d\n", p.Username, balance)
	}
	_ = w.Flush()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
