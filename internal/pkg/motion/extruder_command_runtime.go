package motion

import "fmt"

type LegacyExtruderSyncTarget interface {
	Extruder
	LegacyLastPosition() float64
}

type LegacyExtruderTemperatureRuntime interface {
	ActiveExtruder() Extruder
	LookupExtruder(section string) Extruder
	SetTemperature(extruder Extruder, temp float64, wait bool) error
}

type LegacyExtruderTemperatureRuntimeFuncs struct {
	ActiveExtruderFunc func() Extruder
	LookupExtruderFunc func(string) Extruder
	SetTemperatureFunc func(Extruder, float64, bool) error
}

func (self LegacyExtruderTemperatureRuntimeFuncs) ActiveExtruder() Extruder {
	return self.ActiveExtruderFunc()
}

func (self LegacyExtruderTemperatureRuntimeFuncs) LookupExtruder(section string) Extruder {
	return self.LookupExtruderFunc(section)
}

func (self LegacyExtruderTemperatureRuntimeFuncs) SetTemperature(extruder Extruder, temp float64, wait bool) error {
	return self.SetTemperatureFunc(extruder, temp, wait)
}

type PressureAdvanceScanPlan struct {
	PreviousDelay     float64
	NextDelay         float64
	AppliedSmoothTime float64
}

func BuildPressureAdvanceScanPlan(current interface{}, currentSmoothTime float64, next interface{}, requestedSmoothTime float64) PressureAdvanceScanPlan {
	previousSmoothTime := currentSmoothTime
	if current == nil {
		previousSmoothTime = 0.0
	}
	appliedSmoothTime := requestedSmoothTime
	if next == nil {
		appliedSmoothTime = 0.0
	}
	return PressureAdvanceScanPlan{
		PreviousDelay:     previousSmoothTime * 0.5,
		NextDelay:         appliedSmoothTime * 0.5,
		AppliedSmoothTime: appliedSmoothTime,
	}
}

type ExtruderRotationDistanceUpdate struct {
	RotationDistance float64
	NextInvertDir    uint32
}

type LegacyExtruderSyncState struct {
	Position float64
	Trapq    interface{}
}

func ResolveExtruderRotationDistanceUpdate(distance float64, origInvertDir uint32) ExtruderRotationDistanceUpdate {
	update := ExtruderRotationDistanceUpdate{RotationDistance: distance, NextInvertDir: origInvertDir}
	if distance < 0.0 {
		update.RotationDistance = -distance
		if origInvertDir == 0 {
			update.NextInvertDir = 1
		} else {
			update.NextInvertDir = 0
		}
	}
	return update
}

func DisplayExtruderRotationDistance(rotationDistance float64, invertDir uint32, origInvertDir uint32) float64 {
	if invertDir != origInvertDir {
		return -rotationDistance
	}
	return rotationDistance
}

func legacyExtruderSectionName(index int) string {
	return fmt.Sprintf("extruder%d", index)
}

func ResolveLegacyExtruderSyncTarget(raw interface{}, extruderName string) (LegacyExtruderSyncTarget, error) {
	target, ok := raw.(LegacyExtruderSyncTarget)
	if !ok {
		return nil, fmt.Errorf("%s' is not a valid extruder", extruderName)
	}
	return target, nil
}

func ResolveLegacyExtruderSyncState(raw interface{}, extruderName string) (LegacyExtruderSyncState, error) {
	target, err := ResolveLegacyExtruderSyncTarget(raw, extruderName)
	if err != nil {
		return LegacyExtruderSyncState{}, err
	}
	return LegacyExtruderSyncState{
		Position: target.LegacyLastPosition(),
		Trapq:    target.Get_trapq(),
	}, nil
}

func ResolveLegacyExtruderTemperatureTarget(runtime LegacyExtruderTemperatureRuntime, index int, temp float64) (Extruder, bool) {
	if index == 0 {
		return runtime.ActiveExtruder(), true
	}
	extruder := runtime.LookupExtruder(legacyExtruderSectionName(index))
	if extruder == nil {
		if temp <= 0.0 {
			return nil, false
		}
		panic("Extruder not configured")
	}
	return extruder, true
}

func HandleLegacyExtruderTemperatureCommand(runtime LegacyExtruderTemperatureRuntime, temp float64, index int, wait bool) error {
	target, ok := ResolveLegacyExtruderTemperatureTarget(runtime, index, temp)
	if !ok {
		return nil
	}
	return runtime.SetTemperature(target, temp, wait)
}
