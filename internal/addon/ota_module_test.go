package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	printerpkg "goklipper/internal/pkg/printer"
)

type fakeOTASendCall struct {
	data     interface{}
	minclock int64
	reqclock int64
}

type fakeOTARawCommand struct {
	sends        []fakeOTASendCall
	commandQueue interface{}
}

func (self *fakeOTARawCommand) Send(data interface{}, minclock int64, reqclock int64) {
	self.sends = append(self.sends, fakeOTASendCall{data: data, minclock: minclock, reqclock: reqclock})
}

type fakeOTAQueryCommand struct {
	sends    []fakeOTASendCall
	response interface{}
}

func (self *fakeOTAQueryCommand) Send(data interface{}, minclock int64, reqclock int64) interface{} {
	self.sends = append(self.sends, fakeOTASendCall{data: data, minclock: minclock, reqclock: reqclock})
	return self.response
}

type fakeOTATimer struct {
	callback    func(float64) float64
	waketime    float64
	updateCalls []float64
}

func (self *fakeOTATimer) Update(waketime float64) {
	self.waketime = waketime
	self.updateCalls = append(self.updateCalls, waketime)
}

type fakeOTAReactor struct {
	monotonic float64
	timers    []*fakeOTATimer
	pauseCalls []float64
}

func (self *fakeOTAReactor) RegisterTimer(callback func(float64) float64, waketime float64) printerpkg.TimerHandle {
	timer := &fakeOTATimer{callback: callback, waketime: waketime}
	self.timers = append(self.timers, timer)
	return timer
}

func (self *fakeOTAReactor) Monotonic() float64 {
	return self.monotonic
}

func (self *fakeOTAReactor) Pause(waketime float64) float64 {
	self.pauseCalls = append(self.pauseCalls, waketime)
	return waketime
}

type fakeOTAGCodeMuxCall struct {
	cmd     string
	key     string
	value   string
	desc    string
	handler func(printerpkg.Command) error
}

type fakeOTAGCode struct {
	muxCalls []fakeOTAGCodeMuxCall
}

func (self *fakeOTAGCode) RegisterCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) {
}

func (self *fakeOTAGCode) RegisterMuxCommand(cmd string, key string, value string, handler func(printerpkg.Command) error, desc string) {
	self.muxCalls = append(self.muxCalls, fakeOTAGCodeMuxCall{cmd: cmd, key: key, value: value, desc: desc, handler: handler})
}

func (self *fakeOTAGCode) IsTraditionalGCode(cmd string) bool {
	return false
}

func (self *fakeOTAGCode) RunScriptFromCommand(script string) {
}

func (self *fakeOTAGCode) RunScript(script string) {
}

func (self *fakeOTAGCode) IsBusy() bool {
	return false
}

func (self *fakeOTAGCode) Mutex() printerpkg.Mutex {
	return &fakeOTAMutex{}
}

func (self *fakeOTAGCode) RespondInfo(msg string, log bool) {
}

func (self *fakeOTAGCode) ReplaceCommand(cmd string, handler func(printerpkg.Command) error, whenNotReady bool, desc string) func(printerpkg.Command) error {
	return nil
}

type fakeOTAMutex struct{}

func (self *fakeOTAMutex) Lock() {
}

func (self *fakeOTAMutex) Unlock() {
}

type fakeOTAResponseRegistration struct {
	msg string
	oid interface{}
}

type fakeOTAMCU struct {
	configCallback func()
	nextOID        int
	configCmds     []string
	responses      []fakeOTAResponseRegistration
	status         map[string]interface{}
	commandQueue   interface{}
	rawCommands    map[string]*fakeOTARawCommand
	queryCommands  map[string]*fakeOTAQueryCommand
	lookupQueries  []string
}

func newFakeOTAMCU() *fakeOTAMCU {
	return &fakeOTAMCU{
		nextOID:       1,
		status:        map[string]interface{}{"mcu_version": "\"v1.0.0\""},
		commandQueue:  "ota-command-queue",
		rawCommands:   map[string]*fakeOTARawCommand{},
		queryCommands: map[string]*fakeOTAQueryCommand{},
	}
}

func (self *fakeOTAMCU) CreateOID() int {
	oid := self.nextOID
	self.nextOID++
	return oid
}

func (self *fakeOTAMCU) RegisterConfigCallback(cb func()) {
	self.configCallback = cb
}

func (self *fakeOTAMCU) AddConfigCmd(cmd string, isInit bool, onRestart bool) {
	self.configCmds = append(self.configCmds, cmd)
}

