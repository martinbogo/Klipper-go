package addon

import (
	"fmt"
	"math"
	"sort"
)

type ExcludeTransform interface {
	GetPosition() []float64
	Move([]float64, float64)
}

type ExcludeObject struct {
	transform              ExcludeTransform
	lastPositionExtruded   []float64
	lastPositionExcluded   []float64
	objects                []map[string]interface{}
	excludedObjects        []string
	currentObject          string
	inExcludedRegion       bool
	lastPosition           []float64
	extrusionOffsets       map[string][]float64
	maxPositionExtruded    float64
	maxPositionExcluded    float64
	extruderAdj            float64
	initialExtrusionMoves  int
	lastSpeed              float64
	wasExcludedAtStart     bool
}

func NewExcludeObject() *ExcludeObject {
	self := &ExcludeObject{
		lastPositionExtruded: []float64{0., 0., 0., 0.},
		lastPositionExcluded: []float64{0., 0., 0., 0.},
		extrusionOffsets:     map[string][]float64{},
	}
	self.ResetState()
	return self
}

func (self *ExcludeObject) Objects() []map[string]interface{} {
	return self.objects
}

func (self *ExcludeObject) ExcludedObjects() []string {
	return self.excludedObjects
}

func (self *ExcludeObject) CurrentObject() string {
	return self.currentObject
}

func (self *ExcludeObject) ResetState() {
	self.objects = []map[string]interface{}{}
	self.excludedObjects = []string{}
	self.currentObject = ""
	self.inExcludedRegion = false
	self.wasExcludedAtStart = false
}

func (self *ExcludeObject) AttachTransform(transform ExcludeTransform, extruderName string) {
	self.transform = transform
	self.maxPositionExtruded = 0
	self.maxPositionExcluded = 0
	self.extruderAdj = 0
	self.initialExtrusionMoves = 5
	self.lastPosition = []float64{0., 0., 0., 0.}
	position := self.GetPosition(extruderName)
	copy(self.lastPositionExtruded[:], position[:])
	copy(self.lastPositionExcluded[:], position[:])
	copy(self.lastPosition, position)
	self.lastSpeed = 0
	self.inExcludedRegion = false
	self.wasExcludedAtStart = false
}

func (self *ExcludeObject) DetachTransform() {
	self.transform = nil
	self.maxPositionExtruded = 0
	self.maxPositionExcluded = 0
	self.extruderAdj = 0
	self.initialExtrusionMoves = 0
	self.lastPosition = nil
	self.lastSpeed = 0
	self.inExcludedRegion = false
	self.wasExcludedAtStart = false
}

func (self *ExcludeObject) getExtrusionOffsets(extruderName string) []float64 {
	offset, ok := self.extrusionOffsets[extruderName]
	if !ok {
		offset = []float64{0., 0., 0., 0.}
		self.extrusionOffsets[extruderName] = offset
	}
	return offset
}

func (self *ExcludeObject) GetPosition(extruderName string) []float64 {
	if self.transform == nil {
		return nil
	}
	offset := self.getExtrusionOffsets(extruderName)
	pos := self.transform.GetPosition()
	if self.lastPosition == nil || len(self.lastPosition) != len(pos) {
		self.lastPosition = make([]float64, len(pos))
	}
	for i := 0; i < len(pos) && i < len(offset); i++ {
		self.lastPosition[i] = pos[i] + offset[i]
	}
	lastPosition := make([]float64, len(self.lastPosition))
	copy(lastPosition, self.lastPosition)
	return lastPosition
}

func (self *ExcludeObject) normalMove(newpos []float64, speed float64, extruderName string) {
	offset := self.getExtrusionOffsets(extruderName)

	if self.initialExtrusionMoves > 0 && self.lastPosition[3] != newpos[3] {
		self.initialExtrusionMoves--
	}

	copy(self.lastPosition, newpos)
	copy(self.lastPositionExtruded, self.lastPosition)
	self.maxPositionExtruded = math.Max(self.maxPositionExtruded, newpos[3])

	if (offset[0] != 0 || offset[1] != 0) &&
		(newpos[0] != self.lastPositionExcluded[0] ||
			newpos[1] != self.lastPositionExcluded[1]) {
		offset[0] = 0
		offset[1] = 0
		offset[2] = 0
		offset[3] += self.extruderAdj
		self.extruderAdj = 0
	}

	if offset[2] != 0 && newpos[2] != self.lastPositionExcluded[2] {
		offset[2] = 0
	}

	if self.extruderAdj != 0 && newpos[3] != self.lastPositionExcluded[3] {
		offset[3] += self.extruderAdj
		self.extruderAdj = 0
	}

	txPos := append([]float64{}, newpos...)
	for i := 0; i < len(txPos) && i < len(offset); i++ {
		txPos[i] = newpos[i] - offset[i]
	}
	self.transform.Move(txPos, speed)
}

