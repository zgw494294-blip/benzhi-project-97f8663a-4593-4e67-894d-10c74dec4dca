package application

import (
	"context"
	"sync"

	"seed-vigor-workbench/internal/domain"
)

type assayReadCall struct {
	done chan struct{}

	mu sync.Mutex
	// loadCtx is shared by all callers that joined this in-flight call. It is
	// independent of any single caller's context so that cancelling one caller
	// never aborts the underlying read that other still-valid callers depend
	// on. It is cancelled once the read finishes or no caller remains.
	loadCtx  context.Context
	cancel   context.CancelFunc
	waiters  int
	finished bool
	result   *domain.GerminationAssay
	err      error
}

type assayReadGroup struct {
	mu    sync.Mutex
	calls map[string]*assayReadCall
}

func (g *assayReadGroup) Do(ctx context.Context, id string, load func(context.Context, string) (*domain.GerminationAssay, error)) (*domain.GerminationAssay, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*assayReadCall)
	}
	call, exists := g.calls[id]
	if !exists {
		call = newAssayReadCall()
		g.calls[id] = call
		go g.run(call, id, load)
	}
	joined := call.join(ctx)
	g.mu.Unlock()

	select {
	case <-ctx.Done():
		joined.detach()
		return nil, ctx.Err()
	case <-call.done:
		joined.detach()
		return call.result, call.err
	}
}

// join registers ctx as a waiter on the shared call and returns a handle whose
// detach method must be called when the caller stops waiting (either because
// its own context was cancelled or because the shared call completed). detach
// returns the number of waiters remaining after this one detaches.
type waiterHandle struct {
	call *assayReadCall
	stop func() bool
}

func (c *assayReadCall) join(ctx context.Context) waiterHandle {
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return waiterHandle{call: c, stop: func() bool { return true }}
	}
	c.waiters++
	// When this caller's context is cancelled while it is still waiting, drop
	// the waiter; if no waiter remains, the load is unwanted and may be aborted.
	stop := context.AfterFunc(ctx, func() {
		if c.decWaiters() == 0 {
			c.maybeCancel()
		}
	})
	c.mu.Unlock()
	return waiterHandle{call: c, stop: stop}
}

// detach removes this waiter from the shared call and returns the number of
// waiters remaining. It is safe to call more than once per join; subsequent
// calls are no-ops.
func (h waiterHandle) detach() int {
	// Stop observing the caller's context so a late cancellation does not
	// double-count this waiter. If the context was already cancelled, stop
	// returns false and the AfterFunc will (or already has) performed the
	// detach, so only decrement when stop succeeded.
	if h.stop != nil && h.stop() {
		return h.call.decWaiters()
	}
	return h.call.waitersLocked()
}

func (c *assayReadCall) decWaiters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.waiters > 0 {
		c.waiters--
	}
	return c.waiters
}

func (c *assayReadCall) waitersLocked() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.waiters
}

// maybeCancel aborts the shared load when it has not finished and no waiter is
// left to consume its result.
func (c *assayReadCall) maybeCancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.finished && c.waiters == 0 {
		c.cancel()
	}
}

func (g *assayReadGroup) run(call *assayReadCall, id string, load func(context.Context, string) (*domain.GerminationAssay, error)) {
	assay, err := load(call.loadCtx, id)

	call.mu.Lock()
	call.result = assay
	call.err = err
	call.finished = true
	call.mu.Unlock()
	call.cancel()
	close(call.done)

	g.mu.Lock()
	if g.calls[id] == call {
		delete(g.calls, id)
	}
	g.mu.Unlock()
}

func newAssayReadCall() *assayReadCall {
	loadCtx, cancel := context.WithCancel(context.Background())
	return &assayReadCall{done: make(chan struct{}), loadCtx: loadCtx, cancel: cancel}
}