func (self *fakeOTAMCU) GetQuerySlot(oid int) int64 {
	return 0
}

func (self *fakeOTAMCU) SecondsToClock(time float64) int64 {
	return 0
}

func (self *fakeOTAMCU) RegisterResponse(cb func(map[string]interface{}) error, msg string, oid interface{}) {
	self.responses = append(self.responses, fakeOTAResponseRegistration{msg: msg, oid: oid})
}

func (self *fakeOTAMCU) ClockToPrintTime(clock int64) float64 {
	return 0
}

func (self *fakeOTAMCU) Clock32ToClock64(clock32 int64) int64 {
	return clock32
}

func (self *fakeOTAMCU) AllocCommandQueue() interface{} {
	return self.commandQueue
}

func (self *fakeOTAMCU) LookupCommandRaw(msgformat string, cq interface{}) (interface{}, error) {
	command := self.rawCommands[msgformat]
	if command == nil {
		command = &fakeOTARawCommand{}
		self.rawCommands[msgformat] = command
	}
	command.commandQueue = cq
	return command, nil
}

func (self *fakeOTAMCU) LookupQueryCommand(msgformat string, respformat string, oid int, cq interface{}, isAsync bool) interface{} {
	self.lookupQueries = append(self.lookupQueries, msgformat+"|"+respformat)
	command := self.queryCommands[msgformat]
	if command == nil {
		command = &fakeOTAQueryCommand{}
		self.queryCommands[msgformat] = command
	}
	return command
}

func (self *fakeOTAMCU) GetStatus(eventtime float64) map[string]interface{} {
	return self.status
}

type fakeOTAPrinter struct {
	reactor       *fakeOTAReactor
	gcode         *fakeOTAGCode
	mcu           *fakeOTAMCU
	eventHandlers map[string]func([]interface{}) error
	shutdown      bool
	shutdownMsg   string
}

func newFakeOTAPrinter() *fakeOTAPrinter {
	return &fakeOTAPrinter{
		reactor:       &fakeOTAReactor{monotonic: 12.5},
		gcode:         &fakeOTAGCode{},
		mcu:           newFakeOTAMCU(),
		eventHandlers: map[string]func([]interface{}) error{},
	}
}

func (self *fakeOTAPrinter) LookupObject(name string, defaultValue interface{}) interface{} {
	return defaultValue
}

func (self *fakeOTAPrinter) RegisterEventHandler(event string, callback func([]interface{}) error) {
	self.eventHandlers[event] = callback
}

func (self *fakeOTAPrinter) SendEvent(event string, params []interface{}) {
}

func (self *fakeOTAPrinter) CurrentExtruderName() string {
	return "extruder"
}

func (self *fakeOTAPrinter) AddObject(name string, obj interface{}) error {
	return nil
}

func (self *fakeOTAPrinter) LookupObjects(module string) []interface{} {
	return nil
}

func (self *fakeOTAPrinter) HasStartArg(name string) bool {
	return false
}

func (self *fakeOTAPrinter) LookupHeater(name string) printerpkg.HeaterRuntime {
	return nil
}

func (self *fakeOTAPrinter) TemperatureSensors() printerpkg.TemperatureSensorRegistry {
	return nil
}

func (self *fakeOTAPrinter) LookupMCU(name string) printerpkg.MCURuntime {
	return self.mcu
}

func (self *fakeOTAPrinter) InvokeShutdown(msg string) {
	self.shutdownMsg = msg
	self.shutdown = true
}

func (self *fakeOTAPrinter) IsShutdown() bool {
	return self.shutdown
}

func (self *fakeOTAPrinter) Reactor() printerpkg.ModuleReactor {
	return self.reactor
}

func (self *fakeOTAPrinter) StepperEnable() printerpkg.StepperEnableRuntime {
	return nil
}

func (self *fakeOTAPrinter) GCode() printerpkg.GCodeRuntime {
	return self.gcode
}

func (self *fakeOTAPrinter) GCodeMove() printerpkg.MoveTransformController {
	return nil
}

func (self *fakeOTAPrinter) Webhooks() printerpkg.WebhookRegistry {
	return nil
}

type fakeOTAConfig struct {
	name    string
	printer *fakeOTAPrinter
}

func (self *fakeOTAConfig) Name() string {
	return self.name
}

func (self *fakeOTAConfig) String(option string, defaultValue string, noteValid bool) string {
	return defaultValue
}

func (self *fakeOTAConfig) Bool(option string, defaultValue bool) bool {
	return defaultValue
}

func (self *fakeOTAConfig) Float(option string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeOTAConfig) OptionalFloat(option string) *float64 {
	return nil
}

