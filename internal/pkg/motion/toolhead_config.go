package motion

// Default toolhead timing constants.
const (
	DefaultBufferTimeLow         = 1.0
	DefaultBufferTimeHigh        = 2.0
	DefaultBufferTimeStart       = 0.250
	DefaultBgFlushLowTime        = 0.200
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
		BufferTimeLow:    DefaultBufferTimeLow,
		BgFlushLowTime:   DefaultBgFlushLowTime,
		BgFlushBatchTime: DefaultBgFlushBatchTime,
		BgFlushExtraTime: DefaultBgFlushExtraTime,
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
