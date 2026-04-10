package heater

import (
	"fmt"

	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
)

type pidCalibrateToolhead interface {
	Get_last_move_time() float64
}

type pidCalibrateHeater interface {
	AutoTuneHeater
	Set_control(control interface{}) interface{}
}

type pidCalibrateHeaterManager interface {
	LookupHeater(name string) interface{}
	Set_temperature(heater interface{}, temp float64, wait bool) error
}

type pidCalibrateConfigStore interface {
	Set(section string, option string, val string)
}

type PIDCalibrateModule struct {
	printer printerpkg.ModulePrinter
}

const cmdPIDCalibrateHelp = "Run PID calibration test"

func LoadConfigPIDCalibrate(config printerpkg.ModuleConfig) interface{} {
	self := &PIDCalibrateModule{printer: config.Printer()}
	self.printer.GCode().RegisterCommand("PID_CALIBRATE", self.cmdPIDCalibrate, false, cmdPIDCalibrateHelp)
	return self
}

func (self *PIDCalibrateModule) cmdPIDCalibrate(gcmd printerpkg.Command) error {
	heaterName := gcmd.String("HEATER", "")
	if heaterName == "" {
		panic("heater name must be provided")
	}
	target := gcmd.Float("TARGET", 20.0)
	if target < 10.0 {
		panic(fmt.Sprintf("TARGET must have minimum of %.1f", 10.0))
	}
	writeFile := gcmd.Int("WRITE_FILE", 0, nil, nil)

	heatersObj := self.printer.LookupObject("heaters", nil)
	heaters, ok := heatersObj.(pidCalibrateHeaterManager)
	if !ok {
		panic(fmt.Sprintf("heaters object does not implement pidCalibrateHeaterManager: %T", heatersObj))
	}
	heaterObj := heaters.LookupHeater(heaterName)
	heater, ok := heaterObj.(pidCalibrateHeater)
	if !ok || heater == nil {
		panic(fmt.Sprintf("heater %s not found", heaterName))
	}

	toolheadObj := self.printer.LookupObject("toolhead", nil)
	toolhead, ok := toolheadObj.(pidCalibrateToolhead)
	if !ok {
		panic(fmt.Sprintf("toolhead object does not implement pidCalibrateToolhead: %T", toolheadObj))
	}
	toolhead.Get_last_move_time()

	calibrate := NewControlAutoTune(heater, target)
	oldControl := heater.Set_control(calibrate)
	func() {
		defer func() {
			if r := recover(); r != nil {
				heater.Set_control(oldControl)
				panic(r)
			}
		}()
		if err := heaters.Set_temperature(heaterObj, target, true); err != nil {
			panic(err)
		}
	}()
	heater.Set_control(oldControl)

	if writeFile == 1 {
		calibrate.Write_file("/tmp/heattest.txt")
	}
	if calibrate.Check_busy(0., 0., 0.) {
		panic("pid_calibrate interrupted")
	}

	kp, ki, kd := calibrate.Calc_final_pid()
	logger.Info("PID parameters: pid_Kp=%.3f pid_Ki=%.3f pid_Kd=%.3f", kp, ki, kd)
	gcmd.RespondInfo(fmt.Sprintf(
		"PID parameters: pid_Kp=%.3f pid_Ki=%.3f pid_Kd=%.3f\n"+
			"The SAVE_CONFIG command will update the printer config file\n"+
			"with these parameters and restart the printer.",
		kp, ki, kd,
	), true)

	configfileObj := self.printer.LookupObject("configfile", nil)
	configfile, ok := configfileObj.(pidCalibrateConfigStore)
	if !ok {
		panic(fmt.Sprintf("configfile object does not implement pidCalibrateConfigStore: %T", configfileObj))
	}
	configfile.Set(heaterName, "control", "pid")
	configfile.Set(heaterName, "pid_Kp", fmt.Sprintf("%.3f", kp))
	configfile.Set(heaterName, "pid_Ki", fmt.Sprintf("%.3f", ki))
	configfile.Set(heaterName, "pid_Kd", fmt.Sprintf("%.3f", kd))

	return nil
}