func (self *fakeOTAConfig) LoadObject(section string) interface{} {
	return nil
}

func (self *fakeOTAConfig) LoadTemplate(module string, option string, defaultValue string) printerpkg.Template {
	return nil
}

func (self *fakeOTAConfig) LoadRequiredTemplate(module string, option string) printerpkg.Template {
	return nil
}

func (self *fakeOTAConfig) Printer() printerpkg.ModulePrinter {
	return self.printer
}

type fakeOTAGCodeCommand struct {
	params       map[string]string
	rawResponses []string
	infoResponses []string
}

func (self *fakeOTAGCodeCommand) String(name string, defaultValue string) string {
	value, ok := self.params[name]
	if !ok {
		return defaultValue
	}
	return value
}

func (self *fakeOTAGCodeCommand) Float(name string, defaultValue float64) float64 {
	return defaultValue
}

func (self *fakeOTAGCodeCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return defaultValue
}

func (self *fakeOTAGCodeCommand) Parameters() map[string]string {
	params := map[string]string{}
	for key, value := range self.params {
		params[key] = value
	}
	return params
}

func (self *fakeOTAGCodeCommand) RespondInfo(msg string, log bool) {
	self.infoResponses = append(self.infoResponses, msg)
}

func (self *fakeOTAGCodeCommand) RespondRaw(msg string) {
	self.rawResponses = append(self.rawResponses, msg)
}

func newFakeOTAModule(t *testing.T) (*OTAModule, *fakeOTAPrinter) {
	t.Helper()
	printer := newFakeOTAPrinter()
	config := &fakeOTAConfig{name: "mcu_ota mcu", printer: printer}
	module := LoadConfigMCUOTA(config).(*OTAModule)
	module.core = NewOtaWithVersionFile(module.mcuName, filepath.Join(t.TempDir(), "version"))
	return module, printer
}

func TestLoadConfigMCUOTARegistersMuxCommandAndBuildConfig(t *testing.T) {
	module, printer := newFakeOTAModule(t)
	if printer.mcu.configCallback == nil {
		t.Fatalf("RegisterConfigCallback() was not called")
	}
	if _, ok := printer.eventHandlers["project:ready"]; !ok {
		t.Fatalf("project:ready handler was not registered")
	}
	if len(printer.gcode.muxCalls) != 1 {
		t.Fatalf("RegisterMuxCommand() calls = %d, want 1", len(printer.gcode.muxCalls))
	}
	mux := printer.gcode.muxCalls[0]
	if mux.cmd != "OTA_START" || mux.key != "MCU" || mux.value != "mcu" {
		t.Fatalf("RegisterMuxCommand() = %+v", mux)
	}

	printer.mcu.configCallback()
	if module.oid != 1 {
		t.Fatalf("buildConfig() oid = %d, want 1", module.oid)
	}
	if got := printer.mcu.configCmds; !reflect.DeepEqual(got, []string{"config_ota oid=1 "}) {
		t.Fatalf("AddConfigCmd() = %v", got)
	}
	if len(printer.mcu.responses) != 3 {
		t.Fatalf("RegisterResponse() calls = %d, want 3", len(printer.mcu.responses))
	}
	if _, ok := printer.mcu.rawCommands["ota_transfer_response oid=%c offset=%hu data=%*s"]; !ok {
		t.Fatalf("ota transfer command was not looked up")
	}
	if _, ok := printer.mcu.queryCommands["query_ota_local_info oid=%c"]; !ok {
		t.Fatalf("query ota local info command was not looked up")
	}
}

