package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

type Sample struct {
	Time            float64 `json:"time"`
	SegmentTime     float64 `json:"segment_time,omitempty"`
	FlushTime       float64 `json:"flush_time,omitempty"`
	QueueDepth      float64 `json:"queue_depth,omitempty"`
	NeedFlushTime   float64 `json:"need_flush_time,omitempty"`
	NeedStepGenTime float64 `json:"need_step_gen_time,omitempty"`
}

type Metrics struct {
	SampleCount            int     `json:"sample_count"`
	SegmentTimingMean      float64 `json:"segment_timing_mean"`
	SegmentTimingP95       float64 `json:"segment_timing_p95"`
	FlushCadenceMean       float64 `json:"flush_cadence_mean"`
	FlushCadenceP95        float64 `json:"flush_cadence_p95"`
	QueueDepthMean         float64 `json:"queue_depth_mean"`
	QueueDepthMax          float64 `json:"queue_depth_max"`
	FlushDeadlineSlackMean float64 `json:"flush_deadline_slack_mean"`
	FlushDeadlineSlackMin  float64 `json:"flush_deadline_slack_min"`
	StepDeadlineSlackMean  float64 `json:"step_deadline_slack_mean"`
	StepDeadlineSlackMin   float64 `json:"step_deadline_slack_min"`
}

type DeltaMetrics struct {
	SampleCount            int     `json:"sample_count"`
	SegmentTimingMean      float64 `json:"segment_timing_mean"`
	SegmentTimingP95       float64 `json:"segment_timing_p95"`
	FlushCadenceMean       float64 `json:"flush_cadence_mean"`
	FlushCadenceP95        float64 `json:"flush_cadence_p95"`
	QueueDepthMean         float64 `json:"queue_depth_mean"`
	QueueDepthMax          float64 `json:"queue_depth_max"`
	FlushDeadlineSlackMean float64 `json:"flush_deadline_slack_mean"`
	FlushDeadlineSlackMin  float64 `json:"flush_deadline_slack_min"`
	StepDeadlineSlackMean  float64 `json:"step_deadline_slack_mean"`
	StepDeadlineSlackMin   float64 `json:"step_deadline_slack_min"`
}

type DiffReport struct {
	Baseline  Metrics      `json:"baseline"`
	Candidate Metrics      `json:"candidate"`
	Delta     DeltaMetrics `json:"delta"`
}

func ParseSamples(r io.Reader) ([]Sample, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var samples []Sample
		if err := json.Unmarshal(trimmed, &samples); err != nil {
			return nil, fmt.Errorf("parse telemetry json array: %w", err)
		}
		return samples, nil
	}
	return parseNDJSON(trimmed)
}

func parseNDJSON(data []byte) ([]Sample, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	samples := make([]Sample, 0, 256)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var sample Sample
		if err := json.Unmarshal([]byte(line), &sample); err != nil {
			return nil, fmt.Errorf("parse telemetry ndjson line %d: %w", lineNo, err)
		}
		samples = append(samples, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func ComputeMetrics(samples []Sample) Metrics {
	metrics := Metrics{SampleCount: len(samples)}
	if len(samples) == 0 {
		return metrics
	}

	segmentValues := make([]float64, 0, len(samples))
	queueValues := make([]float64, 0, len(samples))
	flushSlack := make([]float64, 0, len(samples))
	stepSlack := make([]float64, 0, len(samples))
	flushCadence := make([]float64, 0, len(samples))
	lastFlushTime := math.NaN()

	for _, sample := range samples {
		if sample.SegmentTime > 0 {
			segmentValues = append(segmentValues, sample.SegmentTime)
		}
		queueValues = append(queueValues, sample.QueueDepth)
		if sample.NeedFlushTime != 0 {
			flushSlack = append(flushSlack, sample.NeedFlushTime-sample.Time)
		}
		if sample.NeedStepGenTime != 0 {
			stepSlack = append(stepSlack, sample.NeedStepGenTime-sample.Time)
		}
		if !math.IsNaN(lastFlushTime) && sample.FlushTime > lastFlushTime {
			flushCadence = append(flushCadence, sample.FlushTime-lastFlushTime)
		}
		if sample.FlushTime > 0 {
			lastFlushTime = sample.FlushTime
		}
	}

	metrics.SegmentTimingMean = mean(segmentValues)
	metrics.SegmentTimingP95 = percentile(segmentValues, 0.95)
	metrics.FlushCadenceMean = mean(flushCadence)
	metrics.FlushCadenceP95 = percentile(flushCadence, 0.95)
	metrics.QueueDepthMean = mean(queueValues)
	metrics.QueueDepthMax = max(queueValues)
	metrics.FlushDeadlineSlackMean = mean(flushSlack)
	metrics.FlushDeadlineSlackMin = min(flushSlack)
	metrics.StepDeadlineSlackMean = mean(stepSlack)
	metrics.StepDeadlineSlackMin = min(stepSlack)
	return metrics
}

func BuildDiffReport(baseline []Sample, candidate []Sample) DiffReport {
	baseMetrics := ComputeMetrics(baseline)
	candidateMetrics := ComputeMetrics(candidate)
	return DiffReport{
		Baseline:  baseMetrics,
		Candidate: candidateMetrics,
		Delta: DeltaMetrics{
			SampleCount:            candidateMetrics.SampleCount - baseMetrics.SampleCount,
			SegmentTimingMean:      candidateMetrics.SegmentTimingMean - baseMetrics.SegmentTimingMean,
			SegmentTimingP95:       candidateMetrics.SegmentTimingP95 - baseMetrics.SegmentTimingP95,
			FlushCadenceMean:       candidateMetrics.FlushCadenceMean - baseMetrics.FlushCadenceMean,
			FlushCadenceP95:        candidateMetrics.FlushCadenceP95 - baseMetrics.FlushCadenceP95,
			QueueDepthMean:         candidateMetrics.QueueDepthMean - baseMetrics.QueueDepthMean,
			QueueDepthMax:          candidateMetrics.QueueDepthMax - baseMetrics.QueueDepthMax,
			FlushDeadlineSlackMean: candidateMetrics.FlushDeadlineSlackMean - baseMetrics.FlushDeadlineSlackMean,
			FlushDeadlineSlackMin:  candidateMetrics.FlushDeadlineSlackMin - baseMetrics.FlushDeadlineSlackMin,
			StepDeadlineSlackMean:  candidateMetrics.StepDeadlineSlackMean - baseMetrics.StepDeadlineSlackMean,
			StepDeadlineSlackMin:   candidateMetrics.StepDeadlineSlackMin - baseMetrics.StepDeadlineSlackMin,
		},
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	best := values[0]
	for _, v := range values[1:] {
		if v < best {
			best = v
		}
	}
	return best
}

func max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	best := values[0]
	for _, v := range values[1:] {
		if v > best {
			best = v
		}
	}
	return best
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		return min(values)
	}
	if p >= 1 {
		return max(values)
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
