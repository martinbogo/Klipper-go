package probe

import (
	"reflect"
	"testing"
)

func TestProbePointsHelperOffsetsAndSequencing(t *testing.T) {
	calls := 0
	helper := NewProbePointsHelper("bed_mesh", func(offsets []float64, results [][]float64) string {
		calls++
		return "done"
	}, [][]float64{{10, 20}, {30, 40}}, 5, 50)
	helper.UseXYOffsets(true)
	helper.BeginAutomaticSession(12, []float64{1, 2, 3})

	done, retry, target := helper.NextProbePoint()
	if done || retry {
		t.Fatalf("NextProbePoint() unexpectedly finished early")
	}
	if got, want := target, []float64{9, 18}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NextProbePoint() target = %v, want %v", got, want)
	}

	helper.AppendResult([]float64{1, 2, 3})
	_, _, target = helper.NextProbePoint()
	if got, want := target, []float64{29, 38}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NextProbePoint() second target = %v, want %v", got, want)
	}

	helper.AppendResult([]float64{4, 5, 6})
	done, retry, target = helper.NextProbePoint()
	if !done || retry || target != nil {
		t.Fatalf("NextProbePoint() final state = done:%v retry:%v target:%v", done, retry, target)
	}
	if calls != 1 {
		t.Fatalf("finalize callback calls = %d, want 1", calls)
	}
}

func TestProbePointsHelperRetryResetsResults(t *testing.T) {
	helper := NewProbePointsHelper("retry_case", func(offsets []float64, results [][]float64) string {
		return "retry"
	}, [][]float64{{10, 20}}, 5, 50)
	helper.BeginManualSession()
	helper.AppendResult([]float64{1, 2, 3})

	done, retry, target := helper.NextProbePoint()
	if done || !retry {
		t.Fatalf("NextProbePoint() = done:%v retry:%v", done, retry)
	}
	if helper.ResultCount() != 0 {
		t.Fatalf("ResultCount() = %d, want 0 after retry", helper.ResultCount())
	}
	if got, want := target, []float64{10, 20}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NextProbePoint() retry target = %v, want %v", got, want)
	}
}
