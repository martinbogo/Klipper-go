package addon

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"goklipper/common/logger"
	"goklipper/internal/pkg/printer"
)

const (
	cmdExcludeObjectStartHelp    = "Marks the beginning the current object as labeled"
	cmdExcludeObjectEndHelp      = "Marks the end the current object"
	cmdExcludeObjectHelp         = "Cancel moves inside a specified objects"
	cmdExcludeObjectDefineHelp   = "Provides a summary of an object"
	cmdExcludeObjectEndNoObjHelp = "Indicate a non-object, purge tower, or other global feature"
	cmdExcludeObjectSetObjHelp   = "Set the number of objects"
)

type ExcludeObjectModule struct {
	printer       printer.ModulePrinter
	gcode         printer.GCodeRuntime
	gcodeMove     printer.MoveTransformController
	core          *ExcludeObject
	nextTransform printer.MoveTransform
}

func LoadConfigExcludeObject(config printer.ModuleConfig) interface{} {
	printerRef := config.Printer()
	self := &ExcludeObjectModule{
		printer:       printerRef,
		gcode:         printerRef.GCode(),
		gcodeMove:     printerRef.GCodeMove(),
		core:          NewExcludeObject(),
		nextTransform: nil,
	}
	self.printer.RegisterEventHandler("virtual_sdcard:reset_file", self.handleResetFile)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT_START", self.cmdExcludeObjectStart, true, cmdExcludeObjectStartHelp)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT_END", self.cmdExcludeObjectEnd, true, cmdExcludeObjectEndHelp)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT", self.cmdExcludeObject, true, cmdExcludeObjectHelp)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT_DEFINE", self.cmdExcludeObjectDefine, true, cmdExcludeObjectDefineHelp)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT_END_NO_OBJ", self.cmdExcludeObjectEndNoObj, true, cmdExcludeObjectEndNoObjHelp)
	self.gcode.RegisterCommand("EXCLUDE_OBJECT_SET_OBJ", self.cmdExcludeObjectSetObj, true, cmdExcludeObjectSetObjHelp)
	return self
}

func (self *ExcludeObjectModule) currentExtruderName() string {
	return self.printer.CurrentExtruderName()
}

func (self *ExcludeObjectModule) tuningTowerActive() bool {
	obj := self.printer.LookupObject("tuning_tower", nil)
	if obj == nil {
		return false
	}
	tuningTower, ok := obj.(*TuningTowerModule)
	if !ok || tuningTower == nil {
		return false
	}
	return tuningTower.Is_active()
}

func (self *ExcludeObjectModule) registerTransform() {
	if self.nextTransform != nil {
		return
	}
	if self.tuningTowerActive() {
		log.Println("The exclude_object move transform is not being loaded due to Tuning tower being Active")
		return
	}
	self.nextTransform = self.gcodeMove.SetMoveTransform(self, true)
	self.core.AttachTransform(self.nextTransform, self.currentExtruderName())
}

func (self *ExcludeObjectModule) unregisterTransform() error {
	if self.nextTransform == nil {
		return nil
	}
	if self.tuningTowerActive() {
		log.Println("The Exclude Object move transform was not unregistered because it is not at the head of the transform chain.")
		return fmt.Errorf("The Exclude Object move transform was not unregistered because it is not at the head of the transform chain.")
	}
	self.gcodeMove.SetMoveTransform(self.nextTransform, true)
	self.nextTransform = nil
	self.core.DetachTransform()
	self.gcodeMove.ResetLastPosition()
	return nil
}

func (self *ExcludeObjectModule) resetState() {
	self.core.ResetState()
}

func (self *ExcludeObjectModule) handleResetFile([]interface{}) error {
	self.resetState()
	return self.unregisterTransform()
}

func (self *ExcludeObjectModule) GetPosition() []float64 {
	return self.core.GetPosition(self.currentExtruderName())
}

func (self *ExcludeObjectModule) Objects() []map[string]interface{} {
	return self.core.Objects()
}

func (self *ExcludeObjectModule) ExcludedObjects() []string {
	return self.core.ExcludedObjects()
}

func (self *ExcludeObjectModule) CurrentObject() string {
	return self.core.CurrentObject()
}

func (self *ExcludeObjectModule) Get_position() []float64 {
	return self.GetPosition()
}

func (self *ExcludeObjectModule) Get_status(eventtime float64) map[string]interface{} {
	return self.core.GetStatus()
}

func (self *ExcludeObjectModule) Move(newpos []float64, speed float64) {
	self.core.Move(newpos, speed, self.currentExtruderName())
}

func (self *ExcludeObjectModule) cmdExcludeObjectStart(gcmd printer.Command) error {
	name := strings.ToUpper(gcmd.String("NAME", ""))
	self.core.StartObject(name)
	return nil
}

