package motion

import "testing"

type fakeToolheadStatsRuntime struct {
	printTime           float64
	lastFlushTime       float64
	estimatedPrintTime  float64
	printStall          float64
	specialQueuingState string
	clearHistoryTime    float64
	checkCalls          []struct {
		maxQueueTime float64
		eventtime    float64
	}
}

func (self *fakeToolheadStatsRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadStatsRuntime) LastFlushTime() float64 {
	return self.lastFlushTime
}

func (self *fakeToolheadStatsRuntime) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	return self.estimatedPrintTime
}

func (self *fakeToolheadStatsRuntime) PrintStall() float64 {
	return self.printStall
}

func (self *fakeToolheadStatsRuntime) SpecialQueuingState() string {
	return self.specialQueuingState
}

func (self *fakeToolheadStatsRuntime) SetClearHistoryTime(value float64) {
	self.clearHistoryTime = value
}

func (self *fakeToolheadStatsRuntime) CheckActiveDrivers(maxQueueTime float64, eventtime float64) {
	self.checkCalls = append(self.checkCalls, struct {
		maxQueueTime float64
		eventtime    float64
	}{maxQueueTime: maxQueueTime, eventtime: eventtime})
}

type fakeToolheadBusyRuntime struct {
	printTime          float64
	estimatedPrintTime float64
	lookaheadEmpty     bool
}

func (self *fakeToolheadBusyRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadBusyRuntime) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	return self.estimatedPrintTime
}

func (self *fakeToolheadBusyRuntime) LookaheadEmpty() bool {
	return self.lookaheadEmpty
}

type fakeToolheadStatusRuntime struct {
	kinematicsStatus   map[string]interface{}
	printTime          float64
	estimatedPrintTime float64
	printStall         float64
	extruderName       string
	commandedPosition  []float64
	velocitySettings   ToolheadVelocitySettings
}

func (self *fakeToolheadStatusRuntime) KinematicsStatus(eventtime float64) map[string]interface{} {
	_ = eventtime
	cloned := make(map[string]interface{}, len(self.kinematicsStatus))
	for key, value := range self.kinematicsStatus {
		cloned[key] = value
	}
	return cloned
}

func (self *fakeToolheadStatusRuntime) PrintTime() float64 {
	return self.printTime
}

func (self *fakeToolheadStatusRuntime) EstimatedPrintTime(eventtime float64) float64 {
	_ = eventtime
	return self.estimatedPrintTime
}

func (self *fakeToolheadStatusRuntime) PrintStall() float64 {
	return self.printStall
}

func (self *fakeToolheadStatusRuntime) ExtruderName() string {
	return self.extruderName
}

func (self *fakeToolheadStatusRuntime) CommandedPosition() []float64 {
	return append([]float64{}, self.commandedPosition...)
}

func (self *fakeToolheadStatusRuntime) VelocitySettings() ToolheadVelocitySettings {
	return self.velocitySettings
}

type fakeToolheadHomingRuntime struct {
	status map[string]interface{}
}

func (self *fakeToolheadHomingRuntime) KinematicsStatus(eventtime float64) map[string]interface{} {
	_ = eventtime
	return self.status
}

type fakeToolheadZHoming struct {
	noted bool
}

func (self *fakeToolheadZHoming) Note_z_not_homed() {
	self.noted = true
}

func TestBuildToolheadStatsNormalState(t *testing.T) {
	result := BuildToolheadStats(ToolheadStatsSnapshot{
		PrintTime:           25.0,
		LastFlushTime:       24.0,
		EstimatedPrintTime:  23.5,
		PrintStall:          2,
		MoveHistoryExpire:   30.0,
		SpecialQueuingState: "",
	})

	if !almostEqualFloat64(result.MaxQueueTime, 25.0) {
		t.Fatalf("unexpected max queue time %v", result.MaxQueueTime)
	}
	if !almostEqualFloat64(result.ClearHistoryTime, -6.5) {
		t.Fatalf("unexpected clear history time %v", result.ClearHistoryTime)
	}
	if !almostEqualFloat64(result.BufferTime, 1.5) {
		t.Fatalf("unexpected buffer time %v", result.BufferTime)
	}
	if !result.IsActive {
		t.Fatal("expected active toolhead stats")
	}
	if result.Summary != "print_time=25.000 buffer_time=1.500 print_stall=2" {
		t.Fatalf("unexpected summary %q", result.Summary)
	}
}

func TestBuildToolheadStatsReportUpdatesRuntimeState(t *testing.T) {
	runtime := &fakeToolheadStatsRuntime{
		printTime:           25.0,
		lastFlushTime:       24.0,
		estimatedPrintTime:  23.5,
		printStall:          2,
		specialQueuingState: "",
	}

	isActive, summary := BuildToolheadStatsReport(runtime, 12.0, 30.0)

	if !isActive {
		t.Fatal("expected active stats report")
	}
	if summary != "print_time=25.000 buffer_time=1.500 print_stall=2" {
		t.Fatalf("unexpected summary %q", summary)
	}
	if !almostEqualFloat64(runtime.clearHistoryTime, -6.5) {
		t.Fatalf("unexpected clear history time %v", runtime.clearHistoryTime)
	}
	if len(runtime.checkCalls) != 1 || !almostEqualFloat64(runtime.checkCalls[0].maxQueueTime, 25.0) || !almostEqualFloat64(runtime.checkCalls[0].eventtime, 12.0) {
		t.Fatalf("unexpected driver check calls %#v", runtime.checkCalls)
	}
}

