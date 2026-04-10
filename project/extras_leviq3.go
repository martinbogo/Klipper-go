package project

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"goklipper/common/logger"
	"goklipper/common/utils/object"
	addonpkg "goklipper/internal/addon"
	gcodepkg "goklipper/internal/pkg/gcode"
	heaterpkg "goklipper/internal/pkg/heater"
	bedmeshpkg "goklipper/internal/pkg/motion/bed_mesh"
	kinematicspkg "goklipper/internal/pkg/motion/kinematics"
	printpkg "goklipper/internal/print"
)

const (
	leviq3StateVariable        = "leviq3_state"
	defaultLeviQ3ProfileName   = "leviq3"
	defaultLeviQ3TravelSpeed   = 50.0
	defaultLeviQ3VerticalSpeed = 10.0
	defaultLeviQ3SafeTravelZ   = 5.0
	cmdLeviQ3Help              = "Run LeviQ3 preheat, wipe, probe, and build-plate Z-offset recovery"
	cmdLeviQ3PreheatingHelp    = "Preheat the bed for LeviQ3 and re-check if temperature drops below target-5"
	cmdLeviQ3WipingHelp        = "Run the LeviQ3 wipe sequence"
	cmdLeviQ3ProbeHelp         = "Home and probe the LeviQ3 bed mesh"
	cmdLeviQ3AutoZOffsetHelp   = "Run LeviQ3 auto Z-offset recovery"
	cmdLeviQ3TempOffsetHelp    = "Reset LeviQ3 temperature compensation state"
	cmdLeviQ3AutoZOnOffHelp    = "Enable or disable LeviQ3 auto Z-offset"
	cmdLeviQ3SetZOffsetHelp    = "Set the LeviQ3 build-plate Z offset"
	cmdLeviQ3HelpHelp          = "Show LeviQ3 command help"
	cmdLeviQ3ScratchDebugHelp  = "Trigger the recovered LeviQ3 scratch/debug notice path"
)

type LeviQ3Module struct {
	printer      *Printer
	config       *ConfigWrapper
	profileName  string
	helper       *printpkg.LeviQ3Helper
	runtime      *leviq3Runtime
	status       *leviq3StatusSink
	gcode        *GCodeDispatch
	gcodeMove    *gcodepkg.GCodeMoveModule
	toolhead     *Toolhead
	probe        *PrinterProbe
	bedMesh      *BedMesh
	heaters      *heaterpkg.PrinterHeaters
	saveVars     *addonpkg.SaveVariablesModule
	stateLoaded  bool
	commandMutex sync.Mutex
	commandOpen  bool
	cancelled    bool
	cancelReason string
}

type leviq3ConfigSource struct {
	config *ConfigWrapper
}

func (self *leviq3ConfigSource) hasOption(key string) bool {
	return self != nil && self.config != nil && self.config.Fileconfig().Has_option(self.config.Get_name(), key)
}

func (self *leviq3ConfigSource) Float64(key string, fallback float64) float64 {
	if !self.hasOption(key) {
		return fallback
	}
	return self.config.Getfloat(key, fallback, 0, 0, 0, 0, true)
}

func (self *leviq3ConfigSource) Int(key string, fallback int) int {
	if !self.hasOption(key) {
		return fallback
	}
	return self.config.Getint(key, fallback, 0, 0, true)
}

func (self *leviq3ConfigSource) Bool(key string, fallback bool) bool {
	if !self.hasOption(key) {
		return fallback
	}
	return self.config.Getboolean(key, fallback, true)
}

func (self *leviq3ConfigSource) Float64Slice(key string, fallback []float64) []float64 {
	if !self.hasOption(key) {
		return append([]float64(nil), fallback...)
	}
	return append([]float64(nil), self.config.Getfloatlist(key, fallback, ",", 0, true)...)
}

