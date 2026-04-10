package kinematics

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"goklipper/common/utils/object"
	"goklipper/common/value"
)

type StepperDistanceConfig interface {
	Get(option string, default1 interface{}, noteValid bool) interface{}
	Getfloat(option string, default1 interface{}, minval, maxval, above, below float64, noteValid bool) float64
	Getint(option string, default1 interface{}, minval, maxval int, noteValid bool) int
	Get_name() string
}

func parseGearRatio(config StepperDistanceConfig, noteValid bool) float64 {
	raw := config.Get("gear_ratio", []interface{}{}, noteValid)
	return parseGearRatioValue(raw, config.Get_name())
}

func parseGearRatioValue(raw interface{}, section string) float64 {
	switch value := raw.(type) {
	case nil:
		return 1.0
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 1.0
		}
		result := 1.0
		for _, item := range strings.Split(trimmed, ",") {
			pair := strings.Split(strings.TrimSpace(item), ":")
			if len(pair) != 2 {
				panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
			}
			g1, err1 := strconv.ParseFloat(strings.TrimSpace(pair[0]), 64)
			g2, err2 := strconv.ParseFloat(strings.TrimSpace(pair[1]), 64)
			if err1 != nil || err2 != nil {
				panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
			}
			result *= g1 / g2
		}
		return result
	case []interface{}:
		if len(value) == 0 {
			return 1.0
		}
		result := 1.0
		for _, item := range value {
			pair := normalizeGearRatioPair(item, section)
			result *= pair[0] / pair[1]
		}
		return result
	case []float64:
		if len(value) == 0 {
			return 1.0
		}
		if len(value) != 2 {
			panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
		}
		return value[0] / value[1]
	default:
		panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
	}
}

func normalizeGearRatioPair(raw interface{}, section string) [2]float64 {
	switch value := raw.(type) {
	case []float64:
		if len(value) != 2 {
			panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
		}
		return [2]float64{value[0], value[1]}
	case []interface{}:
		if len(value) != 2 {
			panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
		}
		g1, ok1 := value[0].(float64)
		g2, ok2 := value[1].(float64)
		if !ok1 || !ok2 {
			panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
		}
		return [2]float64{g1, g2}
	default:
		panic(fmt.Sprintf("gear_ratio invalid in section '%s'", section))
	}
}

func ParseStepperDistance(config StepperDistanceConfig, unitsInRadians interface{}, noteValid bool) (float64, int) {
	if unitsInRadians == nil {
		rotationDistance := config.Get("rotation_distance", value.None, false)
		gearRatio := config.Get("gear_ratio", value.None, false)
		unitsInRadians = rotationDistance == nil && gearRatio != nil
	}

	var rotationDist float64
	if unitsInRadians.(bool) {
		rotationDist = 2.0 * math.Pi
		config.Get("gear_ratio", object.Sentinel{}, noteValid)
	} else {
		rotationDist = config.Getfloat("rotation_distance", object.Sentinel{}, 0, 0, 0, 0, noteValid)
	}
	microsteps := config.Getint("microsteps", 0, 1, 0, noteValid)
	fullSteps := config.Getint("full_steps_per_rotation", 200, 1, 0, noteValid)
	if fullSteps%4 != 0 {
		panic(fmt.Sprintf("full_steps_per_rotation invalid in section '%s'", config.Get_name()))
	}
	gearing := parseGearRatio(config, noteValid)
	return rotationDist, int(float64(fullSteps) * float64(microsteps) * gearing)
}
