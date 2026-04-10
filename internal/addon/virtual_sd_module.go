package addon

import (
	"errors"
	"fmt"
	"goklipper/common/constants"
	kerror "goklipper/common/errors"
	"goklipper/common/logger"
	printerpkg "goklipper/internal/pkg/printer"
	"goklipper/internal/pkg/util"
	"reflect"
	"runtime/debug"
	"strings"
)

const cmdSDCardResetFileHelp = "Clears a loaded SD File. Stops the print if necessary"
const cmdSDCardPrintFileHelp = "Loads a SD file and starts the print.  May include files in subdirectories."

type virtualSDCommand interface {
	printerpkg.Command
	RawCommandParameters() string
}

type virtualSDGCode interface {
	printerpkg.GCodeRuntime
	RespondRaw(msg string)
}

type virtualSDReactor interface {
	printerpkg.ModuleReactor
	Pause(waketime float64) float64
}

type virtualSDPrintStats interface {
	Set_current_file(filename string)
	Note_start()
	Note_pause()
	Note_complete()
	Note_error(message string)
	Note_cancel()
	Reset()
}

type VirtualSDModule struct {
	printer      printerpkg.ModulePrinter
	reactor      virtualSDReactor
	gcode        virtualSDGCode
	printStats   virtualSDPrintStats
	onErrorGCode printerpkg.Template
	workTimer    printerpkg.TimerHandle
	core         *VirtualSD
}

func LoadConfigVirtualSD(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	reactorObj := printer.Reactor()
	reactor, ok := reactorObj.(virtualSDReactor)
	if !ok {
		panic(fmt.Sprintf("reactor does not implement virtualSDReactor: %T", reactorObj))
	}
	gcodeObj := printer.GCode()
	gcode, ok := gcodeObj.(virtualSDGCode)
	if !ok {
		panic(fmt.Sprintf("gcode runtime does not implement virtualSDGCode: %T", gcodeObj))
	}
	printStatsObj := config.LoadObject("print_stats")
	printStats, ok := printStatsObj.(virtualSDPrintStats)
	if !ok {
		panic(fmt.Sprintf("print_stats object does not implement virtualSDPrintStats: %T", printStatsObj))
	}
	self := &VirtualSDModule{
		printer:      printer,
		reactor:      reactor,
		gcode:        gcode,
		printStats:   printStats,
		onErrorGCode: config.LoadTemplate("gcode_macro_1", "on_error_gcode", ""),
		workTimer:    nil,
		core:         NewVirtualSD(util.Normpath(util.ExpandUser(config.String("path", "", true)))),
	}
	self.printer.RegisterEventHandler("project:shutdown", self.Handle_shutdown)
	for _, cmd := range []struct {
		name    string
		handler func(printerpkg.Command) error
	}{
		{name: "M20", handler: self.Cmd_M20},
		{name: "M21", handler: self.Cmd_M21},
		{name: "M23", handler: self.Cmd_M23},
		{name: "M24", handler: self.Cmd_M24},
		{name: "M25", handler: self.Cmd_M25},
		{name: "M26", handler: self.Cmd_M26},
		{name: "M27", handler: self.Cmd_M27},
	} {
		self.gcode.RegisterCommand(cmd.name, cmd.handler, false, "")
	}
	for _, cmd := range []string{"M28", "M29", "M30"} {
		self.gcode.RegisterCommand(cmd, self.Cmd_error, false, "")
	}
	self.gcode.RegisterCommand("SDCARD_RESET_FILE", self.Cmd_SDCARD_RESET_FILE, false, cmdSDCardResetFileHelp)
	self.gcode.RegisterCommand("SDCARD_PRINT_FILE", self.Cmd_SDCARD_PRINT_FILE, false, cmdSDCardPrintFileHelp)
	return self
}

func (self *VirtualSDModule) Handle_shutdown([]interface{}) error {
	if self.workTimer != nil {
		self.core.MustPauseWork = true
	}
	readpos, previous, upcoming, err := self.core.PreviewWindow(1024, 128)
	if err != nil {
		panic("virtual_sdcard shutdown read")
	}
	logger.Info("Virtual sdcard (%f): %s\nUpcoming (%d): %s", readpos, previous, self.core.FilePosition, upcoming)
	return nil
}

func (self *VirtualSDModule) Stats(eventtime float64) (bool, string) {
	if self.workTimer != nil {
		return false, ""
	}
	return true, fmt.Sprintf("sd_pos=%d", self.core.FilePosition)
}

func (self *VirtualSDModule) GetFileList(checkSubdirs bool) []FileEntry {
	entries, err := self.core.GetFileList(checkSubdirs)
	if err != nil {
		logger.Error(err)
		return []FileEntry{}
	}
	return entries
}

func (self *VirtualSDModule) Get_file_list(checkSubdirs bool) []FileEntry {
	return self.GetFileList(checkSubdirs)
}

