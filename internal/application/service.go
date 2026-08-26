package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"seed-vigor-workbench/internal/domain"
)

type IDGenerator func(string) string

type Service struct {
	repository Repository
	now        func() time.Time
	newID      IDGenerator
	summaries  assaySummaryBuffer
}

func NewService(repository Repository, newID IDGenerator) *Service {
	return &Service{repository: repository, now: time.Now, newID: newID}
}

func (s *Service) CreateAssay(ctx context.Context, command CreateAssayCommand) (*domain.GerminationAssay, error) {
	if s.newID == nil {
		return nil, fmt.Errorf("未配置 ID 生成器")
	}
	assay, err := domain.NewAssay(s.newID("assay"), command.SampleAccession, command.LaboratoryBatchNo,
		command.OperatorName, command.ReviewerName, command.Protocol, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repository.Create(ctx, assay, command.OperatorName); err != nil {
		return nil, err
	}
	return s.repository.Get(ctx, assay.ID)
}

func (s *Service) GetAssay(ctx context.Context, id string) (*AssayView, error) {
	assay, err := s.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) ListAssays(ctx context.Context) ([]*AssaySummary, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.summaries.Project(items), nil
}

func (s *Service) DraftReadiness(ctx context.Context, id string) (domain.ReadinessResult, error) {
	assay, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.ReadinessResult{}, err
	}
	result := assay.Readiness()
	items, err := s.repository.List(ctx)
	if err != nil {
		return domain.ReadinessResult{}, err
	}
	for index := range items {
		other := &items[index]
		if other.ID != assay.ID && strings.EqualFold(strings.TrimSpace(other.LaboratoryBatchNo), strings.TrimSpace(assay.LaboratoryBatchNo)) && strings.EqualFold(strings.TrimSpace(other.SampleAccession), strings.TrimSpace(assay.SampleAccession)) {
			result.Issues = append(result.Issues, domain.ValidationError{Field: "sample_accession", Message: "同一检验批次号下样本标识重复"})
		}
	}
	result.Ready = len(result.Issues) == 0
	return result, nil
}

