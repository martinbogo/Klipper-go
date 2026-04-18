package filament

import (
	"encoding/json"
	"errors"
	"fmt"
	"goklipper/common/logger"
	"strings"
)

// ACEInventorySlotUpdate captures the user-facing inventory fields accepted by
// the ACE_SET_SLOT command.
type ACEInventorySlotUpdate struct {
	Index    int
	Empty    bool
	Color    string
	Material string
	Temp     int
}

// ACERuntimeState owns the generic ACE runtime maps and booleans that are
// shared across status reporting, persistence, and simple command glue.
type ACERuntimeState struct {
	Variables                  map[string]interface{}
	Info                       map[string]interface{}
	Inventory                  []map[string]interface{}
	CustomSlots                []map[string]interface{}
	ConfigPath                 string
	FirmwareInfo               map[string]interface{}
	FeedAssistIndex            int
	EndlessSpoolEnabled        bool
	EndlessSpoolInProgress     bool
	EndlessSpoolRunoutDetected bool
}

type ACEEndlessSpoolChangePlan struct {
	CurrentTool        int
	NextTool           int
	InventorySaveValue string
	InventoryChanged   bool
}

// SavedEndlessSpoolEnabled returns the persisted endless spool flag when it is
// present in the ACE variables map.
func SavedEndlessSpoolEnabled(variables map[string]interface{}) bool {
	if variables == nil {
		return false
	}
	enabled, _ := variables["ace_endless_spool_enabled"].(bool)
	return enabled
}

// NewACERuntimeState initializes the reusable ACE runtime maps from persisted
// variables and the AMS config file.
func NewACERuntimeState(variables map[string]interface{}, endlessSpoolEnabled bool, cfgPath string) *ACERuntimeState {
	if variables == nil {
		variables = map[string]interface{}{}
	}
	EnsureDefaultVariables(variables)

	customSlots := DefaultCustomSlots()
	if cfgPath != "" {
		LoadCustomSlotsFromConfig(customSlots, cfgPath)
	}

	return &ACERuntimeState{
		Variables:           variables,
		Info:                BuildDefaultACEInfo(customSlots),
		Inventory:           InitializeInventory(variables),
		CustomSlots:         customSlots,
		ConfigPath:          cfgPath,
		FeedAssistIndex:     -1,
		EndlessSpoolEnabled: endlessSpoolEnabled,
	}
}

// CurrentIndex returns the current ACE slot index from the persisted variable
// map, or -1 when unavailable.
func (self *ACERuntimeState) CurrentIndex() int {
	return aceIntValue(self.Variables["ace_current_index"], -1)
}

// SavedEndlessSpoolEnabled returns the persisted endless spool state.
func (self *ACERuntimeState) SavedEndlessSpoolEnabled() bool {
	return SavedEndlessSpoolEnabled(self.Variables)
}

func (self *ACERuntimeState) SetCurrentIndex(index int) {
	if self.Variables != nil {
		self.Variables["ace_current_index"] = index
	}
}

func (self *ACERuntimeState) BeginManualToolchange() bool {
	wasEnabled := self.EndlessSpoolEnabled
	if wasEnabled {
		self.EndlessSpoolEnabled = false
		self.EndlessSpoolRunoutDetected = false
	}
	return wasEnabled
}

func (self *ACERuntimeState) RestoreManualToolchange(wasEnabled bool) {
	if wasEnabled {
		self.EndlessSpoolEnabled = true
	}
}

func (self *ACERuntimeState) ToggleEndlessSpool(enabled bool) (saveValue string, response string) {
	self.EndlessSpoolEnabled = enabled
	if !enabled {
		self.EndlessSpoolRunoutDetected = false
		self.EndlessSpoolInProgress = false
	}
	saveValue = UpdateEndlessSpoolVariable(self.Variables, enabled)
	if enabled {
		return saveValue, "ACE: Endless spool enabled (immediate switching on runout)"
	}
	return saveValue, "ACE: Endless spool disabled (saved to persistent variables)"
}

