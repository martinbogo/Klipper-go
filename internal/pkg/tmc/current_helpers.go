package tmc

import (
	"goklipper/common/utils/cast"
	"goklipper/common/utils/maths"
	"goklipper/common/value"
	"math"
)

const MaxCurrent = 2.000

type CurrentHelperConfig interface {
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
}

type TMC2660CurrentHelperConfig interface {
	CurrentHelperConfig
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
}

type EventRegistrar func(event string, callback func([]interface{}) error)

type ReactorCallbackRegistrar func(callback func(interface{}) interface{}, eventtime float64)

type TMCCurrentHelper struct {
	mcuTMC         RegisterAccess
	fields         *FieldHelper
	reqHoldCurrent float64
	senseResistor  float64
}

var _ CurrentControl = (*TMCCurrentHelper)(nil)

func NewTMCCurrentHelper(config CurrentHelperConfig, mcuTMC RegisterAccess) *TMCCurrentHelper {
	self := &TMCCurrentHelper{}
	self.mcuTMC = mcuTMC
	self.fields = mcuTMC.Get_fields()
	runCurrent := config.Getfloat("run_current", 0, 0., MaxCurrent, 0, 0, true)
	holdCurrent := config.Getfloat("hold_current", MaxCurrent, 0., MaxCurrent, 0, 0, true)
	self.reqHoldCurrent = holdCurrent
	self.senseResistor = config.Getfloat("sense_resistor", 0.110, 0., 0, 0, 0, true)
	vsense, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	self.fields.Set_field("vsense", vsense, value.None, nil)
	self.fields.Set_field("ihold", ihold, value.None, nil)
	self.fields.Set_field("irun", irun, value.None, nil)
	return self
}

func (self *TMCCurrentHelper) calcCurrentBits(current float64, vsense bool) int {
	senseResistor := self.senseResistor + 0.020
	vref := 0.32
	if vsense {
		vref = 0.18
	}
	cs := int(32.*senseResistor*current*math.Sqrt(2.)/vref+.5) - 1
	return maths.Max(0, maths.Min(31, cs))
}

func (self *TMCCurrentHelper) calcCurrentFromBits(cs float64, vsense bool) float64 {
	senseResistor := self.senseResistor + 0.020
	vref := 0.32
	if vsense {
		vref = 0.18
	}
	return (cs + 1) * vref / (32. * senseResistor * math.Sqrt(2.))
}

func (self *TMCCurrentHelper) calcCurrent(runCurrent, holdCurrent float64) (bool, int, int) {
	vsense := true
	irun := self.calcCurrentBits(runCurrent, true)
	if irun == 31 {
		cur := self.calcCurrentFromBits(float64(irun), true)
		if cur < runCurrent {
			irun2 := self.calcCurrentBits(runCurrent, false)
			cur2 := self.calcCurrentFromBits(float64(irun2), false)
			if math.Abs(runCurrent-cur2) < math.Abs(runCurrent-cur) {
				vsense = false
				irun = irun2
			}
		}
	}
	ihold := self.calcCurrentBits(math.Min(holdCurrent, runCurrent), vsense)
	return vsense, irun, ihold
}

func (self *TMCCurrentHelper) Get_current() []float64 {
	irun := self.fields.Get_field("irun", value.None, nil)
	ihold := self.fields.Get_field("ihold", value.None, nil)
	vsense := self.fields.Get_field("vsense", value.None, nil)
	runCurrent := self.calcCurrentFromBits(float64(irun), cast.ToBool(vsense))
	holdCurrent := self.calcCurrentFromBits(float64(ihold), cast.ToBool(vsense))
	return []float64{runCurrent, holdCurrent, self.reqHoldCurrent, MaxCurrent}
}

func (self *TMCCurrentHelper) Set_current(runCurrent, holdCurrent, printTime float64) {
	self.reqHoldCurrent = holdCurrent
	vsense, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	if vsense != cast.ToBool(self.fields.Get_field("vsense", value.None, nil)) {
		val := self.fields.Set_field("vsense", vsense, value.None, nil)
		self.mcuTMC.Set_register("CHOPCONF", val, cast.Float64P(printTime))
	}
	self.fields.Set_field("ihold", ihold, value.None, nil)
	val := self.fields.Set_field("irun", irun, value.None, nil)
	self.mcuTMC.Set_register("IHOLD_IRUN", val, cast.Float64P(printTime))
}