func (self *ExcludeObjectModule) cmdExcludeObjectEnd(gcmd printer.Command) error {
	if self.core.CurrentObject() == "" && self.nextTransform != nil {
		gcmd.RespondInfo("EXCLUDE_OBJECT_END called, but no object is currently active", true)
		return nil
	}
	name := strings.ToUpper(gcmd.String("NAME", ""))
	if msg := self.core.EndObject(name); msg != "" {
		gcmd.RespondInfo(msg, true)
	}
	return nil
}

func (self *ExcludeObjectModule) cmdExcludeObject(gcmd printer.Command) error {
	reset := gcmd.String("RESET", "")
	current := gcmd.String("CURRENT", "")
	name := strings.ToUpper(gcmd.String("NAME", ""))
	if reset != "" {
		if name != "" {
			self.unexcludeObject(name)
		} else {
			self.core.ClearExcluded()
		}
		return nil
	}
	if name != "" {
		if !self.objectIsExcluded(name) {
			self.excludeObject(name)
		}
		return nil
	}
	if current != "" {
		if self.core.CurrentObject() == "" {
			gcmd.RespondInfo("There is no current object to cancel", true)
			return nil
		}
		self.excludeObject(self.core.CurrentObject())
		return nil
	}
	self.listExcludedObjects(gcmd)
	return nil
}

func (self *ExcludeObjectModule) cmdExcludeObjectDefine(gcmd printer.Command) error {
	reset := gcmd.String("RESET", "")
	name := gcmd.String("NAME", "")
	if reset != "" {
		return self.handleResetFile(nil)
	}
	if name == "" {
		self.listObjects(gcmd)
		return nil
	}

	parameters := gcmd.Parameters()
	center := parameters["CENTER"]
	polygon := parameters["POLYGON"]
	obj := map[string]interface{}{"name": name}

	if center != "" {
		var centerArr map[string]interface{}
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"center":[%s]}`, center)), &centerArr); err != nil {
			logger.Panic(err)
		}
		coords := centerArr["center"].([]interface{})
		obj["center"] = []float64{coords[0].(float64), coords[1].(float64)}
	}
	if polygon != "" {
		var polygonArr map[string]interface{}
		if err := json.Unmarshal([]byte(fmt.Sprintf(`{"polygon":%s}`, polygon)), &polygonArr); err != nil {
			logger.Panic(err)
		}
		var points [][]float64
		for _, point := range polygonArr["polygon"].([]interface{}) {
			coords := point.([]interface{})
			points = append(points, []float64{coords[0].(float64), coords[1].(float64)})
		}
		obj["polygon"] = points
	}
	self.addObjectDefinition(obj)
	return nil
}

func (self *ExcludeObjectModule) cmdExcludeObjectEndNoObj(gcmd printer.Command) error {
	if self.core.CurrentObject() != "" {
		self.gcode.RunScriptFromCommand(fmt.Sprintf("EXCLUDE_OBJECT_END NAME=%s", self.core.CurrentObject()))
	}
	return nil
}

func (self *ExcludeObjectModule) cmdExcludeObjectSetObj(gcmd printer.Command) error {
	t := gcmd.String("T", "")
	if t == "" {
		return nil
	}
	count, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return err
	}
	for i := 0; i < int(count); i++ {
		self.gcode.RunScriptFromCommand(fmt.Sprintf("EXCLUDE_OBJECT_DEFINE NAME=%d", i))
	}
	return nil
}

func (self *ExcludeObjectModule) addObjectDefinition(definition map[string]interface{}) {
	self.core.AddObjectDefinition(definition)
}

func (self *ExcludeObjectModule) excludeObject(name string) {
	self.registerTransform()
	self.gcode.RespondInfo(fmt.Sprintf("Excluding object %s", name), true)
	self.core.Exclude(name)
}

func (self *ExcludeObjectModule) unexcludeObject(name string) {
	self.gcode.RespondInfo(fmt.Sprintf("Unexcluding object %s", name), true)
	self.core.Unexclude(name)
}

func (self *ExcludeObjectModule) listObjects(gcmd printer.Command) {
	if gcmd.String("JSON", "") != "" {
		objectList, _ := json.Marshal(self.core.Objects())
		gcmd.RespondInfo(fmt.Sprintf("Known objects: %s", string(objectList)), true)
		return
	}
	var objectNames []string
	for _, obj := range self.core.Objects() {
		objectNames = append(objectNames, obj["name"].(string))
	}
	gcmd.RespondInfo(fmt.Sprintf("Known objects: %s", objectNames), true)
}

func (self *ExcludeObjectModule) listExcludedObjects(gcmd printer.Command) {
	gcmd.RespondInfo(fmt.Sprintf("Excluded objects: %s", self.core.ExcludedObjects()), true)
}

func (self *ExcludeObjectModule) objectExists(name string) bool {
	return self.core.ObjectExists(name)
}

func (self *ExcludeObjectModule) objectIsExcluded(name string) bool {
	return self.core.ObjectIsExcluded(name)
}
