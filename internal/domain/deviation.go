package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type DeviationStatus string

const (
	DeviationOpen   DeviationStatus = "open"
	DeviationClosed DeviationStatus = "closed"
)

type DeviationCase struct {
	ID                   string          `json:"id"`
	AssayID              string          `json:"assay_id"`
	RuleCode             string          `json:"rule_code"`
	Occurrence           int             `json:"occurrence"`
	Severity             string          `json:"severity"`
	Status               DeviationStatus `json:"status"`
	TargetDays           []int           `json:"target_days"`
	TargetReplicates     []int           `json:"target_replicates"`
	TriggerMetric        string          `json:"trigger_metric"`
	CurrentVerification  string          `json:"current_verification"`
	Reason               string          `json:"reason,omitempty"`
	CorrectiveAction     string          `json:"corrective_action,omitempty"`
	RetestObservationIDs []string        `json:"retest_observation_ids,omitempty"`
	OpenedAt             time.Time       `json:"opened_at"`
	ClosedAt             *time.Time      `json:"closed_at,omitempty"`
}

type RuleFinding struct {
	RuleCode         string `json:"rule_code"`
	Severity         string `json:"severity"`
	TargetDays       []int  `json:"target_days"`
	TargetReplicates []int  `json:"target_replicates"`
	Result           string `json:"result"`
}

func EvaluateFindings(p AssayProtocol, observations []DailyObservation, requireComplete bool) []RuleFinding {
	current := CurrentObservations(observations)
	findings := make([]RuleFinding, 0)
	if requireComplete {
		for day := 1; day <= p.ObservationDays; day++ {
			for rep := 1; rep <= p.ReplicateCount; rep++ {
				if _, ok := current[ObservationKey(rep, day)]; !ok {
					findings = append(findings, RuleFinding{RuleCode: fmt.Sprintf("MISSING_D%d_R%d", day, rep), Severity: "high", TargetDays: []int{day}, TargetReplicates: []int{rep}, Result: "目标观察单元缺少有效读数"})
				}
			}
		}
	}
	for key, observation := range current {
		total, overflowed := observation.safeTotal(p.SeedsPerReplicate)
		if overflowed || total != p.SeedsPerReplicate {
			findings = append(findings, RuleFinding{RuleCode: "CONSERVATION_" + key, Severity: "critical", TargetDays: []int{observation.DayNo}, TargetReplicates: []int{observation.ReplicateNo}, Result: fmt.Sprintf("分类合计 %d，要求 %d", total, p.SeedsPerReplicate)})
		}
	}
	metrics := CalculateMetrics(p, observations)
	for _, day := range metrics.ByDay {
		if day.ObservedReplicates == p.ReplicateCount && day.ReplicateDispersion > p.DispersionLimit {
			reps := make([]int, p.ReplicateCount)
			for index := range reps {
				reps[index] = index + 1
			}
			findings = append(findings, RuleFinding{RuleCode: fmt.Sprintf("DISPERSION_D%d", day.DayNo), Severity: "medium", TargetDays: []int{day.DayNo}, TargetReplicates: reps, Result: fmt.Sprintf("离散度 %.4f，阈值 %.4f", day.ReplicateDispersion, p.DispersionLimit)})
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].RuleCode < findings[j].RuleCode })
	return findings
}

func VerificationForRule(p AssayProtocol, observations []DailyObservation, ruleCode string, requireComplete bool) (bool, string) {
	for _, finding := range EvaluateFindings(p, observations, requireComplete) {
		if finding.RuleCode == ruleCode {
			return false, finding.Result
		}
	}
	if strings.HasPrefix(ruleCode, "MISSING_") {
		return true, "目标观察单元已补录有效读数"
	}
	if strings.HasPrefix(ruleCode, "DISPERSION_") {
		return true, "目标观察日离散度已回到阈值内"
	}
	return true, "目标规则复验通过"
}

func OpenDeviationCount(items []DeviationCase) int {
	total := 0
	for _, item := range items {
		if item.Status == DeviationOpen {
			total++
		}
	}
	return total
}

func (d DeviationCase) ContainsObservation(observation DailyObservation) bool {
	dayMatch, repMatch := false, false
	for _, day := range d.TargetDays {
		dayMatch = dayMatch || day == observation.DayNo
	}
	for _, rep := range d.TargetReplicates {
		repMatch = repMatch || rep == observation.ReplicateNo
	}
	return dayMatch && repMatch
}

func (d DeviationCase) MissingClosureConditions(rulePassed bool) []string {
	missing := make([]string, 0)
	if strings.TrimSpace(d.Reason) == "" {
		missing = append(missing, "异常原因")
	}
	if strings.TrimSpace(d.CorrectiveAction) == "" {
		missing = append(missing, "补测动作")
	}
	if len(d.RetestObservationIDs) == 0 {
		missing = append(missing, "有效补测证据")
	}
	if !rulePassed {
		missing = append(missing, "规则复验通过")
	}
	return missing
}
