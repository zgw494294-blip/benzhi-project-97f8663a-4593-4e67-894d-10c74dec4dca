package archive_observation_order_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
)

func TestArchiveKeepsLatestObservationVersion(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	frozenAt := now.Add(-4 * time.Hour)
	assay := &domain.GerminationAssay{
		ID: "assay-archive-order", SampleAccession: "ACC-ORDER", LaboratoryBatchNo: "LAB-ORDER",
		State: domain.StateReview, OperatorName: "检验员", ReviewerName: "复核员",
		ProtocolVersion: "SVP-1.0", Revision: 7, CreatedAt: frozenAt, UpdatedAt: now,
		Protocol: domain.AssayProtocol{TemperatureCelsius: 25, Substrate: "滤纸", LightCycleHours: 12,
			ObservationDays: 1, ReplicateCount: 2, SeedsPerReplicate: 10, DispersionLimit: 0.2,
			NormalSeedlingRule: "完整幼苗", FrozenAt: &frozenAt},
		Observations: []domain.DailyObservation{
			{ID: "obs-old", AssayID: "assay-archive-order", ReplicateNo: 1, DayNo: 1, NormalCount: 4,
				UngerminatedCount: 6, RecordedBy: "检验员", RecordedAt: now.Add(-2 * time.Hour)},
			{ID: "obs-new", AssayID: "assay-archive-order", ReplicateNo: 1, DayNo: 1, NormalCount: 9,
				UngerminatedCount: 1, RecordedBy: "检验员", RecordedAt: now.Add(-time.Hour), SupersedesID: "obs-old"},
			{ID: "obs-replicate-2", AssayID: "assay-archive-order", ReplicateNo: 2, DayNo: 1, NormalCount: 9,
				UngerminatedCount: 1, RecordedBy: "检验员", RecordedAt: now.Add(-time.Hour)},
		},
		ReviewChecklist: domain.DefaultReviewChecklist(), ReviewMaterialRevision: 7,
	}
	if err := store.Create(context.Background(), assay, "检验员"); err != nil {
		t.Fatal(err)
	}

	service := application.NewService(store, persistence.NewID)
	checklist := domain.DefaultReviewChecklist()
	for index := range checklist {
		checklist[index].Status = "passed"
	}
	view, err := service.ApproveAndArchive(context.Background(), assay.ID, application.ReviewCommand{
		ExpectedRevision: 7, Reviewer: "复核员", Opinion: "批准补测结论", Checklist: checklist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Assay.Report == nil {
		t.Fatal("批准后缺少归档报告")
	}
	if got := view.Assay.Report.MetricSnapshot.CumulativeRate; math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("归档采用了被取代的旧读数：cumulative_rate=%v，期望 0.9", got)
	}
}
