package domain

import (
	"fmt"
	"strings"
	"time"
)

type ReviewDecision string

const (
	DecisionReturned ReviewDecision = "returned"
	DecisionApproved ReviewDecision = "approved"
	DecisionResubmit ReviewDecision = "resubmitted"
)

type ReviewRecord struct {
	ID                      string                `json:"id"`
	AssayID                 string                `json:"assay_id"`
	Version                 int                   `json:"version"`
	Decision                ReviewDecision        `json:"decision"`
	Reviewer                string                `json:"reviewer"`
	Opinion                 string                `json:"opinion"`
	RequiredScope           string                `json:"required_scope,omitempty"`
	Checklist               []ReviewChecklistItem `json:"checklist"`
	CorrectionScope         []CorrectionTarget    `json:"correction_scope,omitempty"`
	MaterialRevision        int64                 `json:"material_revision"`
	BaselineObservationIDs  []string              `json:"baseline_observation_ids,omitempty"`
	BaselineDeviationStatus map[string]string     `json:"baseline_deviation_status,omitempty"`
	BaselineMetrics         MetricSnapshot        `json:"baseline_metrics"`
	Difference              ReviewDifference      `json:"difference"`
	CreatedAt               time.Time             `json:"created_at"`
}

type ReviewChecklistItem struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Opinion string `json:"opinion,omitempty"`
}

type CorrectionTarget struct {
	Type        string `json:"type"`
	DayNo       int    `json:"day_no,omitempty"`
	ReplicateNo int    `json:"replicate_no,omitempty"`
	DeviationID string `json:"deviation_id,omitempty"`
	Section     string `json:"section,omitempty"`
}

type MetricChange struct {
	Name   string  `json:"name"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
}

type ReviewDifference struct {
	NewObservationIDs []string       `json:"new_observation_ids"`
	DeviationChanges  []string       `json:"deviation_changes"`
	MetricChanges     []MetricChange `json:"metric_changes"`
}

func DefaultReviewChecklist() []ReviewChecklistItem {
	return []ReviewChecklistItem{
		{Code: "protocol", Label: "方案基线", Status: "pending"},
		{Code: "readings", Label: "原始读数", Status: "pending"},
		{Code: "metrics", Label: "指标推导", Status: "pending"},
		{Code: "deviations", Label: "异常闭环", Status: "pending"},
		{Code: "audit", Label: "操作轨迹", Status: "pending"},
	}
}

func ValidateChecklist(items []ReviewChecklistItem, requirePassed bool) error {
	required := DefaultReviewChecklist()
	byCode := make(map[string]ReviewChecklistItem, len(items))
	for _, item := range items {
		if item.Status != "pending" && item.Status != "passed" && item.Status != "returned" {
			return invalid("checklist."+item.Code, "清单状态必须为 pending、passed 或 returned")
		}
		if item.Status == "returned" && strings.TrimSpace(item.Opinion) == "" {
			return invalid("checklist."+item.Code+".opinion", "退回清单项必须填写意见")
		}
		byCode[item.Code] = item
	}
	issues := make([]ValidationError, 0)
	for _, requiredItem := range required {
		item, ok := byCode[requiredItem.Code]
		if !ok || item.Status == "pending" || requirePassed && item.Status != "passed" {
			issues = append(issues, ValidationError{Field: "checklist." + requiredItem.Code, Message: fmt.Sprintf("复核清单“%s”尚未通过", requiredItem.Label)})
		}
	}
	if len(issues) > 0 {
		return ValidationErrors{Issues: issues}
	}
	return nil
}

func (d ReviewDifference) HasChanges() bool {
	return len(d.NewObservationIDs) > 0 || len(d.DeviationChanges) > 0 || len(d.MetricChanges) > 0
}

func (r ReviewRecord) Validate() error {
	if r.Reviewer == "" {
		return invalid("reviewer", "必须填写复核员")
	}
	if r.Decision != DecisionApproved && r.Opinion == "" {
		return invalid("opinion", "退回或重提必须填写明确意见")
	}
	if r.Decision == DecisionReturned && r.RequiredScope == "" {
		if len(r.CorrectionScope) == 0 {
			return invalid("correction_scope", "退回时必须选择结构化整改范围")
		}
	}
	return nil
}

func (a *GerminationAssay) ActiveCorrectionScope() []CorrectionTarget {
	for index := len(a.Reviews) - 1; index >= 0; index-- {
		if a.Reviews[index].Decision == DecisionReturned {
			return a.Reviews[index].CorrectionScope
		}
	}
	return nil
}

func (a *GerminationAssay) ObservationInCorrectionScope(day, replicate int) bool {
	for _, target := range a.ActiveCorrectionScope() {
		if target.Type == "observation" && target.DayNo == day && target.ReplicateNo == replicate {
			return true
		}
	}
	return false
}

func (a *GerminationAssay) DeviationInCorrectionScope(id string) bool {
	for _, target := range a.ActiveCorrectionScope() {
		if target.Type == "deviation" && target.DeviationID == id {
			return true
		}
	}
	return false
}
