package print

import (
	"fmt"
	"runtime/debug"

	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/sys"
	printerpkg "goklipper/internal/pkg/printer"
)

// Here marco is not compatible with pongo2 and will not stop warming up
const DEFAULT_IDLE_GCODE = "TURN_OFF_HEATERS\nM107\nM84\n"

const cmdSetIdleTimeoutHelp = "Set the idle timeout in seconds"

type idleTimeoutToolhead interface {
	Get_last_move_time() float64
	Check_busy(eventtime float64) (float64, float64, bool)
}

type IdleTimeoutModule struct {
	printer      printerpkg.ModulePrinter
	reactor      printerpkg.ModuleReactor
	gcode        printerpkg.GCodeRuntime
	toolhead     idleTimeoutToolhead
	timeoutTimer printerpkg.TimerHandle
	idleGcode    printerpkg.Template
	core         *IdleTimeout
}

func NewIdleTimeoutModule(config printerpkg.ModuleConfig, printer printerpkg.ModulePrinter, reactor printerpkg.ModuleReactor, gcode printerpkg.GCodeRuntime, toolhead idleTimeoutToolhead) *IdleTimeoutModule {
	self := &IdleTimeoutModule{
		printer:      printer,
		reactor:      reactor,
		gcode:        gcode,
		toolhead:     toolhead,
		timeoutTimer: nil,
		idleGcode:    config.LoadTemplate("gcode_macro_1", "gcode", DEFAULT_IDLE_GCODE),
		core:         NewIdleTimeout(config.Float("timeout", 600.)),
	}
	printer.RegisterEventHandler("project:ready", self.Handle_ready)
	gcode.RegisterCommand("SET_IDLE_TIMEOUT", self.cmdSetIdleTimeout, false, cmdSetIdleTimeoutHelp)
	return self
}

func LoadConfigIdleTimeout(config printerpkg.ModuleConfig) interface{} {
	printer := config.Printer()
	return NewIdleTimeoutModule(config, printer, printer.Reactor(), printer.GCode(), nil)
}

func (self *IdleTimeoutModule) Get_status(eventtime float64) map[string]interface{} {
	return self.core.GetStatus(eventtime)
}

func (self *IdleTimeoutModule) Handle_ready([]interface{}) error {
	self.toolhead = self.printer.LookupObject("toolhead", nil).(idleTimeoutToolhead)
	self.timeoutTimer = self.reactor.RegisterTimer(self.Timeout_handler, constants.NEVER)
	self.printer.RegisterEventHandler("toolhead:sync_print_time", self.Handle_sync_print_time)
	return nil
}

func (self *IdleTimeoutModule) transitionIdleState(eventtime float64) (idletime float64) {
	self.core.BeginIdleTransition()
	var (
		err    error
		script string
	)

	defer func() {
		r := recover()
		if r != nil || err != nil {
			s := string(debug.Stack())
			logger.Error("idle timeout gcode execution", sys.GetGID(), err, s)
			self.core.FailIdleTransition()
			idletime = eventtime + 1.
		}
	}()

	script, err = self.idleGcode.Render(nil)
	if err != nil {
		logger.Panic("idle timeout gcode execution")
		self.core.FailIdleTransition()
		return eventtime + 1.
	}
	self.gcode.RunScript(script)

	printTime := self.toolhead.Get_last_move_time()
	decision := self.core.CompleteIdleTransition(printTime)
	self.printer.SendEvent(decision.EventName, decision.EventArgs)
	return decision.NextWake
}

func (self *IdleTimeoutModule) Timeout_handler(eventtime float64) float64 {
	if self.printer.IsShutdown() {
		return constants.NEVER
	}
	printTime, estPrintTime, lookaheadEmpty := self.toolhead.Check_busy(eventtime)
	decision := self.core.TimeoutHandler(eventtime, printTime, estPrintTime, lookaheadEmpty, self.gcode.IsBusy())
	if decision.EnterIdle {
		return self.transitionIdleState(eventtime)
	}
	if decision.EventName != "" {
		self.printer.SendEvent(decision.EventName, decision.EventArgs)
	}
	return decision.NextWake
}

func (self *IdleTimeoutModule) Handle_sync_print_time(argv []interface{}) error {
	curtime := argv[0].(float64)
	estPrintTime := argv[1].(float64)
	printTime := argv[2].(float64)
	decision, changed := self.core.SyncPrintTime(curtime, estPrintTime, printTime)
	if !changed {
		return nil
	}
	self.timeoutTimer.Update(decision.NextWake)
	self.printer.SendEvent(decision.EventName, decision.EventArgs)
	return nil
}

func (self *IdleTimeoutModule) cmdSetIdleTimeout(gcmd printerpkg.Command) error {
	timeout := gcmd.Float("TIMEOUT", self.core.Timeout())
	self.core.SetTimeout(timeout)
	gcmd.RespondInfo("idle_timeout: Timeout set to "+formatIdleTimeout(timeout)+" s", true)
	if self.core.State() == StateReady {
		checktime := self.reactor.Monotonic() + timeout
		if self.timeoutTimer != nil {
			self.timeoutTimer.Update(checktime)
		}
	}
	return nil
}

func formatIdleTimeout(timeout float64) string {
	return fmt.Sprintf("%.2f", timeout)
}