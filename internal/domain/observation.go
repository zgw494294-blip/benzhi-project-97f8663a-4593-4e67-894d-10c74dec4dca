package domain

import (
	"fmt"
	"sort"
	"time"
)

type DailyObservation struct {
	ID                string    `json:"id"`
	AssayID           string    `json:"assay_id"`
	ReplicateNo       int       `json:"replicate_no"`
	DayNo             int       `json:"day_no"`
	NormalCount       int       `json:"normal_count"`
	AbnormalCount     int       `json:"abnormal_count"`
	HardSeedCount     int       `json:"hard_seed_count"`
	RottenCount       int       `json:"rotten_count"`
	UngerminatedCount int       `json:"ungerminated_count"`
	RecordedBy        string    `json:"recorded_by"`
	RecordedAt        time.Time `json:"recorded_at"`
	SupersedesID      string    `json:"supersedes_id,omitempty"`
}

func (o DailyObservation) Total() int {
	return o.NormalCount + o.AbnormalCount + o.HardSeedCount + o.RottenCount + o.UngerminatedCount
}

func (o DailyObservation) Validate(p AssayProtocol) error {
	if o.ReplicateNo < 1 || o.ReplicateNo > p.ReplicateCount {
		return invalid("replicate_no", fmt.Sprintf("重复组必须在 1 到 %d 之间", p.ReplicateCount))
	}
	if o.DayNo < 1 || o.DayNo > p.ObservationDays {
		return invalid("day_no", fmt.Sprintf("观察日必须在 1 到 %d 之间", p.ObservationDays))
	}
	if o.NormalCount < 0 || o.AbnormalCount < 0 || o.HardSeedCount < 0 || o.RottenCount < 0 || o.UngerminatedCount < 0 {
		return invalid("counts", "分类计数不能为负数")
	}
	if o.Total() != p.SeedsPerReplicate {
		return invalid("counts", fmt.Sprintf("分类计数之和必须等于每组粒数 %d，当前为 %d", p.SeedsPerReplicate, o.Total()))
	}
	if o.RecordedBy == "" {
		return invalid("recorded_by", "必须填写记录人员")
	}
	return nil
}

func ObservationKey(replicate, day int) string {
	return fmt.Sprintf("%d:%d", replicate, day)
}

func CurrentObservations(history []DailyObservation) map[string]DailyObservation {
	current := make(map[string]DailyObservation)
	for _, item := range history {
		key := ObservationKey(item.ReplicateNo, item.DayNo)
		old, ok := current[key]
		if !ok || item.RecordedAt.After(old.RecordedAt) || item.ID > old.ID {
			current[key] = item
		}
	}
	return current
}

func ValidateDailyBatch(p AssayProtocol, day int, observations []DailyObservation) []ValidationError {
	issues := make([]ValidationError, 0)
	seen := make(map[int]bool, len(observations))
	for index, item := range observations {
		prefix := fmt.Sprintf("observations[%d]", index)
		if item.DayNo != day {
			issues = append(issues, ValidationError{Field: prefix + ".day_no", Message: fmt.Sprintf("整日提交只能包含第 %d 日读数", day)})
		}
		if seen[item.ReplicateNo] {
			issues = append(issues, ValidationError{Field: prefix + ".replicate_no", Message: fmt.Sprintf("重复组 R%d 在本次提交中重复", item.ReplicateNo)})
		}
		seen[item.ReplicateNo] = true
		if err := item.Validate(p); err != nil {
			var validation ValidationError
			var validations ValidationErrors
			switch typed := err.(type) {
			case ValidationError:
				validation = typed
				issues = append(issues, ValidationError{Field: prefix + "." + validation.Field, Message: validation.Message})
			case ValidationErrors:
				validations = typed
				for _, issue := range validations.Issues {
					issues = append(issues, ValidationError{Field: prefix + "." + issue.Field, Message: issue.Message})
				}
			}
		}
	}
	for rep := 1; rep <= p.ReplicateCount; rep++ {
		if !seen[rep] {
			issues = append(issues, ValidationError{Field: fmt.Sprintf("day_%d.replicate_%d", day, rep), Message: fmt.Sprintf("缺少重复组 R%d 的整日读数", rep)})
		}
	}
	return issues
}

func ValidateObservationTimeline(p AssayProtocol, history []DailyObservation) []ValidationError {
	current := CurrentObservations(history)
	issues := make([]ValidationError, 0)
	for rep := 1; rep <= p.ReplicateCount; rep++ {
		var previous *DailyObservation
		for day := 1; day <= p.ObservationDays; day++ {
			item, ok := current[ObservationKey(rep, day)]
			if !ok {
				continue
			}
			if previous != nil {
				prefix := fmt.Sprintf("day_%d.replicate_%d", day, rep)
				if item.NormalCount < previous.NormalCount {
					issues = append(issues, ValidationError{Field: prefix + ".normal_count", Message: fmt.Sprintf("第 %d 日正常幼苗不能少于前一有效观察日的 %d 粒", day, previous.NormalCount)})
				}
				if item.AbnormalCount < previous.AbnormalCount {
					issues = append(issues, ValidationError{Field: prefix + ".abnormal_count", Message: fmt.Sprintf("第 %d 日异常幼苗不能无依据减少", day)})
				}
				if item.RottenCount < previous.RottenCount {
					issues = append(issues, ValidationError{Field: prefix + ".rotten_count", Message: fmt.Sprintf("第 %d 日腐烂粒不能无依据减少", day)})
				}
				determinedIncrease := item.NormalCount - previous.NormalCount + item.AbnormalCount - previous.AbnormalCount + item.RottenCount - previous.RottenCount
				availableTransfer := previous.HardSeedCount + previous.UngerminatedCount - item.HardSeedCount - item.UngerminatedCount
				if determinedIncrease > availableTransfer {
					issues = append(issues, ValidationError{Field: prefix + ".hard_seed_count", Message: "硬实粒与未发芽粒的转出量不能解释新增判定数量"})
				}
			}
			copy := item
			previous = &copy
		}
	}
	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Field < issues[j].Field })
	return issues
}
