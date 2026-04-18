package motion

type LegacyRailHomingInfo struct {
	Speed             float64
	PositionEndstop   float64
	RetractSpeed      float64
	RetractDist       float64
	PositiveDir       bool
	SecondHomingSpeed float64
}

type LegacyRailState struct {
	rangeMin   float64
	rangeMax   float64
	homingInfo LegacyRailHomingInfo
}

func NewLegacyRailState(rangeMin float64, rangeMax float64, homingInfo LegacyRailHomingInfo) *LegacyRailState {
	return &LegacyRailState{
		rangeMin:   rangeMin,
		rangeMax:   rangeMax,
		homingInfo: homingInfo,
	}
}

func (self *LegacyRailState) Range() (float64, float64) {
	if self == nil {
		return 0., 0.
	}
	return self.rangeMin, self.rangeMax
}

func (self *LegacyRailState) Get_range() (float64, float64) {
	return self.Range()
}

func (self *LegacyRailState) HomingInfo() LegacyRailHomingInfo {
	if self == nil {
		return LegacyRailHomingInfo{}
	}
	return self.homingInfo
}

func (self *LegacyRailState) Get_homing_info() *LegacyRailHomingInfo {
	info := self.HomingInfo()
	return &info
}

func (self *LegacyRailState) SetHomingInfo(info LegacyRailHomingInfo) {
	if self == nil {
		return
	}
	self.homingInfo = info
}

func (self *LegacyRailState) Set_homing_info(info *LegacyRailHomingInfo) {
	if info == nil {
		return
	}
	self.SetHomingInfo(*info)
}
