package homing

import "fmt"

type HomeRecoveryOptions struct {
	IsShutdown func() bool
	MotorOff   func()
}

func RequestedAxes(hasAxis func(string) bool) []int {
	requested := []int{}
	for axis, name := range []string{"X", "Y", "Z"} {
		if hasAxis != nil && hasAxis(name) {
			requested = append(requested, axis)
		}
	}
	if len(requested) == 0 {
		return []int{0, 1, 2}
	}
	return requested
}

func RunHome(home func(), options HomeRecoveryOptions) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, ok := recovered.(error); ok {
				if options.IsShutdown != nil && options.IsShutdown() {
					panic("Homing failed due to printer shutdown")
				}
				if options.MotorOff != nil {
					options.MotorOff()
				}
			}
			panic(recovered)
		}
	}()
	home()
}

func CommandG28(hasAxis func(string) bool, setAxes func([]int), home func(), options HomeRecoveryOptions) {
	axes := RequestedAxes(hasAxis)
	setAxes(axes)
	RunHome(home, options)
}

func ManualHome(execute func([]float64, float64, bool, bool, bool) ([]float64, float64, error), pos []float64, speed float64,
	triggered bool, checkTriggered bool) error {
	_, _, err := execute(pos, speed, false, triggered, checkTriggered)
	return err
}

func ProbingMove(execute func([]float64, float64, bool, bool, bool) ([]float64, float64, error), checkNoMovement func() string,
	pos []float64, speed float64) ([]float64, error) {
	triggerPos, _, err := execute(pos, speed, true, true, true)
	if err != nil {
		return nil, err
	}
	if checkNoMovement != nil && checkNoMovement() != "" {
		return nil, fmt.Errorf("Probe triggered prior to movement")
	}
	return triggerPos, nil
}
