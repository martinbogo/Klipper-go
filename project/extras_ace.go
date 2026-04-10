package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"goklipper/common/utils/sys"
	addonpkg "goklipper/internal/addon"
	filamentpkg "goklipper/internal/pkg/filament"
	iopkg "goklipper/internal/pkg/io"
	probepkg "goklipper/internal/pkg/motion/probe"
	printerpkg "goklipper/internal/pkg/printer"
	printpkg "goklipper/internal/print"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	RECONNECT_COUNT = 10
)

func parseRGB(s string) []interface{} {
	var r, g, self float64
	fmt.Sscanf(s, "%f,%f,%f", &r, &g, &self)
	return []interface{}{r, g, self}
}

func normalizeACEFilamentType(requestedType string, fallbackType string) string {
	trimmedType := strings.TrimSpace(requestedType)
	if trimmedType == "" || trimmedType == "?" {
		fallbackType = strings.TrimSpace(fallbackType)
		if fallbackType != "" {
			return fallbackType
		}
	}
	return trimmedType
}

func hasValidACESlotRFID(slot map[string]interface{}) bool {
	sku, _ := slot["sku"].(string)
	sku = strings.TrimSpace(sku)
	return sku != "" && sku != "custom"
}

func buildACEColorList(color []interface{}) []interface{} {
	if len(color) != 3 {
		return nil
	}
	return []interface{}{
		[]interface{}{color[0], color[1], color[2], 255},
	}
}

type aceFilamentSensor interface {
	FilamentPresent() bool
}

type ACE struct {
	printer  *Printer
	reactor  IReactor
	gcode    *GCodeDispatch
	toolhead *Toolhead

	ace_commun *filamentpkg.AceCommun
	ace_dev_fd *ReactorFileHandler
	//timer
	connect_timer  *ReactorTimer
	heatbeat_timer *ReactorTimer

	endstops map[string]interface{}

	// inventory config variables info
	variables map[string]interface{}
	info      map[string]interface{}
	inventory []map[string]interface{}

	//endless spool
	endless_spool_timer           *ReactorTimer
	endless_spool_enabled         bool
	endless_spool_in_progress     bool
	endless_spool_runout_detected bool
	change_tool_in_progress       bool

	custom_slots []map[string]interface{}

	// assist channel index
	feed_assist_index int
	// feed retract length and speed
	feed_speed                       int
	retract_speed                    int
	toolchange_retract_length        int
	toolchange_load_length           int
	toolhead_sensor_to_nozzle_length int

	//dry temperature
	max_dryer_temperature int
	fw_info               map[string]interface{}
	reconneted_count      int
}

func (self *ACE) persistFilamentSlotConfig(index int, slotData map[string]interface{}) {
	cfgPath := "/userdata/app/gk/config/ams_config.cfg"
	configData := map[string]interface{}{}

	if amsBytes, err := os.ReadFile(cfgPath); err == nil {
		if len(amsBytes) > 0 {
			if err := json.Unmarshal(amsBytes, &configData); err != nil {
				logger.Errorf("Failed to unmarshal ams config: %v", err)
				return
			}
		}
	} else if !os.IsNotExist(err) {
		logger.Errorf("Failed to read ams config: %v", err)
		return
	}

	filaments, ok := configData["filaments"].(map[string]interface{})
	if !ok || filaments == nil {
		filaments = make(map[string]interface{})
		configData["filaments"] = filaments
	}

	iStr := strconv.Itoa(index)
	existingSlot, existingExists := filaments[iStr].(map[string]interface{})
	persistedSlot := make(map[string]interface{})
	if existingExists {
		for k, v := range existingSlot {
			persistedSlot[k] = v
		}
	}
	for k, v := range slotData {
		if v != nil {
			persistedSlot[k] = v
		} else {
			delete(persistedSlot, k)
		}
	}

	if _, ok := persistedSlot["icon_type"]; !ok {
		persistedSlot["icon_type"] = 0
	}
	if color, ok := persistedSlot["color"].([]interface{}); ok {
		if colors, ok := persistedSlot["colors"].([]interface{}); !ok || len(colors) == 0 {
			if generatedColors := buildACEColorList(color); generatedColors != nil {
				persistedSlot["colors"] = generatedColors
			}
		}
	}

	newSlotJSON, err := json.Marshal(persistedSlot)
	if err != nil {
		logger.Errorf("Failed to marshal slot config: %v", err)
		return
	}
	if existingExists {
		existingSlotJSON, err := json.Marshal(existingSlot)
		if err == nil && string(existingSlotJSON) == string(newSlotJSON) {
			return
		}
	}

	filaments[iStr] = persistedSlot
	outBytes, err := json.Marshal(configData)
	if err != nil {
		logger.Errorf("Failed to marshal config: %v", err)
		return
	}
	err = os.WriteFile(cfgPath, outBytes, 0644)
	logger.Infof("Persisted ACE slot %d to ams_config.cfg. err=%v", index, err)
}

// TODO: Ensure the logic in this file (and all ACE/filament routing modules) is 100%
// identical to the original decompiled `gklib_remote` binary. This includes porting the
// exact routines for `hub_feed.go`, `hub_unwind.go`, etc., preventing panics, matching
// timing/delays exactly, and aligning completely with the upstream structures.

// TODO: Ensure the logic in this file (and all ACE/filament routing modules) is 100%
// identical to the original decompiled `gklib_remote` binary. This includes porting the
// exact routines for `hub_feed.go`, `hub_unwind.go`, etc., preventing panics, matching
// timing/delays exactly, and aligning completely with the upstream structures.