type TMC2240CurrentHelper struct {
	mcuTMC         RegisterAccess
	fields         *FieldHelper
	rref           float64
	reqHoldCurrent float64
}

var _ CurrentControl = (*TMC2240CurrentHelper)(nil)

func NewTMC2240CurrentHelper(config CurrentHelperConfig, mcuTMC RegisterAccess) *TMC2240CurrentHelper {
	self := &TMC2240CurrentHelper{}
	self.mcuTMC = mcuTMC
	self.fields = mcuTMC.Get_fields()
	self.rref = config.Getfloat("rref", 12000., 12000., 60000., 0, 0, true)
	maxCur := self.getIFSRMS(3)
	runCurrent := config.Getfloat("run_current", 0., 0., maxCur, 0., 0., true)
	holdCurrent := config.Getfloat("hold_current", maxCur, 0, maxCur, 0., 0., true)
	self.reqHoldCurrent = holdCurrent
	currentRange := self.calcCurrentRange(runCurrent)
	self.fields.Set_field("current_range", currentRange, nil, nil)
	gscaler, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	self.fields.Set_field("globalscaler", gscaler, nil, nil)
	self.fields.Set_field("ihold", ihold, nil, nil)
	self.fields.Set_field("irun", irun, nil, nil)
	return self
}

func (self *TMC2240CurrentHelper) getIFSRMS(currentRange interface{}) float64 {
	if currentRange == nil {
		currentRange = int(self.fields.Get_field("current_range", nil, nil))
	}
	kifs := []float64{11750., 24000., 36000., 36000.}
	return kifs[currentRange.(int)] / self.rref / math.Sqrt(2.)
}

func (self *TMC2240CurrentHelper) calcCurrentRange(current float64) int {
	currentRange := 0
	for currentRange = 0; currentRange < 4; currentRange++ {
		if current <= self.getIFSRMS(currentRange) {
			break
		}
	}
	return currentRange
}

func (self *TMC2240CurrentHelper) calcGlobalscaler(current float64) int {
	ifsRMS := self.getIFSRMS(nil)
	globalscaler := int((current*256.0)/ifsRMS + 0.5)
	globalscaler = maths.Max(32, globalscaler)
	if globalscaler >= 256 {
		globalscaler = 0
	}
	return globalscaler
}

func (self *TMC2240CurrentHelper) calcCurrentBits(current float64, globalscaler int) int {
	ifsRMS := self.getIFSRMS(nil)
	if globalscaler == 0 {
		globalscaler = 256
	}
	cs := int((current*256.0*32.0)/(float64(globalscaler)*ifsRMS) - 1 + 0.5)
	return maths.Max(0, maths.Min(31, cs))
}

func (self *TMC2240CurrentHelper) calcCurrent(runCurrent, holdCurrent float64) (int, int, int) {
	gscaler := self.calcGlobalscaler(runCurrent)
	irun := self.calcCurrentBits(runCurrent, gscaler)
	ihold := self.calcCurrentBits(math.Min(holdCurrent, runCurrent), gscaler)
	return gscaler, irun, ihold
}

func (self *TMC2240CurrentHelper) calcCurrentFromField(fieldName string) float64 {
	ifsRMS := self.getIFSRMS(nil)
	globalscaler := self.fields.Get_field("globalscaler", nil, nil)
	if globalscaler == 0 {
		globalscaler = 256
	}
	bits := self.fields.Get_field(fieldName, nil, nil)
	return float64(globalscaler*(bits+1)) * ifsRMS / (256.0 * 32.0)
}

func (self *TMC2240CurrentHelper) Get_current() []float64 {
	ifsRMS := self.getIFSRMS(nil)
	runCurrent := self.calcCurrentFromField("irun")
	holdCurrent := self.calcCurrentFromField("ihold")
	return []float64{runCurrent, holdCurrent, self.reqHoldCurrent, ifsRMS}
}

func (self *TMC2240CurrentHelper) Set_current(runCurrent, holdCurrent, printTime float64) {
	self.reqHoldCurrent = holdCurrent
	gscaler, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	self.fields.Set_field("globalscaler", gscaler, nil, nil)
	self.mcuTMC.Set_register("GLOBALSCALER", int64(gscaler), &printTime)
	self.fields.Set_field("ihold", ihold, nil, nil)
	self.mcuTMC.Set_register("IHOLD_IRUN", int64(irun), &printTime)
}

