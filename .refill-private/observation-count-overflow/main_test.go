package observation_count_overflow_test

import (
	"context"
	"path/filepath"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
)

func TestObservationCountOverflowRejected(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := application.NewService(store, persistence.NewID)
	ctx := context.Background()
	created, err := service.CreateAssay(ctx, application.CreateAssayCommand{
		SampleAccession: "S-1", LaboratoryBatchNo: "B-1", OperatorName: "检验员", ReviewerName: "复核员",
		Protocol: domain.AssayProtocol{TemperatureCelsius: 25, Substrate: "滤纸", LightCycleHours: 12,
			ObservationDays: 1, ReplicateCount: 2, SeedsPerReplicate: 10, DispersionLimit: 0.2, NormalSeedlingRule: "完整幼苗"},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.FreezeProtocol(ctx, created.ID, application.RevisionCommand{ExpectedRevision: created.Revision,
		Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	maxInt := int(^uint(0) >> 1)
	_, err = service.RecordObservation(ctx, created.ID, application.ObservationCommand{
		ExpectedRevision: view.Assay.Revision, DayNo: 1, ReplicateNo: 1, RecordedBy: "检验员",
		NormalCount: maxInt, AbnormalCount: maxInt, HardSeedCount: 12,
	})
	if err == nil {
		t.Fatal("TestObservationCountOverflowRejected: 溢出后的伪守恒分类计数被事务提交")
	}
}
