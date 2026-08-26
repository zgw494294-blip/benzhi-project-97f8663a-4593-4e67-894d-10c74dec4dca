package application_test

import (
	"context"
	"errors"
	"testing"

	"seed-vigor-workbench/internal/application"
	"seed-vigor-workbench/internal/domain"
)

func passChecklist() []domain.ReviewChecklistItem {
	items := domain.DefaultReviewChecklist()
	for index := range items {
		items[index].Status = "passed"
	}
	return items
}

func freezeAssay(t *testing.T, service *application.Service, sample string) *application.AssayView {
	t.Helper()
	created, err := service.CreateAssay(context.Background(), createCommand(sample))
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.FreezeProtocol(context.Background(), created.ID, application.RevisionCommand{ExpectedRevision: created.Revision, Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func daily(revision int64, day, firstNormal, secondNormal int) application.DailyObservationCommand {
	return application.DailyObservationCommand{ExpectedRevision: revision, DayNo: day, RecordedBy: "检验员", Observations: []application.ObservationReading{
		{ReplicateNo: 1, NormalCount: firstNormal, UngerminatedCount: 10 - firstNormal},
		{ReplicateNo: 2, NormalCount: secondNormal, UngerminatedCount: 10 - secondNormal},
	}}
}

func TestDraftReadinessRevisionAndFreezeBoundary(t *testing.T) {
	service, _ := newHarness(t)
	ctx := context.Background()
	command := createCommand("DRAFT-1")
	command.Protocol.TemperatureCelsius = 50
	command.Protocol.NormalSeedlingRule = ""
	created, err := service.CreateAssay(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := service.DraftReadiness(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || len(readiness.Issues) != 2 {
		t.Fatalf("应同时返回温度和规则错误: %+v", readiness)
	}
	valid := createCommand("DRAFT-1")
	view, err := service.ReviseDraft(ctx, created.ID, application.ReviseDraftCommand{ExpectedRevision: created.Revision, SampleAccession: valid.SampleAccession, LaboratoryBatchNo: valid.LaboratoryBatchNo, OperatorName: valid.OperatorName, ReviewerName: valid.ReviewerName, Protocol: valid.Protocol, Actor: "检验员"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Assay.Revision != created.Revision+1 {
		t.Fatalf("草稿修订应只增加一个 revision: %d", view.Assay.Revision)
	}
	_, err = service.ReviseDraft(ctx, created.ID, application.ReviseDraftCommand{ExpectedRevision: created.Revision, SampleAccession: "STALE", LaboratoryBatchNo: valid.LaboratoryBatchNo, OperatorName: valid.OperatorName, ReviewerName: valid.ReviewerName, Protocol: valid.Protocol, Actor: "检验员"})
	var conflict domain.ConflictError
	if !errors.As(err, &conflict) || conflict.Current != view.Assay.Revision {
		t.Fatalf("陈旧草稿应返回当前 revision: %v", err)
	}
	view, err = service.FreezeProtocol(ctx, created.ID, application.RevisionCommand{ExpectedRevision: view.Assay.Revision, Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ReviseDraft(ctx, created.ID, application.ReviseDraftCommand{ExpectedRevision: view.Assay.Revision, SampleAccession: valid.SampleAccession, LaboratoryBatchNo: valid.LaboratoryBatchNo, OperatorName: valid.OperatorName, ReviewerName: valid.ReviewerName, Protocol: valid.Protocol, Actor: "检验员"})
	if err == nil {
		t.Fatal("冻结后不得修订草稿")
	}
}

func TestDailyBatchAtomicityAndTimelineRevalidation(t *testing.T) {
	service, store := newHarness(t)
	ctx := context.Background()
	view := freezeAssay(t, service, "DAY-1")
	bad := daily(view.Assay.Revision, 1, 6, 6)
	bad.Observations[1].UngerminatedCount = 3
	if _, err := service.RecordDailyObservations(ctx, view.Assay.ID, bad); err == nil {
		t.Fatal("任一组不守恒时整日应拒绝")
	}
	reloaded, err := store.Get(ctx, view.Assay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Revision != view.Assay.Revision || len(reloaded.Observations) != 0 {
		t.Fatalf("失败提交不得留下部分版本: revision=%d observations=%d", reloaded.Revision, len(reloaded.Observations))
	}
	view, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 6, 6))
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 2, 8, 8))
	if err != nil {
		t.Fatal(err)
	}
	before := len(view.Assay.Observations)
	if _, err := service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 9, 9)); err == nil {
		t.Fatal("更正早日后导致后续正常幼苗倒退时应拒绝")
	}
	reloaded, _ = store.Get(ctx, view.Assay.ID)
	if len(reloaded.Observations) != before || reloaded.Revision != view.Assay.Revision {
		t.Fatal("时序冲突不得改变版本链或 revision")
	}
	view, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 2, 9, 9))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 9, 9)); err != nil {
		t.Fatal(err)
	}
}