func (s *Service) ReviseDraft(ctx context.Context, id string, command ReviseDraftCommand) (*AssayView, error) {
	if strings.TrimSpace(command.Actor) == "" {
		return nil, fmt.Errorf("必须填写修订人员")
	}
	details := map[string]any{}
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "assay.draft_revised", command.Actor, details,
		func(a *domain.GerminationAssay) error {
			if a.State != domain.StateDraft {
				return domain.ValidationError{Field: "state", Message: "批次离开草稿状态后不可修订"}
			}
			if a.OperatorName != command.Actor {
				return fmt.Errorf("仅责任检验员可修订草稿")
			}
			if a.SampleAccession != strings.TrimSpace(command.SampleAccession) {
				details["sample_accession"] = command.SampleAccession
			}
			if a.LaboratoryBatchNo != strings.TrimSpace(command.LaboratoryBatchNo) {
				details["laboratory_batch_no"] = command.LaboratoryBatchNo
			}
			if a.OperatorName != strings.TrimSpace(command.OperatorName) {
				details["operator_name"] = command.OperatorName
			}
			if a.ReviewerName != strings.TrimSpace(command.ReviewerName) {
				details["reviewer_name"] = command.ReviewerName
			}
			if a.Protocol != command.Protocol {
				details["protocol"] = command.Protocol
			}
			return a.ReplaceDraft(command.SampleAccession, command.LaboratoryBatchNo, command.OperatorName, command.ReviewerName, command.Protocol, s.now())
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) FreezeProtocol(ctx context.Context, id string, command RevisionCommand) (*AssayView, error) {
	if err := command.Principal.Validate(RoleOperator); err != nil {
		return nil, err
	}
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "protocol.frozen", command.Principal.Name, nil,
		func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.Principal.Name {
				return fmt.Errorf("仅责任检验员可冻结方案")
			}
			return a.Freeze(s.now())
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) RecordObservation(ctx context.Context, id string, command ObservationCommand) (*AssayView, error) {
	if strings.TrimSpace(command.RecordedBy) == "" {
		return nil, fmt.Errorf("必须填写记录人员")
	}
	observationID := s.newID("obs")
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "observation.recorded", command.RecordedBy,
		map[string]any{"day_no": command.DayNo, "replicate_no": command.ReplicateNo}, func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.RecordedBy {
				return fmt.Errorf("仅责任检验员可登记观察")
			}
			if a.State == domain.StateCorrection && !a.ObservationInCorrectionScope(command.DayNo, command.ReplicateNo) {
				return domain.ValidationError{Field: "correction_scope", Message: "该观察单元不在本轮整改范围内"}
			}
			observation := domain.DailyObservation{
				ID: observationID, ReplicateNo: command.ReplicateNo, DayNo: command.DayNo,
				NormalCount: command.NormalCount, AbnormalCount: command.AbnormalCount,
				HardSeedCount: command.HardSeedCount, RottenCount: command.RottenCount,
				UngerminatedCount: command.UngerminatedCount, RecordedBy: command.RecordedBy,
			}
			if err := a.AddObservation(observation, s.now()); err != nil {
				return err
			}
			s.refreshDeviations(a, false)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) RecordDailyObservations(ctx context.Context, id string, command DailyObservationCommand) (*AssayView, error) {
	if strings.TrimSpace(command.RecordedBy) == "" {
		return nil, fmt.Errorf("必须填写记录人员")
	}
	ids := make([]string, len(command.Observations))
	for index := range ids {
		ids[index] = s.newID("obs")
	}
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "observation.day_recorded", command.RecordedBy,
		map[string]any{"day_no": command.DayNo, "replicate_count": len(command.Observations)}, func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.RecordedBy {
				return fmt.Errorf("仅责任检验员可登记观察")
			}
			items := make([]domain.DailyObservation, len(command.Observations))
			for index, reading := range command.Observations {
				if a.State == domain.StateCorrection && !a.ObservationInCorrectionScope(command.DayNo, reading.ReplicateNo) {
					return domain.ValidationError{Field: fmt.Sprintf("observations[%d].correction_scope", index), Message: "该观察单元不在本轮整改范围内"}
				}
				items[index] = domain.DailyObservation{ID: ids[index], ReplicateNo: reading.ReplicateNo, DayNo: command.DayNo,
					NormalCount: reading.NormalCount, AbnormalCount: reading.AbnormalCount, HardSeedCount: reading.HardSeedCount,
					RottenCount: reading.RottenCount, UngerminatedCount: reading.UngerminatedCount, RecordedBy: command.RecordedBy}
			}
			if err := a.AddObservationBatch(command.DayNo, items, s.now()); err != nil {
				return err
			}
			s.refreshDeviations(a, false)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) SealObservation(ctx context.Context, id string, command RevisionCommand) (*AssayView, error) {
	if err := command.Principal.Validate(RoleOperator); err != nil {
		return nil, err
	}
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "observation.finish_checked", command.Principal.Name, nil,
		func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.Principal.Name {
				return fmt.Errorf("仅责任检验员可封存")
			}
			s.refreshDeviations(a, true)
			metrics := domain.CalculateMetrics(a.Protocol, a.Observations)
			if !metrics.CompleteObservation || domain.OpenDeviationCount(a.Deviations) > 0 {
				return nil
			}
			return a.Seal(s.now())
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) refreshDeviations(a *domain.GerminationAssay, requireComplete bool) {
	findings := domain.EvaluateFindings(a.Protocol, a.Observations, requireComplete)
	active := make(map[string]domain.RuleFinding, len(findings))
	for _, finding := range findings {
		active[finding.RuleCode] = finding
	}
	latest := make(map[string]int, len(a.Deviations))
	for index := range a.Deviations {
		knownIndex, ok := latest[a.Deviations[index].RuleCode]
		if !ok || a.Deviations[index].Occurrence > a.Deviations[knownIndex].Occurrence {
			latest[a.Deviations[index].RuleCode] = index
		}
	}
	for code, finding := range active {
		occurrence := 1
		if index, ok := latest[code]; ok {
			item := &a.Deviations[index]
			item.CurrentVerification = finding.Result
			if item.Status == domain.DeviationOpen {
				continue
			}
			occurrence = item.Occurrence + 1
		}
		a.Deviations = append(a.Deviations, domain.DeviationCase{
			ID: s.newID("dev"), AssayID: a.ID, RuleCode: code, Severity: finding.Severity,
			Occurrence: occurrence, Status: domain.DeviationOpen, TargetDays: finding.TargetDays,
			TargetReplicates: finding.TargetReplicates, TriggerMetric: finding.Result,
			CurrentVerification: finding.Result, OpenedAt: s.now().UTC(),
		})
	}
	for index := range a.Deviations {
		item := &a.Deviations[index]
		if item.Status == domain.DeviationClosed {
			continue
		}
		if _, stillActive := active[item.RuleCode]; !stillActive {
			_, item.CurrentVerification = domain.VerificationForRule(a.Protocol, a.Observations, item.RuleCode, requireComplete)
		}
	}
}
