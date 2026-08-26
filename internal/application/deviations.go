package application

import (
	"context"
	"fmt"
	"strings"

	"seed-vigor-workbench/internal/domain"
)

func (s *Service) ResolveDeviation(ctx context.Context, assayID, deviationID string, command ResolveDeviationCommand) (*AssayView, error) {
	if strings.TrimSpace(command.Actor) == "" {
		return nil, fmt.Errorf("必须填写整改人员")
	}
	if strings.TrimSpace(command.Reason) == "" {
		return nil, fmt.Errorf("必须填写异常原因")
	}
	if strings.TrimSpace(command.CorrectiveAction) == "" {
		return nil, fmt.Errorf("必须填写补测动作")
	}
	details := s.auditDetails
	clear(details)
	details["deviation_id"] = deviationID
	assay, err := s.repository.Update(ctx, assayID, command.ExpectedRevision, "deviation.corrected", command.Actor,
		details, func(a *domain.GerminationAssay) error {
			if a.OperatorName != command.Actor {
				return fmt.Errorf("仅责任检验员可提交整改")
			}
			if a.State != domain.StateObserving && a.State != domain.StateCorrection {
				return fmt.Errorf("当前状态不可整改异常")
			}
			if a.State == domain.StateCorrection && !a.DeviationInCorrectionScope(deviationID) {
				return domain.ValidationError{Field: "correction_scope", Message: "该异常不在本轮整改范围内"}
			}
			found := false
			for index := range a.Deviations {
				item := &a.Deviations[index]
				if item.ID != deviationID {
					continue
				}
				found = true
				if item.Status == domain.DeviationClosed {
					return fmt.Errorf("异常已经关闭")
				}
				item.Reason = strings.TrimSpace(command.Reason)
				item.CorrectiveAction = strings.TrimSpace(command.CorrectiveAction)
				item.RetestObservationIDs = append([]string{}, command.EvidenceIDs...)
				for _, evidenceID := range command.EvidenceIDs {
					var evidence *domain.DailyObservation
					for obsIndex := range a.Observations {
						if a.Observations[obsIndex].ID == evidenceID {
							evidence = &a.Observations[obsIndex]
							break
						}
					}
					if evidence == nil {
						return domain.ValidationError{Field: "evidence_ids", Message: "补测证据不存在"}
					}
					if !evidence.RecordedAt.After(item.OpenedAt) || !item.ContainsObservation(*evidence) {
						return domain.ValidationError{Field: "evidence_ids", Message: "补测证据必须在异常打开后产生且位于目标范围内"}
					}
					if !strings.HasPrefix(item.RuleCode, "MISSING_") && evidence.SupersedesID == "" {
						return domain.ValidationError{Field: "evidence_ids", Message: "该规则要求选择相关读数的新版本作为证据"}
					}
				}
				passed, result := domain.VerificationForRule(a.Protocol, a.Observations, item.RuleCode, strings.HasPrefix(item.RuleCode, "MISSING_"))
				item.CurrentVerification = result
				if missing := item.MissingClosureConditions(passed); len(missing) > 0 {
					return domain.ValidationError{Field: "closure_conditions", Message: "尚缺关闭条件：" + strings.Join(missing, "、")}
				}
				closed := s.now().UTC()
				item.Status = domain.DeviationClosed
				item.ClosedAt = &closed
			}
			if !found {
				return fmt.Errorf("异常项不存在")
			}
			s.refreshDeviations(a, a.State == domain.StateCorrection)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return BuildAssayView(assay), nil
}
