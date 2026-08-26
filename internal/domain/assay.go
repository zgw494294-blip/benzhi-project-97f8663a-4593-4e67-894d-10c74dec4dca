package domain

import (
	"fmt"
	"strings"
	"time"
)

type GerminationAssay struct {
	ID                     string                `json:"id"`
	SampleAccession        string                `json:"sample_accession"`
	LaboratoryBatchNo      string                `json:"laboratory_batch_no"`
	State                  AssayState            `json:"state"`
	OperatorName           string                `json:"operator_name"`
	ReviewerName           string                `json:"reviewer_name"`
	ProtocolVersion        string                `json:"protocol_version"`
	Revision               int64                 `json:"revision"`
	CreatedAt              time.Time             `json:"created_at"`
	UpdatedAt              time.Time             `json:"updated_at"`
	Protocol               AssayProtocol         `json:"protocol"`
	Observations           []DailyObservation    `json:"observations"`
	Deviations             []DeviationCase       `json:"deviations"`
	Reviews                []ReviewRecord        `json:"reviews"`
	ReviewChecklist        []ReviewChecklistItem `json:"review_checklist,omitempty"`
	ReviewMaterialRevision int64                 `json:"review_material_revision,omitempty"`
	AuditTrail             []AuditEvent          `json:"audit_trail"`
	Report                 *ArchivedReport       `json:"report,omitempty"`
}

