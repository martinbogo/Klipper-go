package motion

import "goklipper/common/utils/object"

// Default toolhead timing constants.
const (
	DefaultBufferTimeLow         = 1.0
	DefaultBufferTimeHigh        = 1.0
	DefaultBufferTimeStart       = 0.250
	DefaultBgFlushLowTime        = 0.200
	DefaultBgFlushHighTime       = 0.400
	DefaultBgFlushSgLowTime      = 0.450
	DefaultBgFlushSgHighTime     = 0.700
	DefaultBgFlushBatchTime      = 0.200
	DefaultBgFlushExtraTime      = 0.250
	DefaultMinKinTime            = 0.100
	DefaultMoveBatchTime         = 0.500
	DefaultStepcompressFlushTime = 0.050
	DefaultSdsCheckTime          = 0.001 // step+dir+step filter
	DefaultMoveHistoryExpire     = 30.
	DefaultDripSegmentTime       = 0.050
	DefaultDripTime              = 0.100
)

var defaultToolheadSupportModules = []string{
	"gcode_move",
	"homing",
	"statistics",
	"idle_timeout",
	"manual_probe",
	"tuning_tower",
}

type toolheadVelocityConfig interface {
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
}

func ReadToolheadVelocitySettings(config toolheadVelocityConfig) ToolheadVelocitySettings {
	maxVelocity := config.Getfloat("max_velocity", object.Sentinel{}, 0.0, 0.0, 0.0, 0.0, true)
	maxAccel := config.Getfloat("max_accel", object.Sentinel{}, 0.0, 0.0, 0.0, 0.0, true)
	minimumCruiseRatio := config.Getfloat("minimum_cruise_ratio", 0.5, 0.0, 0.0, 0.0, 1.0, true)
	requestedAccelToDecel := config.Getfloat("max_accel_to_decel", maxAccel*(1.0-minimumCruiseRatio), 0.0, 0.0, 0.0, 0.0, true)
	squareCornerVelocity := config.Getfloat("square_corner_velocity", 5.0, 0.0, 0.0, 0.0, 0.0, true)
	return ToolheadVelocitySettings{
		MaxVelocity:           maxVelocity,
		MaxAccel:              maxAccel,
		RequestedAccelToDecel: requestedAccelToDecel,
		SquareCornerVelocity:  squareCornerVelocity,
	}
}

func DefaultToolheadSupportModules() []string {
	return append([]string{}, defaultToolheadSupportModules...)
}

func LoadToolheadSupportModules(load func(name string)) {
	for _, name := range defaultToolheadSupportModules {
		load(name)
	}
}

func DefaultTimingConfig() ToolheadTimingConfig {
	return ToolheadTimingConfig{
		BufferTimeStart:       DefaultBufferTimeStart,
		MinKinTime:            DefaultMinKinTime,
		MoveBatchTime:         DefaultMoveBatchTime,
		MoveHistoryExpire:     DefaultMoveHistoryExpire,
		ScanTimeOffset:        DefaultSdsCheckTime,
		StepcompressFlushTime: DefaultStepcompressFlushTime,
	}
}

func DefaultPauseConfig() ToolheadPauseConfig {
	return ToolheadPauseConfig{
		BufferTimeLow:    DefaultBufferTimeLow,
		BufferTimeHigh:   DefaultBufferTimeHigh,
		PauseCheckOffset: 0.100,
		MaxPauseDuration: 1.0,
		MinPrimingDelay:  0.100,
		WaitMoveDelay:    0.100,
	}
}

func DefaultFlushConfig() ToolheadFlushConfig {
	return ToolheadFlushConfig{
		BufferTimeLow:         DefaultBufferTimeLow,
		BgFlushLowTime:        DefaultBgFlushLowTime,
		BgFlushHighTime:       DefaultBgFlushHighTime,
		BgFlushSgLowTime:      DefaultBgFlushSgLowTime,
		BgFlushSgHighTime:     DefaultBgFlushSgHighTime,
		BgFlushBatchTime:      DefaultBgFlushBatchTime,
		BgFlushExtraTime:      DefaultBgFlushExtraTime,
		StepcompressFlushTime: DefaultStepcompressFlushTime,
	}
}

func DefaultDripConfig() ToolheadDripConfig {
	return ToolheadDripConfig{
		DripTime:              DefaultDripTime,
		StepcompressFlushTime: DefaultStepcompressFlushTime,
		DripSegmentTime:       DefaultDripSegmentTime,
	}
}

func DefaultDripMoveConfig() ToolheadDripMoveConfig {
	return ToolheadDripMoveConfig{
		LookaheadFlushTime: DefaultBufferTimeHigh,
	}
}

func BuildToolheadInitialVelocityResult(settings ToolheadVelocitySettings) ToolheadVelocityLimitResult {
	return ApplyToolheadVelocityLimitUpdate(settings, ToolheadVelocityLimitUpdate{})
}
