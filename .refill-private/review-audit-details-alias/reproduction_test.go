package reviewauditdetailsalias_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

type scheduledRepository struct {
	returnEntered chan struct{}
	releaseReturn chan struct{}

	mu              sync.Mutex
	returnedDetails map[string]any
	approvedDetails map[string]any
}

func newScheduledRepository() *scheduledRepository {
	return &scheduledRepository{
		returnEntered: make(chan struct{}),
		releaseReturn: make(chan struct{}),
	}
}

func (r *scheduledRepository) Create(context.Context, *domain.GerminationAssay, string) error {
	return nil
}

func (r *scheduledRepository) Get(context.Context, string) (*domain.GerminationAssay, error) {
	return nil, fmt.Errorf("unexpected Get")
}

func (r *scheduledRepository) List(context.Context) ([]domain.GerminationAssay, error) {
	return nil, fmt.Errorf("unexpected List")
}

func (r *scheduledRepository) Update(_ context.Context, id string, _ int64, action, _ string, details map[string]any, _ func(*domain.GerminationAssay) error) (*domain.GerminationAssay, error) {
	switch action {
	case "review.returned":
		close(r.returnEntered)
		<-r.releaseReturn
		r.mu.Lock()
		r.returnedDetails = cloneDetails(details)
		r.mu.Unlock()
	case "report.archived":
		r.mu.Lock()
		r.approvedDetails = cloneDetails(details)
		r.mu.Unlock()
		close(r.releaseReturn)
	default:
		return nil, fmt.Errorf("unexpected action %q", action)
	}
	return &domain.GerminationAssay{ID: id, ProtocolVersion: "SVP-1.0"}, nil
}

func cloneDetails(details map[string]any) map[string]any {
	copy := make(map[string]any, len(details))
	for key, value := range details {
		copy[key] = value
	}
	return copy
}

func TestConcurrentReviewsKeepIndependentAuditDetails(t *testing.T) {
	repository := newScheduledRepository()
	service := application.NewService(repository, func(prefix string) string { return prefix + "-1" })

	returned := application.ReviewCommand{
		ExpectedRevision: 7,
		Reviewer:         "复核员甲",
		Checklist:        []domain.ReviewChecklistItem{{Code: "readings", Status: "returned"}},
		CorrectionScope:  []domain.CorrectionTarget{{Type: "section", Section: "readings"}},
	}
	approved := application.ReviewCommand{
		ExpectedRevision: 11,
		Reviewer:         "复核员乙",
		Checklist:        []domain.ReviewChecklistItem{{Code: "protocol", Status: "approved"}},
	}

	returnErr := make(chan error, 1)
	go func() {
		_, err := service.ReturnReview(context.Background(), "assay-returned", returned)
		returnErr <- err
	}()
	<-repository.returnEntered

	approveErr := make(chan error, 1)
	go func() {
		_, err := service.ApproveAndArchive(context.Background(), "assay-approved", approved)
		approveErr <- err
	}()

	if err := <-approveErr; err != nil {
		t.Fatalf("approve request failed: %v", err)
	}
	if err := <-returnErr; err != nil {
		t.Fatalf("return request failed: %v", err)
	}

	repository.mu.Lock()
	got := cloneDetails(repository.returnedDetails)
	repository.mu.Unlock()
	if _, ok := got["correction_scope"]; !ok {
		t.Fatalf("returned review lost correction_scope after concurrent approval rewrote audit details: %#v", got)
	}
	if _, contaminated := got["material_revision"]; contaminated {
		t.Fatalf("returned review received approval material_revision: %#v", got)
	}
}
