package addon

import (
	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
	"os"
	"path/filepath"
)

type SaveVariablesModule struct {
	filename string
	core     *SaveVariables
}

func NewSaveVariablesModule(config printerpkg.ModuleConfig) *SaveVariablesModule {
	self := &SaveVariablesModule{}
	self.filename = expandUser(config.String("filename", "", true))
	self.core = NewSaveVariables(self.filename)

	func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Error(err)
			}
		}()
		if err := self.core.EnsureFile(); err != nil {
			panic(err)
		}
	}()

	_ = self.core.LoadVariables()
	config.Printer().GCode().RegisterCommand("SAVE_VARIABLE", self.cmdSaveVariable, false,
		"Save arbitrary variables to disk")
	return self
}

func expandUser(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func (self *SaveVariablesModule) Variables() map[string]interface{} {
	return self.core.Variables()
}

func (self *SaveVariablesModule) cmdSaveVariable(gcmd printerpkg.Command) error {
	varname := gcmd.String("VARIABLE", "")
	value := gcmd.String("VALUE", "")
	return self.core.SaveVariable(varname, value)
}

func (self *SaveVariablesModule) Get_status(eventtime float64) map[string]interface{} {
	return self.core.GetStatus()
}

func LoadConfigSaveVariables(config printerpkg.ModuleConfig) interface{} {
	return NewSaveVariablesModule(config)
}
