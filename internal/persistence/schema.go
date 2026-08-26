package persistence

const schemaVersion = 1

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS schema_meta (
        version INTEGER NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS assays (
        id TEXT PRIMARY KEY,
        sample_accession TEXT NOT NULL,
        laboratory_batch_no TEXT NOT NULL,
        state TEXT NOT NULL,
        operator_name TEXT NOT NULL,
        reviewer_name TEXT NOT NULL,
        protocol_version TEXT NOT NULL,
        revision INTEGER NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        UNIQUE(laboratory_batch_no, sample_accession)
    )`,
	`CREATE TABLE IF NOT EXISTS protocols (
        assay_id TEXT PRIMARY KEY REFERENCES assays(id) ON DELETE CASCADE,
        temperature_celsius REAL NOT NULL,
        substrate TEXT NOT NULL,
        light_cycle_hours INTEGER NOT NULL,
        observation_days INTEGER NOT NULL,
        replicate_count INTEGER NOT NULL,
        seeds_per_replicate INTEGER NOT NULL,
        dispersion_limit REAL NOT NULL,
        normal_seedling_rule TEXT NOT NULL,
        frozen_at TEXT
    )`,
	`CREATE TABLE IF NOT EXISTS observations (
        id TEXT PRIMARY KEY,
        assay_id TEXT NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
        replicate_no INTEGER NOT NULL,
        day_no INTEGER NOT NULL,
        normal_count INTEGER NOT NULL,
        abnormal_count INTEGER NOT NULL,
        hard_seed_count INTEGER NOT NULL,
        rotten_count INTEGER NOT NULL,
        ungerminated_count INTEGER NOT NULL,
        recorded_by TEXT NOT NULL,
        recorded_at TEXT NOT NULL,
        supersedes_id TEXT
    )`,
	`CREATE INDEX IF NOT EXISTS observations_assay_idx ON observations(assay_id, day_no, replicate_no, recorded_at)`,
	`CREATE TABLE IF NOT EXISTS reviews (
        id TEXT PRIMARY KEY,
        assay_id TEXT NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
        version INTEGER NOT NULL,
        decision TEXT NOT NULL,
        reviewer TEXT NOT NULL,
        opinion TEXT NOT NULL,
        required_scope TEXT NOT NULL,
        created_at TEXT NOT NULL,
        UNIQUE(assay_id, version)
    )`,
	`CREATE TABLE IF NOT EXISTS audit_events (
        id TEXT PRIMARY KEY,
        assay_id TEXT NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
        revision INTEGER NOT NULL,
        action TEXT NOT NULL,
        actor TEXT NOT NULL,
        details_json TEXT NOT NULL,
        created_at TEXT NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS archived_reports (
        id TEXT PRIMARY KEY,
        assay_id TEXT NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
        assay_revision INTEGER NOT NULL,
        decision TEXT NOT NULL,
        metric_json TEXT NOT NULL,
        evidence_digest TEXT NOT NULL,
        approved_by TEXT NOT NULL,
        approved_at TEXT NOT NULL,
        archived_at TEXT NOT NULL,
        UNIQUE(assay_id, assay_revision)
    )`,
	`CREATE TABLE IF NOT EXISTS assay_workflow (
        assay_id TEXT PRIMARY KEY REFERENCES assays(id) ON DELETE CASCADE,
        review_checklist_json TEXT NOT NULL,
        review_material_revision INTEGER NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS deviation_occurrences (
        id TEXT PRIMARY KEY,
        assay_id TEXT NOT NULL REFERENCES assays(id) ON DELETE CASCADE,
        rule_code TEXT NOT NULL,
        occurrence INTEGER NOT NULL,
        severity TEXT NOT NULL,
        status TEXT NOT NULL,
        target_days_json TEXT NOT NULL,
        target_replicates_json TEXT NOT NULL,
        trigger_metric TEXT NOT NULL,
        current_verification TEXT NOT NULL,
        reason TEXT NOT NULL,
        corrective_action TEXT NOT NULL,
        retest_ids_json TEXT NOT NULL,
        opened_at TEXT NOT NULL,
        closed_at TEXT,
        UNIQUE(assay_id, rule_code, occurrence)
    )`,
	`CREATE TABLE IF NOT EXISTS review_details (
        review_id TEXT PRIMARY KEY REFERENCES reviews(id) ON DELETE CASCADE,
        checklist_json TEXT NOT NULL,
        correction_scope_json TEXT NOT NULL,
        material_revision INTEGER NOT NULL,
        baseline_observation_ids_json TEXT NOT NULL,
        baseline_deviation_status_json TEXT NOT NULL,
        baseline_metrics_json TEXT NOT NULL,
        difference_json TEXT NOT NULL
    )`,
}
