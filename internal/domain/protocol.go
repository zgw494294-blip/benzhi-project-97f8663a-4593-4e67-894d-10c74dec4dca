package domain

import (
	"strings"
	"time"
)

type AssayProtocol struct {
	TemperatureCelsius float64    `json:"temperature_celsius"`
	Substrate          string     `json:"substrate"`
	LightCycleHours    int        `json:"light_cycle_hours"`
	ObservationDays    int        `json:"observation_days"`
	ReplicateCount     int        `json:"replicate_count"`
	SeedsPerReplicate  int        `json:"seeds_per_replicate"`
	DispersionLimit    float64    `json:"dispersion_limit"`
	NormalSeedlingRule string     `json:"normal_seedling_rule"`
	FrozenAt           *time.Time `json:"frozen_at,omitempty"`
}

func (p AssayProtocol) Validate() error {
	issues := p.ValidationIssues()
	if len(issues) > 0 {
		return ValidationErrors{Issues: issues}
	}
	return nil
}

func (p AssayProtocol) ValidationIssues() []ValidationError {
	issues := make([]ValidationError, 0)
	if p.TemperatureCelsius < 5 || p.TemperatureCelsius > 45 {
		issues = append(issues, ValidationError{Field: "protocol.temperature_celsius", Message: "温度必须在 5°C 到 45°C 之间"})
	}
	if strings.TrimSpace(p.Substrate) == "" {
		issues = append(issues, ValidationError{Field: "protocol.substrate", Message: "必须填写培养基质"})
	}
	if p.LightCycleHours < 0 || p.LightCycleHours > 24 {
		issues = append(issues, ValidationError{Field: "protocol.light_cycle_hours", Message: "光照时长必须在 0 到 24 小时之间"})
	}
	if p.ObservationDays < 1 || p.ObservationDays > 60 {
		issues = append(issues, ValidationError{Field: "protocol.observation_days", Message: "观察天数必须在 1 到 60 天之间"})
	}
	if p.ReplicateCount < 2 || p.ReplicateCount > 20 {
		issues = append(issues, ValidationError{Field: "protocol.replicate_count", Message: "重复组数量必须在 2 到 20 之间"})
	}
	if p.SeedsPerReplicate < 1 || p.SeedsPerReplicate > 10000 {
		issues = append(issues, ValidationError{Field: "protocol.seeds_per_replicate", Message: "每组粒数必须在 1 到 10000 之间"})
	}
	if p.DispersionLimit < 0 || p.DispersionLimit > 1 {
		issues = append(issues, ValidationError{Field: "protocol.dispersion_limit", Message: "离散度阈值必须在 0 到 1 之间"})
	}
	if strings.TrimSpace(p.NormalSeedlingRule) == "" {
		issues = append(issues, ValidationError{Field: "protocol.normal_seedling_rule", Message: "必须填写正常幼苗判定规则"})
	}
	return issues
}

func (p AssayProtocol) IsFrozen() bool { return p.FrozenAt != nil }

func (p AssayProtocol) TotalSeeds() int { return p.ReplicateCount * p.SeedsPerReplicate }

func (p AssayProtocol) ObservationUnits() int { return p.ReplicateCount * p.ObservationDays }
