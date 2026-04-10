package heater

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/maths"
	"math"
	"os"
	"sort"
	"strings"
)

const PIDParamBase = 255.
const TunePIDDelta = 5.0

type AutoTuneHeater interface {
	Get_max_power() float64
	Get_pwm_delay() float64
	Set_pwm(read_time float64, value float64)
	Alter_target(target_temp float64)
}

type ControlAutoTune struct {
	heater         AutoTuneHeater
	heaterMaxPower float64
	calibrateTemp  float64
	heating        bool
	peak           float64
	peakTime       float64
	peaks          [][2]float64
	lastPWM        float64
	pwmSamples     [][2]float64
	tempSamples    [][2]float64
}

func NewControlAutoTune(heater AutoTuneHeater, target float64) *ControlAutoTune {
	self := new(ControlAutoTune)
	self.heater = heater
	self.heaterMaxPower = heater.Get_max_power()
	self.calibrateTemp = target
	self.heating = false
	self.peak = 0.
	self.peakTime = 0.
	self.peaks = make([][2]float64, 0)
	self.lastPWM = 0.
	self.pwmSamples = make([][2]float64, 0)
	self.tempSamples = make([][2]float64, 0)
	return self
}

func (self *ControlAutoTune) set_pwm(read_time, value float64) {
	if value != self.lastPWM {
		self.pwmSamples = append(self.pwmSamples, [2]float64{
			read_time + self.heater.Get_pwm_delay(),
			value,
		})
		self.lastPWM = value
	}

	self.heater.Set_pwm(read_time, value)
}

func (self *ControlAutoTune) Temperature_update(read_time, temp, target_temp float64) {
	self.tempSamples = append(self.tempSamples, [2]float64{read_time, temp})
	if self.heating && temp >= target_temp {
		self.heating = false
		self.check_peaks()
		self.heater.Alter_target(self.calibrateTemp - TunePIDDelta)
	} else if !self.heating && temp <= target_temp {
		self.heating = true
		self.check_peaks()
		self.heater.Alter_target(self.calibrateTemp)
	}

	if self.heating {
		self.set_pwm(read_time, self.heaterMaxPower)
		if temp < self.peak {
			self.peak = temp
			self.peakTime = read_time
		}
	} else {
		self.set_pwm(read_time, 0.)
		if temp > self.peak {
			self.peak = temp
			self.peakTime = read_time
		}
	}
}

func (self *ControlAutoTune) Check_busy(eventtime, smoothed_temp, target_temp float64) bool {
	return self.heating || len(self.peaks) < 12
}

func (self *ControlAutoTune) check_peaks() {
	self.peaks = append(self.peaks, [2]float64{self.peak, self.peakTime})
	if self.heating {
		self.peak = 9999999.
	} else {
		self.peak = -9999999.
	}
	if len(self.peaks) < 4 {
		return
	}
	self.calc_pid(len(self.peaks) - 1)
}

func (self *ControlAutoTune) calc_pid(pos int) (float64, float64, float64) {
	tempDiff := self.peaks[pos][0] - self.peaks[pos-1][0]
	timeDiff := self.peaks[pos][1] - self.peaks[pos-2][1]
	amplitude := .5 * math.Abs(tempDiff)
	ku := 4. * self.heaterMaxPower / (math.Pi * amplitude)
	tu := timeDiff
	ti := 0.5 * tu
	td := 0.125 * tu
	kp := 0.6 * ku * PIDParamBase
	ki := kp / ti
	kd := kp * td
	logger.Infof("Autotune: raw=%f/%f Ku=%f Tu=%f  Kp=%f Ki=%f Kd=%f",
		tempDiff, self.heaterMaxPower, ku, tu, kp, ki, kd)
	return kp, ki, kd
}

func (self *ControlAutoTune) Calc_final_pid() (float64, float64, float64) {
	cycleTimes := make([][2]float64, 0)
	for pos := 4; pos < len(self.peaks); pos++ {
		cycleTimes = append(cycleTimes, [2]float64{self.peaks[pos][1] - self.peaks[pos-2][1], float64(pos)})
	}

	sort.SliceStable(cycleTimes, func(i, j int) bool {
		if cycleTimes[i][0] < cycleTimes[j][0] {
			return true
		} else if cycleTimes[i][0] > cycleTimes[j][0] {
			return false
		}
		return cycleTimes[i][1] < cycleTimes[j][1]
	})
	midpointPos := cycleTimes[maths.FloorDiv(len(cycleTimes), 2)][1]
	return self.calc_pid(int(midpointPos))
}

func (self *ControlAutoTune) Write_file(filename string) {
	pwm := make([]string, 0, len(self.pwmSamples))
	for _, samples := range self.pwmSamples {
		pwm = append(pwm, fmt.Sprintf("pwm: %.3f %.3f", samples[0], samples[1]))
	}

	out := make([]string, 0, len(self.tempSamples))
	for _, samples := range self.tempSamples {
		out = append(out, fmt.Sprintf("%.3f %.3f", samples[0], samples[1]))
	}

	all := strings.Join(pwm, "\n") + strings.Join(out, "\n")
	os.WriteFile(filename, []byte(all), 0666)
}
