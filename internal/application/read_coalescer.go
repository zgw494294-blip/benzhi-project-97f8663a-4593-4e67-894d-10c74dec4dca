package application

import (
	"context"
	"sync"

	"seed-vigor-workbench/internal/domain"
)

type assayReadCall struct {
	done  chan struct{}
	assay *domain.GerminationAssay
	err   error
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
		call = &assayReadCall{done: make(chan struct{})}
		g.calls[id] = call
		go g.run(ctx, id, call, load)
	}
	g.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		return call.assay, call.err
	}
}

func (g *assayReadGroup) run(ctx context.Context, id string, call *assayReadCall, load func(context.Context, string) (*domain.GerminationAssay, error)) {
	call.assay, call.err = load(ctx, id)
	close(call.done)

	g.mu.Lock()
	if g.calls[id] == call {
		delete(g.calls, id)
	}
	g.mu.Unlock()
}
