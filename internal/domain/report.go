package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type ArchivedReport struct {
	ID             string         `json:"id"`
	AssayID        string         `json:"assay_id"`
	AssayRevision  int64          `json:"assay_revision"`
	Decision       string         `json:"decision"`
	MetricSnapshot MetricSnapshot `json:"metric_snapshot"`
	EvidenceDigest string         `json:"evidence_digest"`
	ApprovedBy     string         `json:"approved_by"`
	ApprovedAt     time.Time      `json:"approved_at"`
	ArchivedAt     time.Time      `json:"archived_at"`
}

type reportEvidence struct {
	AssayID         string             `json:"assay_id"`
	SampleAccession string             `json:"sample_accession"`
	BatchNo         string             `json:"laboratory_batch_no"`
	ProtocolVersion string             `json:"protocol_version"`
	Protocol        AssayProtocol      `json:"protocol"`
	Observations    []DailyObservation `json:"observations"`
	Metrics         MetricSnapshot     `json:"metrics"`
	ReviewHistory   []ReviewRecord     `json:"review_history"`
}

func CanonicalEvidence(a *GerminationAssay) ([]byte, error) {
	evidence := reportEvidence{
		AssayID: a.ID, SampleAccession: a.SampleAccession, BatchNo: a.LaboratoryBatchNo,
		ProtocolVersion: a.ProtocolVersion, Protocol: a.Protocol, Observations: a.Observations,
		Metrics: CalculateMetrics(a.Protocol, a.Observations), ReviewHistory: a.Reviews,
	}
	return json.Marshal(evidence)
}

func EvidenceDigest(a *GerminationAssay) (string, error) {
	payload, err := CanonicalEvidence(a)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func VerifyReport(a *GerminationAssay, report *ArchivedReport) bool {
	if report == nil {
		return false
	}
	digest, err := EvidenceDigest(a)
	return err == nil && digest == report.EvidenceDigest
}
