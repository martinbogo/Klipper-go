package report

import "sort"

const StatusRefreshTime = 0.250

type StatusRefreshDecision struct {
	ShouldUpdate   bool
	NextStatusTime float64
}

func NewPrinterMotionStatus() map[string]interface{} {
	return map[string]interface{}{
		"live_position":          []float64{0., 0., 0., 0.},
		"live_velocity":          0.,
		"live_extruder_velocity": 0.,
		"steppers":               []string{},
		"trapq":                  []string{},
	}
}

func UpdateTrackedObjectStatus(lastStatus map[string]interface{}, steppers map[string]interface{}, trapqs map[string]interface{}) map[string]interface{} {
	status := cloneStatus(lastStatus)
	stepperNames := sortedKeys(steppers)
	trapqNames := sortedKeys(trapqs)
	status["steppers"] = stepperNames
	status["trapq"] = trapqNames
	return status
}

func BuildStatusRefreshDecision(eventtime float64, nextStatusTime float64, trapqCount int, refreshInterval float64) StatusRefreshDecision {
	if trapqCount == 0 || eventtime < nextStatusTime {
		return StatusRefreshDecision{ShouldUpdate: false, NextStatusTime: nextStatusTime}
	}
	return StatusRefreshDecision{ShouldUpdate: true, NextStatusTime: eventtime + refreshInterval}
}

func BuildPrinterMotionStatus(lastStatus map[string]interface{}, xyzpos []float64, xyzvelocity float64, epos []float64, evelocity float64) map[string]interface{} {
	status := cloneStatus(lastStatus)
	xyz := []float64{0., 0., 0.}
	if len(xyzpos) >= 3 {
		xyz = append([]float64(nil), xyzpos[:3]...)
	}
	extruder := []float64{0.}
	if len(epos) >= 1 {
		extruder = []float64{epos[0]}
	}
	status["live_position"] = append(xyz, extruder...)
	status["live_velocity"] = xyzvelocity
	status["live_extruder_velocity"] = evelocity
	return status
}

type ShutdownStepperSnapshot struct {
	Name         string
	ShutdownClock int64
	ShutdownTime float64
	ClockWindow  int64
}

type ShutdownStepperWindow struct {
	StartClock uint64
	EndClock   uint64
}

type ShutdownDumpPlan struct {
	ShutdownTime   float64
	StepperWindows map[string]ShutdownStepperWindow
}

func BuildShutdownDumpPlan(steppers []ShutdownStepperSnapshot, neverTime float64) ShutdownDumpPlan {
	plan := ShutdownDumpPlan{ShutdownTime: neverTime, StepperWindows: map[string]ShutdownStepperWindow{}}
	for _, stepper := range steppers {
		if stepper.ShutdownClock == 0 {
			continue
		}
		if stepper.ShutdownTime < plan.ShutdownTime {
			plan.ShutdownTime = stepper.ShutdownTime
		}
		startClock := stepper.ShutdownClock - stepper.ClockWindow
		if startClock < 0 {
			startClock = 0
		}
		plan.StepperWindows[stepper.Name] = ShutdownStepperWindow{
			StartClock: uint64(startClock),
			EndClock:   uint64(stepper.ShutdownClock + stepper.ClockWindow),
		}
	}
	return plan
}

func sortedKeys(items map[string]interface{}) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneStatus(status map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(status))
	for key, value := range status {
		cloned[key] = value
	}
	return cloned
}