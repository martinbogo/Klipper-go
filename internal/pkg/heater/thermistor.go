package heater

import (
	"log"
	"math"
)

const KELVIN_TO_CELSIUS_THERMISTOR = -273.15

type Thermistor struct {
	Pullup          float64
	Inline_resistor float64
	C1              float64
	C2              float64
	C3              float64
}

func NewThermistor(pullup float64, inline_resistor float64) *Thermistor {
	self := &Thermistor{}
	self.Pullup = pullup
	self.Inline_resistor = inline_resistor
	self.C1 = 0.0
	self.C2 = 0.0
	self.C3 = 0.0
	return self
}

func (self *Thermistor) Setup_coefficients(t1 float64, r1 float64, t2 float64, r2 float64, t3 float64, r3 float64, name string) {
	invT1 := 1.0 / (t1 - KELVIN_TO_CELSIUS_THERMISTOR)
	invT2 := 1.0 / (t2 - KELVIN_TO_CELSIUS_THERMISTOR)
	invT3 := 1.0 / (t3 - KELVIN_TO_CELSIUS_THERMISTOR)
	lnR1 := math.Log(r1)
	lnR2 := math.Log(r2)
	lnR3 := math.Log(r3)
	ln3R1, ln3R2, ln3R3 := math.Pow(lnR1, 3), math.Pow(lnR2, 3), math.Pow(lnR3, 3)
	invT12, invT13 := invT1-invT2, invT1-invT3
	lnR12, lnR13 := lnR1-lnR2, lnR1-lnR3
	ln3R12, ln3R13 := ln3R1-ln3R2, ln3R1-ln3R3
	self.C3 = (invT12 - invT13*lnR12/lnR13) / (ln3R12 - ln3R13*lnR12/lnR13)

	if self.C3 <= 0 {
		beta := lnR13 / invT13
		log.Printf("Using thermistor beta %.3f in heater %s\n", beta, name)
		self.Setup_coefficients_beta(t1, r1, beta)
		return
	}
	self.C2 = (invT12 - self.C3*ln3R12) / lnR12
	self.C1 = invT1 - self.C2*lnR1 - self.C3*ln3R1
}

func (self *Thermistor) Setup_coefficients_beta(t1 float64, r1 float64, beta float64) {
	invT1 := 1.0 / (t1 - KELVIN_TO_CELSIUS_THERMISTOR)
	lnR1 := math.Log(r1)
	self.C3 = 0.0
	self.C2 = 1.0 / beta
	self.C1 = invT1 - self.C2*lnR1
}

func (self *Thermistor) Calc_temp(adc float64) float64 {
	adc = math.Max(.00001, math.Min(.99999, adc))
	r := self.Pullup * adc / (1.0 - adc)
	lnR := math.Log(r - self.Inline_resistor)
	invT := self.C1 + self.C2*lnR + self.C3*math.Pow(lnR, 3)
	return 1.0/invT + KELVIN_TO_CELSIUS_THERMISTOR
}

func (self *Thermistor) Calc_adc(temp float64) float64 {
	if temp <= KELVIN_TO_CELSIUS_THERMISTOR {
		return 1.0
	}
	invT := 1.0 / (temp - KELVIN_TO_CELSIUS_THERMISTOR)
	lnR := 0.0
	if self.C3 != 0.0 {
		y := (self.C1 - invT) / (2.0 * self.C3)
		x := math.Sqrt(math.Pow(self.C2/(3.0*self.C3), 3) + math.Pow(y, 2))
		lnR = math.Pow(x-y, 1.0/3.0) - math.Pow(x+y, 1.0/3.0)
	} else {
		lnR = (invT - self.C1) / self.C2
	}
	r := math.Exp(lnR) + self.Inline_resistor
	return r / (self.Pullup + r)
}