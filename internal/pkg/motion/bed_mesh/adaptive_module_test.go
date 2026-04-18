package bedmesh

import (
	"reflect"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeAdaptiveRawConfig struct {
	sections map[string]map[string]interface{}
}

func (self *fakeAdaptiveRawConfig) Has_section(section string) bool {
	_, ok := self.sections[section]
	return ok
}

func (self *fakeAdaptiveRawConfig) Has_option(section string, option string) bool {
	sectionValues, ok := self.sections[section]
	if !ok {
		return false
	}
	_, ok = sectionValues[option]
	return ok
}

type fakeAdaptiveConfig struct {
	name     string
	printer  printerpkg.ModulePrinter
	sections map[string]map[string]interface{}
	raw      *fakeAdaptiveRawConfig
}

type incompleteAdaptiveConfig struct{}

func newFakeAdaptiveConfig(name string, sections map[string]map[string]interface{}) *fakeAdaptiveConfig {
	return &fakeAdaptiveConfig{
		name:     name,
		sections: sections,
		raw:      &fakeAdaptiveRawConfig{sections: sections},
	}
}

func (self *fakeAdaptiveConfig) Name() string { return self.name }
func (self *fakeAdaptiveConfig) String(option string, defaultValue string, noteValid bool) string {
	_ = noteValid
	if value, ok := self.sections[self.name][option]; ok {
		return value.(string)
	}
	return defaultValue
}
func (self *fakeAdaptiveConfig) Bool(option string, defaultValue bool) bool {
	if value, ok := self.sections[self.name][option]; ok {
		return value.(bool)
	}
	return defaultValue
}
func (self *fakeAdaptiveConfig) Float(option string, defaultValue float64) float64 {
	if value, ok := self.sections[self.name][option]; ok {
		return value.(float64)
	}
	return defaultValue
}
func (self *fakeAdaptiveConfig) OptionalFloat(option string) *float64 {
	if value, ok := self.sections[self.name][option]; ok {
		floatValue := value.(float64)
		return &floatValue
	}
	return nil
}
func (self *fakeAdaptiveConfig) LoadObject(section string) interface{} { return nil }
func (self *fakeAdaptiveConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	panic("unexpected LoadTemplate call")
}
func (self *fakeAdaptiveConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	panic("unexpected LoadRequiredTemplate call")
}
func (self *fakeAdaptiveConfig) Printer() printerpkg.ModulePrinter  { return self.printer }
func (self *fakeAdaptiveConfig) Fileconfig() *fakeAdaptiveRawConfig { return self.raw }
func (self *fakeAdaptiveConfig) Get_name() string                   { return self.name }
func (self *fakeAdaptiveConfig) Getsection(section string) *fakeAdaptiveConfig {
	return &fakeAdaptiveConfig{name: section, printer: self.printer, sections: self.sections, raw: self.raw}
}
func (self *fakeAdaptiveConfig) Get(option string, defaultValue interface{}, noteValid bool) interface{} {
	_ = noteValid
	if value, ok := self.sections[self.name][option]; ok {
		return value
	}
	return defaultValue
}
func (self *fakeAdaptiveConfig) Getfloatlist(option string, defaultValue interface{}, sep string, count int, noteValid bool) []float64 {
	_, _, _ = sep, count, noteValid
	if value, ok := self.sections[self.name][option]; ok {
		return append([]float64(nil), value.([]float64)...)
	}
	if defaultValue == nil {
		return nil
	}
	return append([]float64(nil), defaultValue.([]float64)...)
}
func (self *fakeAdaptiveConfig) Getfloat(option string, defaultValue interface{}, minval, maxval, above, below float64, noteValid bool) float64 {
	_, _, _, _, _, _ = minval, maxval, above, below, noteValid, option
	if value, ok := self.sections[self.name][option]; ok {
		return value.(float64)
	}
	if defaultValue == nil {
		return 0
	}
	return defaultValue.(float64)
}
func (self *fakeAdaptiveConfig) Getint(option string, defaultValue interface{}, minval, maxval int, noteValid bool) int {
	_, _, _, _ = minval, maxval, noteValid, option
	if value, ok := self.sections[self.name][option]; ok {
		return value.(int)
	}
	if defaultValue == nil {
		return 0
	}
	return defaultValue.(int)
}
func (self *fakeAdaptiveConfig) Getboolean(option string, defaultValue interface{}, noteValid bool) bool {
	_, _ = noteValid, option
	if value, ok := self.sections[self.name][option]; ok {
		return value.(bool)
	}
	if defaultValue == nil {
		return false
	}
	return defaultValue.(bool)
}

func (incompleteAdaptiveConfig) Name() string { return "adaptive_bed_mesh" }
func (incompleteAdaptiveConfig) String(option string, defaultValue string, noteValid bool) string {
	_ = option
	_ = noteValid
	return defaultValue
}
func (incompleteAdaptiveConfig) Bool(option string, defaultValue bool) bool {
	_ = option
	return defaultValue
}
func (incompleteAdaptiveConfig) Float(option string, defaultValue float64) float64 {
	_ = option
	return defaultValue
}
func (incompleteAdaptiveConfig) OptionalFloat(option string) *float64 {
	_ = option
	return nil
}
func (incompleteAdaptiveConfig) LoadObject(section string) interface{} {
	_ = section
	return nil
}
func (incompleteAdaptiveConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	panic("unexpected LoadTemplate call")
}
func (incompleteAdaptiveConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	panic("unexpected LoadRequiredTemplate call")
}
func (incompleteAdaptiveConfig) Printer() printerpkg.ModulePrinter { return nil }

func TestReflectiveAdaptiveModuleConfigSourceReadsProjectCompatibleSections(t *testing.T) {
	config := newFakeAdaptiveConfig("adaptive_bed_mesh", map[string]map[string]interface{}{
		"adaptive_bed_mesh": {
			"arc_segments": 96,
			"disable_slicer_min_max_boundary_detection": true,
			"mesh_area_clearance":                       7.5,
			"max_probe_horizontal_distance":             30.0,
			"max_probe_vertical_distance":               35.0,
			"use_relative_reference_index":              true,
		},
		"bed_mesh": {
			"mesh_min":  []float64{10, 20},
			"mesh_max":  []float64{210, 220},
			"fade_end":  5.5,
			"algorithm": " bicubic ",
		},
		"virtual_sdcard": {
			"path": "~/gcodes",
		},
	})

	source, err := newReflectiveAdaptiveModuleConfigSource(config)
	if err != nil {
		t.Fatalf("newReflectiveAdaptiveModuleConfigSource() error = %v", err)
	}
	cfg, err := BuildAdaptiveCalibrationModuleConfig(source)
	if err != nil {
		t.Fatalf("BuildAdaptiveCalibrationModuleConfig() error = %v", err)
	}

	if cfg.DefaultMin != (Vec2{X: 10, Y: 20}) {
		t.Fatalf("DefaultMin = %+v", cfg.DefaultMin)
	}
	if cfg.DefaultMax != (Vec2{X: 210, Y: 220}) {
		t.Fatalf("DefaultMax = %+v", cfg.DefaultMax)
	}
	if cfg.FadeEnd != 5.5 {
		t.Fatalf("FadeEnd = %v, want 5.5", cfg.FadeEnd)
	}
	if cfg.Algorithm != "bicubic" {
		t.Fatalf("Algorithm = %q, want bicubic", cfg.Algorithm)
	}
	if cfg.ArcSegments != 96 {
		t.Fatalf("ArcSegments = %d, want 96", cfg.ArcSegments)
	}
	if !cfg.DisableSlicerBoundary {
		t.Fatal("DisableSlicerBoundary = false, want true")
	}
	if cfg.Margin != 7.5 {
		t.Fatalf("Margin = %v, want 7.5", cfg.Margin)
	}
	if cfg.MaxHDist != 30.0 || cfg.MaxVDist != 35.0 {
		t.Fatalf("unexpected probe distances: h=%v v=%v", cfg.MaxHDist, cfg.MaxVDist)
	}
	if !cfg.IncludeRelativeReferenceIndex {
		t.Fatal("IncludeRelativeReferenceIndex = false, want true")
	}
	if cfg.VirtualSDPath == "~/gcodes" {
		t.Fatal("VirtualSDPath was not normalized")
	}
}

func TestReflectiveAdaptiveModuleConfigSourceRejectsMissingProjectMethods(t *testing.T) {
	_, err := newReflectiveAdaptiveModuleConfigSource(incompleteAdaptiveConfig{})
	if err == nil {
		t.Fatal("expected an error for config without project-compatible section helpers")
	}
}

func TestSetAdaptiveBedMeshZeroReferenceMutatesCompatibleRuntime(t *testing.T) {
	type fakeBedMesh struct {
		Zero_ref_pos []float64
	}

	mesh := &fakeBedMesh{}
	if err := setAdaptiveBedMeshZeroReference(mesh, &Vec2{X: 12.5, Y: 42.25}); err != nil {
		t.Fatalf("setAdaptiveBedMeshZeroReference() error = %v", err)
	}
	if !reflect.DeepEqual(mesh.Zero_ref_pos, []float64{12.5, 42.25}) {
		t.Fatalf("Zero_ref_pos = %#v", mesh.Zero_ref_pos)
	}
	if err := setAdaptiveBedMeshZeroReference(mesh, nil); err != nil {
		t.Fatalf("setAdaptiveBedMeshZeroReference(nil) error = %v", err)
	}
	if mesh.Zero_ref_pos != nil {
		t.Fatalf("Zero_ref_pos = %#v, want nil", mesh.Zero_ref_pos)
	}
}
