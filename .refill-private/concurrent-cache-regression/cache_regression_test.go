package concurrent_cache_regression_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

type controlledRepository struct {
	mu         sync.Mutex
	assay      *domain.GerminationAssay
	getStarted chan struct{}
	releaseGet chan struct{}
	startOnce  sync.Once
}

func (r *controlledRepository) Create(context.Context, *domain.GerminationAssay, string) error {
	return fmt.Errorf("本复现不创建批次")
}

func (r *controlledRepository) Get(ctx context.Context, id string) (*domain.GerminationAssay, error) {
	r.mu.Lock()
	if r.assay.ID != id {
		r.mu.Unlock()
		return nil, fmt.Errorf("批次不存在")
	}
	snapshot := cloneAssay(r.assay)
	r.mu.Unlock()

	r.startOnce.Do(func() { close(r.getStarted) })
	select {
	case <-r.releaseGet:
		return snapshot, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *controlledRepository) List(context.Context) ([]domain.GerminationAssay, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []domain.GerminationAssay{*cloneAssay(r.assay)}, nil
}

func (r *controlledRepository) Update(_ context.Context, id string, expected int64, _, _ string, _ map[string]any, change func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assay.ID != id {
		return nil, fmt.Errorf("批次不存在")
	}
	if r.assay.Revision != expected {
		return nil, domain.ConflictError{Expected: expected, Current: r.assay.Revision}
	}
	candidate := cloneAssay(r.assay)
	if err := change(candidate); err != nil {
		return nil, err
	}
	candidate.Revision++
	r.assay = candidate
	return cloneAssay(candidate), nil
}

func cloneAssay(source *domain.GerminationAssay) *domain.GerminationAssay {
	copyOfAssay := *source
	copyOfAssay.Observations = append([]domain.DailyObservation(nil), source.Observations...)
	copyOfAssay.Deviations = append([]domain.DeviationCase(nil), source.Deviations...)
	copyOfAssay.Reviews = append([]domain.ReviewRecord(nil), source.Reviews...)
	copyOfAssay.AuditTrail = append([]domain.AuditEvent(nil), source.AuditTrail...)
	return &copyOfAssay
}

func TestConcurrentReadMustNotReplaceNewerCachedRevision(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	assay, err := domain.NewAssay("assay-cache", "ACC-CACHE", "LAB-CACHE", "检验员", "复核员", domain.AssayProtocol{
		TemperatureCelsius: 25,
		Substrate:          "湿润滤纸",
		LightCycleHours:    12,
		ObservationDays:    2,
		ReplicateCount:     2,
		SeedsPerReplicate:  10,
		DispersionLimit:    0.2,
		NormalSeedlingRule: "根系与胚轴完整",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	repository := &controlledRepository{
		assay:      assay,
		getStarted: make(chan struct{}),
		releaseGet: make(chan struct{}),
	}
	service := application.NewService(repository, func(prefix string) string { return prefix + "-id" })

	type readResult struct {
		view *application.AssayView
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		view, readErr := service.GetAssay(context.Background(), assay.ID)
		readDone <- readResult{view: view, err: readErr}
	}()
	<-repository.getStarted

	updated, err := service.FreezeProtocol(context.Background(), assay.ID, application.RevisionCommand{
		ExpectedRevision: assay.Revision,
		Principal:        application.Principal{Name: "检验员", Role: application.RoleOperator},
	})
	if err != nil {
		t.Fatal(err)
	}
	close(repository.releaseGet)
	initialRead := <-readDone
	if initialRead.err != nil {
		t.Fatal(initialRead.err)
	}
	if initialRead.view.Assay.Revision != assay.Revision {
		t.Fatalf("受控并发读取应捕获 revision %d，得到 %d", assay.Revision, initialRead.view.Assay.Revision)
	}

	cached, err := service.GetAssay(context.Background(), assay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Assay.Revision != updated.Assay.Revision || cached.Assay.State != domain.StateObserving {
		t.Fatalf("并发旧读取覆盖了已提交投影：缓存 state=%s revision=%d，已提交 state=%s revision=%d",
			cached.Assay.State, cached.Assay.Revision, updated.Assay.State, updated.Assay.Revision)
	}
}
