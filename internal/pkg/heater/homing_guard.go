package heater

import "fmt"

type HomingGuardHeater interface {
	Get_temp(eventtime float64) (float64, float64)
	Set_temp(degrees float64)
}

type HomingGuard struct {
	disableHeaters []string
	flakySteppers  []string
	targetSave     map[string]float64
}

func NewHomingGuard(disableHeaters []string, flakySteppers []string) *HomingGuard {
	return &HomingGuard{
		disableHeaters: append([]string{}, disableHeaters...),
		flakySteppers:  append([]string{}, flakySteppers...),
		targetSave:     make(map[string]float64),
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (self *HomingGuard) ResolveDisableHeaters(allHeaters []string) error {
	if len(self.disableHeaters) == 0 {
		self.disableHeaters = append([]string{}, allHeaters...)
		return nil
	}
	for _, heaterName := range self.disableHeaters {
		if !contains(allHeaters, heaterName) {
			return fmt.Errorf("one or more of these heaters are unknown: %v", self.disableHeaters)
		}
	}
	return nil
}

func (self *HomingGuard) ValidateFlakySteppers(allSteppers []string) error {
	for _, stepperName := range self.flakySteppers {
		if !contains(allSteppers, stepperName) {
			return fmt.Errorf("one or more of these steppers are unknown: %v", self.flakySteppers)
		}
	}
	return nil
}

func (self *HomingGuard) CheckEligible(steppersBeingHomed []string) bool {
	if len(self.flakySteppers) == 0 {
		return true
	}
	for _, stepperName := range steppersBeingHomed {
		if contains(self.flakySteppers, stepperName) {
			return true
		}
	}
	return false
}

func (self *HomingGuard) Begin(heaterLookup func(string) HomingGuardHeater, steppersBeingHomed []string) bool {
	if !self.CheckEligible(steppersBeingHomed) {
		return false
	}
	for _, heaterName := range self.disableHeaters {
		heater := heaterLookup(heaterName)
		_, self.targetSave[heaterName] = heater.Get_temp(0)
		heater.Set_temp(0.0)
	}
	return true
}

func (self *HomingGuard) End(heaterLookup func(string) HomingGuardHeater, steppersBeingHomed []string) bool {
	if !self.CheckEligible(steppersBeingHomed) {
		return false
	}
	for _, heaterName := range self.disableHeaters {
		heater := heaterLookup(heaterName)
		heater.Set_temp(self.targetSave[heaterName])
	}
	return true
}