func (self *leviq3ConfigSource) String(key string, fallback string) string {
	if !self.hasOption(key) {
		return fallback
	}
	value, _ := self.config.Get(key, fallback, true).(string)
	return value
}

type leviq3StatusSink struct {
	module *LeviQ3Module
}

func (self *leviq3StatusSink) Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	logger.Infof(msg)
	if self != nil && self.module != nil && self.module.shouldEmitStatus() && self.module.gcode != nil {
		self.module.gcode.Respond_info(msg, true)
	}
}

func (self *leviq3StatusSink) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	logger.Errorf(msg)
	if self != nil && self.module != nil && self.module.shouldEmitStatus() && self.module.gcode != nil {
		self.module.gcode.Respond_info(msg, true)
	}
}

type leviq3Runtime struct {
	module *LeviQ3Module
}

func NewLeviQ3Module(config *ConfigWrapper) *LeviQ3Module {
	self := &LeviQ3Module{
		printer:     config.Get_printer(),
		config:      config,
		profileName: defaultLeviQ3ProfileName,
	}
	if config.Fileconfig().Has_option(config.Get_name(), "profile_name") {
		if profileName, ok := config.Get("profile_name", defaultLeviQ3ProfileName, true).(string); ok {
			trimmed := strings.TrimSpace(profileName)
			if trimmed != "" {
				self.profileName = trimmed
			}
		}
	}
	self.runtime = &leviq3Runtime{module: self}
	self.status = &leviq3StatusSink{module: self}
	helper, err := printpkg.NewLeviQ3Helper(&leviq3ConfigSource{config: config}, self.runtime, self.status)
	if err != nil {
		panic(err)
	}
	helper.SetHomingRetryCount(0)
	self.helper = helper
	self.gcode = MustLookupGcode(self.printer)
	self.registerCommands()
	self.printer.Register_event_handler("project:connect", self.handleConnect)
	self.printer.Register_event_handler("project:ready", self.handleReady)
	self.printer.Register_event_handler("project:shutdown", self.handleShutdown)
	self.printer.Register_event_handler("project:disconnect", self.handleDisconnect)
	self.printer.Register_event_handler("project:pre_cancel", self.handlePreCancel)
	if webhooksObj := self.printer.Lookup_object("webhooks", nil); webhooksObj != nil {
		if webhooks, ok := webhooksObj.(*WebHooks); ok {
			_ = webhooks.Register_endpoint("leviq3/cancel", self.handleCancelRequest)
		}
	}
	return self
}

func Load_config_LeviQ3(config *ConfigWrapper) interface{} {
	return NewLeviQ3Module(config)
}

func (self *LeviQ3Module) registerCommands() {
	self.gcode.Register_command("LEVIQ3", self.cmdLEVIQ3, false, cmdLeviQ3Help)
	self.gcode.Register_command("LEVIQ3_PREHEATING", self.cmdLEVIQ3_PREHEATING, false, cmdLeviQ3PreheatingHelp)
	self.gcode.Register_command("LEVIQ3_WIPING", self.cmdLEVIQ3_WIPING, false, cmdLeviQ3WipingHelp)
	self.gcode.Register_command("LEVIQ3_PROBE", self.cmdLEVIQ3_PROBE, false, cmdLeviQ3ProbeHelp)
	self.gcode.Register_command("LEVIQ3_AUTO_ZOFFSET", self.cmdLEVIQ3_AUTO_ZOFFSET, false, cmdLeviQ3AutoZOffsetHelp)
	self.gcode.Register_command("LEVIQ3_AUTO_ZOFFSET_ON_OFF", self.cmdLEVIQ3_AUTO_ZOFFSET_ON_OFF, false, cmdLeviQ3AutoZOnOffHelp)
	self.gcode.Register_command("LEVIQ3_TEMP_OFFSET", self.cmdLEVIQ3_TEMP_OFFSET, false, cmdLeviQ3TempOffsetHelp)
	self.gcode.Register_command("LEVIQ3_SET_ZOFFSET", self.cmdLEVIQ3_SET_ZOFFSET, false, cmdLeviQ3SetZOffsetHelp)
	self.gcode.Register_command("LEVIQ3_HELP", self.cmdLEVIQ3_HELP, true, cmdLeviQ3HelpHelp)
	self.gcode.Register_command("G9113", self.cmdG9113, false, cmdLeviQ3ScratchDebugHelp)
}

