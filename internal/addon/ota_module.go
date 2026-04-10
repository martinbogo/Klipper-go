package addon

import (
	"errors"
	"fmt"
	"strings"

	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/cast"
	printerpkg "goklipper/internal/pkg/printer"
)

const cmdOTAStartHelp = "start ota upgrade"

type otaCommand interface {
	Send(data interface{}, minclock int64, reqclock int64)
}

type otaQueryCommand interface {
	Send(data interface{}, minclock int64, reqclock int64) interface{}
}

type otaMCU interface {
	printerpkg.MCURuntime
	AllocCommandQueue() interface{}
	LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error)
	LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{}
	GetStatus(eventtime float64) map[string]interface{}
}

type otaGCode interface {
	printerpkg.GCodeRuntime
	RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string)
}

type otaReactor interface {
	printerpkg.ModuleReactor
	Pause(waketime float64) float64
}

type OTAModule struct {
	printer         printerpkg.ModulePrinter
	reactor         otaReactor
	mcu             otaMCU
	mcuName         string
	gcode           otaGCode
	sendOTADataCmd  otaCommand
	startOTACmd     otaCommand
	otaEraseCmd     otaCommand
	queryVersionCmd otaQueryCommand
	core            *Ota
	oid             int
	timeoutTimer    printerpkg.TimerHandle
}

func LoadConfigMCUOTA(config printerpkg.ModuleConfig) interface{} {
	return NewOTAModule(config)
}

func NewOTAModule(config printerpkg.ModuleConfig) *OTAModule {
	printerRef := config.Printer()
	reactorObj := printerRef.Reactor()
	reactor, ok := reactorObj.(otaReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement otaReactor: %T", reactorObj))
	}
	nameParts := strings.Fields(config.Name())
	if len(nameParts) < 2 {
		panic(fmt.Sprintf("invalid mcu_ota section %q", config.Name()))
	}
	mcuName := nameParts[1]
	mcuObj := printerRef.LookupMCU(mcuName)
	mcu, ok := mcuObj.(otaMCU)
	if !ok {
		panic(fmt.Sprintf("mcu runtime does not implement otaMCU: %T", mcuObj))
	}
	gcodeObj := printerRef.GCode()
	gcode, ok := gcodeObj.(otaGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement otaGCode: %T", gcodeObj))
	}
	self := &OTAModule{
		printer:      printerRef,
		reactor:      reactor,
		mcu:          mcu,
		mcuName:      mcuName,
		gcode:        gcode,
		core:         NewOta(mcuName),
		timeoutTimer: nil,
	}
	self.mcu.RegisterConfigCallback(self.buildConfig)
	self.printer.RegisterEventHandler("project:ready", self.handleReady)
	self.gcode.RegisterMuxCommand("OTA_START", "MCU", self.mcuName, self.cmdOTAStart, cmdOTAStartHelp)
	return self
}

func (self *OTAModule) buildConfig() {
	self.oid = self.mcu.CreateOID()
	commandQueue := self.mcu.AllocCommandQueue()

	self.mcu.AddConfigCmd(fmt.Sprintf("config_ota oid=%d ", self.oid), false, false)
	self.sendOTADataCmd = self.lookupCommand("ota_transfer_response oid=%c offset=%hu data=%*s", commandQueue)
	self.otaEraseCmd = self.lookupCommand("ota_erase oid=%c offset=%u is_transfer=%c", commandQueue)
	self.mcu.RegisterResponse(self.handleOTATransfer, "ota_transfer", self.oid)
	self.mcu.RegisterResponse(self.handleOTAStatus, "ota_status", self.oid)
	self.mcu.RegisterResponse(self.handleOTALocalInfo, "ota_local_info", self.oid)
	self.startOTACmd = self.lookupCommand("ota_start oid=%c crc32=%u version_major=%c version_minor=%c version_patch=%c", commandQueue)
	self.queryVersionCmd = self.lookupQueryCommand(
		"query_ota_local_info oid=%c",
		"ota_local_info oid=%c flag=%c crc32=%u version_major=%c version_minor=%c version_patch=%c",
		self.oid,
		commandQueue,
		false,
	)
}

func (self *OTAModule) lookupCommand(msgformat string, commandQueue interface{}) otaCommand {
	command, err := self.mcu.LookupCommandRaw(msgformat, commandQueue)
	if err != nil {
		panic(err)
	}
	typed, ok := command.(otaCommand)
	if !ok {
		panic(fmt.Sprintf("command does not implement otaCommand: %T", command))
	}
	return typed
}

func (self *OTAModule) lookupQueryCommand(msgformat string, respformat string, oid int, commandQueue interface{}, isAsync bool) otaQueryCommand {
	command := self.mcu.LookupQueryCommand(msgformat, respformat, oid, commandQueue, isAsync)
	typed, ok := command.(otaQueryCommand)
	if !ok {
		panic(fmt.Sprintf("query command does not implement otaQueryCommand: %T", command))
	}
	return typed
}

func (self *OTAModule) handleReady([]interface{}) error {
	version, _ := self.mcu.GetStatus(0)["mcu_version"].(string)
	return self.core.HandleReady(version)
}

func (self *OTAModule) handleTimeout(float64) float64 {
	if self.core.ShouldTimeout() {
		self.core.SetTimeout(true)
		if self.timeoutTimer != nil {
			self.timeoutTimer.Update(constants.NEVER)
			self.timeoutTimer = nil
		}
	}
	return constants.NEVER
}

