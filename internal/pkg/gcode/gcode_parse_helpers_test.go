package gcode

import (
	"testing"
)

func TestIsDigitChar(t *testing.T) {
	for _, c := range []byte("0123456789") {
		if !IsDigitChar(c) {
			t.Errorf("IsDigitChar(%q) = false, want true", c)
		}
	}
	for _, c := range []byte("abcxyzABCXYZ.-+! ") {
		if IsDigitChar(c) {
			t.Errorf("IsDigitChar(%q) = true, want false", c)
		}
	}
}

func TestIsDigitString(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"", false},
		{"0", true},
		{"123", true},
		{"12a", false},
		{"abc", false},
		{" ", false},
	}
	for _, tc := range cases {
		if got := IsDigitString(tc.s); got != tc.want {
			t.Errorf("IsDigitString(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestIsQuote(t *testing.T) {
	if !IsQuote('"') {
		t.Error("IsQuote('\"') = false, want true")
	}
	if !IsQuote('\'') {
		t.Error("IsQuote('\\'') = false, want true")
	}
	if IsQuote('a') {
		t.Error("IsQuote('a') = true, want false")
	}
	if IsQuote('`') {
		t.Error("IsQuote('`') = true, want false")
	}
}

func TestIsCommandValid(t *testing.T) {
	cases := []struct {
		cmd  string
		want bool
	}{
		{"SET_FAN_SPEED", true},
		{"QUERY_ENDSTOPS", true},
		{"M110", false}, // second char is digit
		{"G28", false},  // second char is digit
		{"lowerCase", false},
		{"123ABC", false},  // starts with digit
		{"ABC DEF", false}, // has space
		{"", true},         // edge case: empty passes (no digits, but empty rune slices)
	}
	for _, tc := range cases {
		if got := IsCommandValid(tc.cmd); got != tc.want {
			t.Errorf("IsCommandValid(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestIsValuePart(t *testing.T) {
	for _, c := range []byte("0123456789.-") {
		if !IsValuePart(c) {
			t.Errorf("IsValuePart(%q) = false, want true", c)
		}
	}
	for _, c := range []byte("abcABC+ ") {
		if IsValuePart(c) {
			t.Errorf("IsValuePart(%q) = true, want false", c)
		}
	}
}

func TestGetKeyGetVal(t *testing.T) {
	cases := []struct {
		input   string
		wantKey string
		wantVal string
		wantPos int
	}{
		{"X100", "X", "100", 4},
		{"Y-12.5", "Y", "-12.5", 6},
		{"E", "E", "", 1},
		{"100", "", "100", 3},
	}
	for _, tc := range cases {
		pos := 0
		key := GetKey(tc.input, &pos)
		val := GetVal(tc.input, &pos)
		if key != tc.wantKey || val != tc.wantVal || pos != tc.wantPos {
			t.Errorf("input=%q: key=%q val=%q pos=%d, want key=%q val=%q pos=%d",
				tc.input, key, val, pos, tc.wantKey, tc.wantVal, tc.wantPos)
		}
	}
}

func TestParseExtendedParams(t *testing.T) {
	cases := []struct {
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			"SPEED=100 ACCEL=500",
			map[string]string{"SPEED": "100", "ACCEL": "500"},
			false,
		},
		{
			`NAME="my profile" SPEED=200`,
			map[string]string{"NAME": "my profile", "SPEED": "200"},
			false,
		},
		{
			`MSG='hello world'`,
			map[string]string{"MSG": "hello world"},
			false,
		},
		{
			"",
			map[string]string{},
			false,
		},
		{
			"NOEQUAL",
			map[string]string{},
			false,
		},
		{
			`BAD="unclosed`,
			nil,
			true,
		},
		{
			"  KEY=value  ",
			map[string]string{"KEY": "value"},
			false,
		},
	}
	for _, tc := range cases {
		got, err := ParseExtendedParams(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseExtendedParams(%q): expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseExtendedParams(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("ParseExtendedParams(%q): got %v, want %v", tc.input, got, tc.want)
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("ParseExtendedParams(%q): key %q = %q, want %q", tc.input, k, got[k], v)
			}
		}
	}
}
