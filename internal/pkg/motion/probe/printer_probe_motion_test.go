package probe

import (
	"reflect"
	"testing"
)

type fakeProbeMotionRuntime struct {
	core        *PrinterProbe
	homedAxes   string
	toolheadPos []float64
	probeTarget []float64
	probeSpeed  float64
	probeResult []float64
	responseLog []string
}

func (self *fakeProbeMotionRuntime) Core() *PrinterProbe {
	return self.core
}

func (self *fakeProbeMotionRuntime) HomedAxes() string {
	return self.homedAxes
}

func (self *fakeProbeMotionRuntime) ToolheadPosition() []float64 {
	return append([]float64{}, self.toolheadPos...)
}

func (self *fakeProbeMotionRuntime) ProbingMove(target []float64, speed float64) []float64 {
	self.probeTarget = append([]float64{}, target...)
	self.probeSpeed = speed
	return append([]float64{}, self.probeResult...)
}

func (self *fakeProbeMotionRuntime) RespondInfo(msg string, log bool) {
	_ = log
	self.responseLog = append(self.responseLog, msg)
}

func TestRunProbeMoveTargetsProbeZAndResponds(t *testing.T) {
	runtime := &fakeProbeMotionRuntime{
		core:        &PrinterProbe{ZPosition: -2.5},
		homedAxes:   "xyz",
		toolheadPos: []float64{10, 20, 30, 40},
		probeResult: []float64{11, 21, -1.25, 99},
	}

	result := RunProbeMove(runtime, 7.5)

	if got, want := runtime.probeTarget, []float64{10, 20, -2.5, 40}; !reflect.DeepEqual(got, want) {
		t.Fatalf("probe target = %v, want %v", got, want)
	}
	if runtime.probeSpeed != 7.5 {
		t.Fatalf("probe speed = %v, want 7.5", runtime.probeSpeed)
	}
	if got, want := result, []float64{11, 21, -1.25}; !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %v, want %v", got, want)
	}
	if got, want := runtime.responseLog, []string{"probe at 11.000,21.000 is z=-1.250000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("responseLog = %v, want %v", got, want)
	}
}

func TestRunProbeMoveRequiresZToBeHomed(t *testing.T) {
	runtime := &fakeProbeMotionRuntime{
		core:        &PrinterProbe{ZPosition: 0},
		homedAxes:   "xy",
		toolheadPos: []float64{10, 20, 30},
		probeResult: []float64{11, 21, -1.25},
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic when z is not homed")
		}
		if recovered != "Must home before probe" {
			t.Fatalf("panic = %v, want %q", recovered, "Must home before probe")
		}
	}()

	RunProbeMove(runtime, 7.5)
}
