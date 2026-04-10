package filament

import (
	"encoding/json"
	"goklipper/common/logger"
	"os"
	"strconv"
)

// AmsConfigPath is the default path for the ACE AMS configuration file.
const AmsConfigPath = "/userdata/app/gk/config/ams_config.cfg"

// PersistFilamentSlotConfig reads the AMS config file at cfgPath, merges the
// given slotData into the slot at position index using MergeFilamentSlotConfig,
// and writes the result back. A no-op if the merged result matches the existing data.
func PersistFilamentSlotConfig(index int, slotData map[string]interface{}, cfgPath string) {
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
	persistedSlot := MergeFilamentSlotConfig(existingSlot, slotData)

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