func (self *LeviQ3Module) handleConnect([]interface{}) error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	if self.stateLoaded {
		return nil
	}
	if err := self.loadPersistentState(); err != nil {
		return err
	}
	self.stateLoaded = true
	return nil
}

func (self *LeviQ3Module) handleReady([]interface{}) error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	return self.applyRuntimeZOffset(self.helper.CurrentZOffset())
}

func (self *LeviQ3Module) handleShutdown([]interface{}) error {
	self.requestCancel("leviq3 cancelled by printer shutdown")
	return nil
}

func (self *LeviQ3Module) handleDisconnect([]interface{}) error {
	self.requestCancel("leviq3 cancelled by printer disconnect")
	return nil
}

func (self *LeviQ3Module) handlePreCancel([]interface{}) error {
	self.requestCancel("leviq3 cancelled by CANCEL_PRINT")
	return nil
}

func (self *LeviQ3Module) handleCancelRequest(_ *WebRequest) (interface{}, error) {
	self.requestCancel("leviq3 cancelled by webhook request")
	return map[string]interface{}{"status": "cancelled"}, nil
}

func (self *LeviQ3Module) refreshRuntimeObjects() error {
	self.gcode = MustLookupGcode(self.printer)
	self.toolhead = MustLookupToolhead(self.printer)
	self.gcodeMove = MustLookupGCodeMove(self.printer)
	heaters, ok := self.printer.Lookup_object("heaters", object.Sentinel{}).(*heaterpkg.PrinterHeaters)
	if !ok {
		panic(fmt.Errorf("lookup object %s type invalid: %#v", "heaters", self.printer.Lookup_object("heaters", object.Sentinel{})))
	}
	self.heaters = heaters
	probeObj := self.printer.Lookup_object("probe", nil)
	if probeObj == nil {
		return fmt.Errorf("LEVIQ3 requires [probe] to be configured")
	}
	probe, ok := probeObj.(*PrinterProbe)
	if !ok || probe == nil {
		return fmt.Errorf("LEVIQ3 requires a native PrinterProbe, got %T", probeObj)
	}
	self.probe = probe
	bedMeshObj := self.printer.Lookup_object("bed_mesh", nil)
	if bedMeshObj == nil {
		return fmt.Errorf("LEVIQ3 requires [bed_mesh] to be configured")
	}
	bedMesh, ok := bedMeshObj.(*BedMesh)
	if !ok || bedMesh == nil {
		return fmt.Errorf("LEVIQ3 requires a native BedMesh, got %T", bedMeshObj)
	}
	self.bedMesh = bedMesh
	if self.printer.Lookup_object("heater_bed", nil) == nil {
		return fmt.Errorf("LEVIQ3 requires [heater_bed] to be configured")
	}
	if saveObj := self.printer.Lookup_object("save_variables", nil); saveObj != nil {
		if saveVars, ok := saveObj.(*addonpkg.SaveVariablesModule); ok {
			self.saveVars = saveVars
		}
	}
	return nil
}

func (self *LeviQ3Module) buildHelpText() string {
	return strings.Join([]string{
		"LEVIQ3 runs full preheat, wipe, probe, and build-plate Z-offset recovery.",
		"LEVIQ3_PREHEATING waits for the bed target and waits again if temperature falls below target-5.",
		"LEVIQ3_WIPING executes the configured wipe sequence with cancellation checks.",
		"LEVIQ3_PROBE homes, probes the bed, and applies a native bed_mesh profile.",
		"LEVIQ3_AUTO_ZOFFSET validates and applies the recovered auto Z-offset logic.",
		"LEVIQ3_SET_ZOFFSET sets the current build-plate Z offset.",
		"LEVIQ3_AUTO_ZOFFSET_ON_OFF toggles automatic build-plate Z offset application.",
		"LEVIQ3_TEMP_OFFSET clears recovered temperature-compensation state.",
		"G9113 triggers the recovered scratch/debug notice path.",
	}, "\n")
}

