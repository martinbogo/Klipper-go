package config

import (
	"goklipper/common/logger"
	"strings"
)

// AutosaveHeader is the delimiter block written before autosave data in printer.cfg.
const AutosaveHeader = `
#*# <---------------------- SAVE_CONFIG ---------------------->
#*# DO NOT EDIT THIS BLOCK OR BELOW. The contents are auto-generated.
#*#
`

// FindAutosaveData splits a config file's raw text into the regular portion
// and the decoded autosave portion (with the "#*# " prefix stripped).
func FindAutosaveData(data string) (string, string) {
	regular_data := data
	autosave_data := ""
	pos := strings.Index(data, AutosaveHeader)
	if pos >= 0 {
		regular_data = data[:pos]
		autosave_data = strings.TrimSpace(data[pos+len(AutosaveHeader):])
	}
	// Check for errors and strip line prefixes
	if strings.Index(regular_data, "\n#*# ") != -1 {
		logger.Debug("Can't read autosave from config file - autosave state corrupted")
		return data, ""
	}
	out := []string{""}
	lines := strings.Split(autosave_data, "\n")
	for _, line := range lines {

		if (!strings.HasPrefix(line, "#*#") ||
			(len(line) >= 4 && !strings.HasPrefix(line, "#*# "))) && autosave_data == "" {

			logger.Warn("Can't read autosave from config file - modifications after header")
			return data, ""
		}
		if len(line) > 4 {
			out = append(out, line[4:])
		} else {
			out = append(out, "")
		}
	}
	out = append(out, "")
	return regular_data, strings.Join(out, "\n")
}

// FormatAutosaveBlock wraps the given config string in the autosave block
// format, prefixing every line with "#*# " and prepending the header.
func FormatAutosaveBlock(autosaveData string) string {
	lines := strings.Split(autosaveData, "\n")
	processedLines := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		processedLine := strings.TrimSpace("#*# " + line)
		processedLines = append(processedLines, processedLine)
	}
	header := strings.TrimRight(AutosaveHeader, "\r\n\t ") + "\n"
	processedLines = append([]string{header}, processedLines...)
	processedLines = append(processedLines, "")
	return strings.Join(processedLines, "\n")
}
