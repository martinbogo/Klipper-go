package mcu

import "fmt"

type StepperConfigSetupPlan struct {
	ConfigCmds          []string
	StepQueueLookup     string
	DirLookup           string
	ResetLookup         string
	PositionQueryFormat string
	PositionReplyFormat string
	MaxErrorTicks       uint32
}

func BuildStepperConfigSetupPlan(oid int, stepPin interface{}, dirPin interface{}, invertStep int, stepPulseTicks int64, maxError float64, secondsToClock func(float64) int64) StepperConfigSetupPlan {
	return StepperConfigSetupPlan{
		ConfigCmds: []string{
			fmt.Sprintf("config_stepper oid=%d step_pin=%s dir_pin=%s invert_step=%d step_pulse_ticks=%d", oid, stepPin, dirPin, invertStep, stepPulseTicks),
			fmt.Sprintf("reset_step_clock oid=%d clock=0", oid),
		},
		StepQueueLookup:     "queue_step oid=%c interval=%u count=%hu add=%hi",
		DirLookup:           "set_next_step_dir oid=%c dir=%c",
		ResetLookup:         "reset_step_clock oid=%c clock=%u",
		PositionQueryFormat: "stepper_get_position oid=%c",
		PositionReplyFormat: "stepper_position oid=%c pos=%i",
		MaxErrorTicks:       uint32(secondsToClock(maxError)),
	}
}