const (
	TMC2660MaxCurrent = 2.400
	TMC5160VREF       = 0.325
	TMC5160MaxCurrent = 3.000
)

type TMC2660CurrentHelper struct {
	mcuTMC                RegisterAccess
	fields                *FieldHelper
	current               float64
	senseResistor         float64
	idleCurrentPercentage int
	scheduleCallback      ReactorCallbackRegistrar
}

var _ CurrentControl = (*TMC2660CurrentHelper)(nil)

func NewTMC2660CurrentHelper(config TMC2660CurrentHelperConfig, mcuTMC RegisterAccess, registerEvent EventRegistrar, scheduleCallback ReactorCallbackRegistrar) *TMC2660CurrentHelper {
	self := &TMC2660CurrentHelper{}
	self.mcuTMC = mcuTMC
	self.fields = mcuTMC.Get_fields()
	self.current = config.Getfloat("run_current", 0, 0.1, TMC2660MaxCurrent, 0, 0, true)
	self.senseResistor = config.Getfloat("sense_resistor", 0, 0, 0, 0, 0, true)
	self.scheduleCallback = scheduleCallback
	vsense, cs := self.calcCurrent(self.current)
	self.fields.Set_field("cs", cs, nil, nil)
	self.fields.Set_field("vsense", vsense, nil, nil)

	self.idleCurrentPercentage = config.Getint("idle_current_percent", 100, 0, 100, true)
	if self.idleCurrentPercentage < 100 && registerEvent != nil {
		registerEvent("idle_timeout:printing", self.handlePrinting)
		registerEvent("idle_timeout:ready", self.handleReady)
	}
	return self
}

func (self *TMC2660CurrentHelper) calcCurrentBits(current float64, vsense bool) int {
	vref := 0.165
	if !vsense {
		vref = 0.310
	}
	cs := int(32.*self.senseResistor*current*math.Sqrt(2.)/vref+.5) - 1
	return maths.Max(0, maths.Min(31, cs))
}

func (self *TMC2660CurrentHelper) calcCurrentFromBits(cs float64, vsense bool) float64 {
	vref := 0.165
	if !vsense {
		vref = 0.310
	}
	return (cs + 1) * vref / (32. * self.senseResistor * math.Sqrt(2.))
}

func (self *TMC2660CurrentHelper) calcCurrent(runCurrent float64) (bool, float64) {
	vsense := true
	irun := self.calcCurrentBits(runCurrent, true)
	if irun == 31 {
		cur := self.calcCurrentFromBits(float64(irun), true)
		if cur < runCurrent {
			irun2 := self.calcCurrentBits(runCurrent, false)
			cur2 := self.calcCurrentFromBits(float64(irun2), false)
			if math.Abs(runCurrent-cur2) < math.Abs(runCurrent-cur) {
				vsense = false
				irun = irun2
			}
		}
	}
	return vsense, float64(irun)
}

func (self *TMC2660CurrentHelper) handlePrinting(argv []interface{}) error {
	if self.scheduleCallback == nil {
		return nil
	}
	printTime := cast.ToFloat64(argv[0]) - 0.100
	self.scheduleCallback(func(interface{}) interface{} {
		return self.updateCurrent(self.current, printTime)
	}, 0)
	return nil
}

func (self *TMC2660CurrentHelper) handleReady(argv []interface{}) error {
	if self.scheduleCallback == nil {
		return nil
	}
	printTime := cast.ToFloat64(argv[0])
	current := self.current * float64(self.idleCurrentPercentage) / 100.
	self.scheduleCallback(func(interface{}) interface{} {
		return self.updateCurrent(current, printTime)
	}, 0)
	return nil
}

func (self *TMC2660CurrentHelper) updateCurrent(current, printTime float64) interface{} {
	vsense, cs := self.calcCurrent(current)
	val := self.fields.Set_field("cs", cs, nil, nil)
	self.mcuTMC.Set_register("SGCSCONF", val, cast.Float64P(printTime))
	if vsense != cast.ToBool(self.fields.Get_field("vsense", 0, nil)) {
		val = self.fields.Set_field("vsense", vsense, nil, nil)
		self.mcuTMC.Set_register("DRVCONF", val, cast.Float64P(printTime))
	}
	return nil
}

