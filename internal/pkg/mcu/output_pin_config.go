package mcu

type DigitalOutConfigPlan struct {
	MaxDurationTicks int64
}

func BuildDigitalOutConfigPlan(maxDuration float64, startValue int, shutdownValue int, secondsToClock func(float64) int64) DigitalOutConfigPlan {
	if maxDuration != 0 && startValue != shutdownValue {
		panic("Pin with max duration must have start value equal to shutdown value")
	}
	mdurTicks := secondsToClock(maxDuration)
	if mdurTicks >= (1 << 31) {
		panic("Digital pin max duration too large")
	}
	return DigitalOutConfigPlan{MaxDurationTicks: mdurTicks}
}

type PWMConfigPlan struct {
	LastClock           int64
	CycleTicks          int64
	MaxDurationTicks    int64
	PWMMax              float64
	StartConfigValue    int
	ShutdownConfigValue int
	InitialQueueValue   int
}

func BuildPWMConfigPlan(maxDuration float64, cycleTime float64, startValue float64, shutdownValue float64, hardwarePWM bool, mcuPWMMax float64, monotonic func() float64, estimatedPrintTime func(float64) float64, printTimeToClock func(float64) int64, secondsToClock func(float64) int64) PWMConfigPlan {
	if maxDuration != 0 && startValue != shutdownValue {
		panic("Pin with max duration must have start value equal to shutdown value")
	}
	lastClock := printTimeToClock(estimatedPrintTime(monotonic()) + 0.2)
	cycleTicks := secondsToClock(cycleTime)
	mdurTicks := secondsToClock(maxDuration)
	if mdurTicks >= (1 << 31) {
		panic("PWM pin max duration too large")
	}
	if hardwarePWM {
		return PWMConfigPlan{
			LastClock:           lastClock,
			CycleTicks:          cycleTicks,
			MaxDurationTicks:    mdurTicks,
			PWMMax:              mcuPWMMax,
			StartConfigValue:    int(startValue * mcuPWMMax),
			ShutdownConfigValue: int(shutdownValue * mcuPWMMax),
			InitialQueueValue:   int(startValue*mcuPWMMax + 0.5),
		}
	}
	if shutdownValue != 0.0 && shutdownValue != 1.0 {
		panic("shutdown value must be 0.0 or 1.0 on soft pwm")
	}
	if cycleTicks >= (1 << 31) {
		panic("PWM pin cycle time too large")
	}
	startConfigValue := 0
	if startValue >= 1.0 {
		startConfigValue = 1
	}
	shutdownConfigValue := 0
	if shutdownValue >= 0.5 {
		shutdownConfigValue = 1
	}
	pwmMax := float64(cycleTicks)
	return PWMConfigPlan{
		LastClock:           lastClock,
		CycleTicks:          cycleTicks,
		MaxDurationTicks:    mdurTicks,
		PWMMax:              pwmMax,
		StartConfigValue:    startConfigValue,
		ShutdownConfigValue: shutdownConfigValue,
		InitialQueueValue:   int(startValue*float64(cycleTicks) + 0.5),
	}
}