func (self *VirtualSDModule) GetStatus(eventtime float64) map[string]interface{} {
	return self.core.GetStatus(self.Is_active())
}

func (self *VirtualSDModule) Get_status(eventtime float64) map[string]interface{} {
	return self.GetStatus(eventtime)
}

func (self *VirtualSDModule) FilePath() string {
	return self.core.FilePath()
}

func (self *VirtualSDModule) File_path() string {
	return self.FilePath()
}

func (self *VirtualSDModule) Progress() float64 {
	return self.core.Progress()
}

func (self *VirtualSDModule) Is_active() bool {
	return self.workTimer != nil
}

func (self *VirtualSDModule) Do_pause() {
	if self.workTimer != nil {
		self.core.MustPauseWork = true
		for self.workTimer != nil && !self.core.CmdFromSD {
			self.reactor.Pause(self.reactor.Monotonic() + .001)
		}
	}
}

func (self *VirtualSDModule) Do_resume() error {
	if self.workTimer != nil {
		return errors.New("SD busy")
	}
	self.core.MustPauseWork = false
	self.workTimer = self.reactor.RegisterTimer(self.Work_handler, constants.NOW)
	return nil
}

func (self *VirtualSDModule) Do_cancel() {
	if self.core.CurrentFile != nil {
		self.Do_pause()
		_ = self.core.CloseCurrentFile()
		self.printStats.Note_cancel()
	}
	self.core.Reset()
}

func (self *VirtualSDModule) Cmd_error(printerpkg.Command) error {
	return errors.New("SD write not supported")
}

func (self *VirtualSDModule) Reset_file() {
	if self.core.CurrentFile != nil {
		self.Do_pause()
		_ = self.core.CloseCurrentFile()
	}
	self.core.Reset()
	self.printStats.Reset()
	self.printer.SendEvent("virtual_sdcard:reset_file", nil)
}

func (self *VirtualSDModule) Cmd_SDCARD_RESET_FILE(gcmd printerpkg.Command) error {
	if self.core.CmdFromSD {
		return errors.New("SDCARD_RESET_FILE cannot be run from the sdcard")
	}
	self.Reset_file()
	_ = gcmd
	return nil
}

func (self *VirtualSDModule) Cmd_SDCARD_PRINT_FILE(gcmd printerpkg.Command) error {
	if self.workTimer != nil {
		panic("SD busy")
	}
	self.Reset_file()
	if err := self.Load_file(gcmd, gcmd.String("FILENAME", ""), true); err != nil {
		return err
	}
	if err := self.Do_resume(); err != nil {
		logger.Error(err)
	}
	return nil
}

func (self *VirtualSDModule) Cmd_M20(gcmd printerpkg.Command) error {
	files := self.GetFileList(false)
	gcmd.RespondRaw("Begin file list")
	for _, item := range files {
		gcmd.RespondRaw(fmt.Sprintf("%s %d", item.Path, item.Size))
	}
	gcmd.RespondRaw("End file list")
	return nil
}

func (self *VirtualSDModule) Cmd_M21(gcmd printerpkg.Command) error {
	gcmd.RespondRaw("SD card ok")
	return nil
}

func (self *VirtualSDModule) Cmd_M23(gcmd printerpkg.Command) error {
	if self.workTimer != nil {
		return errors.New("SD busy")
	}
	self.Reset_file()
	filename := strings.TrimSpace(rawCommandParameters(gcmd))
	filename = strings.TrimPrefix(filename, "/")
	return self.Load_file(gcmd, filename, true)
}

func (self *VirtualSDModule) Load_file(gcmd printerpkg.Command, filename string, checkSubdirs bool) error {
	filename = strings.Trim(filename, "\"")
	logger.Debug("Load gcode file: ", filename)
	selected, err := self.core.LoadFile(filename)
	if err != nil {
		return err
	}
	logger.Debugf("File opened:%s Size:%d", selected, self.core.FileSize)
	logger.Debug("File selected")
	self.printStats.Set_current_file(selected)
	_ = gcmd
	_ = checkSubdirs
	return nil
}

func (self *VirtualSDModule) Cmd_M24(printerpkg.Command) error {
	err := self.Do_resume()
	if err != nil {
		logger.Error(err)
	}
	return err
}

func (self *VirtualSDModule) Cmd_M25(printerpkg.Command) error {
	self.Do_pause()
	return nil
}

func (self *VirtualSDModule) Cmd_M26(gcmd printerpkg.Command) error {
	if self.workTimer != nil {
		panic("SD busy")
	}
	minval := 0
	self.core.FilePosition = gcmd.Int("S", 0, &minval, &minval)
	return nil
}

func (self *VirtualSDModule) Cmd_M27(gcmd printerpkg.Command) error {
	if self.core.CurrentFile == nil {
		gcmd.RespondRaw("Not SD printing.")
		return nil
	}
	gcmd.RespondRaw(fmt.Sprintf("SD printing byte %d/%d", self.core.FilePosition, self.core.FileSize))
	return nil
}