func TestDeviationEvidenceCloseAndRecurrenceHistory(t *testing.T) {
	service, store := newHarness(t)
	ctx := context.Background()
	view := freezeAssay(t, service, "DEV-1")
	view, err := service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 2, 9))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Assay.Deviations) != 1 || view.Assay.Deviations[0].Occurrence != 1 {
		t.Fatalf("应打开第一次离散度异常: %+v", view.Assay.Deviations)
	}
	first := view.Assay.Deviations[0]
	view, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 5, 5))
	if err != nil {
		t.Fatal(err)
	}
	evidence := []string{}
	for _, observation := range view.CurrentReadings {
		evidence = append(evidence, observation.ID)
	}
	view, err = service.ResolveDeviation(ctx, view.Assay.ID, first.ID, application.ResolveDeviationCommand{ExpectedRevision: view.Assay.Revision, Actor: "检验员", Reason: "重复组培养条件不均", CorrectiveAction: "统一条件后补测", EvidenceIDs: evidence})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.RecordDailyObservations(ctx, view.Assay.ID, daily(view.Assay.Revision, 1, 2, 9))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Assay.Deviations) != 2 || view.Assay.Deviations[0].Status != domain.DeviationClosed || view.Assay.Deviations[1].Occurrence != 2 || view.Assay.Deviations[1].Status != domain.DeviationOpen {
		t.Fatalf("复发应保留第一次闭环并生成第二次发生: %+v", view.Assay.Deviations)
	}
	reloaded, err := store.Get(ctx, view.Assay.ID)
	if err != nil || len(reloaded.Deviations) != 2 || reloaded.Deviations[0].ClosedAt == nil {
		t.Fatalf("异常历史持久化不完整: %v %+v", err, reloaded.Deviations)
	}
}

func TestReviewChecklistAndStructuredCorrectionScope(t *testing.T) {
	service, _ := newHarness(t)
	ctx := context.Background()
	command := createCommand("REVIEW-1")
	command.Protocol.ObservationDays = 1
	created, err := service.CreateAssay(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	view, err := service.FreezeProtocol(ctx, created.ID, application.RevisionCommand{ExpectedRevision: created.Revision, Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.RecordDailyObservations(ctx, created.ID, daily(view.Assay.Revision, 1, 5, 5))
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.SealObservation(ctx, created.ID, application.RevisionCommand{ExpectedRevision: view.Assay.Revision, Principal: application.Principal{Name: "检验员", Role: application.RoleOperator}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApproveAndArchive(ctx, created.ID, application.ReviewCommand{ExpectedRevision: view.Assay.Revision, Reviewer: "复核员"}); err == nil {
		t.Fatal("清单未完成不得批准")
	}
	checklist := passChecklist()
	for index := range checklist {
		if checklist[index].Code == "readings" {
			checklist[index].Status = "returned"
			checklist[index].Opinion = "第一重复组需补测"
		}
	}
	view, err = service.ReturnReview(ctx, created.ID, application.ReviewCommand{ExpectedRevision: view.Assay.Revision, Reviewer: "复核员", Opinion: "定向补测", Checklist: checklist, CorrectionScope: []domain.CorrectionTarget{{Type: "observation", DayNo: 1, ReplicateNo: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.RecordObservation(ctx, created.ID, application.ObservationCommand{ExpectedRevision: view.Assay.Revision, DayNo: 1, ReplicateNo: 2, NormalCount: 6, UngerminatedCount: 4, RecordedBy: "检验员"}); err == nil {
		t.Fatal("范围外观察读数不得修改")
	}
	view, err = service.RecordObservation(ctx, created.ID, application.ObservationCommand{ExpectedRevision: view.Assay.Revision, DayNo: 1, ReplicateNo: 1, NormalCount: 6, UngerminatedCount: 4, RecordedBy: "检验员"})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.ResubmitReview(ctx, created.ID, application.ResubmitCommand{ExpectedRevision: view.Assay.Revision, Operator: "检验员", Opinion: "完成定向补测"})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Assay.Reviews) != 2 || len(view.Assay.Reviews[1].Difference.NewObservationIDs) != 1 {
		t.Fatalf("重提差异摘要不完整: %+v", view.Assay.Reviews)
	}
	view, err = service.ApproveAndArchive(ctx, created.ID, application.ReviewCommand{ExpectedRevision: view.Assay.Revision, Reviewer: "复核员", Opinion: "批准", Checklist: passChecklist()})
	if err != nil || view.Assay.State != domain.StateArchived {
		t.Fatalf("清单通过后应归档: %v", err)
	}
}