func (self *LeviQ3Module) beginCommand() error {
	if err := self.refreshRuntimeObjects(); err != nil {
		return err
	}
	self.helper.ResetCancelState()
	self.commandMutex.Lock()
	self.commandOpen = true
	self.cancelled = false
	self.cancelReason = ""
	self.commandMutex.Unlock()
	return nil
}

func (self *LeviQ3Module) finishCommand() {
	self.commandMutex.Lock()
	self.commandOpen = false
	self.commandMutex.Unlock()
}

func (self *LeviQ3Module) shouldEmitStatus() bool {
	self.commandMutex.Lock()
	defer self.commandMutex.Unlock()
	return self.commandOpen
}

func (self *LeviQ3Module) noteCancellation(reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "leviq3 cancelled"
	}
	self.commandMutex.Lock()
	self.cancelled = true
	self.cancelReason = reason
	self.commandMutex.Unlock()
}

func (self *LeviQ3Module) requestCancel(reason string) {
	self.noteCancellation(reason)
	if self.helper != nil {
		self.helper.CancelEvent(reason)
	}
}

func (self *LeviQ3Module) cancelledState() (bool, string) {
	self.commandMutex.Lock()
	defer self.commandMutex.Unlock()
	return self.cancelled, self.cancelReason
}

func (self *LeviQ3Module) ensureNotCancelled(stage string) error {
	if self.printer.Is_shutdown() {
		return fmt.Errorf("%s: printer shutdown", stage)
	}
	if cancelled, reason := self.cancelledState(); cancelled {
		if stage == "" {
			return fmt.Errorf("%s", reason)
		}
		return fmt.Errorf("%s: %s", stage, reason)
	}
	return nil
}

func (self *LeviQ3Module) runLeviQ3Command(fn func(context.Context) error) error {
	if err := self.beginCommand(); err != nil {
		return err
	}
	defer self.finishCommand()
	if err := fn(context.Background()); err != nil {
		return err
	}
	return self.persistState()
}

