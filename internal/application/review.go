package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"seed-vigor-workbench/internal/domain"
)

func (s *Service) ReturnReview(ctx context.Context, id string, command ReviewCommand) (*AssayView, error) {
	if strings.TrimSpace(command.Reviewer) == "" {
		return nil, fmt.Errorf("必须填写复核员")
	}
	details := make(map[string]any)
	details["correction_scope"] = command.CorrectionScope
	details["checklist"] = command.Checklist
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "review.returned", command.Reviewer,
		details, func(a *domain.GerminationAssay) error {
			if a.ReviewerName != command.Reviewer {
				return fmt.Errorf("仅指定复核员可退回")
			}
			if len(command.CorrectionScope) == 0 {
				return domain.ValidationError{Field: "correction_scope", Message: "退回时必须选择具体整改范围"}
			}
			if err := validateReturnedScope(a, command.Checklist, command.CorrectionScope); err != nil {
				return err
			}
			hasReturned := false
			for _, item := range command.Checklist {
				hasReturned = hasReturned || item.Status == "returned"
			}
			if !hasReturned {
				return domain.ValidationError{Field: "checklist", Message: "退回决定至少需要一个退回清单项"}
			}
			a.ReviewChecklist = append([]domain.ReviewChecklistItem{}, command.Checklist...)
			a.ReviewMaterialRevision = a.Revision
			record := domain.ReviewRecord{ID: s.newID("review"), Decision: domain.DecisionReturned,
				Reviewer: command.Reviewer, Opinion: strings.TrimSpace(command.Opinion), RequiredScope: strings.TrimSpace(command.RequiredScope),
				Checklist: append([]domain.ReviewChecklistItem{}, command.Checklist...), CorrectionScope: append([]domain.CorrectionTarget{}, command.CorrectionScope...),
				MaterialRevision: a.Revision, BaselineObservationIDs: currentObservationIDs(a), BaselineDeviationStatus: deviationStatusSnapshot(a),
				BaselineMetrics: domain.CalculateMetrics(a.Protocol, a.Observations)}
			return a.ReturnForCorrection(record, s.now())
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func validateReturnedScope(a *domain.GerminationAssay, checklist []domain.ReviewChecklistItem, scope []domain.CorrectionTarget) error {
	statuses := make(map[string]string, len(checklist))
	for _, item := range checklist {
		statuses[item.Code] = item.Status
	}
	current := domain.CurrentObservations(a.Observations)
	validSections := map[string]bool{"protocol": true, "readings": true, "metrics": true, "deviations": true, "audit": true}
	issues := make([]domain.ValidationError, 0)
	for index, target := range scope {
		valid, section := false, ""
		switch target.Type {
		case "observation":
			_, valid = current[domain.ObservationKey(target.ReplicateNo, target.DayNo)]
			section = "readings"
		case "deviation":
			for _, item := range a.Deviations {
				valid = valid || item.ID == target.DeviationID
			}
			section = "deviations"
		case "section":
			valid = validSections[target.Section]
			section = target.Section
		}
		if !valid {
			issues = append(issues, domain.ValidationError{Field: fmt.Sprintf("correction_scope[%d]", index), Message: "整改目标不存在或类型无效"})
			continue
		}
		if statuses[section] != "returned" {
			issues = append(issues, domain.ValidationError{Field: fmt.Sprintf("correction_scope[%d]", index), Message: "整改目标所属清单项必须标记为退回"})
		}
	}
	if len(issues) > 0 {
		return domain.ValidationErrors{Issues: issues}
	}
	return nil
}

func (s *Service) ResubmitReview(ctx context.Context, id string, command ResubmitCommand) (*AssayView, error) {
	details := make(map[string]any)
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "review.resubmitted", command.Operator, details,
		func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.Operator {
				return fmt.Errorf("仅责任检验员可重提")
			}
			returned := latestReturnedReview(a)
			if returned == nil {
				return domain.ValidationError{Field: "review", Message: "缺少本轮退回复核基线"}
			}
			difference := buildReviewDifference(a, returned)
			details["difference"] = difference
			if err := validateCorrectionTargets(a, returned, difference); err != nil {
				return err
			}
			record := domain.ReviewRecord{ID: s.newID("review"), Reviewer: command.Operator, Opinion: strings.TrimSpace(command.Opinion),
				MaterialRevision: a.Revision + 1, Difference: difference}
			return a.Resubmit(record, s.now())
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func (s *Service) ApproveAndArchive(ctx context.Context, id string, command ReviewCommand) (*AssayView, error) {
	details := make(map[string]any)
	details["checklist"] = command.Checklist
	details["material_revision"] = command.ExpectedRevision
	assay, err := s.repository.Update(ctx, id, command.ExpectedRevision, "report.archived", command.Reviewer,
		details,
		func(a *domain.GerminationAssay) error {
			if a.ReviewerName != command.Reviewer {
				return fmt.Errorf("仅指定复核员可批准")
			}
			now := s.now().UTC()
			a.ReviewChecklist = append([]domain.ReviewChecklistItem{}, command.Checklist...)
			a.ReviewMaterialRevision = a.Revision
			record := domain.ReviewRecord{ID: s.newID("review"), Reviewer: command.Reviewer,
				Opinion: strings.TrimSpace(command.Opinion), Decision: domain.DecisionApproved,
				Checklist: append([]domain.ReviewChecklistItem{}, command.Checklist...), MaterialRevision: a.Revision}
			if err := a.Approve(record, now); err != nil {
				return err
			}
			digest, err := domain.EvidenceDigest(a)
			if err != nil {
				return err
			}
			a.Report = &domain.ArchivedReport{
				ID: s.newID("report"), AssayID: a.ID, AssayRevision: a.Revision + 1,
				Decision: "approved", MetricSnapshot: domain.CalculateMetrics(a.Protocol, a.Observations),
				EvidenceDigest: digest, ApprovedBy: command.Reviewer, ApprovedAt: now, ArchivedAt: now,
			}
			return a.Transition(domain.StateArchived, now)
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}

func currentObservationIDs(a *domain.GerminationAssay) []string {
	current := domain.CurrentObservations(a.Observations)
	ids := make([]string, 0, len(current))
	for _, observation := range current {
		ids = append(ids, observation.ID)
	}
	sort.Strings(ids)
	return ids
}

func deviationStatusSnapshot(a *domain.GerminationAssay) map[string]string {
	result := make(map[string]string, len(a.Deviations))
	for _, item := range a.Deviations {
		result[item.ID] = string(item.Status)
	}
	return result
}

func latestReturnedReview(a *domain.GerminationAssay) *domain.ReviewRecord {
	for index := len(a.Reviews) - 1; index >= 0; index-- {
		if a.Reviews[index].Decision == domain.DecisionReturned {
			return &a.Reviews[index]
		}
	}
	return nil
}

func buildReviewDifference(a *domain.GerminationAssay, baseline *domain.ReviewRecord) domain.ReviewDifference {
	known := make(map[string]bool, len(baseline.BaselineObservationIDs))
	for _, id := range baseline.BaselineObservationIDs {
		known[id] = true
	}
	difference := domain.ReviewDifference{}
	observationsByID := make(map[string]domain.DailyObservation, len(a.Observations))
	for _, observation := range a.Observations {
		observationsByID[observation.ID] = observation
	}
	for _, id := range currentObservationIDs(a) {
		if known[id] {
			continue
		}
		current := observationsByID[id]
		previous, hasPrevious := observationsByID[current.SupersedesID]
		if !hasPrevious || observationCountsDiffer(current, previous) {
			difference.NewObservationIDs = append(difference.NewObservationIDs, id)
		}
	}
	for _, item := range a.Deviations {
		before, existed := baseline.BaselineDeviationStatus[item.ID]
		if !existed || before != string(item.Status) {
			difference.DeviationChanges = append(difference.DeviationChanges, fmt.Sprintf("%s#%d: %s→%s", item.RuleCode, item.Occurrence, before, item.Status))
		}
	}
	metrics := domain.CalculateMetrics(a.Protocol, a.Observations)
	if metrics.CumulativeRate != baseline.BaselineMetrics.CumulativeRate {
		difference.MetricChanges = append(difference.MetricChanges, domain.MetricChange{Name: "累计发芽率", Before: baseline.BaselineMetrics.CumulativeRate, After: metrics.CumulativeRate})
	}
	if metrics.GerminationVigor != baseline.BaselineMetrics.GerminationVigor {
		difference.MetricChanges = append(difference.MetricChanges, domain.MetricChange{Name: "发芽势", Before: baseline.BaselineMetrics.GerminationVigor, After: metrics.GerminationVigor})
	}
	if metrics.MaxDispersion != baseline.BaselineMetrics.MaxDispersion {
		difference.MetricChanges = append(difference.MetricChanges, domain.MetricChange{Name: "最大组间离散度", Before: baseline.BaselineMetrics.MaxDispersion, After: metrics.MaxDispersion})
	}
	return difference
}

func observationCountsDiffer(left, right domain.DailyObservation) bool {
	return left.NormalCount != right.NormalCount || left.AbnormalCount != right.AbnormalCount || left.HardSeedCount != right.HardSeedCount || left.RottenCount != right.RottenCount || left.UngerminatedCount != right.UngerminatedCount
}

func validateCorrectionTargets(a *domain.GerminationAssay, baseline *domain.ReviewRecord, difference domain.ReviewDifference) error {
	newIDs := make(map[string]bool, len(difference.NewObservationIDs))
	for _, id := range difference.NewObservationIDs {
		newIDs[id] = true
	}
	current := domain.CurrentObservations(a.Observations)
	issues := make([]domain.ValidationError, 0)
	for index, target := range baseline.CorrectionScope {
		solved := false
		switch target.Type {
		case "observation":
			if item, ok := current[domain.ObservationKey(target.ReplicateNo, target.DayNo)]; ok {
				solved = newIDs[item.ID]
			}
		case "deviation":
			for _, item := range a.Deviations {
				if item.ID == target.DeviationID {
					solved = item.Status == domain.DeviationClosed
				}
			}
		case "section":
			solved = difference.HasChanges()
		}
		if !solved {
			issues = append(issues, domain.ValidationError{Field: fmt.Sprintf("correction_scope[%d]", index), Message: "整改范围内仍有未解决项"})
		}
	}
	if len(issues) > 0 {
		return domain.ValidationErrors{Issues: issues}
	}
	return nil
}
