package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"seed-vigor-workbench/internal/domain"
)

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) Get(ctx context.Context, id string) (*domain.GerminationAssay, error) {
	return loadAssay(ctx, s.db, id)
}

func loadAssay(ctx context.Context, q queryer, id string) (*domain.GerminationAssay, error) {
	a := &domain.GerminationAssay{}
	var created, updated string
	err := q.QueryRowContext(ctx, `SELECT id,sample_accession,laboratory_batch_no,state,operator_name,reviewer_name,protocol_version,revision,created_at,updated_at FROM assays WHERE id=?`, id).
		Scan(&a.ID, &a.SampleAccession, &a.LaboratoryBatchNo, &a.State, &a.OperatorName, &a.ReviewerName, &a.ProtocolVersion, &a.Revision, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if a.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if a.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	var frozen sql.NullString
	err = q.QueryRowContext(ctx, `SELECT temperature_celsius,substrate,light_cycle_hours,observation_days,replicate_count,seeds_per_replicate,dispersion_limit,normal_seedling_rule,frozen_at FROM protocols WHERE assay_id=?`, id).
		Scan(&a.Protocol.TemperatureCelsius, &a.Protocol.Substrate, &a.Protocol.LightCycleHours, &a.Protocol.ObservationDays,
			&a.Protocol.ReplicateCount, &a.Protocol.SeedsPerReplicate, &a.Protocol.DispersionLimit, &a.Protocol.NormalSeedlingRule, &frozen)
	if err != nil {
		return nil, err
	}
	if frozen.Valid {
		when, parseErr := parseTime(frozen.String)
		if parseErr != nil {
			return nil, parseErr
		}
		a.Protocol.FrozenAt = &when
	}
	var checklistJSON string
	if err = q.QueryRowContext(ctx, `SELECT review_checklist_json,review_material_revision FROM assay_workflow WHERE assay_id=?`, id).Scan(&checklistJSON, &a.ReviewMaterialRevision); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if checklistJSON != "" {
		if err := json.Unmarshal([]byte(checklistJSON), &a.ReviewChecklist); err != nil {
			return nil, err
		}
	}
	if a.Observations, err = loadObservations(ctx, q, id); err != nil {
		return nil, err
	}
	if a.Deviations, err = loadDeviations(ctx, q, id); err != nil {
		return nil, err
	}
	if a.Reviews, err = loadReviews(ctx, q, id); err != nil {
		return nil, err
	}
	if a.AuditTrail, err = loadAudit(ctx, q, id); err != nil {
		return nil, err
	}
	if a.Report, err = loadReport(ctx, q, id); err != nil {
		return nil, err
	}
	return a, nil
}

func loadObservations(ctx context.Context, q queryer, id string) ([]domain.DailyObservation, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,replicate_no,day_no,normal_count,abnormal_count,hard_seed_count,rotten_count,ungerminated_count,recorded_by,recorded_at,COALESCE(supersedes_id,'') FROM observations WHERE assay_id=? ORDER BY recorded_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DailyObservation{}
	for rows.Next() {
		var item domain.DailyObservation
		var recorded string
		item.AssayID = id
		if err := rows.Scan(&item.ID, &item.ReplicateNo, &item.DayNo, &item.NormalCount, &item.AbnormalCount, &item.HardSeedCount, &item.RottenCount, &item.UngerminatedCount, &item.RecordedBy, &recorded, &item.SupersedesID); err != nil {
			return nil, err
		}
		item.RecordedAt, err = parseTime(recorded)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadDeviations(ctx context.Context, q queryer, id string) ([]domain.DeviationCase, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,rule_code,occurrence,severity,status,target_days_json,target_replicates_json,trigger_metric,current_verification,reason,corrective_action,retest_ids_json,opened_at,closed_at FROM deviation_occurrences WHERE assay_id=? ORDER BY opened_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.DeviationCase{}
	for rows.Next() {
		var item domain.DeviationCase
		var ids, days, replicates, opened string
		var closed sql.NullString
		item.AssayID = id
		if err := rows.Scan(&item.ID, &item.RuleCode, &item.Occurrence, &item.Severity, &item.Status, &days, &replicates, &item.TriggerMetric, &item.CurrentVerification, &item.Reason, &item.CorrectiveAction, &ids, &opened, &closed); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(ids), &item.RetestObservationIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(days), &item.TargetDays); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(replicates), &item.TargetReplicates); err != nil {
			return nil, err
		}
		item.OpenedAt, err = parseTime(opened)
		if err != nil {
			return nil, err
		}
		if closed.Valid {
			value, parseErr := parseTime(closed.String)
			if parseErr != nil {
				return nil, parseErr
			}
			item.ClosedAt = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadReviews(ctx context.Context, q queryer, id string) ([]domain.ReviewRecord, error) {
	rows, err := q.QueryContext(ctx, `SELECT r.id,r.version,r.decision,r.reviewer,r.opinion,r.required_scope,r.created_at,
		COALESCE(d.checklist_json,'[]'),COALESCE(d.correction_scope_json,'[]'),COALESCE(d.material_revision,0),
		COALESCE(d.baseline_observation_ids_json,'[]'),COALESCE(d.baseline_deviation_status_json,'{}'),COALESCE(d.baseline_metrics_json,'{}'),COALESCE(d.difference_json,'{}')
		FROM reviews r LEFT JOIN review_details d ON d.review_id=r.id WHERE r.assay_id=? ORDER BY r.version`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ReviewRecord{}
	for rows.Next() {
		var item domain.ReviewRecord
		var created, checklist, scope, observationIDs, deviationStatus, metrics, difference string
		item.AssayID = id
		if err := rows.Scan(&item.ID, &item.Version, &item.Decision, &item.Reviewer, &item.Opinion, &item.RequiredScope, &created,
			&checklist, &scope, &item.MaterialRevision, &observationIDs, &deviationStatus, &metrics, &difference); err != nil {
			return nil, err
		}
		item.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(checklist), &item.Checklist); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(scope), &item.CorrectionScope); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(observationIDs), &item.BaselineObservationIDs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(deviationStatus), &item.BaselineDeviationStatus); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(metrics), &item.BaselineMetrics); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(difference), &item.Difference); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadAudit(ctx context.Context, q queryer, id string) ([]domain.AuditEvent, error) {
	rows, err := q.QueryContext(ctx, `SELECT id,revision,action,actor,details_json,created_at FROM audit_events WHERE assay_id=? ORDER BY created_at,id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditEvent{}
	for rows.Next() {
		var item domain.AuditEvent
		var details, created string
		item.AssayID = id
		if err := rows.Scan(&item.ID, &item.Revision, &item.Action, &item.Actor, &details, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(details), &item.Details); err != nil {
			return nil, err
		}
		item.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadReport(ctx context.Context, q queryer, id string) (*domain.ArchivedReport, error) {
	item := &domain.ArchivedReport{AssayID: id}
	var metric, approved, archived string
	err := q.QueryRowContext(ctx, `SELECT id,assay_revision,decision,metric_json,evidence_digest,approved_by,approved_at,archived_at FROM archived_reports WHERE assay_id=? ORDER BY archived_at DESC LIMIT 1`, id).
		Scan(&item.ID, &item.AssayRevision, &item.Decision, &metric, &item.EvidenceDigest, &item.ApprovedBy, &approved, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(metric), &item.MetricSnapshot); err != nil {
		return nil, err
	}
	item.ApprovedAt, err = parseTime(approved)
	if err != nil {
		return nil, err
	}
	item.ArchivedAt, err = parseTime(archived)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) List(ctx context.Context) ([]domain.GerminationAssay, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM assays ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]domain.GerminationAssay, 0, len(ids))
	for _, assayID := range ids {
		item, loadErr := s.Get(ctx, assayID)
		if loadErr != nil {
			return nil, fmt.Errorf("加载批次 %s: %w", assayID, loadErr)
		}
		items = append(items, *item)
	}
	return items, nil
}

var _ = time.Time{}