func (self *VirtualSDModule) Get_file_position() int {
	return self.core.NextFilePosition
}

func (self *VirtualSDModule) Set_file_position(pos int) {
	self.core.NextFilePosition = pos
}

func (self *VirtualSDModule) Is_cmd_from_sd() bool {
	return self.core.CmdFromSD
}

func (self *VirtualSDModule) Work_handler(eventtime float64) float64 {
	logger.Debugf("Starting SD card print (position %d)", self.core.FilePosition)
	if err := self.core.SeekToFilePosition(); err != nil {
		logger.Error("virtual_sdcard seek", err.Error())
		self.workTimer = nil
		_ = self.core.CloseCurrentFile()
		return constants.NEVER
	}
	errorMessage := ""
	self.printStats.Note_start()
	partialInput := ""
	lines := []string{}

	for !self.core.MustPauseWork {
		if len(lines) == 0 {
			readLines, newPartial, eof, err := self.core.ReadLines(partialInput)
			if err != nil {
				logger.Error("virtual_sdcard", err)
				_ = self.core.CloseCurrentFile()
				self.gcode.RespondRaw("Read printing file error")
				errorMessage = "Printing file error"
				break
			}
			partialInput = newPartial
			if eof {
				logger.Debug("Finished SD card print")
				self.gcode.RespondRaw("Done printing file")
				break
			}
			lines = readLines
			self.reactor.Pause(constants.NOW)
			continue
		}
		if self.gcode.IsBusy() {
			self.reactor.Pause(self.reactor.Monotonic() + 0.100)
			continue
		}
		self.core.CmdFromSD = true
		rawLine := lines[len(lines)-1]
		lines = append([]string{}, lines[:len(lines)-1]...)
		line, nextFilePosition := self.core.AdvanceLine(rawLine)
		isBreak := false
		self.tryCatchWorkHandler(line, &errorMessage, &isBreak)
		if isBreak {
			_ = self.core.CloseCurrentFile()
			logger.Error(errorMessage)
			break
		}
		self.core.CmdFromSD = false
		self.core.CommitFilePosition()
		if self.core.NextFilePosition != nextFilePosition {
			self.core.FilePosition = self.core.NextFilePosition
			if err := self.core.SeekToNextPosition(); err != nil {
				logger.Panic("virtual_sdcard seek")
				self.workTimer = nil
				_ = self.core.CloseCurrentFile()
				return constants.NEVER
			}
			lines = []string{}
			partialInput = ""
		}
	}

	logger.Info("Exiting SD card print (position %d)", self.core.FilePosition)
	self.workTimer = nil
	self.core.CmdFromSD = false
	if errorMessage != "" {
		self.printStats.Note_error(errorMessage)
	} else if self.core.CurrentFile != nil {
		self.printStats.Note_pause()
	} else {
		self.printStats.Note_complete()
	}
	return constants.NEVER
}

func (self *VirtualSDModule) tryCatchWorkHandler(line string, errorMessage *string, isBreak *bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recovered == "exit" {
				panic(recovered)
			}
			if msg, ok := commandErrorMessage(recovered); ok {
				*errorMessage = msg
				self.tryCatchOnErrorGCode()
			} else if err, ok := recovered.(*kerror.Error); ok {
				*errorMessage = err.Error()
			} else if err, ok := recovered.(error); ok {
				*errorMessage = err.Error()
			} else if msg, ok := recovered.(string); ok {
				*errorMessage = msg
			}

			logger.Error("virtual_sdcard dispatch", line, recovered, string(debug.Stack()))
			if *errorMessage == "" {
				self.gcode.RespondRaw("print file error")
			} else {
				self.gcode.RespondRaw(*errorMessage)
			}
			*isBreak = true
			return
		}
	}()
	*isBreak = false
	self.gcode.RunScript(line)
}

func (self *VirtualSDModule) tryCatchOnErrorGCode() {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recovered == "exit" {
				panic(recovered)
			}
			logger.Error("virtual_sdcard on_error", recovered, string(debug.Stack()))
		}
	}()
	script, err := self.onErrorGCode.Render(nil)
	if err != nil {
		return
	}
	trimmed := strings.TrimSpace(script)
	if strings.HasPrefix(trimmed, "M") || strings.HasPrefix(trimmed, "m") || strings.HasPrefix(trimmed, "G") || strings.HasPrefix(trimmed, "g") {
		self.gcode.RunScript(script)
	}
}

func rawCommandParameters(gcmd printerpkg.Command) string {
	if typed, ok := gcmd.(virtualSDCommand); ok {
		return typed.RawCommandParameters()
	}
	return ""
}

func commandErrorMessage(recovered interface{}) (string, bool) {
	value := reflect.ValueOf(recovered)
	if !value.IsValid() {
		return "", false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return "", false
	}
	field := value.FieldByName("E")
	if !field.IsValid() || field.Kind() != reflect.String {
		return "", false
	}
	return field.String(), true
}