func TestBuildToolheadStatsDripStateClampsDisplayBuffer(t *testing.T) {
	result := BuildToolheadStats(ToolheadStatsSnapshot{
		PrintTime:           10.0,
		LastFlushTime:       9.0,
		EstimatedPrintTime:  8.0,
		PrintStall:          0,
		MoveHistoryExpire:   30.0,
		SpecialQueuingState: "Drip",
	})

	if !almostEqualFloat64(result.BufferTime, 0.0) {
		t.Fatalf("expected drip buffer time to clamp to zero, got %v", result.BufferTime)
	}
	if result.Summary != "print_time=10.000 buffer_time=0.000 print_stall=0" {
		t.Fatalf("unexpected drip summary %q", result.Summary)
	}
	if !result.IsActive {
		t.Fatal("expected drip toolhead stats to remain active")
	}
}

func TestBuildToolheadBusyState(t *testing.T) {
	result := BuildToolheadBusyState(12.0, 11.5, true)
	if !almostEqualFloat64(result.PrintTime, 12.0) || !almostEqualFloat64(result.EstimatedPrintTime, 11.5) || !result.LookaheadEmpty {
		t.Fatalf("unexpected busy state %#v", result)
	}
}

func TestBuildToolheadBusyReportReadsRuntime(t *testing.T) {
	runtime := &fakeToolheadBusyRuntime{printTime: 12.0, estimatedPrintTime: 11.5, lookaheadEmpty: true}
	result := BuildToolheadBusyReport(runtime, 9.0)
	if !almostEqualFloat64(result.PrintTime, 12.0) || !almostEqualFloat64(result.EstimatedPrintTime, 11.5) || !result.LookaheadEmpty {
		t.Fatalf("unexpected busy report %#v", result)
	}
}

func TestBuildToolheadStatusClonesInputs(t *testing.T) {
	kinematicsStatus := map[string]interface{}{"homed_axes": "xyz"}
	position := []float64{1, 2, 3, 4}

	status := BuildToolheadStatus(ToolheadStatusSnapshot{
		KinematicsStatus:      kinematicsStatus,
		PrintTime:             7.5,
		EstimatedPrintTime:    7.0,
		PrintStall:            3,
		ExtruderName:          "extruder",
		CommandedPosition:     position,
		MaxVelocity:           300,
		MaxAccel:              5000,
		RequestedAccelToDecel: 2500,
		SquareCornerVelocity:  5,
	})

	position[0] = 99
	kinematicsStatus["homed_axes"] = "xy"

	if status["homed_axes"] != "xyz" {
		t.Fatalf("expected cloned kinematics status, got %#v", status)
	}
	statusPosition, ok := status["position"].([]float64)
	if !ok {
		t.Fatalf("unexpected position type %T", status["position"])
	}
	if statusPosition[0] != 1 {
		t.Fatalf("expected cloned position, got %#v", statusPosition)
	}
	if status["extruder"] != "extruder" || status["max_accel_to_decel"] != 2500.0 {
		t.Fatalf("unexpected status payload %#v", status)
	}
	if status["minimum_cruise_ratio"] != 0.5 {
		t.Fatalf("unexpected minimum_cruise_ratio in status %#v", status)
	}
}

func TestBuildToolheadStatusReportUsesRuntimeSnapshot(t *testing.T) {
	runtime := &fakeToolheadStatusRuntime{
		kinematicsStatus:   map[string]interface{}{"homed_axes": "xyz"},
		printTime:          7.5,
		estimatedPrintTime: 7.0,
		printStall:         3,
		extruderName:       "extruder",
		commandedPosition:  []float64{1, 2, 3, 4},
		velocitySettings: ToolheadVelocitySettings{
			MaxVelocity:           300,
			MaxAccel:              5000,
			RequestedAccelToDecel: 2500,
			SquareCornerVelocity:  5,
		},
	}

	status := BuildToolheadStatusReport(runtime, 8.0)

	if status["homed_axes"] != "xyz" || status["extruder"] != "extruder" {
		t.Fatalf("unexpected status payload %#v", status)
	}
	position, ok := status["position"].([]float64)
	if !ok || len(position) != 4 || !almostEqualFloat64(position[3], 4.0) {
		t.Fatalf("unexpected runtime position %#v", status["position"])
	}
	if status["max_accel_to_decel"] != 2500.0 {
		t.Fatalf("unexpected velocity settings in status %#v", status)
	}
	if status["minimum_cruise_ratio"] != 0.5 {
		t.Fatalf("unexpected minimum_cruise_ratio in status %#v", status)
	}
}

func TestToolheadHomedAxesReadsRuntimeStatus(t *testing.T) {
	runtime := &fakeToolheadHomingRuntime{status: map[string]interface{}{"homed_axes": "xz"}}
	if axes := ToolheadHomedAxes(runtime, 1.0); axes != "xz" {
		t.Fatalf("unexpected homed axes %q", axes)
	}
}

func TestNoteToolheadZNotHomedUsesOptionalInterface(t *testing.T) {
	kinematics := &fakeToolheadZHoming{}
	NoteToolheadZNotHomed(kinematics)
	if !kinematics.noted {
		t.Fatal("expected z-not-homed notification")
	}
	NoteToolheadZNotHomed(struct{}{})
}
