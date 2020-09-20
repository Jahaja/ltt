package ltt

import (
	"sort"
	"sync"
	"time"
)

type TaskStats struct {
	sync.Mutex
	Name            string           `json:"name"`
	TotalRuns       int64            `json:"total_runs"`
	NumSuccessful   int64            `json:"num_successful"`
	NumFailed       int64            `json:"num_failed"`
	TotalDuration   int64            `json:"total_duration"`
	Metrics         map[int64]int64  `json:"-"`
	Percentiles     map[int]int64    `json:"percentiles"`
	AverageDuration float32          `json:"average_duration"`
	Errors          map[string]int64 `json:"errors"`
}

func (ts *TaskStats) Calculate() {
	const MinRunsToCalculate = 10
	percentiles := []float64{0.5, 0.75, 0.85, 0.95, 0.99}

	if ts.TotalRuns < MinRunsToCalculate {
		// set to zero to make output more consistent
		for _, p := range percentiles {
			ts.Percentiles[int(p*100)] = 0
		}

		return
	}

	type flatTaskStats struct {
		Duration int64
		Count    int64
	}

	flatMetrics := make([]flatTaskStats, len(ts.Metrics))
	for d, c := range ts.Metrics {
		flatMetrics = append(flatMetrics, flatTaskStats{d, c})
	}

	sort.Slice(flatMetrics, func(i, j int) bool {
		return flatMetrics[i].Duration < flatMetrics[j].Duration
	})

	for _, p := range percentiles {
		idx := int64(float64(ts.TotalRuns) * p)

		var i int64
		for _, fm := range flatMetrics {
			i += fm.Count

			if i >= idx {
				ts.Percentiles[int(p*100)] = fm.Duration
				break
			}
		}
	}

	ts.AverageDuration = float32(ts.TotalDuration) / float32(ts.TotalRuns)
}

func NewTaskStat(name string) *TaskStats {
	return &TaskStats{
		Name:        name,
		Metrics:     make(map[int64]int64),
		Percentiles: make(map[int]int64),
		Errors:      make(map[string]int64),
	}
}

type Statistics struct {
	sync.Mutex
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	NumTotal      int64     `json:"num_total"`
	RunningUsers  int       `json:"num_users"`
	NumSuccessful int64     `json:"num_successful"`
	NumFailed     int64     `json:"num_failed"`
	TotalDuration int64     `json:"total_duration"`
	// unix-timestamp -> count map to calculate a current RPS value
	RPSMap map[int64]int64 `json:"-"`

	Tasks           map[string]*TaskStats `json:"tasks"`
	CurrentRPS      float32               `json:"current_rps"`
	AverageDuration float32               `json:"average_duration"`
}

const RPSTimeWindow = 10

func (ts *Statistics) Reset() {
	ts.StartTime = time.Now()
	ts.NumTotal = 0
	ts.NumSuccessful = 0
	ts.NumFailed = 0
	ts.TotalDuration = 0
	ts.RPSMap = map[int64]int64{}
	ts.Tasks = map[string]*TaskStats{}
	ts.CurrentRPS = 0
	ts.AverageDuration = 0
}

func (ts *Statistics) CleanRPSMap() {
	now := time.Now().Unix()

	var count int64
	for uts := range ts.RPSMap {
		if uts < now-RPSTimeWindow {
			delete(ts.RPSMap, uts)
		}
	}

	ts.CurrentRPS = float32(count) / float32(RPSTimeWindow)
}

func (ts *Statistics) Calculate() {
	const MinRunsToCalculate = 10
	if ts.NumTotal < MinRunsToCalculate {
		return
	}

	now := time.Now().Unix()

	var count int64
	for uts, c := range ts.RPSMap {
		if uts >= now-RPSTimeWindow {
			count += c
		}
	}

	ts.CurrentRPS = float32(count) / float32(RPSTimeWindow)
	ts.AverageDuration = float32(ts.TotalDuration) / float32(ts.NumTotal)

	for _, t := range ts.Tasks {
		t.Calculate()
	}
}

func NewStatistics() *Statistics {
	return &Statistics{
		Tasks:           make(map[string]*TaskStats),
		RPSMap:          make(map[int64]int64),
		CurrentRPS:      0,
		AverageDuration: 0,
	}
}
