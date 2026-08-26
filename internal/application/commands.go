package application

import "seed-vigor-workbench/internal/domain"

type CreateAssayCommand struct {
	SampleAccession   string               `json:"sample_accession"`
	LaboratoryBatchNo string               `json:"laboratory_batch_no"`
	OperatorName      string               `json:"operator_name"`
	ReviewerName      string               `json:"reviewer_name"`
	Protocol          domain.AssayProtocol `json:"protocol"`
}

type RevisionCommand struct {
	ExpectedRevision int64     `json:"expected_revision"`
	Principal        Principal `json:"principal"`
}

type ReviseDraftCommand struct {
	ExpectedRevision  int64                `json:"expected_revision"`
	SampleAccession   string               `json:"sample_accession"`
	LaboratoryBatchNo string               `json:"laboratory_batch_no"`
	OperatorName      string               `json:"operator_name"`
	ReviewerName      string               `json:"reviewer_name"`
	Protocol          domain.AssayProtocol `json:"protocol"`
	Actor             string               `json:"actor"`
}

type ObservationCommand struct {
	ExpectedRevision  int64  `json:"expected_revision"`
	ReplicateNo       int    `json:"replicate_no"`
	DayNo             int    `json:"day_no"`
	NormalCount       int    `json:"normal_count"`
	AbnormalCount     int    `json:"abnormal_count"`
	HardSeedCount     int    `json:"hard_seed_count"`
	RottenCount       int    `json:"rotten_count"`
	UngerminatedCount int    `json:"ungerminated_count"`
	RecordedBy        string `json:"recorded_by"`
}

type DailyObservationCommand struct {
	ExpectedRevision int64                `json:"expected_revision"`
	DayNo            int                  `json:"day_no"`
	RecordedBy       string               `json:"recorded_by"`
	Observations     []ObservationReading `json:"observations"`
}

type ObservationReading struct {
	ReplicateNo       int `json:"replicate_no"`
	NormalCount       int `json:"normal_count"`
	AbnormalCount     int `json:"abnormal_count"`
	HardSeedCount     int `json:"hard_seed_count"`
	RottenCount       int `json:"rotten_count"`
	UngerminatedCount int `json:"ungerminated_count"`
}

type ResolveDeviationCommand struct {
	ExpectedRevision int64    `json:"expected_revision"`
	Reason           string   `json:"reason"`
	CorrectiveAction string   `json:"corrective_action"`
	EvidenceIDs      []string `json:"evidence_ids"`
	Actor            string   `json:"actor"`
}

type ReviewCommand struct {
	ExpectedRevision int64                        `json:"expected_revision"`
	Reviewer         string                       `json:"reviewer"`
	Opinion          string                       `json:"opinion"`
	RequiredScope    string                       `json:"required_scope"`
	Checklist        []domain.ReviewChecklistItem `json:"checklist"`
	CorrectionScope  []domain.CorrectionTarget    `json:"correction_scope"`
}

type ResubmitCommand struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Operator         string `json:"operator"`
	Opinion          string `json:"opinion"`
}
