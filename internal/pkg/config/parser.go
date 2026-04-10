package config

import (
	"fmt"
	"goklipper/common/configparser"
	"goklipper/common/logger"
	"goklipper/common/utils/file"
	"path/filepath"
	"sort"
	"strings"
)

// ReadConfigFile reads a config file from disk and normalises line endings.
func ReadConfigFile(filename string) (string, error) {
	bs, err := file.GetBytes(filename)
	if err != nil {
		logger.Errorf("Unable to open config file %s", filename)
		return "", err
	}
	data := string(bs)
	return strings.ReplaceAll(data, "\r\n", "\n"), nil
}

// StripDuplicates comments out fields in data that are already defined in
// fileconfig, preventing autosave entries from overriding base config.
func StripDuplicates(data string, fileconfig *configparser.RawConfigParser) string {
	lines := strings.Split(data, "\n")
	section := ""
	is_dup_field := false
	for lineno, line := range lines {
		pruned_line := strings.Split(line, "#")[0]
		if pruned_line == "" {
			continue
		}
		if pruned_line[0] == ' ' || pruned_line[0] == '\t' {
			if is_dup_field {
				lines[lineno] = "#" + line
			}
			continue
		}
		is_dup_field = false
		pruned_line = strings.TrimSpace(pruned_line)
		if pruned_line[0] == '[' {
			section = pruned_line[1 : len(pruned_line)-1]
			continue
		}
		field := strings.TrimSpace(strings.Split(pruned_line, "=")[0])
		if fileconfig.Has_option(section, field) {
			is_dup_field = true
			lines[lineno] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}

// ParseConfigBuffer parses a buffered slice of config lines into fileconfig.
func ParseConfigBuffer(buffer []string, filename string, fileconfig *configparser.RawConfigParser) {
	if len(buffer) == 0 {
		return
	}
	fileconfig.Readfp(strings.NewReader(strings.Join(buffer, "\n")), filename)
}

// ResolveInclude expands an include glob pattern relative to source_filename
// and recursively parses each matched file into fileconfig.
func ResolveInclude(source_filename, include_spec string, fileconfig *configparser.RawConfigParser, visited map[string]string) ([]string, error) {
	dirname := filepath.Dir(source_filename)
	include_spec = strings.TrimSpace(include_spec)
	include_glob := filepath.Join(dirname, include_spec)

	hasMagic := strings.ContainsAny(include_glob, "*?[]")

	matches, err := filepath.Glob(include_glob)
	if err != nil {
		return nil, fmt.Errorf("glob error: %v", err)
	}

	if len(matches) == 0 && !hasMagic {
		panic(fmt.Sprintf("include file '%s' does not exist", include_glob))
	}

	sort.Strings(matches)

	for _, includeFilename := range matches {
		data, err := ReadConfigFile(includeFilename)
		if err != nil {
			return nil, err
		}
		ParseConfig(data, includeFilename, fileconfig, visited)
	}

	return matches, nil
}

// ParseConfig recursively parses a config data string, resolving include
// directives and populating fileconfig.
func ParseConfig(data, filename string, fileconfig *configparser.RawConfigParser, visited map[string]string) {
	path, err := filepath.Abs(filename)
	if err != nil {
		logger.Error(err.Error())
	}
	if _, ok := visited[path]; ok {
		panic(fmt.Sprintf("Recursive include of config file '%s'", filename))
	}
	visited[path] = ""
	lines := strings.Split(data, "\n")
	buffer := []string{}
	for _, line := range lines {
		// Strip trailing comment
		pos := strings.Index(line, "#")
		if pos >= 0 {
			line = line[:pos]
		}
		header := line
		if len(header) != 0 && strings.HasPrefix(header, "include ") {
			ParseConfigBuffer(buffer, filename, fileconfig)
			buffer = []string{}
			include_spec := strings.TrimSpace(header[8:])
			ResolveInclude(filename, include_spec, fileconfig, visited)
		} else {
			buffer = append(buffer, line)
		}
	}
	ParseConfigBuffer(buffer, filename, fileconfig)
	delete(visited, path)
}
