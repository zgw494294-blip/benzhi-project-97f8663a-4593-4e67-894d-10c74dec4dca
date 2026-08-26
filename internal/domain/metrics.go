package domain

import "math"

type DayMetric struct {
	DayNo               int     `json:"day_no"`
	ObservedReplicates  int     `json:"observed_replicates"`
	NormalSeeds         int     `json:"normal_seeds"`
	GerminationRate     float64 `json:"germination_rate"`
	ReplicateDispersion float64 `json:"replicate_dispersion"`
}

type MetricSnapshot struct {
	TotalSeeds          int         `json:"total_seeds"`
	LatestDay           int         `json:"latest_day"`
	CumulativeNormal    int         `json:"cumulative_normal"`
	CumulativeRate      float64     `json:"cumulative_rate"`
	GerminationVigor    float64     `json:"germination_vigor"`
	MaxDispersion       float64     `json:"max_dispersion"`
	CompleteObservation bool        `json:"complete_observation"`
	ByDay               []DayMetric `json:"by_day"`
}

func CalculateMetrics(p AssayProtocol, history []DailyObservation) MetricSnapshot {
	current := CurrentObservations(history)
	result := MetricSnapshot{TotalSeeds: p.TotalSeeds()}
	for day := 1; day <= p.ObservationDays; day++ {
		metric := DayMetric{DayNo: day}
		rates := make([]float64, 0, p.ReplicateCount)
		for rep := 1; rep <= p.ReplicateCount; rep++ {
			o, ok := current[ObservationKey(rep, day)]
			if !ok {
				continue
			}
			metric.ObservedReplicates++
			metric.NormalSeeds += o.NormalCount
			rates = append(rates, float64(o.NormalCount)/float64(p.SeedsPerReplicate))
		}
		if metric.ObservedReplicates > 0 {
			metric.GerminationRate = float64(metric.NormalSeeds) / float64(metric.ObservedReplicates*p.SeedsPerReplicate)
			metric.ReplicateDispersion = rangeOf(rates)
			result.LatestDay = day
			result.CumulativeNormal = metric.NormalSeeds
			result.CumulativeRate = metric.GerminationRate
			if metric.ReplicateDispersion > result.MaxDispersion {
				result.MaxDispersion = metric.ReplicateDispersion
			}
		}
		result.ByDay = append(result.ByDay, metric)
	}
	if len(result.ByDay) > 0 {
		result.GerminationVigor = result.ByDay[0].GerminationRate
	}
	result.CompleteObservation = len(current) == p.ObservationDays*p.ReplicateCount
	return result
}

func rangeOf(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	min, max := values[0], values[0]
	for _, value := range values[1:] {
		min = math.Min(min, value)
		max = math.Max(max, value)
	}
	return max - min
}