func NewAce(config *ConfigWrapper) *ACE {
	self := new(ACE)
	self.printer = config.Get_printer()
	self.reactor = self.printer.Get_reactor()
	self.gcode = MustLookupGcode(self.printer)

	serial := config.Get("serial", "/dev/ttyACM0", true).(string)
	if serial == "" {
		matches, _ := filepath.Glob("/dev/ttyACM*")
		if len(matches) > 0 {
			serial = matches[0]
		} else {
			serial = "/dev/ttyACM0"
		}
	}
	baud := config.Getint("v2_baud", 230400, 0, 0, true)
	self.ace_commun = filamentpkg.NewAceCommunication(serial, baud)

	_ = config.Get("switch_pin", "", true).(string)
	//toolhead_sensor_pin := config.Get("toolhead_sensor_pin", "", true).(string)
	self.feed_speed = config.Getint("feed_speed", 50, 0, 0, true)
	self.retract_speed = config.Getint("retract_speed", 50, 0, 0, true)
	self.toolchange_retract_length = config.Getint("toolchange_retract_length", 150, 0, 0, true)
	self.toolchange_load_length = config.Getint("toolchange_load_length", 630, 0, 0, true)
	self.toolhead_sensor_to_nozzle_length = config.Getint("toolhead_sensor_to_nozzle", 0, 0, 0, true)

	self.max_dryer_temperature = config.Getint("max_dryer_temperature", 55, 0, 0, true)

	v_obj := self.printer.Lookup_object("save_variables", object.Sentinel{})
	saved_endless_spool_enabled := false
	if _, ok := v_obj.(object.Sentinel); !ok {
		sv := v_obj.(*addonpkg.SaveVariablesModule)
		if sv != nil {
			self.variables = sv.Variables()
			if val, ok := self.variables["ace_endless_spool_enabled"]; ok {
				if bval, ok := val.(bool); ok {
					saved_endless_spool_enabled = bval
				}
			}
		}
	}
	//# Endless spool configuration - load from persistent variables if available
	self.endless_spool_enabled = config.Getboolean("endless_spool", saved_endless_spool_enabled, true)
	self.endless_spool_in_progress = false
	self.endless_spool_runout_detected = false

	self.feed_assist_index = -1

	self.change_tool_in_progress = false
	self.endstops = map[string]interface{}{}

	self.custom_slots = []map[string]interface{}{
		{"type": "", "color": []interface{}{0.0, 0.0, 0.0}, "colors": []interface{}{[]interface{}{0, 0, 0, 0}}, "sku": "", "rfid": 1, "source": 2, "icon_type": 0},
		{"type": "", "color": []interface{}{0.0, 0.0, 0.0}, "colors": []interface{}{[]interface{}{0, 0, 0, 0}}, "sku": "", "rfid": 1, "source": 2, "icon_type": 0},
		{"type": "", "color": []interface{}{0.0, 0.0, 0.0}, "colors": []interface{}{[]interface{}{0, 0, 0, 0}}, "sku": "", "rfid": 1, "source": 2, "icon_type": 0},
		{"type": "", "color": []interface{}{0.0, 0.0, 0.0}, "colors": []interface{}{[]interface{}{0, 0, 0, 0}}, "sku": "", "rfid": 1, "source": 2, "icon_type": 0},
	}

	if amsBytes, err := os.ReadFile("/userdata/app/gk/config/ams_config.cfg"); err == nil {
		var configData map[string]interface{}
		if err := json.Unmarshal(amsBytes, &configData); err == nil {
			if filaments, ok := configData["filaments"].(map[string]interface{}); ok {
				for iStr, fData := range filaments {
					i, err := strconv.Atoi(iStr)
					if err == nil && i >= 0 && i < len(self.custom_slots) {
						if fMap, ok := fData.(map[string]interface{}); ok {
							if typ, ok := fMap["type"].(string); ok {
								self.custom_slots[i]["type"] = typ
							}
							if color, ok := fMap["color"].([]interface{}); ok {
								self.custom_slots[i]["color"] = color
							}
							if colors, ok := fMap["colors"].([]interface{}); ok {
								self.custom_slots[i]["colors"] = colors
							}
							if sku, ok := fMap["sku"].(string); ok {
								self.custom_slots[i]["sku"] = sku
							}
							if rfid, ok := fMap["rfid"]; ok {
								self.custom_slots[i]["rfid"] = rfid
							}
							if source, ok := fMap["source"]; ok {
								self.custom_slots[i]["source"] = source
							}
							if iconType, ok := fMap["icon_type"]; ok {
								self.custom_slots[i]["icon_type"] = iconType
							}
						}
					}
				}
			}
		}
	}

	//# Default data to prevent exceptions
	self.info = map[string]interface{}{
		"status": "ready",
		"dryer": map[string]interface{}{
			"status":      "stop",
			"target_temp": 0,
			"duration":    0,
			"remain_time": 0,
		},
		"temp":              0,
		"enable_rfid":       1,
		"fan_speed":         7000,
		"feed_assist_count": 0,
		"cont_assist_time":  0.0,
		"slots": []interface{}{
			map[string]interface{}{
				"index":     0,
				"status":    "ready",
				"sku":       "",
				"type":      self.custom_slots[0]["type"],
				"color":     self.custom_slots[0]["color"],
				"colors":    self.custom_slots[0]["colors"],
				"icon_type": 0,
				"remain":    0,
				"decorder":  0,
				"rfid":      1,
				"source":    2,
			},
			map[string]interface{}{
				"index":     1,
				"status":    "ready",
				"sku":       "",
				"type":      self.custom_slots[1]["type"],
				"color":     self.custom_slots[1]["color"],
				"colors":    self.custom_slots[1]["colors"],
				"icon_type": 0,
				"remain":    0,
				"decorder":  0,
				"rfid":      1,
				"source":    2,
			},
			map[string]interface{}{
				"index":     2,
				"status":    "ready",
				"sku":       "",
				"type":      self.custom_slots[2]["type"],
				"color":     self.custom_slots[2]["color"],
				"colors":    self.custom_slots[2]["colors"],
				"icon_type": 0,
				"remain":    0,
				"decorder":  0,
				"rfid":      1,
				"source":    2,
			},
			map[string]interface{}{
				"index":     3,
				"status":    "ready",
				"sku":       "",
				"type":      self.custom_slots[3]["type"],
				"color":     self.custom_slots[3]["color"],
				"colors":    self.custom_slots[3]["colors"],
				"icon_type": 0,
				"remain":    0,
				"decorder":  0,
				"rfid":      1,
				"source":    2,
			},
		},
	}

	// If a slot has an empty type, revert its status to "empty" to match physical state if unknown
	for i, s := range self.info["slots"].([]interface{}) {
		sm := s.(map[string]interface{})
		if i < len(self.custom_slots) {
			cust := self.custom_slots[i]
			if sku, ok := cust["sku"].(string); ok && strings.TrimSpace(sku) != "" {
				sm["sku"] = sku
			}
			if rfid, ok := cust["rfid"]; ok {
				sm["rfid"] = rfid
			}
			if source, ok := cust["source"]; ok {
				sm["source"] = source
			}
			if iconType, ok := cust["icon_type"]; ok {
				sm["icon_type"] = iconType
			}
		}
		if t, ok := sm["type"].(string); ok && t == "" {
			sm["status"] = "empty"
		}
	}

	if self.variables == nil {
		self.variables = map[string]interface{}{}
	}
	if _, ok := self.variables["ace_current_index"]; !ok {
		self.variables["ace_current_index"] = int64(0)
	}
	if _, ok := self.variables["ace_filament_pos"]; !ok {
		self.variables["ace_filament_pos"] = "bowden"
	}
	if _, ok := self.variables["ace_endless_spool_enabled"]; !ok {
		self.variables["ace_endless_spool_enabled"] = false
	}
	if _, ok := self.variables["ace_inventory"]; !ok {
		self.variables["ace_inventory"] = []interface{}{
			map[string]interface{}{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			map[string]interface{}{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			map[string]interface{}{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			map[string]interface{}{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
		}
	}

	//# Add inventory for 4 slots - load from persistent variables if available
	var saved_inventory []interface{}
	if val, ok := self.variables["ace_inventory"]; ok && val != nil {
		if inv_slice, ok := val.([]interface{}); ok {
			saved_inventory = inv_slice
		}
	}

	if saved_inventory != nil {
		for _, inv := range saved_inventory {
			self.inventory = append(self.inventory, inv.(map[string]interface{}))
		}
	} else {
		self.inventory = []map[string]interface{}{
			{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
			{"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0},
		}
	}

	//self._create_mmu_sensor(config, extruder_sensor_pin, "extruder_sensor")
	//self._create_mmu_sensor(config, toolhead_sensor_pin, "toolhead_sensor")
	self.printer.Register_event_handler("project:ready", self._handle_ready)
	self.printer.Register_event_handler("project:disconnect", self._handle_disconnect)

	//# Register inventory commands
	self.gcode.Register_command(
		"ACE_SET_SLOT", self.cmd_ACE_SET_SLOT, true,
		"Set slot inventory: INDEX= COLOR= MATERIAL= TEMP= | Set status to empty with EMPTY=1")
	self.gcode.Register_command(
		"ACE_QUERY_SLOTS", self.cmd_ACE_QUERY_SLOTS, true,
		"Query all slot inventory as JSON")

	self.gcode.Register_command(
		"ACE_DEBUG", self.cmd_ACE_DEBUG, true,
		"self.cmd_ACE_DEBUG_help")
	self.gcode.Register_command(
		"ACE_START_DRYING", self.cmd_ACE_START_DRYING, true,
		"Starts ACE Pro dryer")
	self.gcode.Register_command(
		"ACE_STOP_DRYING", self.cmd_ACE_STOP_DRYING, true,
		"Stops ACE Pro dryer")
	self.gcode.Register_command(
		"ACE_ENABLE_FEED_ASSIST", self.cmd_ACE_ENABLE_FEED_ASSIST, true,
		"Enables ACE feed assist")
	self.gcode.Register_command(
		"ACE_DISABLE_FEED_ASSIST", self.cmd_ACE_DISABLE_FEED_ASSIST, true,
		"Disables ACE feed assist")
	self.gcode.Register_command(
		"ACE_FEED", self.cmd_ACE_FEED, true,
		"Feeds filament from ACE")
	self.gcode.Register_command(
		"ACE_RETRACT", self.cmd_ACE_RETRACT, true,
		"Retracts filament back to ACE")
	self.gcode.Register_command(
		"ACE_CHANGE_TOOL", self.cmd_ACE_CHANGE_TOOL, true,
		"Changes tool")
	self.gcode.Register_command(
		"ACE_ENABLE_ENDLESS_SPOOL", self.cmd_ACE_ENABLE_ENDLESS_SPOOL, true,
		"Enable endless spool feature")
	self.gcode.Register_command(
		"ACE_DISABLE_ENDLESS_SPOOL", self.cmd_ACE_DISABLE_ENDLESS_SPOOL, true,
		"Disable endless spool feature")
	self.gcode.Register_command(
		"ACE_ENDLESS_SPOOL_STATUS", self.cmd_ACE_ENDLESS_SPOOL_STATUS, true,
		"Show endless spool status")
	self.gcode.Register_command(
		"ACE_SAVE_INVENTORY", self.cmd_ACE_SAVE_INVENTORY, true,
		"Manually save current inventory to persistent storage")
	self.gcode.Register_command(
		"ACE_TEST_RUNOUT_SENSOR", self.cmd_ACE_TEST_RUNOUT_SENSOR, true,
		"Test and display runout sensor states")
	self.gcode.Register_command("FEED_FILAMENT", self.cmd_FEED_FILAMENT, true, "Native UI Feed Filament")
	self.gcode.Register_command("UNWIND_FILAMENT", self.cmd_UNWIND_FILAMENT, true, "Native UI Unwind Filament")
	self.heatbeat_timer = self.reactor.Register_timer(self._periodic_heartbeat_event, constants.NEVER)
	return self
}

func (self *ACE) _handle_ready([]interface{}) error {
	self.toolhead = MustLookupToolhead(self.printer)
	logger.Debug("ACE: Connecting to", self.ace_commun.Name())
	//# We can catch timing where ACE reboots itself when no data is available from host. We're avoiding it with this hack
	self.connect_timer = self.reactor.Register_timer(self._connect, constants.NOW)

	return nil
}

func (self *ACE) _handle_disconnect([]interface{}) error {
	logger.Debugf("ACE: Closing connection", self.ace_commun.Name())
	self._disconnect()
	return nil
}

func (self *ACE) _create_mmu_sensor(config *ConfigWrapper, pin string, name string) {
	section := fmt.Sprintf("filament_switch_sensor %s", name)
	config.fileconfig.Add_section(section)
	config.fileconfig.Set(section, "switch_pin", pin)
	config.fileconfig.Set(section, "pause_on_runout", "False")
	self.printer.Add_object(section, iopkg.LoadConfigSwitchSensor(config.Getsection(section)))
	self.printer.Load_object(config, section, nil)

	ppins := self.printer.Lookup_object("pins", object.Sentinel{}).(*printerpkg.PrinterPins)
	pin_params := ppins.Parse_pin(pin, true, true)
	share_name := fmt.Sprintf("%s:%s", pin_params["chip_name"], pin_params["pin"])
	ppins.Allow_multi_use_pin(share_name)
	mcu_endstop := ppins.Setup_pin("endstop", pin)

	query_endstops := self.printer.Load_object(config, "query_endstops", nil).(*probepkg.QueryEndstopsModule)
	query_endstops.Register_endstop(mcu_endstop, share_name)
	self.endstops[name] = mcu_endstop
}

func (self *ACE) lookupFilamentSensor(name string) aceFilamentSensor {
	section := fmt.Sprintf("filament_switch_sensor %s", name)
	obj := self.printer.Lookup_object(section, nil)
	sensor, ok := obj.(aceFilamentSensor)
	if !ok {
		return nil
	}
	return sensor
}

func (self *ACE) lookupRequiredFilamentSensor(name string) aceFilamentSensor {
	sensor := self.lookupFilamentSensor(name)
	if sensor == nil {
		panic(fmt.Sprintf("filament sensor %s not found", name))
	}
	return sensor
}

func (self *ACE) _connect(eventtime float64) float64 {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(err)
			self._disconnect()
		}
	}()

	self.gcode.Respond_info("ACE: Try connecting ACE", true)
	err := self.ace_commun.Connect()
	if err != nil {
		logger.Warnf("Unable to open ace_commun port %s: %s", self.ace_commun.Name(), err)
		if self.reconneted_count <= RECONNECT_COUNT {
			self.reconneted_count++
			delay := self.calc_reconnect_timeout(self.reconneted_count)
			self.gcode.Respond_info(fmt.Sprintf("ACE: Will auto reconnect after %f S ", delay), true)
			return eventtime + delay
		}
		// Long-term disconnection exceeding 10 seconds, and multiple failed reconnection attempt
		self.gcode.Respond_info("ACE: Reconnection exceeded the number of times, timeout exceeded 10 seconds.", true)
		return eventtime + 10.0
	}
	logger.Infof("ACE: Connected to %s", self.ace_commun.Name())
	self.gcode.Respond_info(fmt.Sprintf("ACE: Connected to %s ", self.ace_commun.Name()), true)
	self.ace_dev_fd = self.reactor.Register_fd(self.ace_commun.Fd(), self.read_handle, self.write_handle)

	// Obtain ACE information immediately after a successful connection.
	self.send_request(map[string]interface{}{"method": "get_info"},
		func(response map[string]interface{}) {
			fw_info, _ := json.Marshal(response["result"])
			self.fw_info = response["result"].(map[string]interface{})

			if model, ok := self.fw_info["model"].(string); ok {
				if strings.Contains(model, "2.0") {
					self.ace_commun.IsV2 = true
				} else {
					self.ace_commun.IsV2 = false
				}
			}

			self.gcode.Respond_info(fmt.Sprintf("ACE: Firmware info %s", string(fw_info[1:len(fw_info)-2])), true)
		})

	if self.heatbeat_timer != nil {
		self.reactor.Update_timer(self.heatbeat_timer, constants.NOW)
	}

	//# Start endless spool monitoring timer
	if self.endless_spool_enabled {
		self.endless_spool_timer = self.reactor.Register_timer(self._endless_spool_monitor, eventtime+1.0)
	}

	//# --- Added: Check ace_current_index and enable feed assist if needed ---
	ace_current_index := -1
	ace_current_index = int(self.variables["ace_current_index"].(int64))
	if ace_current_index != -1 {
		self.gcode.Respond_info(fmt.Sprintf("ACE: Re-enabling feed assist on reconnect for index {%d}", ace_current_index), true)
		self._enable_feed_assist(ace_current_index)
	}
	// clear reconneted count
	self.reconneted_count = 0
	return constants.NEVER
}

func (self *ACE) _disconnect() {
	logger.Debug("ACE: Disconnet...")
	self.gcode.Respond_info("ACE: Disconnet...", true)

	if self.heatbeat_timer != nil {
		self.reactor.Update_timer(self.heatbeat_timer, constants.NEVER)
	}

	if self.endless_spool_timer != nil {
		self.reactor.Unregister_timer(self.endless_spool_timer)
		self.endless_spool_timer = nil
	}

	self.reactor.Set_fd_wake(self.ace_dev_fd, false, false)
	self.ace_commun.Disconnect()
	self.ace_dev_fd = nil
}

func (self *ACE) calc_reconnect_timeout(attempt int) float64 {
	return 0.8*float64(attempt) + math.Cos(float64(attempt))*0.5
}

func (self *ACE) write_handle(eventtime float64) interface{} {
	defer func() {
		if err := recover(); err != nil {
			logger.Error(eventtime, err)
		}
	}()
	if self.ace_commun != nil && self.ace_dev_fd != nil {
		self.ace_commun.Writer(eventtime)
		if self.ace_commun.Is_send_queue_empty() {
			self.reactor.Set_fd_wake(self.ace_dev_fd, true, false)
		}
	}
	return nil
}

func (self *ACE) read_handle(eventtime float64) interface{} {
	err := self.ace_commun.Reader(eventtime)
	if err != nil {
		logger.Error(err)
		self._disconnect()
		// Response timeout or Unable to communicate requires reconnecting ACE
		if strings.Contains(err.Error(), filamentpkg.RespondTimeoutError) ||
			strings.Contains(err.Error(), filamentpkg.UnableToCommunError) {
			if self.reconneted_count <= RECONNECT_COUNT {
				self.reconneted_count++
				delay := self.calc_reconnect_timeout(self.reconneted_count)
				self.gcode.Respond_info(fmt.Sprintf("ACE: Will auto reconnect after %f S ", delay), true)
				self.reactor.Update_timer(self.connect_timer, eventtime+delay)
			}
		}
	}
	return nil
}

func (self *ACE) _periodic_heartbeat_event(eventtime float64) float64 {
	self.send_request(map[string]interface{}{"method": "get_status"},
		func(response map[string]interface{}) {
			res, ok := response["result"].(map[string]interface{})
			if !ok {
				return
			}
			if slots, ok := res["slots"].([]interface{}); ok {
				for i, s := range slots {
					slot, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					if i < len(self.custom_slots) {
						cust := self.custom_slots[i]
						custType, _ := cust["type"].(string)
						custType = strings.TrimSpace(custType)
						custColor, _ := cust["color"].([]interface{})
						custColors, hasCustColors := cust["colors"].([]interface{})

						if hasValidACESlotRFID(slot) {
							if typ, ok := slot["type"].(string); ok {
								self.custom_slots[i]["type"] = strings.TrimSpace(typ)
							}
							if color, ok := slot["color"].([]interface{}); ok && len(color) == 3 {
								self.custom_slots[i]["color"] = color
								if colors, ok := slot["colors"].([]interface{}); ok && len(colors) > 0 {
									self.custom_slots[i]["colors"] = colors
								} else if generatedColors := buildACEColorList(color); generatedColors != nil {
									self.custom_slots[i]["colors"] = generatedColors
									slot["colors"] = generatedColors
								}
							}
							self.custom_slots[i]["sku"] = slot["sku"]
							self.custom_slots[i]["rfid"] = slot["rfid"]
							self.custom_slots[i]["source"] = slot["source"]
							self.custom_slots[i]["icon_type"] = slot["icon_type"]

							persistedSlot := map[string]interface{}{
								"type":      self.custom_slots[i]["type"],
								"color":     self.custom_slots[i]["color"],
								"colors":    self.custom_slots[i]["colors"],
								"sku":       self.custom_slots[i]["sku"],
								"rfid":      self.custom_slots[i]["rfid"],
								"source":    self.custom_slots[i]["source"],
								"icon_type": self.custom_slots[i]["icon_type"],
							}
							self.persistFilamentSlotConfig(i, persistedSlot)
							continue
						}

						if slot["sku"] == "" || slot["sku"] == nil || slot["sku"] == "custom" {
							slot["status"] = "ready"
							slot["sku"] = ""
							slot["rfid"] = 1
							slot["source"] = 2
							slot["icon_type"] = 0
							slot["remain"] = 0
							slot["decorder"] = 0

							if custType != "" {
								slot["type"] = custType
							}

							if len(custColor) == 3 {
								slot["color"] = custColor
							}

							if hasCustColors {
								slot["colors"] = custColors
							} else {
								slot["colors"] = []interface{}{[]interface{}{0, 0, 0, 0}}
							}
						} else {
							if custType != "" && custType != "?" {
								slot["type"] = custType
							}
							if len(custColor) == 3 {
								slot["color"] = custColor
							}
							if hasCustColors {
								slot["colors"] = custColors
							}
						}
					}
				}
			}
			self.info = res
		})
	//If there is no response within 3 seconds of the last command sent,
	//disconnect ACE and reconnect.
	return eventtime + 1.5
}

func (self *ACE) dwell(delay float64) {
	currTs := self.reactor.Monotonic()
	self.reactor.Pause(currTs + delay)
}

func (self *ACE) send_request(request map[string]interface{}, callback func(map[string]interface{})) {
	// self.info["status"] = "busy"
	self.ace_commun.Push_send_queue(request, callback)
	self.reactor.Set_fd_wake(self.ace_dev_fd, true, true)
}

func (self *ACE) _check_endstop_state(name string) bool {
	print_time := self.toolhead.Get_last_move_time()
	sta := 0
	if _, ok := self.endstops[name].(*ProbeEndstopWrapper); ok {
		sta = self.endstops[name].(*ProbeEndstopWrapper).Query_endstop(print_time)
	} else if _, ok := self.endstops[name].(*MCU_endstop); ok {
		sta = self.endstops[name].(*MCU_endstop).Query_endstop(print_time)
	}

	return sta > 0
}

func (self *ACE) wait_ace_ready() {
	for {
		if status, ok := self.info["status"].(string); ok && status == "ready" {
			break
		}
		self.dwell(0.5)
	}
}

func (self *ACE) _extruder_move(length, speed float64) float64 {
	pos := self.toolhead.Get_position()
	pos[3] += length
	self.toolhead.Move(pos, speed)
	return pos[3]
}

func (self *ACE) _endless_spool_monitor(eventtime float64) float64 {
	//"""Monitor for runout detection during printing"""
	if !self.endless_spool_enabled || self.change_tool_in_progress || self.endless_spool_in_progress {
		return eventtime + 0.1
	}
	//# Only monitor if we have an active tool and we're not already in runout state
	current_tool := self.variables["ace_current_index"].(int64)
	if current_tool == -1 {
		return eventtime + 0.1
	}
	time := eventtime + 0.05
	is_printing := false
	//# Check if we're currently printing - be more aggressive about detecting print state
	defer func() {
		if err := recover(); err != nil {
			//# If idle_timeout doesn't exist, assume we might be printing
			is_printing = false
			logger.Error(fmt.Errorf("ACE: Endless spool monitor error: %v", err))
			time = eventtime + 0.1
		}
		//# Check multiple indicators that we might be printing
		toolhead := MustLookupToolhead(self.printer)
		print_stats := self.printer.Lookup_object("print_stats", nil).(*printpkg.PrintStatsModule)
		is_printing = false

		//# Method 1: Check if toolhead is moving
		toolhead_status := toolhead.Get_status(eventtime)
		if toolhead_status != nil {
			if toolhead_status["homed_axes"] != nil {
				is_printing = true
			}
		}

		//# Method 2: Check print stats if available
		if print_stats != nil {
			stats := print_stats.Get_status(eventtime)
			if stats["state"] == "printing" {
				is_printing = true
			}
		}

		//# Method 3: Check idle timeout state
		printer_idle := self.printer.Lookup_object("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
		idle_state := printer_idle.Get_status(eventtime)["state"]
		//# Ready means potentially printing
		if idle_state == "Printing" || idle_state == "Ready" {
			is_printing = true
		}
		//# Always check for runout if endless spool is enabled and we have an active tool
		//# Don't rely only on print state detection
		if current_tool >= 0 {
			self._endless_spool_runout_handler()
		}

		//# Adjust monitoring frequency based on state
		if is_printing {
			//# Check every 50ms during printing
			time = eventtime + 0.05
		} else {
			//# Check every 200ms when idle
			time = eventtime + 0.2
		}
	}()
	return time
}

func (self *ACE) cmd_ACE_START_DRYING(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	temperature := gcmd.Get_int("TEMP", 0, nil, nil)
	duration := gcmd.Get_int("DURATION", 240, nil, nil)

	if duration <= 0 {
		panic("Wrong duration")
	}

	if temperature <= 0 || temperature > self.max_dryer_temperature {
		panic("Wrong temperature")
	}
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		}
	}

	self.gcode.Respond_info("ACE: Started ACE drying", true)

	self.send_request(
		map[string]interface{}{
			"method": "drying",
			"params": map[string]interface{}{
				"temp":      temperature,
				"fan_speed": 7000,
				"duration":  duration},
		},
		callback)
	return nil
}

func (self *ACE) cmd_ACE_STOP_DRYING(argv interface{}) error {
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		}
	}
	self.gcode.Respond_info("ACE: Stopped ACE drying", true)
	self.send_request(map[string]interface{}{"method": "drying_stop"}, callback)
	return nil
}

func (self *ACE) _enable_feed_assist(index int) {
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		} else {
			self.feed_assist_index = index
			self.gcode.Respond_info("ACE: Enabled ACE feed assist", true)
		}
	}

	self.send_request(map[string]interface{}{
		"method": "start_feed_assist",
		"params": map[string]interface{}{"index": index},
	},
		callback)
	self.dwell(0.7)
}

