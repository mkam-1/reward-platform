package domain

import (
	"fmt"
	"sync"
)

// Engine is the in-memory application core: it owns players, games,
// rewards and the points ledger, and enforces the rules that connect them
// (you can't redeem a reward you can't afford, or one that's out of
// stock). Real persistence and HTTP transport are layered on top of this
// in later phases; Engine itself knows about neither.
type Engine struct {
	clock Clock
	ids   *idGenerator

	players *Store[Player]
	games   *Store[Game]
	rewards *Store[Reward]

	ledgerMu sync.RWMutex
	ledger   []LedgerEntry
}

// NewEngine builds an Engine using the supplied Clock. Tests pass a fake
// clock for deterministic timestamps; see NewSystemEngine for production
// use.
func NewEngine(clock Clock) *Engine {
	return &Engine{
		clock:   clock,
		ids:     newIDGenerator(),
		players: NewStore[Player](),
		games:   NewStore[Game](),
		rewards: NewStore[Reward](),
	}
}

// NewSystemEngine is a convenience constructor wired to the real wall
// clock, for production/CLI use.
func NewSystemEngine() *Engine {
	return NewEngine(SystemClock{})
}

func (e *Engine) RegisterPlayer(username string) (Player, error) {
	if username == "" {
		return Player{}, &ValidationError{Field: "username", Msg: "must not be empty"}
	}
	p := Player{
		Model:    Model{ID: e.ids.next("player"), CreatedAt: e.clock.Now()},
		Username: username,
	}
	e.players.Put(p.ID, p)
	return p, nil
}

func (e *Engine) RegisterGame(name string, pointsPerSession int) (Game, error) {
	if name == "" {
		return Game{}, &ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if pointsPerSession <= 0 {
		return Game{}, &ValidationError{Field: "pointsPerSession", Msg: "must be positive"}
	}
	g := Game{
		Model:            Model{ID: e.ids.next("game"), CreatedAt: e.clock.Now()},
		Name:             name,
		PointsPerSession: pointsPerSession,
	}
	e.games.Put(g.ID, g)
	return g, nil
}

func (e *Engine) RegisterReward(name string, costPoints, stock int) (Reward, error) {
	if name == "" {
		return Reward{}, &ValidationError{Field: "name", Msg: "must not be empty"}
	}
	if costPoints <= 0 {
		return Reward{}, &ValidationError{Field: "costPoints", Msg: "must be positive"}
	}
	if stock < 0 {
		return Reward{}, &ValidationError{Field: "stock", Msg: "must not be negative"}
	}
	r := Reward{
		Model:      Model{ID: e.ids.next("reward"), CreatedAt: e.clock.Now()},
		Name:       name,
		CostPoints: costPoints,
		Stock:      stock,
	}
	e.rewards.Put(r.ID, r)
	return r, nil
}

// RecordSession credits a player with the game's PointsPerSession.
func (e *Engine) RecordSession(playerID, gameID string) (LedgerEntry, error) {
	player, err := e.players.Get(playerID)
	if err != nil {
		return LedgerEntry{}, fmt.Errorf("record session: player: %w", err)
	}
	game, err := e.games.Get(gameID)
	if err != nil {
		return LedgerEntry{}, fmt.Errorf("record session: game: %w", err)
	}

	entry := LedgerEntry{
		Model:    Model{ID: e.ids.next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: player.ID,
		Delta:    game.PointsPerSession,
		Reason:   fmt.Sprintf("played %s", game.Name),
	}

	e.ledgerMu.Lock()
	e.ledger = append(e.ledger, entry)
	e.ledgerMu.Unlock()

	return entry, nil
}

// Balance sums every ledger entry for the player. It errors if the player
// doesn't exist at all, distinguishing "unknown player" from "known
// player, zero balance".
func (e *Engine) Balance(playerID string) (int, error) {
	if _, err := e.players.Get(playerID); err != nil {
		return 0, fmt.Errorf("balance: %w", err)
	}

	e.ledgerMu.RLock()
	defer e.ledgerMu.RUnlock()

	balance := 0
	for _, entry := range e.ledger {
		if entry.PlayerID == playerID {
			balance += entry.Delta
		}
	}
	return balance, nil
}

// RedeemReward exchanges a player's points for a reward, failing if they
// can't afford it or it's out of stock.
//
// Known limitation, left in deliberately: the balance check, stock
// decrement and ledger append below are three separate steps rather than
// one atomic operation, so two concurrent redemptions for the same player
// or reward can race past the checks together. Engine isn't safe for
// concurrent redemptions yet — that gap is exactly what phase 2
// (concurrency) exists to close.
func (e *Engine) RedeemReward(playerID, rewardID string) (Redemption, error) {
	if _, err := e.players.Get(playerID); err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	reward, err := e.rewards.Get(rewardID)
	if err != nil {
		return Redemption{}, fmt.Errorf("redeem: %w", err)
	}
	if reward.Stock <= 0 {
		return Redemption{}, fmt.Errorf("redeem: reward %q: %w", reward.Name, ErrOutOfStock)
	}

	balance, err := e.Balance(playerID)
	if err != nil {
		return Redemption{}, err
	}
	if balance < reward.CostPoints {
		return Redemption{}, fmt.Errorf("redeem: have %d, need %d: %w", balance, reward.CostPoints, ErrInsufficientBalance)
	}

	reward.Stock--
	e.rewards.Put(reward.ID, reward)

	entry := LedgerEntry{
		Model:    Model{ID: e.ids.next("entry"), CreatedAt: e.clock.Now()},
		PlayerID: playerID,
		Delta:    -reward.CostPoints,
		Reason:   fmt.Sprintf("redeemed %s", reward.Name),
	}
	e.ledgerMu.Lock()
	e.ledger = append(e.ledger, entry)
	e.ledgerMu.Unlock()

	redemption := Redemption{
		Model:       Model{ID: e.ids.next("redemption"), CreatedAt: e.clock.Now()},
		PlayerID:    playerID,
		RewardID:    rewardID,
		PointsSpent: reward.CostPoints,
	}
	return redemption, nil
}

func (e *Engine) Players() []Player { return e.players.List() }
func (e *Engine) Games() []Game     { return e.games.List() }
func (e *Engine) Rewards() []Reward { return e.rewards.List() }
