package print

import (
	"goklipper/common/constants"
	"math"
)

const (
	ReadyTimeout = .500
	PinMinTime   = 0.100
	StateIdle    = "Idle"
	StatePrinting = "Printing"
	StateReady   = "Ready"
)

type Decision struct {
	NextWake  float64
	EventName string
	EventArgs []interface{}
	EnterIdle bool
}

type IdleTimeout struct {
	idleTimeout            float64
	state                  string
	lastPrintStartSystime  float64
}

func NewIdleTimeout(timeout float64) *IdleTimeout {
	return &IdleTimeout{
		idleTimeout:           timeout,
		state:                 StateIdle,
		lastPrintStartSystime: 0,
	}
}

func (self *IdleTimeout) Timeout() float64 {
	return self.idleTimeout
}

func (self *IdleTimeout) SetTimeout(timeout float64) {
	self.idleTimeout = timeout
}

func (self *IdleTimeout) State() string {
	return self.state
}

func (self *IdleTimeout) GetStatus(eventtime float64) map[string]interface{} {
	printingTime := 0.
	if self.state == StatePrinting {
		printingTime = eventtime - self.lastPrintStartSystime
	}
	return map[string]interface{}{
		"state":         self.state,
		"printing_time": printingTime,
	}
}

func (self *IdleTimeout) BeginIdleTransition() {
	self.state = StatePrinting
}

func (self *IdleTimeout) FailIdleTransition() {
	self.state = StateReady
}

func (self *IdleTimeout) CompleteIdleTransition(printTime float64) Decision {
	self.state = StateIdle
	return Decision{
		NextWake:  constants.NEVER,
		EventName: "idle_timeout:idle",
		EventArgs: []interface{}{printTime},
	}
}

func (self *IdleTimeout) CheckIdleTimeout(eventtime, printTime, estPrintTime float64, lookaheadEmpty, gcodeBusy bool) Decision {
	idleTime := estPrintTime - printTime

	if !lookaheadEmpty || idleTime < 1. {
		return Decision{NextWake: eventtime + self.idleTimeout}
	}

	if idleTime < self.idleTimeout {
		return Decision{NextWake: eventtime + self.idleTimeout - idleTime}
	}

	if gcodeBusy {
		return Decision{NextWake: eventtime + 1.}
	}

	return Decision{EnterIdle: true}
	}

func (self *IdleTimeout) TimeoutHandler(eventtime, printTime, estPrintTime float64, lookaheadEmpty, gcodeBusy bool) Decision {
	if self.state == StateReady {
		return self.CheckIdleTimeout(eventtime, printTime, estPrintTime, lookaheadEmpty, gcodeBusy)
	}

	bufferTime := math.Min(2., printTime-estPrintTime)

	if !lookaheadEmpty {
		return Decision{NextWake: eventtime + ReadyTimeout + math.Max(0., bufferTime)}
	}

	if bufferTime > -ReadyTimeout {
		return Decision{NextWake: eventtime + ReadyTimeout + bufferTime}
	}

	if gcodeBusy {
		return Decision{NextWake: eventtime + ReadyTimeout}
	}

	self.state = StateReady
	return Decision{
		NextWake:  eventtime + self.idleTimeout,
		EventName: "idle_timeout:ready",
		EventArgs: []interface{}{estPrintTime + PinMinTime},
	}
}

func (self *IdleTimeout) SyncPrintTime(curtime, estPrintTime, printTime float64) (Decision, bool) {
	if self.state == StatePrinting {
		return Decision{}, false
	}

	self.state = StatePrinting
	self.lastPrintStartSystime = curtime
	checkTime := ReadyTimeout + printTime - estPrintTime
	return Decision{
		NextWake:  curtime + checkTime,
		EventName: "idle_timeout:printing",
		EventArgs: []interface{}{estPrintTime + PinMinTime},
	}, true
}