func (self *ACERuntimeState) MarkRunoutIfTriggered(runoutHelperPresent, endstopTriggered bool) bool {
	if !self.EndlessSpoolEnabled || self.EndlessSpoolInProgress {
		return false
	}
	if !ShouldTriggerEndlessSpoolRunout(runoutHelperPresent, endstopTriggered) || self.EndlessSpoolRunoutDetected {
		return false
	}
	self.EndlessSpoolRunoutDetected = true
	return true
}

func (self *ACERuntimeState) PrepareEndlessSpoolChangePlan() (ACEEndlessSpoolChangePlan, error) {
	currentTool := self.CurrentIndex()
	nextTool, inventorySaveValue, inventoryChanged, err := PrepareEndlessSpoolChange(
		currentTool, self.Inventory, self.Info, self.Variables)
	if err != nil {
		return ACEEndlessSpoolChangePlan{}, err
	}
	return ACEEndlessSpoolChangePlan{
		CurrentTool:        currentTool,
		NextTool:           nextTool,
		InventorySaveValue: inventorySaveValue,
		InventoryChanged:   inventoryChanged,
	}, nil
}

func (self *ACERuntimeState) BeginEndlessSpoolChange() bool {
	if self.EndlessSpoolInProgress {
		return false
	}
	self.EndlessSpoolInProgress = true
	self.EndlessSpoolRunoutDetected = false
	return true
}

func (self *ACERuntimeState) CompleteEndlessSpoolChange(nextTool int) {
	self.SetCurrentIndex(nextTool)
	self.EndlessSpoolInProgress = false
}

func (self *ACERuntimeState) AbortEndlessSpoolChange() {
	self.EndlessSpoolInProgress = false
}

// ApplyFirmwareInfoResult records a successful get_info payload and returns the
// message that should be emitted to the user together with the ACE V2 mode flag.
func (self *ACERuntimeState) ApplyFirmwareInfoResult(result interface{}) (message string, useV2 bool) {
	resultMap, _ := result.(map[string]interface{})
	if resultMap == nil {
		self.FirmwareInfo = nil
		return "", false
	}
	self.FirmwareInfo = resultMap
	if model, ok := resultMap["model"].(string); ok && strings.Contains(model, "2.0") {
		useV2 = true
	}
	infoBytes, _ := json.Marshal(result)
	return fmt.Sprintf("ACE: Firmware info %s", aceFirmwareInfoLogString(infoBytes)), useV2
}

// SyncStatusResult reconciles a successful get_status payload with persisted
// custom slot metadata and writes any RFID-backed updates back to disk.
func (self *ACERuntimeState) SyncStatusResult(result map[string]interface{}) {
	for index, persistedSlot := range SyncACEStatusSlots(result, self.CustomSlots) {
		if self.ConfigPath != "" {
			PersistFilamentSlotConfig(index, persistedSlot, self.ConfigPath)
		}
	}
	self.Info = result
}

// BuildHubStatus returns the outward-facing filament hub status payload.
func (self *ACERuntimeState) BuildHubStatus() map[string]interface{} {
	return BuildACEHubStatus(
		self.Info,
		self.FirmwareInfo,
		self.EndlessSpoolEnabled,
		self.EndlessSpoolRunoutDetected,
		self.EndlessSpoolInProgress,
	)
}

// BuildEndlessSpoolStatusLines returns the user-facing status lines for the
// ACE_ENDLESS_SPOOL_STATUS command.
func (self *ACERuntimeState) BuildEndlessSpoolStatusLines() []string {
	return BuildEndlessSpoolStatusLines(
		self.EndlessSpoolEnabled,
		self.SavedEndlessSpoolEnabled(),
		self.EndlessSpoolRunoutDetected,
		self.EndlessSpoolInProgress,
	)
}

// BuildRunoutSensorStatusLines returns the user-facing status lines for the
// ACE_TEST_RUNOUT_SENSOR command.
func (self *ACERuntimeState) BuildRunoutSensorStatusLines(runoutHelperPresent, endstopTriggered bool) []string {
	return BuildRunoutSensorStatusLines(
		runoutHelperPresent,
		endstopTriggered,
		self.EndlessSpoolEnabled,
		self.Variables["ace_current_index"],
		self.EndlessSpoolRunoutDetected,
	)
}