func (self *ACE) cmd_ACE_ENABLE_FEED_ASSIST(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := gcmd.Get_int("INDEX", -1, nil, nil)

	if index < 0 || index >= 4 {
		panic("Wrong index")
	}

	self._enable_feed_assist(index)
	return nil
}

func (self *ACE) _disable_feed_assist(index int) {
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		}
		self.feed_assist_index = -1
		self.gcode.Respond_info("ACE: Disabled ACE feed assist", true)
	}
	self.send_request(map[string]interface{}{
		"method": "stop_feed_assist",
		"params": map[string]interface{}{"index": index},
	},
		callback)
	self.dwell(0.3)
}

func (self *ACE) cmd_ACE_DISABLE_FEED_ASSIST(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := -1
	if self.feed_assist_index != -1 {
		index = gcmd.Get_int("INDEX", self.feed_assist_index, nil, nil)
	} else {
		index = gcmd.Get_int("INDEX", -1, nil, nil)
	}

	if index < 0 || index >= 4 {
		panic("Wrong index")
	}

	self._disable_feed_assist(index)
	return nil
}

func (self *ACE) _feed(index, length, speed int) {
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		}
	}
	self.send_request(map[string]interface{}{
		"method": "feed_filament",
		"params": map[string]interface{}{
			"index":  index,
			"length": length,
			"speed":  speed,
		},
	},
		callback)
	self.dwell(float64(length/speed) + 0.1)
}

