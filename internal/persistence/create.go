package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"seed-vigor-workbench/internal/domain"
)

func (s *Store) Create(ctx context.Context, assay *domain.GerminationAssay, actor string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	event := domain.AuditEvent{
		ID: newID("evt"), AssayID: assay.ID, Revision: assay.Revision,
		Action: "assay.created", Actor: actor, Details: map[string]any{"sample_accession": assay.SampleAccession}, CreatedAt: assay.CreatedAt,
	}
	assay.AuditTrail = append(assay.AuditTrail, event)
	if err := saveAssay(ctx, tx, assay); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: assays.laboratory_batch_no") {
			return domain.ValidationErrors{Issues: []domain.ValidationError{{Field: "sample_accession", Message: "同一检验批次号下样本标识重复"}}}
		}
		return err
	}
	return tx.Commit()
}

func saveAssay(ctx context.Context, tx *sql.Tx, assay *domain.GerminationAssay) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO assays(id,sample_accession,laboratory_batch_no,state,operator_name,reviewer_name,protocol_version,revision,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET sample_accession=excluded.sample_accession,laboratory_batch_no=excluded.laboratory_batch_no,
		state=excluded.state,operator_name=excluded.operator_name,reviewer_name=excluded.reviewer_name,
		protocol_version=excluded.protocol_version,revision=excluded.revision,updated_at=excluded.updated_at`,
		assay.ID, assay.SampleAccession, assay.LaboratoryBatchNo, assay.State, assay.OperatorName, assay.ReviewerName,
		assay.ProtocolVersion, assay.Revision, formatTime(assay.CreatedAt), formatTime(assay.UpdatedAt))
	if err != nil {
		return err
	}
	checklistJSON, _ := json.Marshal(assay.ReviewChecklist)
	_, err = tx.ExecContext(ctx, `INSERT INTO assay_workflow(assay_id,review_checklist_json,review_material_revision) VALUES(?,?,?)
		ON CONFLICT(assay_id) DO UPDATE SET review_checklist_json=excluded.review_checklist_json,review_material_revision=excluded.review_material_revision`,
		assay.ID, string(checklistJSON), assay.ReviewMaterialRevision)
	if err != nil {
		return err
	}
	var frozen any
	if assay.Protocol.FrozenAt != nil {
		frozen = formatTime(*assay.Protocol.FrozenAt)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO protocols(assay_id,temperature_celsius,substrate,light_cycle_hours,observation_days,replicate_count,seeds_per_replicate,dispersion_limit,normal_seedling_rule,frozen_at)
        VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(assay_id) DO UPDATE SET temperature_celsius=excluded.temperature_celsius,substrate=excluded.substrate,
        light_cycle_hours=excluded.light_cycle_hours,observation_days=excluded.observation_days,replicate_count=excluded.replicate_count,
        seeds_per_replicate=excluded.seeds_per_replicate,dispersion_limit=excluded.dispersion_limit,normal_seedling_rule=excluded.normal_seedling_rule,frozen_at=excluded.frozen_at`,
		assay.ID, assay.Protocol.TemperatureCelsius, assay.Protocol.Substrate, assay.Protocol.LightCycleHours,
		assay.Protocol.ObservationDays, assay.Protocol.ReplicateCount, assay.Protocol.SeedsPerReplicate,
		assay.Protocol.DispersionLimit, assay.Protocol.NormalSeedlingRule, frozen)
	if err != nil {
		return err
	}
	for _, observation := range assay.Observations {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO observations(id,assay_id,replicate_no,day_no,normal_count,abnormal_count,hard_seed_count,rotten_count,ungerminated_count,recorded_by,recorded_at,supersedes_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			observation.ID, assay.ID, observation.ReplicateNo, observation.DayNo, observation.NormalCount,
			observation.AbnormalCount, observation.HardSeedCount, observation.RottenCount, observation.UngerminatedCount,
			observation.RecordedBy, formatTime(observation.RecordedAt), nullable(observation.SupersedesID))
		if err != nil {
			return err
		}
	}
	for _, deviation := range assay.Deviations {
		ids, _ := json.Marshal(deviation.RetestObservationIDs)
		days, _ := json.Marshal(deviation.TargetDays)
		replicates, _ := json.Marshal(deviation.TargetReplicates)
		_, err = tx.ExecContext(ctx, `INSERT INTO deviation_occurrences(id,assay_id,rule_code,occurrence,severity,status,target_days_json,target_replicates_json,trigger_metric,current_verification,reason,corrective_action,retest_ids_json,opened_at,closed_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(assay_id,rule_code,occurrence) DO UPDATE SET severity=excluded.severity,status=excluded.status,
			current_verification=excluded.current_verification,reason=excluded.reason,corrective_action=excluded.corrective_action,retest_ids_json=excluded.retest_ids_json,closed_at=excluded.closed_at`,
			deviation.ID, assay.ID, deviation.RuleCode, deviation.Occurrence, deviation.Severity, deviation.Status, string(days), string(replicates),
			deviation.TriggerMetric, deviation.CurrentVerification, deviation.Reason, deviation.CorrectiveAction, string(ids), formatTime(deviation.OpenedAt), nullableTime(deviation.ClosedAt))
		if err != nil {
			return err
		}
	}
	for _, review := range assay.Reviews {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO reviews(id,assay_id,version,decision,reviewer,opinion,required_scope,created_at) VALUES(?,?,?,?,?,?,?,?)`,
			review.ID, assay.ID, review.Version, review.Decision, review.Reviewer, review.Opinion, review.RequiredScope, formatTime(review.CreatedAt))
		if err != nil {
			return err
		}
		checklist, _ := json.Marshal(review.Checklist)
		scope, _ := json.Marshal(review.CorrectionScope)
		observationIDs, _ := json.Marshal(review.BaselineObservationIDs)
		deviationStatus, _ := json.Marshal(review.BaselineDeviationStatus)
		metrics, _ := json.Marshal(review.BaselineMetrics)
		difference, _ := json.Marshal(review.Difference)
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO review_details(review_id,checklist_json,correction_scope_json,material_revision,baseline_observation_ids_json,baseline_deviation_status_json,baseline_metrics_json,difference_json) VALUES(?,?,?,?,?,?,?,?)`,
			review.ID, string(checklist), string(scope), review.MaterialRevision, string(observationIDs), string(deviationStatus), string(metrics), string(difference))
		if err != nil {
			return err
		}
	}
	for _, event := range assay.AuditTrail {
		details, _ := json.Marshal(event.Details)
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO audit_events(id,assay_id,revision,action,actor,details_json,created_at) VALUES(?,?,?,?,?,?,?)`,
			event.ID, assay.ID, event.Revision, event.Action, event.Actor, string(details), formatTime(event.CreatedAt))
		if err != nil {
			return err
		}
	}
	if assay.Report != nil {
		metric, _ := json.Marshal(assay.Report.MetricSnapshot)
		_, err = tx.ExecContext(ctx, `INSERT INTO archived_reports(id,assay_id,assay_revision,decision,metric_json,evidence_digest,approved_by,approved_at,archived_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			assay.Report.ID, assay.ID, assay.Report.AssayRevision, assay.Report.Decision, string(metric), assay.Report.EvidenceDigest,
			assay.Report.ApprovedBy, formatTime(assay.Report.ApprovedAt), formatTime(assay.Report.ArchivedAt))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}
