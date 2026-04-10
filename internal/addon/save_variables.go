package addon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"goklipper/common/ini"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type SaveVariables struct {
	filename     string
	allVariables map[string]interface{}
	mu           sync.RWMutex
}

func NewSaveVariables(filename string) *SaveVariables {
	return &SaveVariables{
		filename:     filename,
		allVariables: map[string]interface{}{},
	}
}

func (self *SaveVariables) EnsureFile() error {
	if _, err := os.Stat(self.filename); os.IsNotExist(err) {
		file, createErr := os.Create(self.filename)
		if createErr != nil {
			return createErr
		}
		return file.Close()
	} else if err != nil {
		return err
	}
	return nil
}

func (self *SaveVariables) LoadVariables() error {
	cfg, err := ini.Load(self.filename)
	if err != nil {
		return err
	}

	loaded := map[string]interface{}{}
	if section, err := cfg.GetSection("Variables"); err == nil {
		for _, key := range section.Keys() {
			value, parseErr := ParsePythonLiteral(key.String())
			if parseErr == nil {
				loaded[key.Name()] = value
			}
		}
	}

	self.mu.Lock()
	self.allVariables = loaded
	self.mu.Unlock()
	return nil
}

func IsFloat(s string) bool {
	if s == "" || strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return false
	}

	_, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return false
	}

	return strings.ContainsAny(s, ".eE") ||
		strings.EqualFold(s, "inf") ||
		strings.EqualFold(s, "+inf") ||
		strings.EqualFold(s, "-inf") ||
		strings.EqualFold(s, "nan")
}

func ParsePythonLiteral(s string) (interface{}, error) {
	switch s {
	case "True", "true":
		return true, nil
	case "False", "false":
		return false, nil
	case "None", "null":
		return nil, nil
	}
	if IsFloat(s) {
		if num, err := strconv.ParseFloat(s, 64); err == nil {
			return num, nil
		}
	} else {
		if num, err := strconv.ParseInt(s, 10, 64); err == nil {
			return num, nil
		}
	}

	if len(s) > 0 {
		switch s[0] {
		case '[', '{', '(':
			var result interface{}
			if err := json.Unmarshal([]byte(ReplaceString(s)), &result); err == nil {
				return result, nil
			}
		case '\'', '"':
			return strings.Trim(s, `"'`), nil
		}
	}

	return s, nil
}

func ReplaceString(s string) string {
	re := regexp.MustCompile(`'(true|false|null)'`)
	s = re.ReplaceAllString(s, `$1`)

	s = strings.ReplaceAll(s, "(", "[")
	s = strings.ReplaceAll(s, ")", "]")
	s = strings.ReplaceAll(s, "'", "\"")
	s = strings.ReplaceAll(s, "True", "true")
	s = strings.ReplaceAll(s, "False", "false")
	s = strings.ReplaceAll(s, "None", "null")

	return s
}

func (self *SaveVariables) SaveVariable(varname string, value string) error {
	val, err := ParsePythonLiteral(value)
	if err != nil {
		return err
	}

	self.mu.Lock()
	self.allVariables[varname] = val

	cfg := ini.Empty()
	section := cfg.NewSection("Variables")
	for name, item := range self.allVariables {
		section.NewKey(name, Literal(item))
	}
	self.mu.Unlock()

	file, err := os.Create(self.filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(cfg.IniString()); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return self.LoadVariables()
}

func Literal(v interface{}) string {
	switch val := v.(type) {
	case string:
		return `"` + strings.ReplaceAll(val, `"`, `\"`) + `"`
	case bool:
		if val {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%f", val)
	case []interface{}:
		items := make([]string, len(val))
		for i, item := range val {
			items[i] = Literal(item)
		}
		return "[" + strings.Join(items, ",") + "]"
	case map[string]interface{}:
		items := make([]string, 0, len(val))
		for k, item := range val {
			items = append(items, Literal(k)+":"+Literal(item))
		}
		return "{" + strings.Join(items, ",") + "}"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (self *SaveVariables) Variables() map[string]interface{} {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return self.allVariables
}

func (self *SaveVariables) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"variables": self.Variables(),
	}
}