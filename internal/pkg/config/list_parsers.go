package config

import (
	"goklipper/common/logger"
	"reflect"
	"strconv"
	"strings"
)

// ParseIntListFromString splits str by sep, parses each element as int, and pads
// the result with zeros until it has at least count elements.
// count=0 means no padding.
func ParseIntListFromString(str string, sep string, count int) []int {
	ret := []int{}
	parts := strings.Split(str, sep)
	for _, s := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			logger.Error(err.Error())
		} else {
			ret = append(ret, v)
		}
	}
	for i := 0; i < count-len(ret); i++ {
		ret = append(ret, 0)
	}
	return ret
}

// ParseFloatListFromString splits str by sep, parses each element as float64, and pads
// the result with zeros until it has at least count elements.
// count=0 means no padding.
func ParseFloatListFromString(str string, sep string, count int) []float64 {
	ret := []float64{}
	parts := strings.Split(str, sep)
	for _, s := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			logger.Error(err.Error())
		} else {
			ret = append(ret, v)
		}
	}
	for i := 0; i < count-len(ret); i++ {
		ret = append(ret, 0)
	}
	return ret
}

// parseKindValue converts a trimmed string token to the requested reflect.Kind.
// Supports reflect.Int and reflect.Float64; returns nil for other kinds.
func parseKindValue(s string, kind reflect.Kind) interface{} {
	switch kind {
	case reflect.Int:
		v, _ := strconv.Atoi(s)
		return v
	case reflect.Float64:
		v, _ := strconv.ParseFloat(s, 64)
		return v
	default:
		return nil
	}
}

// ParseMultilineList parses a newline-separated list of comma-separated rows.
// Each element is converted to the given reflect.Kind.
// This handles the seps == [",", "\n"] case from ConfigWrapper.Getlists.
func ParseMultilineList(str string, kind reflect.Kind) [][]interface{} {
	ret := [][]interface{}{}
	str = strings.TrimSpace(str)
	for _, line := range strings.Split(str, "\n") {
		row := []interface{}{}
		for _, cell := range strings.Split(line, ",") {
			row = append(row, parseKindValue(strings.TrimSpace(cell), kind))
		}
		ret = append(ret, row)
	}
	return ret
}

// ParseSeparatedList parses str using each separator in seps in order,
// converting each element to the given reflect.Kind.
// This handles the general seps case from ConfigWrapper.Getlists.
func ParseSeparatedList(str string, seps []string, kind reflect.Kind) []interface{} {
	ret := []interface{}{}
	for _, sep := range seps {
		s := strings.TrimSpace(str)
		s = strings.ReplaceAll(s, "\t", "")
		for _, item := range strings.Split(s, sep) {
			ret = append(ret, parseKindValue(item, kind))
		}
	}
	return ret
}