func NewAssay(id, sample, batch, operator, reviewer string, protocol AssayProtocol, now time.Time) (*GerminationAssay, error) {
	a := &GerminationAssay{
		ID: strings.TrimSpace(id), SampleAccession: strings.TrimSpace(sample),
		LaboratoryBatchNo: strings.TrimSpace(batch), OperatorName: strings.TrimSpace(operator),
		ReviewerName: strings.TrimSpace(reviewer), ProtocolVersion: "SVP-1.0",
		State: StateDraft, Revision: 1, CreatedAt: now.UTC(), UpdatedAt: now.UTC(), Protocol: protocol,
	}
	if err := a.ValidateIdentity(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *GerminationAssay) ValidateIdentity() error {
	issues := a.IdentityIssues()
	if len(issues) > 0 {
		return ValidationErrors{Issues: issues}
	}
	return nil
}

func (a *GerminationAssay) IdentityIssues() []ValidationError {
	issues := make([]ValidationError, 0)
	if a.ID == "" {
		issues = append(issues, ValidationError{Field: "id", Message: "批次 ID 不能为空"})
	}
	if a.SampleAccession == "" {
		issues = append(issues, ValidationError{Field: "sample_accession", Message: "样本标识不能为空"})
	}
	if a.LaboratoryBatchNo == "" {
		issues = append(issues, ValidationError{Field: "laboratory_batch_no", Message: "检验批次号不能为空"})
	}
	if a.OperatorName == "" {
		issues = append(issues, ValidationError{Field: "operator_name", Message: "责任检验员不能为空"})
	}
	if a.ReviewerName == "" {
		issues = append(issues, ValidationError{Field: "reviewer_name", Message: "复核员不能为空"})
	}
	if a.OperatorName != "" && a.OperatorName == a.ReviewerName {
		issues = append(issues, ValidationError{Field: "reviewer_name", Message: "复核员不得与检验员相同"})
	}
	return issues
}

type ReadinessPreview struct {
	TotalSeeds       int `json:"total_seeds"`
	ObservationUnits int `json:"observation_units"`
}

type ReadinessResult struct {
	Ready    bool              `json:"ready"`
	Revision int64             `json:"revision"`
	Issues   []ValidationError `json:"issues"`
	Preview  ReadinessPreview  `json:"preview"`
}

func (a *GerminationAssay) Readiness() ReadinessResult {
	issues := append([]ValidationError{}, a.IdentityIssues()...)
	issues = append(issues, a.Protocol.ValidationIssues()...)
	return ReadinessResult{Ready: len(issues) == 0, Revision: a.Revision, Issues: issues,
		Preview: ReadinessPreview{TotalSeeds: a.Protocol.TotalSeeds(), ObservationUnits: a.Protocol.ObservationUnits()}}
}

func (a *GerminationAssay) ReplaceDraft(sample, batch, operator, reviewer string, protocol AssayProtocol, now time.Time) error {
	if a.State != StateDraft {
		return invalid("state", "批次离开草稿状态后不可修订")
	}
	candidate := *a
	candidate.SampleAccession = strings.TrimSpace(sample)
	candidate.LaboratoryBatchNo = strings.TrimSpace(batch)
	candidate.OperatorName = strings.TrimSpace(operator)
	candidate.ReviewerName = strings.TrimSpace(reviewer)
	protocol.FrozenAt = nil
	candidate.Protocol = protocol
	ready := candidate.Readiness()
	if !ready.Ready {
		return ValidationErrors{Issues: ready.Issues}
	}
	a.SampleAccession = candidate.SampleAccession
	a.LaboratoryBatchNo = candidate.LaboratoryBatchNo
	a.OperatorName = candidate.OperatorName
	a.ReviewerName = candidate.ReviewerName
	a.Protocol = candidate.Protocol
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *GerminationAssay) Transition(next AssayState, now time.Time) error {
	if !a.State.CanTransition(next) {
		return invalid("state", fmt.Sprintf("不允许从 %s 转换为 %s", a.State, next))
	}
	a.State = next
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *GerminationAssay) Freeze(now time.Time) error {
	if a.State != StateDraft {
		return invalid("state", "只有草稿批次可冻结方案")
	}
	ready := a.Readiness()
	if !ready.Ready {
		return ValidationErrors{Issues: ready.Issues}
	}
	when := now.UTC()
	a.Protocol.FrozenAt = &when
	if err := a.Transition(StateFrozen, now); err != nil {
		return err
	}
	return a.Transition(StateObserving, now)
}

func (a *GerminationAssay) AddObservation(observation DailyObservation, now time.Time) error {
	if a.State != StateObserving && a.State != StateCorrection {
		return invalid("state", "当前状态不可登记观察")
	}
	if err := observation.Validate(a.Protocol); err != nil {
		return err
	}
	observation.AssayID = a.ID
	current := CurrentObservations(a.Observations)
	if previous, ok := current[ObservationKey(observation.ReplicateNo, observation.DayNo)]; ok {
		observation.SupersedesID = previous.ID
	}
	observation.RecordedAt = now.UTC()
	candidate := append(append([]DailyObservation{}, a.Observations...), observation)
	if issues := ValidateObservationTimeline(a.Protocol, candidate); len(issues) > 0 {
		return ValidationErrors{Issues: issues}
	}
	a.Observations = candidate
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *GerminationAssay) AddObservationBatch(day int, observations []DailyObservation, now time.Time) error {
	if a.State != StateObserving && a.State != StateCorrection {
		return invalid("state", "当前状态不可登记观察")
	}
	issues := ValidateDailyBatch(a.Protocol, day, observations)
	if len(issues) > 0 {
		return ValidationErrors{Issues: issues}
	}
	current := CurrentObservations(a.Observations)
	candidate := append([]DailyObservation{}, a.Observations...)
	when := now.UTC()
	for index := range observations {
		item := observations[index]
		item.AssayID = a.ID
		if previous, ok := current[ObservationKey(item.ReplicateNo, day)]; ok {
			item.SupersedesID = previous.ID
		}
		item.RecordedAt = when
		observations[index] = item
		candidate = append(candidate, item)
	}
	if temporal := ValidateObservationTimeline(a.Protocol, candidate); len(temporal) > 0 {
		return ValidationErrors{Issues: temporal}
	}
	a.Observations = candidate
	a.UpdatedAt = when
	return nil
}

func (a *GerminationAssay) Seal(now time.Time) error {
	if a.State != StateObserving {
		return invalid("state", "只有观察中批次可封存")
	}
	metrics := CalculateMetrics(a.Protocol, a.Observations)
	if !metrics.CompleteObservation {
		return invalid("observations", "观察窗口尚有缺失读数")
	}
	if OpenDeviationCount(a.Deviations) > 0 {
		return invalid("deviations", "仍有未关闭异常")
	}
	if len(EvaluateFindings(a.Protocol, a.Observations, true)) > 0 {
		return invalid("deviations", "规则复验仍存在异常")
	}
	if err := a.Transition(StateSealed, now); err != nil {
		return err
	}
	if err := a.Transition(StateReview, now); err != nil {
		return err
	}
	a.ReviewChecklist = DefaultReviewChecklist()
	a.ReviewMaterialRevision = a.Revision + 1
	return nil
}

func (a *GerminationAssay) ReturnForCorrection(record ReviewRecord, now time.Time) error {
	if a.State != StateReview {
		return invalid("state", "只有待复核批次可退回")
	}
	if record.Decision != DecisionReturned {
		return invalid("decision", "复核决定必须为 returned")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if err := ValidateChecklist(record.Checklist, false); err != nil {
		return err
	}
	record.Version = len(a.Reviews) + 1
	record.AssayID = a.ID
	record.CreatedAt = now.UTC()
	a.Reviews = append(a.Reviews, record)
	return a.Transition(StateCorrection, now)
}

func (a *GerminationAssay) Resubmit(record ReviewRecord, now time.Time) error {
	if a.State != StateCorrection {
		return invalid("state", "只有整改中批次可重提")
	}
	if OpenDeviationCount(a.Deviations) > 0 {
		return invalid("deviations", "异常关闭后方可重提")
	}
	if !record.Difference.HasChanges() {
		return invalid("difference", "没有实际整改，不能重提")
	}
	record.Decision = DecisionResubmit
	if err := record.Validate(); err != nil {
		return err
	}
	record.Version = len(a.Reviews) + 1
	record.AssayID = a.ID
	record.CreatedAt = now.UTC()
	a.Reviews = append(a.Reviews, record)
	if err := a.Transition(StateReview, now); err != nil {
		return err
	}
	a.ReviewChecklist = DefaultReviewChecklist()
	a.ReviewMaterialRevision = a.Revision + 1
	return nil
}

func (a *GerminationAssay) Approve(record ReviewRecord, now time.Time) error {
	if a.State != StateReview {
		return invalid("state", "只有待复核批次可批准")
	}
	record.Decision = DecisionApproved
	if err := ValidateChecklist(record.Checklist, true); err != nil {
		return err
	}
	if OpenDeviationCount(a.Deviations) > 0 {
		return invalid("deviations", "仍有开放异常，不得批准")
	}
	if record.MaterialRevision != a.Revision {
		return invalid("material_revision", "复核材料摘要与当前 revision 不一致，请刷新后复核")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	record.Version = len(a.Reviews) + 1
	record.AssayID = a.ID
	record.CreatedAt = now.UTC()
	a.Reviews = append(a.Reviews, record)
	return a.Transition(StateApproved, now)
}
