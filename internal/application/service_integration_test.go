package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
)

func newHarness(t *testing.T) (*application.Service, *persistence.Store) {
	t.Helper()
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return application.NewService(store, persistence.NewID), store
}

func createCommand(sample string) application.CreateAssayCommand {
	return application.CreateAssayCommand{SampleAccession: sample, LaboratoryBatchNo: "LAB-1", OperatorName: "检验员", ReviewerName: "复核员",
		Protocol: domain.AssayProtocol{TemperatureCelsius: 25, Substrate: "滤纸", LightCycleHours: 12, ObservationDays: 2,
			ReplicateCount: 2, SeedsPerReplicate: 10, DispersionLimit: 0.2, NormalSeedlingRule: "完整幼苗"}}
}

func TestCompleteArchiveFlowAndRevisionConflict(t *testing.T) {
	service, store := newHarness(t)
	ctx := context.Background()
	assay, err := service.CreateAssay(ctx, createCommand("ACC-1"))
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.FreezeProtocol(ctx, assay.ID, application.RevisionCommand{ExpectedRevision: assay.Revision,
		Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RecordObservation(ctx, assay.ID, application.ObservationCommand{ExpectedRevision: assay.Revision,
		ReplicateNo: 1, DayNo: 1, NormalCount: 7, UngerminatedCount: 3, RecordedBy: "检验员"})
	var conflict domain.ConflictError
	if !errors.As(err, &conflict) || conflict.Current != view.Assay.Revision {
		t.Fatalf("预期 revision 冲突，得到 %v", err)
	}
	for day := 1; day <= 2; day++ {
		for rep := 1; rep <= 2; rep++ {
			view, err = service.RecordObservation(ctx, assay.ID, application.ObservationCommand{ExpectedRevision: view.Assay.Revision,
				ReplicateNo: rep, DayNo: day, NormalCount: 5 + day, UngerminatedCount: 5 - day, RecordedBy: "检验员"})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	view, err = service.SealObservation(ctx, assay.ID, application.RevisionCommand{ExpectedRevision: view.Assay.Revision,
		Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Assay.State != domain.StateReview {
		t.Fatalf("封存后状态 = %s", view.Assay.State)
	}
	checklist := domain.DefaultReviewChecklist()
	for index := range checklist {
		checklist[index].Status = "passed"
	}
	view, err = service.ApproveAndArchive(ctx, assay.ID, application.ReviewCommand{ExpectedRevision: view.Assay.Revision, Reviewer: "复核员", Opinion: "批准", Checklist: checklist})
	if err != nil {
		t.Fatal(err)
	}
	if view.Assay.State != domain.StateArchived || view.Assay.Report == nil || !view.ReportConsistent {
		t.Fatalf("归档视图异常: %+v", view)
	}
	reloaded, err := store.Get(ctx, assay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.VerifyReport(reloaded, reloaded.Report) {
		t.Fatal("重载后报告摘要校验失败")
	}
}

func TestDuplicateSampleAndMissingDeviationPersistence(t *testing.T) {
	service, _ := newHarness(t)
	ctx := context.Background()
	first, err := service.CreateAssay(ctx, createCommand("ACC-DUP"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAssay(ctx, createCommand("ACC-DUP")); err == nil {
		t.Fatal("同批次重复样本应被拒绝")
	}
	view, err := service.FreezeProtocol(ctx, first.ID, application.RevisionCommand{ExpectedRevision: first.Revision,
		Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.SealObservation(ctx, first.ID, application.RevisionCommand{ExpectedRevision: view.Assay.Revision,
		Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Assay.State != domain.StateObserving {
		t.Fatalf("缺失观察不得封存，状态 = %s", view.Assay.State)
	}
	if domain.OpenDeviationCount(view.Assay.Deviations) != 4 {
		t.Fatalf("应持久化 4 个缺失异常，得到 %+v", view.Assay.Deviations)
	}
}