func (self *OTAModule) handleOTATransfer(params map[string]interface{}) error {
	if self.core.IsTimeout() {
		return errors.New("ota transfer timeout")
	}
	logger.Debugf("ota_transfer handle")
	offset := cast.ToInt(params["offset"])
	count := cast.ToInt(params["count"])
	chunk, err := self.core.BuildTransferChunk(offset, count)
	if err != nil {
		if self.core.State() == "open file error" {
			self.sendOTADataCmd.Send([]interface{}{int64(self.oid)}, 0, 0)
		}
		return err
	}
	self.sendOTADataCmd.Send([]interface{}{int64(self.oid), chunk.NextOffset, chunk.Data}, 0, 0)
	if self.timeoutTimer != nil {
		self.timeoutTimer.Update(self.reactor.Monotonic() + 5.0)
	}
	if chunk.Finished {
		logger.Debugf("ota transfer_finish")
	}
	return nil
}

func (self *OTAModule) handleOTAStatus(params map[string]interface{}) error {
	status := cast.ToInt(params["status"])
	errCode := cast.ToInt(params["err_code"])
	logger.Debug("status:", self.core.State())
	self.core.HandleStatus(status, errCode)
	return nil
}

func (self *OTAModule) handleOTALocalInfo(params map[string]interface{}) error {
	self.core.SetLocalInfo(params)
	return nil
}

func (self *OTAModule) cmdOTAStart(gcmd printerpkg.Command) error {
	updatePath := gcmd.String("UPDATE_PATH", "./firmware_v0.1.1_20230712.bin")
	version, err := self.core.BeginUpdate(updatePath)
	if err != nil {
		return err
	}
	crc32, err := self.core.FirmwareCRC()
	if err != nil {
		logger.Error(err)
		return err
	}
	versionParts, err := self.validateVersion(version, crc32)
	if err != nil {
		self.core.SetState("not_need_upgrad")
		logger.Error(err)
		return nil
	}
	logger.Debugf("crc32: %v version0:%v version1:%v version2:%v", crc32, versionParts[0], versionParts[1], versionParts[2])
	if self.timeoutTimer == nil {
		self.timeoutTimer = self.reactor.RegisterTimer(self.handleTimeout, self.reactor.Monotonic()+10.0)
	} else {
		self.timeoutTimer.Update(self.reactor.Monotonic() + 10.0)
	}
	self.startOTACmd.Send([]int64{int64(self.oid), int64(crc32), int64(versionParts[0]), int64(versionParts[1]), int64(versionParts[2])}, 0, 0)
	self.core.MarkDownloadStarted()
	if self.otaEraseCmd != nil {
		self.otaEraseCmd.Send([]int64{int64(self.oid), 0, 1}, 0, 0)
	}
	for {
		gcmd.RespondRaw(fmt.Sprintf("progress = %.1f%%", self.core.Progress()*100))
		eventtime := self.reactor.Monotonic()
		self.reactor.Pause(eventtime + 0.1)
		if self.printer.IsShutdown() {
			break
		}
		state := self.core.State()
		if state == "upgrading_restart" {
			return nil
		}
		if strings.Contains(state, "error") {
			return errors.New(state)
		}
		if state == "writing" && self.core.IsTimeout() {
			return errors.New("ota transfer timeout")
		}
		if state == "transfer_finish" && self.core.IsTimeout() {
			return errors.New("MCU responds to ota transfer finish timeout")
		}
		if self.core.IsTimeout() {
			return errors.New("wait for mcu erase flash timeout")
		}
	}
	return nil
}

func (self *OTAModule) GetStatus(eventtime float64) map[string]interface{} {
	status := self.mcu.GetStatus(eventtime)
	return self.core.GetStatus(status["mcu_version"])
}

func (self *OTAModule) Get_Status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

func (self *OTAModule) out_put_version_file(newVersion string) error {
	return self.core.StorePendingVersion(newVersion)
}

func (self *OTAModule) validateVersion(newVersion string, crc32 uint32) ([]int, error) {
	if self.core.LocalInfo() == nil {
		if self.queryVersionCmd == nil {
			return nil, fmt.Errorf("ota query version command not configured")
		}
		localInfo := self.queryVersionCmd.Send([]interface{}{int64(self.oid)}, 0, 0)
		infoMap, ok := localInfo.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid ota local info")
		}
		self.core.SetLocalInfo(infoMap)
	}
	localCRC := int64(0)
	if infoMap := self.core.LocalInfo(); infoMap != nil {
		if value, ok := infoMap["crc32"]; ok {
			switch typed := value.(type) {
			case int64:
				localCRC = typed
			case int:
				localCRC = int64(typed)
			case float64:
				localCRC = int64(typed)
			}
		}
	}
	currentVersion, _ := self.mcu.GetStatus(0)["mcu_version"].(string)
	return self.core.ValidateVersion(newVersion, currentVersion, crc32, localCRC)
}

func (self *OTAModule) get_update_version() string {
	return self.core.PendingVersion()
}

func (self *OTAModule) ota_update_state(stats string) error {
	err := self.core.UpdateState(stats)
	if err == nil {
		logger.Debug("ota firmware update_success")
	}
	return err
}