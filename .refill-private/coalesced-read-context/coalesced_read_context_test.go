package coalesced_read_context_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

type readResult struct {
	view *application.AssayView
	err  error
}

type controlledRepository struct {
	started  chan context.Context
	release  chan struct{}
	observed chan error
	assay    *domain.GerminationAssay
}

func newControlledRepository() *controlledRepository {
	return &controlledRepository{
		started:  make(chan context.Context),
		release:  make(chan struct{}),
		observed: make(chan error, 1),
		assay: &domain.GerminationAssay{
			ID: "assay-context", State: domain.StateDraft, Revision: 1,
			Protocol: domain.AssayProtocol{ObservationDays: 1, ReplicateCount: 1},
		},
	}
}

func (r *controlledRepository) Create(context.Context, *domain.GerminationAssay, string) error {
	return nil
}

func (r *controlledRepository) Get(ctx context.Context, _ string) (*domain.GerminationAssay, error) {
	r.started <- ctx
	<-r.release
	err := ctx.Err()
	r.observed <- err
	if err != nil {
		return nil, err
	}
	copy := *r.assay
	return &copy, nil
}

func (r *controlledRepository) List(context.Context) ([]domain.GerminationAssay, error) {
	return nil, nil
}

func (r *controlledRepository) Update(context.Context, string, int64, string, string, map[string]any, func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error) {
	return nil, errors.New("unexpected Update")
}

type doneProbeContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
}

func (c *doneProbeContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func getAsync(service *application.Service, ctx context.Context) <-chan readResult {
	result := make(chan readResult, 1)
	go func() {
		view, err := service.GetAssay(ctx, "assay-context")
		result <- readResult{view: view, err: err}
	}()
	return result
}

func TestCoalescedReadKeepsIndependentCallerContext(t *testing.T) {
	t.Run("last waiter cancellation reaches repository", func(t *testing.T) {
		repository := newControlledRepository()
		service := application.NewService(repository, nil)
		ctx, cancel := context.WithCancel(context.Background())
		result := getAsync(service, ctx)
		<-repository.started

		cancel()
		if got := <-result; !errors.Is(got.err, context.Canceled) {
			t.Fatalf("调用方取消应立即返回 context.Canceled，得到 %v", got.err)
		}
		close(repository.release)
		if err := <-repository.observed; !errors.Is(err, context.Canceled) {
			t.Fatalf("最后一个等待者取消后仓储 context 仍然存活: %v", err)
		}
	})

	t.Run("one canceled waiter does not poison another", func(t *testing.T) {
		repository := newControlledRepository()
		service := application.NewService(repository, nil)
		leaderContext, cancelLeader := context.WithCancel(context.Background())
		leader := getAsync(service, leaderContext)
		<-repository.started

		probe := &doneProbeContext{Context: context.Background(), entered: make(chan struct{})}
		follower := getAsync(service, probe)
		<-probe.entered

		cancelLeader()
		if got := <-leader; !errors.Is(got.err, context.Canceled) {
			t.Fatalf("已取消首请求应返回 context.Canceled，得到 %v", got.err)
		}
		close(repository.release)
		<-repository.observed

		got := <-follower
		if got.err != nil {
			t.Fatalf("仍存活的合并读取被首请求取消污染: %v", got.err)
		}
		if got.view == nil || got.view.Assay.ID != "assay-context" {
			t.Fatalf("仍存活的调用方未获得批次详情: %+v", got.view)
		}
	})
}