func (self *ExcludeObject) ignoreMove(newpos []float64, speed float64, extruderName string) {
	offset := self.getExtrusionOffsets(extruderName)
	for i := 0; i < 3 && i < len(offset) && i < len(newpos) && i < len(self.lastPositionExtruded); i++ {
		offset[i] = newpos[i] - self.lastPositionExtruded[i]
	}
	offset[3] = offset[3] + newpos[3] - self.lastPosition[3]
	copy(self.lastPosition, newpos)
	copy(self.lastPositionExcluded, self.lastPosition)
	self.maxPositionExcluded = math.Max(self.maxPositionExcluded, newpos[3])
}

func (self *ExcludeObject) moveIntoExcludedRegion(newpos []float64, speed float64, extruderName string) {
	self.inExcludedRegion = true
	self.ignoreMove(newpos, speed, extruderName)
}

func (self *ExcludeObject) moveFromExcludedRegion(newpos []float64, speed float64, extruderName string) {
	self.inExcludedRegion = false
	self.extruderAdj = self.maxPositionExcluded - self.lastPositionExcluded[3] -
		(self.maxPositionExtruded - self.lastPositionExtruded[3])
	self.normalMove(newpos, speed, extruderName)
}

func (self *ExcludeObject) testInExcludedRegion() bool {
	return self.currentObject != "" && self.initialExtrusionMoves == 0
}

func (self *ExcludeObject) Move(newpos []float64, speed float64, extruderName string) {
	if self.transform == nil {
		return
	}
	moveInExcludedRegion := self.testInExcludedRegion()
	self.lastSpeed = speed

	if moveInExcludedRegion {
		if self.inExcludedRegion {
			self.ignoreMove(newpos, speed, extruderName)
		} else {
			self.moveIntoExcludedRegion(newpos, speed, extruderName)
		}
	} else {
		if self.inExcludedRegion {
			self.moveFromExcludedRegion(newpos, speed, extruderName)
		} else {
			self.normalMove(newpos, speed, extruderName)
		}
	}
}

func (self *ExcludeObject) AddObjectDefinition(definition map[string]interface{}) {
	self.objects = append(self.objects, definition)
	sort.Slice(self.objects, func(i, j int) bool {
		return self.objects[i]["name"].(string) < self.objects[j]["name"].(string)
	})
}

func (self *ExcludeObject) ObjectExists(name string) bool {
	for _, obj := range self.objects {
		if obj["name"] == name {
			return true
		}
	}
	return false
}

func (self *ExcludeObject) ObjectIsExcluded(name string) bool {
	for _, obj := range self.excludedObjects {
		if obj == name {
			return true
		}
	}
	return false
}

func (self *ExcludeObject) StartObject(name string) {
	if !self.ObjectExists(name) {
		self.AddObjectDefinition(map[string]interface{}{"name": name})
	}
	self.currentObject = name
	self.wasExcludedAtStart = self.testInExcludedRegion()
}

func (self *ExcludeObject) EndObject(name string) string {
	if name != "" && name != self.currentObject {
		msg := fmt.Sprintf("EXCLUDE_OBJECT_END NAME=%s does not match the current object NAME=%s", name, self.currentObject)
		self.currentObject = ""
		return msg
	}
	self.currentObject = ""
	return ""
}

func (self *ExcludeObject) Exclude(name string) {
	if !self.ObjectIsExcluded(name) {
		self.excludedObjects = append(self.excludedObjects, name)
		sort.Strings(self.excludedObjects)
	}
}

func (self *ExcludeObject) Unexclude(name string) {
	for i, obj := range self.excludedObjects {
		if obj == name {
			self.excludedObjects = append(self.excludedObjects[:i], self.excludedObjects[i+1:]...)
			sort.Strings(self.excludedObjects)
			break
		}
	}
}

func (self *ExcludeObject) ClearExcluded() {
	self.excludedObjects = []string{}
}

func (self *ExcludeObject) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"objects":          self.objects,
		"excluded_objects": self.excludedObjects,
		"current_object":   self.currentObject,
	}
}