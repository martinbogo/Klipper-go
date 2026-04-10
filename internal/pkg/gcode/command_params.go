package gcode

import (
	"fmt"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	"goklipper/common/utils/object"
	"strconv"
	"strings"
	"unicode"
)

// CommandParams provides typed parameter access for a parsed G-code command.
// It depends only on the raw parameter map and the command line string.
type CommandParams struct {
	Params      map[string]string
	Commandline string
	Command     string // needed for GetRawCommandParameters
}

func (self *CommandParams) Get(name string, _default interface{}, parser interface{}, minval *float64, maxval *float64,
	above *float64, below *float64) string {
	var value = self.Params[name]
	if value == "" {
		if _, ok := _default.(object.Sentinel); ok {
			panic(fmt.Errorf("Error on '%s': missing %s", self.Commandline, name))
		}
		if _default != nil {
			return _default.(string)
		} else {
			return ""
		}
	}

	value = string(value)
	f, _ := strconv.ParseFloat(value, 64)
	if minval != nil && f < *minval {
		logger.Errorf("Error on '%s': %s must have minimum of %f", self.Commandline, name, *minval)
	}

	if maxval != nil && f > *maxval {
		logger.Errorf("Error on '%s': %s must have maximum of %f", self.Commandline, name, *maxval)
	}

	if above != nil && f <= *above {
		logger.Errorf("Error on '%s': %s must have above of %f", self.Commandline, name, *above)
	}

	if below != nil && f >= *below {
		logger.Errorf("Error on '%s': %s must have below of %f", self.Commandline, name, *below)
	}
	return value
}

func (self *CommandParams) Get_int(name string, _default interface{}, minval *int, maxval *int) int {
	var value = self.Params[name]
	if value == "" {
		if _, ok := _default.(object.Sentinel); ok {
			panic(fmt.Errorf("Error on '%s': missing %s", self.Commandline, name))
		}
		if _default != nil {
			return _default.(int)
		} else {
			return 0
		}
	}

	var val, _ = strconv.ParseInt(value, 10, 32)
	if minval != nil && int(val) < cast.Int(minval) {
		logger.Errorf("Error on '%s': %s must have minimum of %d", self.Commandline, name, cast.Int(minval))
	}

	if maxval != nil && int(val) > cast.Int(maxval) {
		logger.Errorf("Error on '%s': %s must have maximum of %d", self.Commandline, name, cast.Int(maxval))
	}

	return int(val)
}

func (self *CommandParams) Get_float(name string, _default interface{}, minval *float64, maxval *float64,
	above *float64, below *float64) float64 {

	var value = self.Params[name]
	if value == "" {
		if _, ok := _default.(object.Sentinel); ok {
			panic(fmt.Errorf("Error on '%s': missing %s", self.Commandline, name))
		}
		if _default != nil {
			return _default.(float64)
		} else {
			return 0
		}
	}

	var val, _ = strconv.ParseFloat(value, 64)
	if minval != nil && val < *minval {
		logger.Errorf("Error on '%s': %s must have minimum of %f", self.Commandline, name, *minval)
		return *minval
	}

	if maxval != nil && val > *maxval {
		logger.Errorf("Error on '%s': %s must have maximum of %f", self.Commandline, name, *maxval)
		return *maxval
	}

	if above != nil && val <= *above {
		logger.Errorf("Error on '%s': %s must have above of %f", self.Commandline, name, *above)
		return *above
	}

	if below != nil && val >= *below {
		logger.Errorf("Error on '%s': %s must have below of %f", self.Commandline, name, *below)
		return *below
	}
	return val
}

func (self *CommandParams) Get_floatP(name string, _default *float64, minval *float64, maxval *float64,
	above *float64, below *float64) *float64 {

	value, ok := self.Params[name]
	if !ok {
		return _default
	}

	var val, _ = strconv.ParseFloat(value, 64)
	if minval != nil && val < cast.Float64(minval) {
		logger.Errorf("Error on '%s': %s must have minimum of %f", self.Commandline, name, cast.Float64(minval))
	}

	if maxval != nil && val > cast.Float64(maxval) {
		logger.Errorf("Error on '%s': %s must have maximum of %f", self.Commandline, name, cast.Float64(maxval))
	}

	if above != nil && val <= cast.Float64(above) {
		logger.Errorf("Error on '%s': %s must have above of %f", self.Commandline, name, cast.Float64(above))
	}

	if below != nil && val >= cast.Float64(below) {
		logger.Errorf("Error on '%s': %s must have below of %f", self.Commandline, name, cast.Float64(above))
	}
	return &val
}

func (self *CommandParams) Has(name string) bool {
	_, ok := self.Params[name]
	return ok
}

// GetRawCommandParameters returns the parameter portion of the command line,
// stripping the command name prefix and any trailing checksum.
func (self *CommandParams) GetRawCommandParameters() string {
	command := self.Command
	origline := self.Commandline
	param_start := len(command)
	param_end := len(origline)

	if strings.ToUpper(origline[:param_start]) != command {
		cmdIdx := strings.Index(strings.ToUpper(origline), command)
		if cmdIdx >= 0 {
			param_start = cmdIdx + len(command)
		}

		end := strings.LastIndex(origline, "*")
		if end >= 0 && IsDigitString(origline[end+1:]) {
			param_end = end
		}
	}

	if param_start < len(origline) && unicode.IsSpace(rune(origline[param_start])) {
		param_start++
	}

	if param_start >= len(origline) {
		return ""
	}
	if param_end > len(origline) {
		param_end = len(origline)
	}
	if param_start >= param_end {
		return ""
	}

	return origline[param_start:param_end]
}
