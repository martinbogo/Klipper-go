// Pure G-code string parsing helpers.
// These functions contain no project-level dependencies and can be used
// by any package that needs to tokenize or validate G-code text.
package gcode

import (
	"fmt"
	"strings"
	"unicode"
)

// IsDigitChar reports whether the byte c is an ASCII decimal digit.
func IsDigitChar(c byte) bool {
	return c >= '0' && c <= '9'
}

// IsDigitString reports whether s consists entirely of Unicode digits.
// Returns false for the empty string.
func IsDigitString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// IsQuote reports whether c is a single-quote or double-quote character.
func IsQuote(c rune) bool {
	return c == '"' || c == '\''
}

// IsCommandValid reports whether cmd is a valid extended G-code command name.
// Valid names are all-uppercase, contain only letters/digits/underscores,
// do not start with a digit, and have at least two characters where the second
// character is not a digit (to avoid collisions with traditional G/M codes).
func IsCommandValid(cmd string) bool {
	if strings.ToUpper(cmd) != cmd {
		return false
	}

	noUnderscore := strings.ReplaceAll(cmd, "_", "A")
	for _, r := range noUnderscore {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}

	runes := []rune(cmd)
	if len(runes) > 0 && unicode.IsDigit(runes[0]) {
		return false
	}
	if len(runes) > 1 && unicode.IsDigit(runes[1]) {
		return false
	}

	return true
}

// IsValuePart reports whether byte c can be part of a numeric token
// (digit, decimal point, or minus sign).
func IsValuePart(c uint8) bool {
	return IsDigitChar(c) || c == '.' || c == '-'
}

// GetKey reads a non-numeric key token from line starting at *pos,
// advancing *pos past the consumed characters.
func GetKey(line string, pos *int) string {
	key := []byte{}
	for *pos < len(line) {
		c := line[*pos]
		if !IsValuePart(c) {
			key = append(key, c)
			*pos++
		} else {
			break
		}
	}
	return string(key)
}

// GetVal reads a numeric value token from line starting at *pos,
// advancing *pos past the consumed characters.
func GetVal(line string, pos *int) string {
	val := []byte{}
	for *pos < len(line) {
		c := line[*pos]
		if IsValuePart(c) {
			val = append(val, c)
			*pos++
		} else {
			break
		}
	}
	return string(val)
}

// ParseExtendedParams parses an extended G-code parameter string of the form
// KEY1=VALUE1 KEY2=VALUE2 ... and returns a map of upper-cased keys to values.
// Values may be optionally wrapped in single or double quotes.
func ParseExtendedParams(input string) (map[string]string, error) {
	const (
		ReadKey = iota
		ReadVal
		ReadEnd
	)

	pos := 0
	currentKey := ""
	params := strings.TrimSpace(input)
	results := make(map[string]string)

	state := ReadKey
	for state != ReadEnd {
		switch state {
		case ReadKey:
			eqIdx := strings.IndexByte(params[pos:], '=')
			if eqIdx == -1 {
				state = ReadEnd
				continue
			}
			eqIdx += pos

			key := params[pos:eqIdx]
			currentKey = strings.ToUpper(strings.TrimSpace(key))

			params = params[eqIdx+1:]
			pos = 0
			state = ReadVal

		case ReadVal:
			if len(params) == 0 {
				state = ReadEnd
				continue
			}

			firstChar := rune(params[0])
			if IsQuote(firstChar) {
				endQuoteIdx := strings.IndexRune(params[1:], firstChar)
				if endQuoteIdx == -1 {
					return nil, fmt.Errorf("malformed command: missing closing quote")
				}
				endQuoteIdx++
				value := params[1:endQuoteIdx]
				results[currentKey] = value
				if endQuoteIdx+1 < len(params) {
					params = strings.TrimLeft(params[endQuoteIdx+1:], " \t")
					pos = 0
					state = ReadKey
				} else {
					state = ReadEnd
				}
			} else {
				spaceIdx := strings.IndexByte(params, ' ')
				if spaceIdx == -1 {
					results[currentKey] = strings.TrimSpace(params)
					state = ReadEnd
				} else {
					value := params[:spaceIdx]
					results[currentKey] = strings.TrimSpace(value)
					params = strings.TrimLeft(params[spaceIdx+1:], " \t")
					pos = 0
					state = ReadKey
				}
			}
		}
	}

	return results, nil
}

// IsTraditionalGCode reports whether cmd is a traditional G/M-code command
// (first character uppercase letter, second character a digit).
func IsTraditionalGCode(cmd string) bool {
	if cmd == "" {
		return false
	}
	cmd = strings.Split(strings.ToUpper(cmd), " ")[0]
	runes := []rune(cmd)
	return len(runes) > 1 && unicode.IsUpper(runes[0]) && unicode.IsDigit(runes[1])
}

// ParseGcodeTokens tokenises a single G-code input line into alternating key/value tokens.
// The returned slice always starts with an empty string sentinel at index 0.
func ParseGcodeTokens(input string) []string {
	tokens := []string{""}
	if input == "" {
		return tokens
	}
	parts := strings.Fields(input)
	for _, part := range parts {
		pos := 0
		for pos < len(part) {
			tokens = append(tokens, GetKey(part, &pos))
			tokens = append(tokens, GetVal(part, &pos))
		}
	}
	return tokens
}

// ParseBooleanString interprets a normalized (lowercase, trimmed) state string as a
// boolean. Returns (value, true) when the string matches a known truthy or falsy token,
// or (false, false) when the string is unrecognized.
func ParseBooleanString(state string) (bool, bool) {
	switch state {
	case "1", "true", "on", "enable", "enabled":
		return true, true
	case "0", "false", "off", "disable", "disabled":
		return false, true
	default:
		return false, false
	}
}
