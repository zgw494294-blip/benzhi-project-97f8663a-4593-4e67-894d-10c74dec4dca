package failedwritecache_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"seed-vigor-workbench/internal/domain"
	"seed-vigor-workbench/internal/persistence"
)

func TestFailedWriteDoesNotPoisonTransactionCache(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "failed-write.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	protocol := domain.AssayProtocol{
		TemperatureCelsius: 25,
		Substrate:          "滤纸",
		LightCycleHours:    12,
		ObservationDays:    2,
		ReplicateCount:     2,
		SeedsPerReplicate:  10,
		DispersionLimit:    0.2,
		NormalSeedlingRule: "完整幼苗",
	}
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	first, err := domain.NewAssay("assay-first", "ACC-A", "LOT-1", "检验员", "复核员", protocol, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewAssay("assay-second", "ACC-B", "LOT-1", "检验员", "复核员", protocol, now)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Create(ctx, first, "检验员"); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, second, "检验员"); err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(ctx, second.ID, second.Revision, "assay.draft_revised", "检验员", nil,
		func(assay *domain.GerminationAssay) error {
			return assay.ReplaceDraft("ACC-A", "LOT-1", "检验员", "复核员", protocol, now.Add(time.Minute))
		})
	if err == nil {
		t.Fatal("制造唯一约束冲突的事务应当回滚")
	}
	persisted, err := store.Get(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != second.Revision || persisted.SampleAccession != "ACC-B" {
		t.Fatalf("SQLite 回滚后的记录异常: revision=%d sample=%s", persisted.Revision, persisted.SampleAccession)
	}

	_, err = store.Update(ctx, second.ID, persisted.Revision, "assay.draft_revised", "检验员", nil,
		func(assay *domain.GerminationAssay) error {
			return assay.ReplaceDraft("ACC-C", "LOT-1", "检验员", "复核员", protocol, now.Add(2*time.Minute))
		})
	if err != nil {
		t.Fatalf("回滚后按持久层 revision 重试应成功，得到: %v", err)
	}
}