func (self *TMC2660CurrentHelper) Get_current() []float64 {
	return []float64{self.current, 0, 0, TMC2660MaxCurrent}
}

func (self *TMC2660CurrentHelper) Set_current(runCurrent, holdCurrent, printTime float64) {
	self.current = runCurrent
	self.updateCurrent(runCurrent, printTime)
}

type TMC5160CurrentHelper struct {
	mcuTMC         RegisterAccess
	fields         *FieldHelper
	reqHoldCurrent float64
	senseResistor  float64
}

var _ CurrentControl = (*TMC5160CurrentHelper)(nil)

func NewTMC5160CurrentHelper(config CurrentHelperConfig, mcuTMC RegisterAccess) *TMC5160CurrentHelper {
	self := &TMC5160CurrentHelper{}
	self.mcuTMC = mcuTMC
	self.fields = mcuTMC.Get_fields()
	runCurrent := config.Getfloat("run_current", 0, 0, MaxCurrent, 0., 0, true)
	holdCurrent := config.Getfloat("hold_current", MaxCurrent, 0, MaxCurrent, 0., 0, true)
	self.reqHoldCurrent = holdCurrent
	self.senseResistor = config.Getfloat("sense_resistor", 0.075, 0, 0, 0., 0, true)
	gscaler, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	self.fields.Set_field("globalscaler", gscaler, nil, nil)
	self.fields.Set_field("ihold", ihold, nil, nil)
	self.fields.Set_field("irun", irun, nil, nil)
	return self
}

func (self *TMC5160CurrentHelper) calcGlobalscaler(current float64) int {
	globalscaler := int((current*256.*math.Sqrt(2.)*self.senseResistor/TMC5160VREF) + .5)
	globalscaler = maths.Max(32, globalscaler)
	if globalscaler >= 256 {
		globalscaler = 0
	}
	return globalscaler
}

func (self *TMC5160CurrentHelper) calcCurrentBits(current float64, globalscaler int) int {
	if globalscaler == 0 {
		globalscaler = 256
	}
	cs := int((current*256.*32.*math.Sqrt(2.)*self.senseResistor)/
		(float64(globalscaler)*TMC5160VREF) - 1. + .5)
	return maths.Max(0, maths.Min(31, cs))
}

func (self *TMC5160CurrentHelper) calcCurrent(runCurrent, holdCurrent float64) (int, int, int) {
	gscaler := self.calcGlobalscaler(runCurrent)
	irun := self.calcCurrentBits(runCurrent, gscaler)
	ihold := self.calcCurrentBits(math.Min(holdCurrent, runCurrent), gscaler)
	return gscaler, irun, ihold
}

func (self *TMC5160CurrentHelper) calcCurrentFromField(fieldName string) float64 {
	globalscaler := float64(self.fields.Get_field("globalscaler", 0, nil))
	if value.False(globalscaler) {
		globalscaler = 256
	}
	bits := float64(self.fields.Get_field(fieldName, 0, nil))
	return globalscaler * (bits + 1) * TMC5160VREF /
		(256. * 32. * math.Sqrt(2.) * self.senseResistor)
}

func (self *TMC5160CurrentHelper) Get_current() []float64 {
	runCurrent := self.calcCurrentFromField("irun")
	holdCurrent := self.calcCurrentFromField("ihold")
	return []float64{runCurrent, holdCurrent, self.reqHoldCurrent, MaxCurrent}
}

func (self *TMC5160CurrentHelper) Set_current(runCurrent, holdCurrent, printTime float64) {
	self.reqHoldCurrent = holdCurrent
	gscaler, irun, ihold := self.calcCurrent(runCurrent, holdCurrent)
	val := self.fields.Set_field("globalscaler", gscaler, nil, nil)
	self.mcuTMC.Set_register("GLOBALSCALER", val, cast.Float64P(printTime))
	self.fields.Set_field("ihold", ihold, nil, nil)
	val = self.fields.Set_field("irun", irun, nil, nil)
	self.mcuTMC.Set_register("IHOLD_IRUN", val, cast.Float64P(printTime))
}