// ApplyInventorySlotUpdate mutates the runtime inventory according to the
// ACE_SET_SLOT command and returns the SAVE_VARIABLE payload plus the user
// response line.
func (self *ACERuntimeState) ApplyInventorySlotUpdate(update ACEInventorySlotUpdate) (saveValue string, response string, err error) {
	if update.Index < 0 || update.Index >= 4 {
		return "", "", errors.New("invalid slot index")
	}

	if update.Empty {
		self.Inventory[update.Index] = EmptyInventorySlot()
		saveValue, err = self.SaveInventory()
		if err != nil {
			return "", "", err
		}
		return saveValue, fmt.Sprintf("ACE: Slot %d set to empty", update.Index), nil
	}

	if update.Color == "" || update.Material == "" || update.Temp <= 0 {
		return "", "", errors.New("COLOR, MATERIAL, TEMP must be set unless EMPTY=1")
	}

	color := ParseRGB(update.Color)
	if len(color) != 3 {
		return "", "", errors.New("COLOR must be R,G,B")
	}

	self.Inventory[update.Index] = ReadyInventorySlot(color, update.Material, update.Temp)
	saveValue, err = self.SaveInventory()
	if err != nil {
		return "", "", err
	}
	return saveValue, fmt.Sprintf("ACE: Slot %d set: color=%v, material=%s, temp=%d", update.Index, color, update.Material, update.Temp), nil
}

// SaveInventory persists the current inventory into the ACE variables map and
// returns the JSON payload expected by SAVE_VARIABLE.
func (self *ACERuntimeState) SaveInventory() (string, error) {
	return SaveInventoryState(self.Variables, self.Inventory)
}

// InventoryJSON returns the serialized inventory payload used by
// ACE_QUERY_SLOTS.
func (self *ACERuntimeState) InventoryJSON() (string, error) {
	data, err := json.Marshal(self.Inventory)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UpdatePanelFilamentInfo applies web panel metadata updates to the slot state
// and returns the slot payload that should be persisted to ams_config.cfg.
func (self *ACERuntimeState) UpdatePanelFilamentInfo(index int, typ string, color []interface{}) (persistedSlot map[string]interface{}, shouldPersist bool) {
	if index < 0 || index >= len(self.CustomSlots) {
		return nil, false
	}

	if self.Info != nil {
		if slots, ok := self.Info["slots"].([]interface{}); ok && index < len(slots) {
			if slotMap, ok := slots[index].(map[string]interface{}); ok && HasValidACESlotRFID(slotMap) {
				return map[string]interface{}{
					"type":      slotMap["type"],
					"color":     slotMap["color"],
					"colors":    slotMap["colors"],
					"sku":       slotMap["sku"],
					"rfid":      slotMap["rfid"],
					"source":    slotMap["source"],
					"icon_type": slotMap["icon_type"],
				}, true
			}
		}
	}

	updatedInfo, persistedSlot := UpdateACESlotInfo(self.CustomSlots[index], self.Info, index, typ, color)
	self.Info = updatedInfo
	return persistedSlot, true
}

// SetPanelFilamentInfo applies a panel-provided slot update and persists the
// effective slot metadata when a config path is configured.
func (self *ACERuntimeState) SetPanelFilamentInfo(index int, typ string, sku string, color []interface{}) {
	_ = sku
	persistedSlot, shouldPersist := self.UpdatePanelFilamentInfo(index, typ, color)
	if !shouldPersist {
		return
	}
	if self.Info != nil {
		if slots, ok := self.Info["slots"].([]interface{}); ok && index < len(slots) {
			if slotMap, ok := slots[index].(map[string]interface{}); ok && HasValidACESlotRFID(slotMap) {
				logger.Infof("Ignoring panel filament info for RFID-backed slot %d with sku=%v", index, slotMap["sku"])
			}
		}
	}
	if self.ConfigPath != "" {
		PersistFilamentSlotConfig(index, persistedSlot, self.ConfigPath)
	}
}

func aceFirmwareInfoLogString(infoBytes []byte) string {
	if len(infoBytes) > 3 {
		return string(infoBytes[1 : len(infoBytes)-2])
	}
	return string(infoBytes)
}

func aceIntValue(value interface{}, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}
