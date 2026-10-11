package domain

import (
	"context"
	"sync"
	"sync/atomic"
)

// SessionEvent is a play-session to apply to the engine: "this player
// played this game." It's deliberately the same shape as an event you'd
// later receive off a real queue — phase 6 replaces the channel below
// with AWS SQS. This phase is about processing a stream of them
// concurrently and safely before a real queue is in the picture.
type SessionEvent struct {
	PlayerID string
	GameID   string
}

// SessionResult pairs an event with the outcome of applying it.
type SessionResult struct {
	Event SessionEvent
	Entry LedgerEntry
	Err   error
}

// Processor fans a stream of SessionEvents out across a fixed pool of
// worker goroutines, each calling Engine.RecordSession.
type Processor struct {
	engine  *Engine
	workers int

	processed atomic.Int64
}

// NewProcessor builds a Processor with the given worker count (minimum
// 1). More workers means more RecordSession calls in flight at once —
// Engine's own locking (see engine.go) is what makes that safe.
func NewProcessor(engine *Engine, workers int) *Processor {
	if workers < 1 {
		workers = 1
	}
	return &Processor{engine: engine, workers: workers}
}

// Processed returns how many events have been handled so far (success or
// failure). It's an atomic.Int64 rather than a plain int64 guarded by a
// mutex: a single counter that's only ever incremented is exactly the
// case sync/atomic exists for — cheaper than a mutex, and there's nothing
// else to protect alongside it.
func (p *Processor) Processed() int64 {
	return p.processed.Load()
}

// Run starts the worker pool, consuming from events until that channel is
// closed or ctx is cancelled, and returns a results channel that itself
// closes once every worker has exited — callers can simply `range` over
// it without knowing how many workers there were.
//
// That "close only after every worker has stopped" guarantee is the
// WaitGroup's job: each worker calls wg.Done() on exit, and a separate
// goroutine blocks on wg.Wait() and then closes the output channel. This
// fan-out/fan-in shape — one input channel, N worker goroutines, one
// output channel — is the standard way to parallelize a stream of work
// in Go.
func (p *Processor) Run(ctx context.Context, events <-chan SessionEvent) <-chan SessionResult {
	results := make(chan SessionResult)

	var wg sync.WaitGroup
	wg.Add(p.workers)
	for i := 0; i < p.workers; i++ {
		go func() {
			defer wg.Done()
			p.worker(ctx, events, results)
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func (p *Processor) worker(ctx context.Context, events <-chan SessionEvent, results chan<- SessionResult) {
	for {
		select {
		case <-ctx.Done():
			// Cancelled (timeout or explicit cancel) — stop pulling new
			// events and exit; any event not yet read is simply dropped.
			return
		case event, ok := <-events:
			if !ok {
				// events was closed and drained: no more work, exit.
				return
			}
			entry, err := p.engine.RecordSession(ctx, event.PlayerID, event.GameID)
			p.processed.Add(1)

			select {
			case results <- SessionResult{Event: event, Entry: entry, Err: err}:
			case <-ctx.Done():
				return
			}
		}
	}
}
