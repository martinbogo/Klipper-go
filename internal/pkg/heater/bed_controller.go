package heater

type BedController struct {
	Get_status     func(eventtime float64) map[string]float64
	Stats          func(eventtime float64) (bool, string)
	setTemperature func(temp float64, wait bool) error
}

func NewBedController(
	getStatus func(eventtime float64) map[string]float64,
	stats func(eventtime float64) (bool, string),
	setTemperature func(temp float64, wait bool) error) *BedController {
	self := &BedController{}
	self.Get_status = getStatus
	self.Stats = stats
	self.setTemperature = setTemperature
	return self
}

func (self *BedController) SetTemperature(temp float64, wait bool) error {
	return self.setTemperature(temp, wait)
}