func TestOTAModuleCmdOTAStartSendsStartAndEraseCommands(t *testing.T) {
	module, printer := newFakeOTAModule(t)
	printer.mcu.configCallback()
	printer.shutdown = true

	firmwarePath := filepath.Join(t.TempDir(), "firmware_v1.2.3_20240101.bin")
	if err := os.WriteFile(firmwarePath, []byte("abcdefghijklm"), 0o644); err != nil {
		t.Fatalf("WriteFile(firmware) failed: %v", err)
	}
	expectedCore := NewOtaWithVersionFile(module.mcuName, filepath.Join(t.TempDir(), "expected-version"))
	if _, err := expectedCore.BeginUpdate(firmwarePath); err != nil {
		t.Fatalf("BeginUpdate(expected) failed: %v", err)
	}
	expectedCRC, err := expectedCore.FirmwareCRC()
	if err != nil {
		t.Fatalf("FirmwareCRC(expected) failed: %v", err)
	}
	printer.mcu.queryCommands["query_ota_local_info oid=%c"].response = map[string]interface{}{"crc32": int64(77)}
	gcmd := &fakeOTAGCodeCommand{params: map[string]string{"UPDATE_PATH": firmwarePath}}

	if err := module.cmdOTAStart(gcmd); err != nil {
		t.Fatalf("cmdOTAStart() unexpected error: %v", err)
	}
	startCalls := printer.mcu.rawCommands["ota_start oid=%c crc32=%u version_major=%c version_minor=%c version_patch=%c"].sends
	if len(startCalls) != 1 {
		t.Fatalf("ota_start sends = %d, want 1", len(startCalls))
	}
	if got, want := startCalls[0].data, []int64{1, int64(expectedCRC), 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ota_start send = %#v, want %#v", got, want)
	}
	eraseCalls := printer.mcu.rawCommands["ota_erase oid=%c offset=%u is_transfer=%c"].sends
	if len(eraseCalls) != 1 {
		t.Fatalf("ota_erase sends = %d, want 1", len(eraseCalls))
	}
	if got, want := eraseCalls[0].data, []int64{1, 0, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ota_erase send = %#v, want %#v", got, want)
	}
	queryCalls := printer.mcu.queryCommands["query_ota_local_info oid=%c"].sends
	if len(queryCalls) != 1 {
		t.Fatalf("query_ota_local_info sends = %d, want 1", len(queryCalls))
	}
	if got, want := queryCalls[0].data, []interface{}{int64(1)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("query_ota_local_info send = %#v, want %#v", got, want)
	}
	if got := module.core.PendingVersion(); got != "\"v1.2.3\"" {
		t.Fatalf("PendingVersion() = %q, want %q", got, "\"v1.2.3\"")
	}
	if len(gcmd.rawResponses) == 0 || !strings.Contains(gcmd.rawResponses[0], "progress = ") {
		t.Fatalf("RespondRaw() calls = %v", gcmd.rawResponses)
	}
	if len(printer.reactor.timers) != 1 {
		t.Fatalf("RegisterTimer() calls = %d, want 1", len(printer.reactor.timers))
	}
}

func TestOTAModuleHandleOTATransferSendsMixedPayloadAndFinalChunk(t *testing.T) {
	module, printer := newFakeOTAModule(t)
	printer.mcu.configCallback()
	firmwarePath := filepath.Join(t.TempDir(), "firmware_v1.2.3_20240101.bin")
	if err := os.WriteFile(firmwarePath, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile(firmware) failed: %v", err)
	}
	if _, err := module.core.BeginUpdate(firmwarePath); err != nil {
		t.Fatalf("BeginUpdate() failed: %v", err)
	}
	timeoutTimer := &fakeOTATimer{}
	module.timeoutTimer = timeoutTimer

	if err := module.handleOTATransfer(map[string]interface{}{"offset": int64(0), "count": int64(10)}); err != nil {
		t.Fatalf("handleOTATransfer(first) unexpected error: %v", err)
	}
	transferCmd := printer.mcu.rawCommands["ota_transfer_response oid=%c offset=%hu data=%*s"]
	if len(transferCmd.sends) != 1 {
		t.Fatalf("ota_transfer_response sends after first chunk = %d, want 1", len(transferCmd.sends))
	}
	if got, want := transferCmd.sends[0].data, []interface{}{int64(1), 3, []int64{97, 98, 99}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first ota_transfer_response send = %#v, want %#v", got, want)
	}
	if got, want := timeoutTimer.updateCalls, []float64{printer.reactor.monotonic + 5.0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("timeout timer updates after first chunk = %v, want %v", got, want)
	}

	if err := module.handleOTATransfer(map[string]interface{}{"offset": int64(3), "count": int64(10)}); err != nil {
		t.Fatalf("handleOTATransfer(second) unexpected error: %v", err)
	}
	if len(transferCmd.sends) != 2 {
		t.Fatalf("ota_transfer_response sends after second chunk = %d, want 2", len(transferCmd.sends))
	}
	if got, want := transferCmd.sends[1].data, []interface{}{int64(1), 3, []int64{}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("final ota_transfer_response send = %#v, want %#v", got, want)
	}
	if module.core.State() != "transfer_finish" {
		t.Fatalf("core state after final chunk = %q, want %q", module.core.State(), "transfer_finish")
	}
	if got, want := timeoutTimer.updateCalls, []float64{printer.reactor.monotonic + 5.0, printer.reactor.monotonic + 5.0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("timeout timer updates = %v, want %v", got, want)
	}
}