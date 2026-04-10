package homing

import (
	"errors"
	"reflect"
	"testing"
)

func TestRequestedAxesDefaultsToAllAxes(t *testing.T) {
	got := RequestedAxes(func(string) bool { return false })
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RequestedAxes() = %#v, want %#v", got, want)
	}
}

func TestRequestedAxesHonorsExplicitSelection(t *testing.T) {
	got := RequestedAxes(func(axis string) bool {
		return axis == "Y" || axis == "Z"
	})
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RequestedAxes() = %#v, want %#v", got, want)
	}
}

func TestCommandG28SetsAxesAndHomes(t *testing.T) {
	var setAxes [][]int
	homed := false

	CommandG28(
		func(axis string) bool { return axis == "X" },
		func(axes []int) {
			setAxes = append(setAxes, append([]int{}, axes...))
		},
		func() {
			homed = true
		},
		HomeRecoveryOptions{},
	)

	if len(setAxes) != 1 || !reflect.DeepEqual(setAxes[0], []int{0}) {
		t.Fatalf("unexpected axes set calls %#v", setAxes)
	}
	if !homed {
		t.Fatal("expected home callback to run")
	}
}

func TestRunHomeDisablesMotorsOnRecoveredError(t *testing.T) {
	motorOffCalls := 0
	defer func() {
		recovered := recover()
		err, ok := recovered.(error)
		if !ok || err == nil || err.Error() != "boom" {
			t.Fatalf("expected recovered boom error, got %#v", recovered)
		}
		if motorOffCalls != 1 {
			t.Fatalf("expected one motor-off call, got %d", motorOffCalls)
		}
	}()

	RunHome(func() {
		panic(errors.New("boom"))
	}, HomeRecoveryOptions{
		IsShutdown: func() bool { return false },
		MotorOff: func() {
			motorOffCalls++
		},
	})
}

func TestRunHomeUsesShutdownFailureMessage(t *testing.T) {
	defer func() {
		recovered := recover()
		message, ok := recovered.(string)
		if !ok || message != "Homing failed due to printer shutdown" {
			t.Fatalf("expected shutdown homing failure, got %#v", recovered)
		}
	}()

	RunHome(func() {
		panic(errors.New("boom"))
	}, HomeRecoveryOptions{
		IsShutdown: func() bool { return true },
	})
}

func TestManualHomeReturnsExecutionError(t *testing.T) {
	expected := errors.New("move failed")
	err := ManualHome(
		func([]float64, float64, bool, bool, bool) ([]float64, float64, error) {
			return nil, 0, expected
		},
		[]float64{1, 2, 3, 4},
		25,
		true,
		true,
	)
	if !errors.Is(err, expected) {
		t.Fatalf("expected manual home error %v, got %v", expected, err)
	}
}

func TestProbingMoveReturnsTriggerPosition(t *testing.T) {
	got, err := ProbingMove(
		func(pos []float64, speed float64, probePos bool, triggered bool, checkTriggered bool) ([]float64, float64, error) {
			if !reflect.DeepEqual(pos, []float64{5, 0, 0, 0}) || speed != 10 || !probePos || !triggered || !checkTriggered {
				t.Fatalf("unexpected probing invocation pos=%#v speed=%v probe=%v triggered=%v check=%v", pos, speed, probePos, triggered, checkTriggered)
			}
			return []float64{4.5, 0, 0, 0}, 1.25, nil
		},
		func() string { return "" },
		[]float64{5, 0, 0, 0},
		10,
	)
	if err != nil {
		t.Fatalf("unexpected probing move error: %v", err)
	}
	if !reflect.DeepEqual(got, []float64{4.5, 0, 0, 0}) {
		t.Fatalf("unexpected probing position %#v", got)
	}
}

func TestProbingMoveDetectsEarlyTrigger(t *testing.T) {
	_, err := ProbingMove(
		func([]float64, float64, bool, bool, bool) ([]float64, float64, error) {
			return []float64{0, 0, 0, 0}, 0, nil
		},
		func() string { return "probe" },
		[]float64{0, 0, 0, 0},
		5,
	)
	if err == nil || err.Error() != "Probe triggered prior to movement" {
		t.Fatalf("expected early-trigger error, got %v", err)
	}
}
