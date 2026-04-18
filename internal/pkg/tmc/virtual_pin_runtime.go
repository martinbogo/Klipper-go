package tmc

type VirtualPinRuntime struct {
	core   *VirtualPinHelperCore
	mcuTMC RegisterAccess
}

func NewVirtualPinRuntime(config VirtualPinConfig, mcuTMC RegisterAccess) *VirtualPinRuntime {
	return &VirtualPinRuntime{
		core:   NewVirtualPinHelperCore(config, mcuTMC),
		mcuTMC: mcuTMC,
	}
}

func (self *VirtualPinRuntime) ChipName(config VirtualPinConfig) string {
	return self.core.ChipName(config)
}

func (self *VirtualPinRuntime) DiagPin() interface{} {
	return self.core.DiagPin()
}

func (self *VirtualPinRuntime) BeginMoveHoming() error {
	return self.core.BeginMoveHoming(self.mcuTMC)
}

func (self *VirtualPinRuntime) EndMoveHoming() error {
	return self.core.EndHoming(self.mcuTMC, nil)
}

func (self *VirtualPinRuntime) BeginHoming() error {
	return self.core.BeginHoming(self.mcuTMC, 0xfffff)
}

func (self *VirtualPinRuntime) EndHoming() error {
	return self.core.EndHoming(self.mcuTMC, nil)
}

type VirtualPinEventRuntime interface {
	MatchesHomingMoveEndstop(endstop interface{}) bool
	BeginMoveHoming() error
	EndMoveHoming() error
}

func HandleVirtualPinHomingMoveBegin(runtime VirtualPinEventRuntime, endstops []interface{}) error {
	if hasMatchingVirtualPinEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		return runtime.BeginMoveHoming()
	}
	return nil
}

func HandleVirtualPinHomingMoveEnd(runtime VirtualPinEventRuntime, endstops []interface{}) error {
	if hasMatchingVirtualPinEndstop(endstops, runtime.MatchesHomingMoveEndstop) {
		return runtime.EndMoveHoming()
	}
	return nil
}

type virtualPinEndstopMatcher func(interface{}) bool

func hasMatchingVirtualPinEndstop(endstops []interface{}, matches virtualPinEndstopMatcher) bool {
	for _, endstop := range endstops {
		if matches(endstop) {
			return true
		}
	}
	return false
}
