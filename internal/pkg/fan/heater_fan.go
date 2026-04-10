package fan

type PrinterHeaterFan struct {
	heaterTemp float64
	heaters    []Heater
	fan        SpeedController
	fanSpeed   float64
	lastSpeed  float64
}

func NewPrinterHeaterFan(fan SpeedController, heaterTemp float64, fanSpeed float64) *PrinterHeaterFan {
	self := &PrinterHeaterFan{}
	self.heaterTemp = heaterTemp
	self.heaters = make([]Heater, 0)
	self.fan = fan
	self.fanSpeed = fanSpeed
	self.lastSpeed = 0.0
	return self
}

func (self *PrinterHeaterFan) SetHeaters(heaters []Heater) {
	self.heaters = append([]Heater{}, heaters...)
}

func (self *PrinterHeaterFan) Get_status(eventtime float64) map[string]float64 {
	return self.fan.Get_status(eventtime)
}

func (self *PrinterHeaterFan) Callback(eventtime float64) float64 {
	speed := 0.0
	for _, heater := range self.heaters {
		currentTemp, targetTemp := heater.Get_temp(eventtime)
		if targetTemp > 0 || currentTemp > self.heaterTemp {
			speed = self.fanSpeed
		}
	}

	if speed != self.lastSpeed {
		self.lastSpeed = speed
		self.fan.SetSpeed(speed, nil)
	}

	return eventtime + 1.0
}