func (self *LeviQ3Module) loadPersistentState() error {
	if self.saveVars == nil || self.helper == nil {
		return nil
	}
	raw := self.saveVars.Variables()[leviq3StateVariable]
	if raw == nil {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return self.helper.RestorePersistentStateFromJSON(payload)
}

func (self *LeviQ3Module) persistState() error {
	if self.saveVars == nil || self.gcode == nil || self.helper == nil {
		return nil
	}
	payload, err := self.helper.PersistentStateJSON()
	if err != nil {
		return err
	}
	self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=%s VALUE=%s", leviq3StateVariable, string(payload)))
	return nil
}

func (self *LeviQ3Module) applyRecoveredMesh(recovered *printpkg.BedMesh) error {
	if recovered == nil || self.bedMesh == nil {
		return nil
	}
	zMesh := bedmeshpkg.NewZMesh(self.helper.MeshBuildParams())
	if err := self.bedMesh.Pmgr.Build_mesh_catch(zMesh, self.helper.Get_probed_matrix()); err != nil {
		return err
	}
	self.bedMesh.Set_mesh(zMesh)
	if self.profileName != "" {
		return self.bedMesh.Save_profile(self.profileName)
	}
	return nil
}

func (self *LeviQ3Module) applyRuntimeZOffset(z float64) error {
	if self.gcode == nil || self.gcodeMove == nil {
		return nil
	}
	// CurrentZOffset is the nozzle/build-plate runtime offset. In the native
	// repository model that belongs to gcode_move's homing origin, while the
	// probe's physical z_offset and the Z rail endstop remain native calibration
	// values.
	value := strconv.FormatFloat(z, 'f', -1, 64)
	command := self.gcode.Create_gcode_command(
		"SET_GCODE_OFFSET",
		fmt.Sprintf("SET_GCODE_OFFSET Z=%s", value),
		map[string]string{"Z": value},
	)
	self.gcodeMove.Cmd_SET_GCODE_OFFSET(command)
	self.gcodeMove.ResetLastPosition()
	return nil
}

func (self *LeviQ3Module) currentKinematicsRails() []*PrinterRail {
	if self.toolhead == nil {
		return nil
	}
	switch kin := self.toolhead.Get_kinematics().(type) {
	case *CartKinematics:
		return append([]*PrinterRail(nil), kin.rails...)
	case *CorexyKinematics:
		return append([]*PrinterRail(nil), kin.rails...)
	default:
		return nil
	}
}

func leviqAxisIndex(axis printpkg.Axis) int {
	return kinematicspkg.AxisIndex(string(axis))
}

func leviqAxisIndexes(axes []printpkg.Axis) []int {
	strs := make([]string, len(axes))
	for i, a := range axes {
		strs[i] = string(a)
	}
	return kinematicspkg.UniqueAxisIndexes(strs)
}

type leviq3RailHomingSnapshot struct {
	rail               *PrinterRail
	homingSpeed        float64
	secondHomingSpeed  float64
	homingRetractDist  float64
	homingRetractSpeed float64
	homingPositiveDir  bool
}

func (self *LeviQ3Module) overrideRailHomingParams(axisIndexes []int, params printpkg.HomingParams) func() {
	rails := self.currentKinematicsRails()
	snapshots := make([]leviq3RailHomingSnapshot, 0, len(axisIndexes))
	for _, axisIndex := range axisIndexes {
		if axisIndex < 0 || axisIndex >= len(rails) || rails[axisIndex] == nil {
			continue
		}
		rail := rails[axisIndex]
		snapshots = append(snapshots, leviq3RailHomingSnapshot{
			rail:               rail,
			homingSpeed:        rail.homing_speed,
			secondHomingSpeed:  rail.second_homing_speed,
			homingRetractDist:  rail.homing_retract_dist,
			homingRetractSpeed: rail.homing_retract_speed,
			homingPositiveDir:  rail.homing_positive_dir,
		})
		if params.SecondHomingSpeed() > 0 {
			rail.second_homing_speed = params.SecondHomingSpeed()
		}
		if params.HomingRetractDist() > 0 {
			rail.homing_retract_dist = params.HomingRetractDist()
		}
		if params.HomingRetractSpeed() > 0 {
			rail.homing_retract_speed = params.HomingRetractSpeed()
		}
		if params.HomingSpeed() > 0 {
			rail.homing_speed = params.HomingSpeed()
		}
		rail.homing_positive_dir = params.HomingPositiveDir()
	}
	return func() {
		for _, snapshot := range snapshots {
			snapshot.rail.homing_speed = snapshot.homingSpeed
			snapshot.rail.second_homing_speed = snapshot.secondHomingSpeed
			snapshot.rail.homing_retract_dist = snapshot.homingRetractDist
			snapshot.rail.homing_retract_speed = snapshot.homingRetractSpeed
			snapshot.rail.homing_positive_dir = snapshot.homingPositiveDir
		}
	}
}

func parseEnableState(gcmd *GCodeCommand, defaultValue bool) (bool, bool) {
	if gcmd == nil {
		return defaultValue, false
	}
	if gcmd.Has("ENABLE") {
		return gcmd.Get_int("ENABLE", 1, nil, nil) != 0, true
	}
	state := strings.TrimSpace(strings.ToLower(gcmd.Get("STATE", "", nil, nil, nil, nil, nil)))
	if state == "" {
		state = strings.TrimSpace(strings.ToLower(gcmd.Get("MODE", "", nil, nil, nil, nil, nil)))
	}
	if val, ok := gcodepkg.ParseBooleanString(state); ok {
		return val, true
	}
	return defaultValue, false
}

func (self *LeviQ3Module) cmdLEVIQ3(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		mesh, err := self.helper.CMD_LEVIQ3(ctx)
		if err != nil {
			return err
		}
		return self.applyRecoveredMesh(mesh)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_PREHEATING(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	mode := "enable"
	if enabled, specified := parseEnableState(gcmd, true); specified && !enabled {
		mode = "disable"
	}
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.CMD_LEVIQ3_PREHEATING(ctx, mode)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_WIPING(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.CMD_LEVIQ3_WIPING(ctx)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_PROBE(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		mesh, err := self.helper.CMD_LEVIQ3_PROBE(ctx)
		if err != nil {
			return err
		}
		return self.applyRecoveredMesh(mesh)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_AUTO_ZOFFSET(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.CMD_LEVIQ3_auto_zoffset(ctx)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_AUTO_ZOFFSET_ON_OFF(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	enabled, _ := parseEnableState(gcmd, true)
	return self.runLeviQ3Command(func(ctx context.Context) error {
		self.helper.CMD_LEVIQ3_auto_zoffset_ON_OFF(enabled)
		return nil
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_TEMP_OFFSET(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.CMD_LEVIQ3_TEMP_OFFSET(ctx)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_SET_ZOFFSET(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	z := gcmd.Get_float("Z", self.helper.CurrentZOffset(), nil, nil, nil, nil)
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.LEVIQ3_set_zoffset(ctx, z)
	})
}

func (self *LeviQ3Module) cmdLEVIQ3_HELP(arg interface{}) error {
	gcmd := arg.(*GCodeCommand)
	gcmd.Respond_info(self.buildHelpText(), true)
	return nil
}

func (self *LeviQ3Module) cmdG9113(arg interface{}) error {
	return self.runLeviQ3Command(func(ctx context.Context) error {
		return self.helper.CMD_G9113(ctx)
	})
}

func (self *leviq3Runtime) moduleReady() error {
	if self == nil || self.module == nil {
		return fmt.Errorf("leviq3 runtime unavailable")
	}
	return self.module.refreshRuntimeObjects()
}

func (self *leviq3Runtime) Is_printer_ready(ctx context.Context) bool {
	if err := self.moduleReady(); err != nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return self.module.gcode != nil && self.module.gcode.Is_printer_ready && !self.module.printer.Is_shutdown()
}

func (self *leviq3Runtime) Clear_homing_state(ctx context.Context) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if err := self.module.ensureNotCancelled("clear homing state"); err != nil {
		return err
	}
	if kin, ok := self.module.toolhead.Get_kinematics().(interface{ Note_z_not_homed() }); ok {
		kin.Note_z_not_homed()
	}
	return nil
}

func (self *leviq3Runtime) Home_axis(ctx context.Context, axis printpkg.Axis, params printpkg.HomingParams) error {
	return self.Home_rails(ctx, []printpkg.Axis{axis}, params)
}

func (self *leviq3Runtime) Home_rails(ctx context.Context, axes []printpkg.Axis, params printpkg.HomingParams) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("home rails"); err != nil {
		return err
	}
	homing := NewHoming(self.module.printer)
	axisIndexes := leviqAxisIndexes(axes)
	homing.Set_axes(axisIndexes)
	restore := self.module.overrideRailHomingParams(axisIndexes, params)
	defer restore()
	kin, ok := self.module.toolhead.Get_kinematics().(IKinematics)
	if !ok {
		return fmt.Errorf("unsupported kinematics runtime %T", self.module.toolhead.Get_kinematics())
	}
	kin.Home(homing)
	return nil
}

func (self *leviq3Runtime) Homing_move(ctx context.Context, axis printpkg.Axis, target float64, speed float64) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("homing move"); err != nil {
		return err
	}
	coord := []interface{}{nil, nil, nil, nil}
	index := leviqAxisIndex(axis)
	if index < 0 {
		return fmt.Errorf("unsupported axis %q", axis)
	}
	coord[index] = target
	if speed <= 0 {
		speed = defaultLeviQ3TravelSpeed
	}
	self.module.toolhead.Manual_move(coord, speed)
	self.module.toolhead.Wait_moves()
	return nil
}

func (self *leviq3Runtime) bedHeater() (*heaterpkg.Heater, error) {
	if err := self.moduleReady(); err != nil {
		return nil, err
	}
	return self.module.heaters.Lookup_heater("heater_bed"), nil
}

func (self *leviq3Runtime) Set_bed_temperature(ctx context.Context, target float64) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("set bed temperature"); err != nil {
		return err
	}
	bed, err := self.bedHeater()
	if err != nil {
		return err
	}
	return self.module.heaters.Set_temperature(bed, target, false)
}

func (self *leviq3Runtime) Wait_for_temperature(ctx context.Context, target float64, timeout time.Duration) error {
	bed, err := self.bedHeater()
	if err != nil {
		return err
	}
	reactor := self.module.printer.Get_reactor()
	deadline := reactor.Monotonic() + timeout.Seconds()
	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := self.module.ensureNotCancelled("wait for temperature"); err != nil {
			return err
		}
		now := reactor.Monotonic()
		current, _ := bed.Get_temp(now)
		if current >= target {
			return nil
		}
		if timeout > 0 && now >= deadline {
			return fmt.Errorf("bed temperature wait timed out at %.2f/%.2f", current, target)
		}
		wake := now + 0.25
		if timeout > 0 && wake > deadline {
			wake = deadline
		}
		reactor.Pause(wake)
	}
}

func (self *leviq3Runtime) GetHotbedTemp(ctx context.Context) (float64, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err := self.module.ensureNotCancelled("get hotbed temperature"); err != nil {
		return 0, err
	}
	bed, err := self.bedHeater()
	if err != nil {
		return 0, err
	}
	current, _ := bed.Get_temp(self.module.printer.Get_reactor().Monotonic())
	return current, nil
}

func (self *leviq3Runtime) Lower_probe(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("lower probe")
}

func (self *leviq3Runtime) Raise_probe(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("raise probe")
}

func (self *leviq3Runtime) safeTravelZ() float64 {
	safeZ := defaultLeviQ3SafeTravelZ
	if self.module.bedMesh != nil && self.module.bedMesh.Horizontal_move_z > safeZ {
		safeZ = self.module.bedMesh.Horizontal_move_z
	}
	if self.module.toolhead != nil {
		current := self.module.toolhead.Get_position()
		if len(current) > 2 && current[2] > safeZ {
			safeZ = current[2]
		}
	}
	return safeZ
}

func (self *leviq3Runtime) moveNozzle(coord []interface{}, speed float64) error {
	if err := self.module.ensureNotCancelled("move nozzle"); err != nil {
		return err
	}
	self.module.toolhead.Manual_move(coord, speed)
	self.module.toolhead.Wait_moves()
	return nil
}

func (self *leviq3Runtime) Run_probe(ctx context.Context, position printpkg.XY) (float64, error) {
	if err := self.moduleReady(); err != nil {
		return 0, err
	}
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if err := self.module.ensureNotCancelled("run probe"); err != nil {
		return 0, err
	}
	xOffset, yOffset, _ := self.module.probe.Get_offsets()
	if err := self.moveNozzle([]interface{}{nil, nil, self.safeTravelZ(), nil}, defaultLeviQ3VerticalSpeed); err != nil {
		return 0, err
	}
	if err := self.moveNozzle([]interface{}{position.X - xOffset, position.Y - yOffset, nil, nil}, defaultLeviQ3TravelSpeed); err != nil {
		return 0, err
	}
	result := self.module.probe.Run_probe(self.module.gcode.Create_gcode_command("PROBE", "PROBE", map[string]string{}))
	if len(result) < 3 {
		return 0, fmt.Errorf("probe returned invalid result %#v", result)
	}
	return result[2], nil
}

func (self *leviq3Runtime) Wipe_nozzle(ctx context.Context, position printpkg.XYZ) error {
	if err := self.moduleReady(); err != nil {
		return err
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("wipe nozzle"); err != nil {
		return err
	}
	current := self.module.toolhead.Get_position()
	safeZ := math.Max(self.safeTravelZ(), position.Z)
	if len(current) > 2 && current[2] < safeZ {
		if err := self.moveNozzle([]interface{}{nil, nil, safeZ, nil}, defaultLeviQ3VerticalSpeed); err != nil {
			return err
		}
	}
	if err := self.moveNozzle([]interface{}{position.X, position.Y, nil, nil}, defaultLeviQ3TravelSpeed); err != nil {
		return err
	}
	if err := self.moveNozzle([]interface{}{nil, nil, position.Z, nil}, defaultLeviQ3VerticalSpeed); err != nil {
		return err
	}
	return nil
}

func (self *leviq3Runtime) Set_gcode_offset(ctx context.Context, z float64) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("set gcode offset"); err != nil {
		return err
	}
	return self.module.applyRuntimeZOffset(z)
}

func (self *leviq3Runtime) Current_z_offset(ctx context.Context) float64 {
	if ctx != nil && ctx.Err() != nil {
		return self.module.helper.CurrentZOffset()
	}
	if self.module.gcodeMove == nil {
		return self.module.helper.CurrentZOffset()
	}
	status := self.module.gcodeMove.Get_status(0)
	homingOrigin, ok := status["homing_origin"].([]float64)
	if !ok || len(homingOrigin) < 3 {
		return self.module.helper.CurrentZOffset()
	}
	return homingOrigin[2]
}

func (self *leviq3Runtime) Save_mesh(ctx context.Context, mesh *printpkg.BedMesh) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("save mesh"); err != nil {
		return err
	}
	return nil
}

func (self *leviq3Runtime) Sleep(ctx context.Context, d time.Duration) error {
	reactor := self.module.printer.Get_reactor()
	deadline := reactor.Monotonic() + d.Seconds()
	for {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := self.module.ensureNotCancelled("sleep"); err != nil {
			return err
		}
		now := reactor.Monotonic()
		if now >= deadline {
			return nil
		}
		wake := now + 0.1
		if wake > deadline {
			wake = deadline
		}
		reactor.Pause(wake)
	}
}

func (self *leviq3Runtime) GetAutoZOffsetTemperature(ctx context.Context) (float64, error) {
	return self.GetHotbedTemp(ctx)
}

func (self *leviq3Runtime) Z_offset_apply_probe(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync probe offset")
}

func (self *leviq3Runtime) Z_offset_apply_probe_absolute(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync absolute probe offset")
}

func (self *leviq3Runtime) Set_mesh_offsets(ctx context.Context, offsets printpkg.XYZ) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if err := self.module.ensureNotCancelled("sync mesh offsets"); err != nil {
		return err
	}
	if self.module.bedMesh != nil && self.module.bedMesh.Get_mesh() != nil {
		self.module.bedMesh.Get_mesh().Set_mesh_offsets([]float64{offsets.X, offsets.Y})
	}
	return nil
}

func (self *leviq3Runtime) Set_rails_z_offset(ctx context.Context, z float64) error {
	_ = z
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return self.module.ensureNotCancelled("sync rail z offset")
}

func (self *leviq3Runtime) CancelLeviQ3(ctx context.Context, reason string) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	self.module.noteCancellation(reason)
	return nil
}
