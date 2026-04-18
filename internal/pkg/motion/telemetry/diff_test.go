package telemetry

import (
	"strings"
	"testing"
)

func almostEqual(got float64, want float64) bool {
	const eps = 1e-9
	if got > want {
		return got-want < eps
	}
	return want-got < eps
}

func TestParseSamplesSupportsJSONArrayAndNDJSON(t *testing.T) {
	arrayInput := `[
  {"time": 1.0, "segment_time": 0.05, "flush_time": 1.2, "queue_depth": 2, "need_flush_time": 1.4, "need_step_gen_time": 1.5},
  {"time": 2.0, "segment_time": 0.06, "flush_time": 1.4, "queue_depth": 3, "need_flush_time": 2.2, "need_step_gen_time": 2.4}
]`
	ndjsonInput := "{\"time\":1.0,\"segment_time\":0.05}\n{\"time\":2.0,\"segment_time\":0.06}\n"

	arraySamples, err := ParseSamples(strings.NewReader(arrayInput))
	if err != nil {
		t.Fatalf("parse array input: %v", err)
	}
	if len(arraySamples) != 2 {
		t.Fatalf("expected 2 samples from array input, got %d", len(arraySamples))
	}

	ndjsonSamples, err := ParseSamples(strings.NewReader(ndjsonInput))
	if err != nil {
		t.Fatalf("parse ndjson input: %v", err)
	}
	if len(ndjsonSamples) != 2 {
		t.Fatalf("expected 2 samples from ndjson input, got %d", len(ndjsonSamples))
	}
}

func TestComputeMetricsAndDiffReport(t *testing.T) {
	baseline := []Sample{
		{Time: 1.0, SegmentTime: 0.05, FlushTime: 1.1, QueueDepth: 2, NeedFlushTime: 1.3, NeedStepGenTime: 1.4},
		{Time: 2.0, SegmentTime: 0.06, FlushTime: 1.4, QueueDepth: 4, NeedFlushTime: 2.3, NeedStepGenTime: 2.6},
		{Time: 3.0, SegmentTime: 0.05, FlushTime: 1.9, QueueDepth: 3, NeedFlushTime: 3.4, NeedStepGenTime: 3.8},
	}
	candidate := []Sample{
		{Time: 1.0, SegmentTime: 0.04, FlushTime: 1.05, QueueDepth: 2, NeedFlushTime: 1.25, NeedStepGenTime: 1.35},
		{Time: 2.0, SegmentTime: 0.05, FlushTime: 1.35, QueueDepth: 3, NeedFlushTime: 2.2, NeedStepGenTime: 2.4},
		{Time: 3.0, SegmentTime: 0.04, FlushTime: 1.8, QueueDepth: 3, NeedFlushTime: 3.25, NeedStepGenTime: 3.6},
	}

	metrics := ComputeMetrics(baseline)
	if metrics.SampleCount != 3 {
		t.Fatalf("expected sample count 3, got %d", metrics.SampleCount)
	}
	if !almostEqual(metrics.SegmentTimingMean, (0.05+0.06+0.05)/3.0) {
		t.Fatalf("unexpected segment timing mean %v", metrics.SegmentTimingMean)
	}
	if !almostEqual(metrics.QueueDepthMean, 3.0) || metrics.QueueDepthMax != 4 {
		t.Fatalf("unexpected queue depth metrics %#v", metrics)
	}
	if !almostEqual(metrics.FlushDeadlineSlackMin, 0.3) {
		t.Fatalf("unexpected flush slack min %v", metrics.FlushDeadlineSlackMin)
	}
	if !almostEqual(metrics.StepDeadlineSlackMin, 0.4) {
		t.Fatalf("unexpected step slack min %v", metrics.StepDeadlineSlackMin)
	}

	report := BuildDiffReport(baseline, candidate)
	if report.Delta.SampleCount != 0 {
		t.Fatalf("expected equal sample counts, got delta %d", report.Delta.SampleCount)
	}
	if report.Delta.SegmentTimingMean >= 0 {
		t.Fatalf("expected candidate segment mean to be lower, got delta %v", report.Delta.SegmentTimingMean)
	}
	if report.Delta.FlushCadenceMean >= 0 {
		t.Fatalf("expected candidate flush cadence to be lower, got delta %v", report.Delta.FlushCadenceMean)
	}
}
