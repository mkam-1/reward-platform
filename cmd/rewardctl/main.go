// Command rewardctl exercises the reward-platform domain engine from the
// command line. It exists so phase 1 has something runnable, not just
// tests: `go run ./cmd/rewardctl demo` plays through the whole domain
// model end to end.
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

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
      rewardctl demo    run a scripted walkthrough: register players, games and
                         rewards, simulate play sessions, and redeem a reward
      rewardctl help    show this message`)
}

func runDemo() {
	engine := domain.NewSystemEngine()

	alice, err := engine.RegisterPlayer("alice")
	must(err)
	bob, err := engine.RegisterPlayer("bob")
	must(err)

	diceRun, err := engine.RegisterGame("dice-run", 15)
	must(err)
	towerClimb, err := engine.RegisterGame("tower-climb", 25)
	must(err)

	stickerPack, err := engine.RegisterReward("sticker-pack", 30, 5)
	must(err)
	giftCard, err := engine.RegisterReward("$5-gift-card", 500, 2)
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
		entry, err := engine.RecordSession(s.player.ID, s.game.ID)
		must(err)
		fmt.Printf("session: %s earned %d points playing %s\n", s.player.Username, entry.Delta, s.game.Name)
	}

	fmt.Println()
	printBalances(engine, []domain.Player{alice, bob})

	fmt.Println()
	redemption, err := engine.RedeemReward(alice.ID, stickerPack.ID)
	must(err)
	fmt.Printf("redemption: %s spent %d points on %s\n", alice.Username, redemption.PointsSpent, stickerPack.Name)

	if _, err := engine.RedeemReward(bob.ID, giftCard.ID); err != nil {
		fmt.Printf("redemption: bob tried %s and failed: %v\n", giftCard.Name, err)
	}

	fmt.Println()
	printBalances(engine, []domain.Player{alice, bob})
}

func printBalances(engine *domain.Engine, players []domain.Player) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "PLAYER\tBALANCE")
	for _, p := range players {
		balance, err := engine.Balance(p.ID)
		must(err)
		fmt.Fprintf(w, "%s\t%d\n", p.Username, balance)
	}
	w.Flush()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
