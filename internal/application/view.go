package application

import (
	"fmt"

	"seed-vigor-workbench/internal/domain"
)

type AssaySummary struct {
	ID                string            `json:"id"`
	SampleAccession   string            `json:"sample_accession"`
	LaboratoryBatchNo string            `json:"laboratory_batch_no"`
	State             domain.AssayState `json:"state"`
	Revision          int64             `json:"revision"`
	CumulativeRate    float64           `json:"cumulative_rate"`
	OpenDeviations    int               `json:"open_deviations"`
}

type DerivationLine struct {
	Label   string `json:"label"`
	Formula string `json:"formula"`
	Result  string `json:"result"`
}

type ReviewMaterial struct {
	ProtocolVersion string                    `json:"protocol_version"`
	Protocol        domain.AssayProtocol      `json:"protocol"`
	CurrentReadings []domain.DailyObservation `json:"current_readings"`
	Metrics         domain.MetricSnapshot     `json:"metrics"`
	Derivations     []DerivationLine          `json:"derivations"`
	ReviewHistory   []domain.ReviewRecord     `json:"review_history"`
}

type AssayView struct {
	Assay            *domain.GerminationAssay  `json:"assay"`
	Metrics          domain.MetricSnapshot     `json:"metrics"`
	CurrentReadings  []domain.DailyObservation `json:"current_readings"`
	ReviewMaterial   ReviewMaterial            `json:"review_material"`
	ReportConsistent bool                      `json:"report_consistent"`
}

func BuildSummary(a *domain.GerminationAssay) AssaySummary {
	metrics := domain.CalculateMetrics(a.Protocol, a.Observations)
	return AssaySummary{ID: a.ID, SampleAccession: a.SampleAccession, LaboratoryBatchNo: a.LaboratoryBatchNo,
		State: a.State, Revision: a.Revision, CumulativeRate: metrics.CumulativeRate, OpenDeviations: domain.OpenDeviationCount(a.Deviations)}
}

func BuildAssayView(a *domain.GerminationAssay) *AssayView {
	metrics := domain.CalculateMetrics(a.Protocol, a.Observations)
	currentMap := domain.CurrentObservations(a.Observations)
	current := make([]domain.DailyObservation, 0, len(currentMap))
	for day := 1; day <= a.Protocol.ObservationDays; day++ {
		for rep := 1; rep <= a.Protocol.ReplicateCount; rep++ {
			if item, ok := currentMap[domain.ObservationKey(rep, day)]; ok {
				current = append(current, item)
			}
		}
	}
	derivations := []DerivationLine{
		{Label: "累计发芽率", Formula: "末次观察正常幼苗数 ÷ 已观察种子总数", Result: fmt.Sprintf("%.2f%%", metrics.CumulativeRate*100)},
		{Label: "发芽势", Formula: "首日正常幼苗数 ÷ 首日已观察种子总数", Result: fmt.Sprintf("%.2f%%", metrics.GerminationVigor*100)},
		{Label: "组间离散度", Formula: "同日重复组发芽率最大值 − 最小值", Result: fmt.Sprintf("%.4f", metrics.MaxDispersion)},
	}
	return &AssayView{
		Assay: a, Metrics: metrics, CurrentReadings: current,
		ReviewMaterial: ReviewMaterial{ProtocolVersion: a.ProtocolVersion, Protocol: a.Protocol, CurrentReadings: current,
			Metrics: metrics, Derivations: derivations, ReviewHistory: a.Reviews},
		ReportConsistent: domain.VerifyReport(a, a.Report),
	}
}