func (self *ACE) cmd_ACE_FEED(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := gcmd.Get_int("INDEX", -1, nil, nil)
	length := gcmd.Get_int("LENGTH", -1, nil, nil)
	speed := gcmd.Get_int("SPEED", self.feed_speed, nil, nil)

	if index < 0 || index >= 4 {
		panic("Wrong index")
	}

	if length <= 0 {
		panic("Wrong length")
	}

	if speed <= 0 {
		panic("Wrong speed")
	}
	self._feed(index, length, speed)
	return nil
}

func (self *ACE) cmd_FEED_FILAMENT(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := gcmd.Get_int("INDEX", 0, nil, nil)
	length := gcmd.Get_int("LENGTH", self.toolchange_load_length, nil, nil)
	speed := gcmd.Get_int("SPEED", self.feed_speed, nil, nil)
	if length <= 0 {
		length = 100
	}
	if speed <= 0 {
		speed = 50
	}
	self.gcode.Respond_info(fmt.Sprintf("ACE UI FEED idx=%d", index), true)
	self._feed(index, length, speed)
	return nil
}

func (self *ACE) _retract(index, length, speed int) {
	callback := func(response map[string]interface{}) {
		if response["code"] != nil && response["code"].(float64) != 0 {
			panic(fmt.Errorf("ACE Error: %v", response["msg"]))
		}
	}
	self.send_request(map[string]interface{}{
		"method": "unwind_filament",
		"params": map[string]interface{}{
			"index":  index,
			"length": length,
			"speed":  speed,
		},
	},
		callback)
	self.dwell(float64(length/speed) + 0.1)
}

