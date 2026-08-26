package domain

import (
	"errors"
	"testing"
	"time"
)

func testProtocol() AssayProtocol {
	return AssayProtocol{TemperatureCelsius: 25, Substrate: "滤纸", LightCycleHours: 12,
		ObservationDays: 2, ReplicateCount: 2, SeedsPerReplicate: 10,
		DispersionLimit: 0.2, NormalSeedlingRule: "根系与胚轴完整"}
}

func TestObservationConservationAndMetrics(t *testing.T) {
	p := testProtocol()
	bad := DailyObservation{ReplicateNo: 1, DayNo: 1, NormalCount: 7, UngerminatedCount: 2, RecordedBy: "甲"}
	var validation ValidationError
	if err := bad.Validate(p); !errors.As(err, &validation) || validation.Field != "counts" {
		t.Fatalf("预期计数守恒错误，得到 %v", err)
	}
	observations := []DailyObservation{
		{ID: "1", ReplicateNo: 1, DayNo: 1, NormalCount: 6, UngerminatedCount: 4, RecordedAt: time.Unix(1, 0)},
		{ID: "2", ReplicateNo: 2, DayNo: 1, NormalCount: 8, UngerminatedCount: 2, RecordedAt: time.Unix(1, 0)},
		{ID: "3", ReplicateNo: 1, DayNo: 2, NormalCount: 8, UngerminatedCount: 2, RecordedAt: time.Unix(2, 0)},
		{ID: "4", ReplicateNo: 2, DayNo: 2, NormalCount: 9, UngerminatedCount: 1, RecordedAt: time.Unix(2, 0)},
	}
	metrics := CalculateMetrics(p, observations)
	if !metrics.CompleteObservation {
		t.Fatal("完整读数应被识别为完整")
	}
	if metrics.CumulativeRate != 0.85 {
		t.Fatalf("累计发芽率 = %v", metrics.CumulativeRate)
	}
	if metrics.GerminationVigor != 0.7 {
		t.Fatalf("发芽势 = %v", metrics.GerminationVigor)
	}
	if metrics.MaxDispersion < 0.199 || metrics.MaxDispersion > 0.201 {
		t.Fatalf("离散度 = %v", metrics.MaxDispersion)
	}
}

func TestStateMachineRejectsIllegalTransition(t *testing.T) {
	now := time.Now()
	assay, err := NewAssay("a", "s", "b", "检验员", "复核员", testProtocol(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := assay.Transition(StateReview, now); err == nil {
		t.Fatal("草稿不得直接进入复核")
	}
	if err := assay.Freeze(now); err != nil {
		t.Fatal(err)
	}
	if assay.State != StateObserving || assay.Protocol.FrozenAt == nil {
		t.Fatalf("冻结状态异常: %+v", assay)
	}
}

func TestDeviationRules(t *testing.T) {
	p := testProtocol()
	observations := []DailyObservation{
		{ID: "1", ReplicateNo: 1, DayNo: 1, NormalCount: 2, UngerminatedCount: 8},
		{ID: "2", ReplicateNo: 2, DayNo: 1, NormalCount: 9, UngerminatedCount: 1},
	}
	findings := EvaluateFindings(p, observations, true)
	if len(findings) != 3 {
		t.Fatalf("预期 2 个缺失项和 1 个离散项，得到 %+v", findings)
	}
}
