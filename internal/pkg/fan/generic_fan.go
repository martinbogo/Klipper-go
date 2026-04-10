package fan

type GenericFan struct {
	fan     CommandSpeedController
	fanName string
}

func NewGenericFan(fanName string, fan CommandSpeedController) *GenericFan {
	self := &GenericFan{}
	self.fan = fan
	self.fanName = fanName
	return self
}

func (self *GenericFan) FanName() string {
	return self.fanName
}

func (self *GenericFan) Get_status(eventtime float64) map[string]float64 {
	return self.fan.Get_status(eventtime)
}

func (self *GenericFan) SetSpeedFromCommand(speed float64) {
	self.fan.SetSpeedFromCommand(speed)
}