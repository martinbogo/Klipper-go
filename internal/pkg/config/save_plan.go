package config

import (
	"goklipper/common/configparser"
	"strings"
	"time"
)

type SaveConfigPlan struct {
	Data       string
	BackupName string
	TempName   string
}

func BuildSaveConfigPlan(cfgname, existingData string, autosave *configparser.RawConfigParser, now func() time.Time) (SaveConfigPlan, error) {
	regularData, _ := FindAutosaveData(existingData)
	ParseConfigText(regularData, cfgname)
	regularData = StripDuplicates(regularData, autosave)
	if err := ValidateAutosaveConflicts(regularData, cfgname, autosave); err != nil {
		return SaveConfigPlan{}, err
	}
	autosaveData := FormatAutosaveBlock(RenderConfig(autosave))
	data := strings.TrimSpace(regularData) + "\n" + autosaveData + "\n"
	datestr := now().Format("-20060102_150405")
	backupName := cfgname + datestr
	tempName := cfgname + "_autosave"
	if strings.HasSuffix(cfgname, ".cfg") {
		backupName = cfgname[:len(cfgname)-4] + datestr + ".cfg"
		tempName = cfgname[:len(cfgname)-4] + "_autosave.cfg"
	}
	return SaveConfigPlan{
		Data:       data,
		BackupName: backupName,
		TempName:   tempName,
	}, nil
}