func (self *ACE) cmd_ACE_RETRACT(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := gcmd.Get_int("INDEX", -1, nil, nil)
	length := gcmd.Get_int("LENGTH", -1, nil, nil)
	speed := gcmd.Get_int("SPEED", self.retract_speed, nil, nil)

	if index < 0 || index >= 4 {
		panic("Wrong index")
	}

	if length <= 0 {
		panic("Wrong length")
	}

	if speed <= 0 {
		panic("Wrong speed")
	}
	self._retract(index, length, speed)
	return nil
}

func (self *ACE) cmd_UNWIND_FILAMENT(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	index := gcmd.Get_int("INDEX", -1, nil, nil)
	length := gcmd.Get_int("LENGTH", self.toolchange_retract_length, nil, nil)
	speed := gcmd.Get_int("SPEED", self.feed_speed, nil, nil)
	if index < 0 {
		index = self.feed_assist_index
		if index < 0 {
			index = 0
		}
	}
	if length <= 0 {
		length = 100
	}
	if speed <= 0 {
		speed = 50
	}
	self.gcode.Respond_info(fmt.Sprintf("ACE UI UNWIND idx=%d", index), true)
	self._retract(index, length, speed)
	return nil
}

func (self *ACE) _feed_to_toolhead(tool int) error {
	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	//self.wait_ace_ready()
	//
	//self._feed(tool, self.toolchange_load_length, self.retract_speed)
	//self.variables["ace_filament_pos"] = "bowden"
	//
	//self.wait_ace_ready()

	//self._enable_feed_assist(tool)

	for {
		if sensor_extruder.FilamentPresent() {
			break
		}
		self.wait_ace_ready()
		if self.info["slots"].([]interface{})[tool].(map[string]interface{})["status"] == "ready" {
			self._feed(tool, self.toolchange_load_length, self.retract_speed)
			self.variables["ace_filament_pos"] = "bowden"
			self.dwell(0.1)
		} else {
			logger.Info("Spool is empty")
			printer_idle := self.printer.Lookup_object("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
			idle_state := printer_idle.Get_status(self.reactor.Monotonic())["state"]
			if idle_state == "Printing" {
				self.gcode.Run_script_from_command("PAUSE")
			}
			return fmt.Errorf("Spool is empty")
		}
	}

	if !sensor_extruder.FilamentPresent() {
		panic(fmt.Errorf("Filament stuck %v", sensor_extruder.FilamentPresent()))
	} else {
		self.variables["ace_filament_pos"] = "spliter"
	}

	self._enable_feed_assist(tool)

	//for {
	//	if !self._check_endstop_state("toolhead_sensor") {
	//		break
	//	}
	//	self._extruder_move(1, 5)
	//	self.dwell(0.01)
	//}

	self.variables["ace_filament_pos"] = "toolhead"

	self._extruder_move(float64(self.toolhead_sensor_to_nozzle_length), 5)
	self.variables["ace_filament_pos"] = "nozzle"
	self.gcode.Run_script_from_command("MOVE_THROW_POS")

	return nil
}

func (self *ACE) cmd_ACE_CHANGE_TOOL(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)

	tool := gcmd.Get_int("TOOL", -1, nil, nil)
	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	if tool < 0 || tool >= 4 {
		panic("Wrong index")
	}

	was := int(self.variables["ace_current_index"].(int64))
	if was == tool {
		gcmd.Respond_info(fmt.Sprintf("ACE: Already tool %d", tool), true)
		return nil
	}

	if tool != -1 {
		status := self.info["slots"].([]interface{})[tool].(map[string]interface{})["status"]
		if status != "ready" {
			gcmd.Respond_info("ACE: Spool is empty", true)
			printer_idle := self.printer.Lookup_object("idle_timeout", nil).(*printpkg.IdleTimeoutModule)
			idle_state := printer_idle.Get_status(self.reactor.Monotonic())["state"]
			if idle_state == "Printing" {
				self.gcode.Run_script_from_command("PAUSE")
			}
			return nil
		}
	}

	//# Temporarily disable endless spool during manual toolchange
	endless_spool_was_enabled := self.endless_spool_enabled
	if endless_spool_was_enabled {
		self.endless_spool_enabled = false
		self.endless_spool_runout_detected = false
	}
	self.change_tool_in_progress = true
	self.gcode.Run_script_from_command(fmt.Sprintf("_ACE_PRE_TOOLCHANGE FROM=%d TO=%d", was, tool))

	logger.Infof(fmt.Sprintf("ACE: Toolchange %d => %d", was, tool))
	var err error
	if was != -1 {
		self._disable_feed_assist(was)
		self.wait_ace_ready()
		ace_filament_pos := self.variables["ace_filament_pos"]
		if ace_filament_pos == nil {
			ace_filament_pos = "spliter"
		}

		if ace_filament_pos.(string) == "nozzle" {
			self.gcode.Run_script_from_command("CUT_TIP")
			self.variables["ace_filament_pos"] = "toolhead"
		}

		if ace_filament_pos.(string) == "toolhead" {
			for {
				if sensor_extruder.FilamentPresent() {
					break
				}
				self._extruder_move(-50, 10)
				self._retract(was, 100, self.retract_speed)
				self.wait_ace_ready()
			}
			self.variables["ace_filament_pos"] = "bowden"
		}
		self.wait_ace_ready()

		self._retract(was, self.toolchange_retract_length, self.retract_speed)
		self.wait_ace_ready()
		self.variables["ace_filament_pos"] = "spliter"

		if tool != -1 {
			err = self._feed_to_toolhead(tool)
		}
	} else {
		err = self._feed_to_toolhead(tool)
	}

	if err != nil {
		self.change_tool_in_progress = false
		if endless_spool_was_enabled {
			self.endless_spool_enabled = true
		}
		logger.Error(err)
		return nil
	}

	gcode_move := MustLookupGCodeMove(self.printer)
	gcode_move.Reset_last_position(nil)

	self.gcode.Run_script_from_command(fmt.Sprintf("_ACE_POST_TOOLCHANGE FROM=%d TO=%d", was, tool))

	self.variables["ace_current_index"] = tool
	gcode_move.Reset_last_position(nil)
	//# Force save to disk
	self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_current_index VALUE=%d", tool))
	self.gcode.Run_script_from_command(
		fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_filament_pos VALUE=%s", self.variables["ace_filament_pos"]))
	self.change_tool_in_progress = false

	//# Re-enable endless spool if it was enabled before
	if endless_spool_was_enabled {
		self.endless_spool_enabled = true
	}

	gcmd.Respond_info(fmt.Sprintf("ACE: Tool {%d} load", tool), true)
	return nil
}

func (self *ACE) _find_next_available_slot(current_slot int) int {
	//"""Find the next available slot with filament for endless spool"""
	for i := 0; i < 4; i++ {
		next_slot := (current_slot + 1 + i) % 4
		if next_slot != current_slot {
			if self.inventory[next_slot]["status"] == "ready" &&
				self.info["slots"].([]interface{})[next_slot].(map[string]interface{})["status"] == "ready" {
				return next_slot
			}
		}
	}
	return -1
}

func (self *ACE) _endless_spool_runout_handler() {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("ACE: Runout detection error: %v", err)
		}
	}()

	//"""Handle runout detection for endless spool"""
	if !self.endless_spool_enabled || self.endless_spool_in_progress {
		return
	}

	current_tool := self.variables["ace_current_index"].(int64)
	if current_tool == -1 {
		return
	}
	sensor_extruder := self.lookupFilamentSensor("extruder_sensor")

	if sensor_extruder != nil {
		//# Check both runout helper and direct endstop state
		runout_helper_present := sensor_extruder.FilamentPresent()
		endstop_triggered := self._check_endstop_state("extruder_sensor")
		//# Log sensor states for debugging (remove after testing)
		//logger.Infof("ACE Debug: runout_helper=%v, endstop=%v", runout_helper_present, endstop_triggered)
		//
		//# Runout detected if filament is not present
		if !runout_helper_present && !endstop_triggered {
			//# Only trigger once
			if !self.endless_spool_runout_detected {
				self.endless_spool_runout_detected = true
				self.gcode.Respond_info("ACE: Endless spool runout detected, switching immediately", true)
				logger.Debugf("ACE: Runout detected - helper=%v, endstop=%v", runout_helper_present, endstop_triggered)
				self._execute_endless_spool_change()
			}
		}
	}
}

