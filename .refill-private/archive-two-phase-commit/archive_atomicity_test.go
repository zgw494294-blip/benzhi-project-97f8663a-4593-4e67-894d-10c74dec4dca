package archive_two_phase_commit_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

type cancelAfterCommitRepository struct {
	assay     *domain.GerminationAssay
	cancel    context.CancelFunc
	didCancel bool
}

func (r *cancelAfterCommitRepository) Create(context.Context, *domain.GerminationAssay, string) error {
	return errors.New("unexpected Create")
}

func (r *cancelAfterCommitRepository) Get(ctx context.Context, id string) (*domain.GerminationAssay, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.assay.ID != id {
		return nil, errors.New("not found")
	}
	copy := *r.assay
	return &copy, nil
}

func (r *cancelAfterCommitRepository) List(context.Context) ([]domain.GerminationAssay, error) {
	return nil, errors.New("unexpected List")
}

func (r *cancelAfterCommitRepository) Update(
	ctx context.Context,
	id string,
	expected int64,
	action string,
	actor string,
	details map[string]any,
	change func(*domain.GerminationAssay) error,
) (*domain.GerminationAssay, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.assay.ID != id {
		return nil, errors.New("not found")
	}
	if r.assay.Revision != expected {
		return nil, domain.ConflictError{Expected: expected, Current: r.assay.Revision}
	}
	candidate := *r.assay
	candidate.Reviews = append([]domain.ReviewRecord(nil), r.assay.Reviews...)
	if err := change(&candidate); err != nil {
		return nil, err
	}
	candidate.Revision++
	r.assay = &candidate
	if !r.didCancel {
		r.didCancel = true
		r.cancel()
	}
	result := candidate
	return &result, nil
}

func TestArchiveCancellationRollsBackApproval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repository := &cancelAfterCommitRepository{
		cancel: cancel,
		assay: &domain.GerminationAssay{
			ID: "assay-atomic", State: domain.StateReview, Revision: 7,
			ReviewerName: "复核员", OperatorName: "检验员",
		},
	}
	nextID := 0
	service := application.NewService(repository, func(prefix string) string {
		nextID++
		return fmt.Sprintf("%s-%d", prefix, nextID)
	})
	checklist := domain.DefaultReviewChecklist()
	for index := range checklist {
		checklist[index].Status = "passed"
	}

	_, err := service.ApproveAndArchive(ctx, repository.assay.ID, application.ReviewCommand{
		ExpectedRevision: 7,
		Reviewer:         "复核员",
		Opinion:          "批准归档",
		Checklist:        checklist,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("预期归档阶段观察到 context canceled，得到 %v", err)
	}

	persisted := repository.assay
	if persisted.State != domain.StateReview || persisted.Revision != 7 || len(persisted.Reviews) != 0 || persisted.Report != nil {
		t.Fatalf("归档失败必须回滚批准事务，实际 state=%s revision=%d reviews=%d report=%v",
			persisted.State, persisted.Revision, len(persisted.Reviews), persisted.Report)
	}
}