func (self *ACE) _execute_endless_spool_change() {
	defer func() {
		if err := recover(); err != nil {
			self.gcode.Respond_info(fmt.Sprintf("ACE: Endless spool change failed: {%v}", err), true)
			self.gcode.Run_script_from_command("PAUSE")
			self.endless_spool_in_progress = false
		}
	}()
	//"""Execute the endless spool toolchange - simplified for extruder sensor only"""
	if self.endless_spool_in_progress {
		return
	}

	current_tool := int(self.variables["ace_current_index"].(int64))
	next_tool := self._find_next_available_slot(current_tool)

	if next_tool == -1 {
		self.gcode.Respond_info("ACE: No available slots for endless spool, pausing print", true)
		self.gcode.Run_script_from_command("PAUSE")
		self.endless_spool_runout_detected = false
		return
	}

	self.endless_spool_in_progress = true
	self.endless_spool_runout_detected = false

	self.gcode.Respond_info(fmt.Sprintf("ACE: Endless spool changing from slot %d to slot %d", current_tool, next_tool), true)

	// Mark current slot as empty in inventory
	if current_tool >= 0 {
		self.inventory[current_tool] = map[string]interface{}{
			"status": "empty", "color": []interface{}{0, 0, 0}, "material": "", "temp": 0,
		}
		//Save updated inventory to persistent variables
		self.variables["ace_inventory"] = self.inventory

		s, _ := json.Marshal(self.inventory)
		self.gcode.Run_script_from_command(
			fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", string(s)))
	}

	//# Direct endless spool change - no toolchange macros needed for runout response

	//# Step 1: Disable feed assist on empty slot
	if current_tool != -1 {
		self._disable_feed_assist(current_tool)
		self.wait_ace_ready()
	}

	//# Step 2: Feed filament from next slot until it reaches extruder sensor
	sensor_extruder := self.lookupRequiredFilamentSensor("extruder_sensor")

	//# Feed filament from new slot until extruder sensor triggers
	//self._feed(next_tool, self.toolchange_load_length, self.retract_speed)
	//self.wait_ace_ready()

	//# Wait for filament to reach extruder sensor
	for {
		if !sensor_extruder.FilamentPresent() {
			break
		}
		self._feed(next_tool, self.toolchange_load_length, self.retract_speed)
		self.wait_ace_ready()
		self.dwell(0.1)
	}

	if !sensor_extruder.FilamentPresent() {
		panic("Filament stuck during endless spool change")
	}

	//# Step 3: Enable feed assist for new slot
	self._enable_feed_assist(next_tool)

	//# Step 4: Update current index and save state
	self.variables["ace_current_index"] = next_tool
	self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_current_index VALUE=%d", next_tool))

	self.endless_spool_in_progress = false

	self.gcode.Respond_info(fmt.Sprintf("ACE: Endless spool completed, now using slot {%d}", next_tool), true)
}

func (self *ACE) cmd_ACE_ENABLE_ENDLESS_SPOOL(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	self.endless_spool_enabled = true
	//# Save to persistent variables
	self.variables["ace_endless_spool_enabled"] = true
	self.gcode.Run_script_from_command("SAVE_VARIABLE VARIABLE=ace_endless_spool_enabled VALUE=true")
	gcmd.Respond_info("ACE: Endless spool enabled (immediate switching on runout)", true)
	return nil
}

func (self *ACE) cmd_ACE_DISABLE_ENDLESS_SPOOL(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	self.endless_spool_enabled = false
	self.endless_spool_runout_detected = false
	self.endless_spool_in_progress = false

	//# Save to persistent variables
	self.variables["ace_endless_spool_enabled"] = false
	self.gcode.Run_script_from_command("SAVE_VARIABLE VARIABLE=ace_endless_spool_enabled VALUE=false")
	gcmd.Respond_info("ACE: Endless spool disabled (saved to persistent variables)", true)
	return nil
}

func (self *ACE) cmd_ACE_ENDLESS_SPOOL_STATUS(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	status := self.Get_status(0)["endless_spool"].(map[string]interface{})
	saved_enabled := self.variables["ace_endless_spool_enabled"].(bool)

	gcmd.Respond_info("ACE: Endless spool status:", true)
	gcmd.Respond_info(fmt.Sprintf("  - Currently enabled: %v", status["enabled"]), true)
	gcmd.Respond_info(fmt.Sprintf("  - Saved enabled: %v", saved_enabled), true)
	gcmd.Respond_info("  - Mode: Immediate switching on runout detection", true)
	if status["enabled"] != nil {
		gcmd.Respond_info(fmt.Sprintf("  - Runout detected: %v", status["runout_detected"]), true)
		gcmd.Respond_info(fmt.Sprintf("  - In progress: %v", status["in_progress"]), true)
	}
	return nil
}

func (self *ACE) cmd_ACE_DEBUG(argv interface{}) error {
	defer func() {
		if err := recover(); err != nil {
			self.gcode.Respond_info(fmt.Sprintf("Error: %v", err), true)
		}
	}()
	gcmd := argv.(*GCodeCommand)
	method := gcmd.Get("METHOD", object.Sentinel{}, "", nil, nil, nil, nil)
	params := gcmd.Get("PARAMS", "", "", nil, nil, nil, nil)
	callback := func(response map[string]interface{}) {
		s, _ := json.Marshal(response)
		self.gcode.Respond_info("ACE: Response:"+string(s), true)
	}

	if params != "" {
		_params, _ := json.Marshal(params)
		self.send_request(map[string]interface{}{
			"method": method,
			"params": _params,
		},
			callback)
	} else {
		self.send_request(map[string]interface{}{
			"method": method,
		},
			callback)
	}

	return nil
}
func (self *ACE) Get_status(eventtime float64) map[string]interface{} {
	status := self.info
	status["id"] = 0
	status["endless_spool"] = map[string]interface{}{
		"enabled":         self.endless_spool_enabled,
		"runout_detected": self.endless_spool_runout_detected,
		"in_progress":     self.endless_spool_in_progress,
	}
	status["fw_info"] = self.fw_info

	autoRefillVal := 0
	if self.endless_spool_enabled {
		autoRefillVal = 1
	}

	return map[string]interface{}{
		"auto_refill":              autoRefillVal,
		"current_filament":         "",
		"cutter_state":             0,
		"ext_spool":                1,
		"ext_spool_status":         "runout",
		"filament_hubs":            []interface{}{sys.DeepCopyMap(status)},
		"filament_present":         0,
		"tracker_detection_length": 0,
		"tracker_filament_present": 0,
	}
}
func (self *ACE) cmd_ACE_SET_SLOT(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	idx := gcmd.Get_int("INDEX", -1, nil, nil)
	if idx < 0 || idx >= 4 {
		panic("Invalid slot index")
	}

	if gcmd.Get_int("EMPTY", 0, nil, nil) == 1 {
		self.inventory[idx] = map[string]interface{}{
			"status":   "empty",
			"color":    []interface{}{0, 0, 0},
			"material": "",
			"temp":     0,
		}
		//# Save to persistent variables
		self.variables["ace_inventory"] = self.inventory
		s, _ := json.Marshal(self.inventory)
		self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", string(s)))
		gcmd.Respond_info(fmt.Sprintf("ACE: Slot %d set to empty", idx), true)
		return nil
	}

	color_str := gcmd.Get("COLOR", nil, "", nil, nil, nil, nil)
	material := gcmd.Get("MATERIAL", "", "", nil, nil, nil, nil)

	temp := gcmd.Get_int("TEMP", 0, nil, nil)
	if color_str == "" || material == "" || temp <= 0 {
		return errors.New("COLOR, MATERIAL, TEMP must be set unless EMPTY=1")
	}

	color := parseRGB(color_str)
	if len(color) != 3 {
		return errors.New("COLOR must be R,G,B")
	}

	self.inventory[idx] = map[string]interface{}{
		"status":   "ready",
		"color":    color,
		"material": material,
		"temp":     temp,
	}
	//# Save to persistent variables
	self.variables["ace_inventory"] = self.inventory
	s, _ := json.Marshal(self.inventory)
	self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", string(s)))
	gcmd.Respond_info(fmt.Sprintf("ACE: Slot %d set: color=%v, material=%s, temp=%d", idx, color, material, temp), true)
	return nil
}

func (self *ACE) cmd_ACE_QUERY_SLOTS(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	data, _ := json.Marshal(self.inventory)
	gcmd.Respond_info("ACE: query slots:"+string(data), true)
	return nil
}

func (self *ACE) cmd_ACE_SAVE_INVENTORY(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	self.variables["ace_inventory"] = self.inventory
	s, _ := json.Marshal(self.inventory)
	self.gcode.Run_script_from_command(fmt.Sprintf("SAVE_VARIABLE VARIABLE=ace_inventory VALUE=%s", string(s)))
	gcmd.Respond_info("ACE: Inventory saved to persistent storage", true)
	return nil
}

func (self *ACE) cmd_ACE_TEST_RUNOUT_SENSOR(argv interface{}) error {
	gcmd := argv.(*GCodeCommand)
	sensor_extruder := self.lookupFilamentSensor("extruder_sensor")

	if sensor_extruder != nil {
		runout_helper_present := sensor_extruder.FilamentPresent()
		endstop_triggered := self._check_endstop_state("extruder_sensor")

		gcmd.Respond_info("ACE: Extruder sensor states:", true)
		gcmd.Respond_info(fmt.Sprintf("  - Runout helper filament present: %v", runout_helper_present), true)
		gcmd.Respond_info(fmt.Sprintf("  - Endstop triggered: %v", endstop_triggered), true)
		gcmd.Respond_info(fmt.Sprintf("  - Endless spool enabled: %v", self.endless_spool_enabled), true)
		gcmd.Respond_info(fmt.Sprintf("  - Current tool: %d", self.variables["ace_current_index"]), true)
		gcmd.Respond_info(fmt.Sprintf("  - Runout detected: %v", self.endless_spool_runout_detected), true)

		//# Test runout detection logic
		would_trigger := !runout_helper_present || !endstop_triggered
		gcmd.Respond_info(fmt.Sprintf("  - Would trigger runout: %v", would_trigger), true)
	} else {
		gcmd.Respond_info("ACE: Extruder sensor not found", true)
	}
	return nil
}

func Load_config_ace(config *ConfigWrapper) interface{} {
	return NewAce(config)
}

func Load_config_filament_hub(config *ConfigWrapper) interface{} {
	return NewAce(config)
}

func (self *ACE) Set_filament_info(index int, typ string, sku string, color []interface{}) {
	if index >= 0 && index < len(self.custom_slots) {
		if self.info != nil {
			if slots, ok := self.info["slots"].([]interface{}); ok && index < len(slots) {
				if slotMap, ok := slots[index].(map[string]interface{}); ok && hasValidACESlotRFID(slotMap) {
					logger.Infof("Ignoring panel filament info for RFID-backed slot %d with sku=%v", index, slotMap["sku"])
					persistedSlot := map[string]interface{}{
						"type":      slotMap["type"],
						"color":     slotMap["color"],
						"colors":    slotMap["colors"],
						"sku":       slotMap["sku"],
						"rfid":      slotMap["rfid"],
						"source":    slotMap["source"],
						"icon_type": slotMap["icon_type"],
					}
					self.persistFilamentSlotConfig(index, persistedSlot)
					return
				}
			}
		}

		existingType, _ := self.custom_slots[index]["type"].(string)
		typ = normalizeACEFilamentType(typ, existingType)
		self.custom_slots[index]["type"] = typ
		self.custom_slots[index]["color"] = color
		self.custom_slots[index]["sku"] = ""
		self.custom_slots[index]["rfid"] = 1
		self.custom_slots[index]["source"] = 2
		self.custom_slots[index]["icon_type"] = 0

		var colorsList []interface{}
		if len(color) == 3 {
			colorsList = buildACEColorList(color)
			self.custom_slots[index]["colors"] = colorsList
		}

		// Instant update to self.info if available
		if self.info != nil {
			import_json := true
			_ = import_json
			encoded, _ := json.Marshal(self.info)
			var newInfo map[string]interface{}
			json.Unmarshal(encoded, &newInfo)

			if slots, ok := newInfo["slots"].([]interface{}); ok {
				if index < len(slots) {
					if slotMap, ok2 := slots[index].(map[string]interface{}); ok2 {
						slotMap["type"] = typ
						slotMap["status"] = "ready"
						slotMap["sku"] = ""
						slotMap["rfid"] = 1
						slotMap["source"] = 2
						slotMap["icon_type"] = 0
						slotMap["remain"] = 0
						slotMap["decorder"] = 0
						slotMap["color"] = color
						if colorsList != nil {
							slotMap["colors"] = colorsList
						} else {
							slotMap["colors"] = []interface{}{[]interface{}{0, 0, 0, 0}}
						}
					}
				}
			}
			self.info = newInfo
		}

		persistedSlot := map[string]interface{}{
			"type":      typ,
			"color":     color,
			"colors":    colorsList,
			"sku":       "",
			"rfid":      1,
			"source":    2,
			"icon_type": 0,
		}
		self.persistFilamentSlotConfig(index, persistedSlot)
	}
}
