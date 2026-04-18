package printer

import (
	"errors"
	"goklipper/common/constants"
	"goklipper/common/logger"
	"goklipper/common/utils/object"
	"strings"
	"time"
)

const (
	MessageReady    = "Printer is ready"
	MessageStartup  = "Printer is not ready\nThe project host software is attempting to connect.Please\nretry in a few moments."
	MessageShutdown = "Once the underlying issue is corrected, use the\n\"FIRMWARE_RESTART\" command to reset the firmware, reload the\nconfig, and restart the host software.Printer is shutdown"
)

type Reactor interface {
	Run() error
	Monotonic() float64
	End()
	Register_callback(callback func(interface{}) interface{}, eventtime float64)
	Register_async_callback(callback func(argv interface{}) interface{}, waketime float64)
	Get_gc_stats() [3]float64
}

type EventHandler func([]interface{}) error

type Runtime struct {
	startArgs       map[string]interface{}
	reactor         Reactor
	stateMessage    string
	inShutdownState bool
	runResult       string
	eventHandlers   map[string][]EventHandler
	objects         map[string]interface{}
}

func NewRuntime(reactor Reactor, startArgs map[string]interface{}) *Runtime {
	self := Runtime{}
	self.startArgs = startArgs
	self.reactor = reactor
	self.stateMessage = MessageStartup
	self.inShutdownState = false
	self.runResult = ""
	self.eventHandlers = map[string][]EventHandler{}
	self.objects = map[string]interface{}{}
	return &self
}

func (self *Runtime) GetStartArgs() map[string]interface{} {
	return self.startArgs
}

func (self *Runtime) StateMessage() (string, string) {
	var category string
	if self.stateMessage == MessageReady {
		category = "ready"
	} else if self.stateMessage == MessageStartup {
		category = "startup"
	} else if self.inShutdownState {
		category = "shutdown"
	} else {
		category = "error"
	}
	return self.stateMessage, category
}

func (self *Runtime) IsShutdown() bool {
	return self.inShutdownState
}

func (self *Runtime) SetState(msg string) {
	if self.stateMessage == MessageReady || self.stateMessage == MessageStartup {
		self.stateMessage = msg
	}
	if msg != MessageReady && self.startArgs["debuginput"] != nil {
		self.RequestExit("error_exit")
	}
}

func (self *Runtime) AddObject(name string, obj interface{}) error {
	_, ok := self.objects[name]
	if ok {
		return errors.New(strings.Join([]string{"Printer object '", name, "' already created"}, ""))
	}
	self.objects[name] = obj
	return nil
}

func (self *Runtime) StoreObject(name string, obj interface{}) {
	self.objects[name] = obj
}

func (self *Runtime) LookupObject(name string, default1 interface{}) interface{} {
	_, ok := self.objects[name]
	if ok {
		return self.objects[name]
	}
	if _, ok := default1.(object.Sentinel); ok {
		logger.Error(strings.Join([]string{"Unknown config object '", name, "' "}, ""))
	}
	return default1
}

func (self *Runtime) LookupObjects(module string) []interface{} {
	mods := []interface{}{}
	if module == "" {
		for _, v := range self.objects {
			mods = append(mods, v)
		}
		return mods
	}
	prefix := module + " "
	for k, v := range self.objects {
		mod := map[string]interface{}{}
		if strings.HasPrefix(k, prefix) {
			mod[k] = v
			mods = append(mods, mod)
		}
	}
	obj, ok := self.objects[module]
	if ok {
		mod := map[string]interface{}{}
		mod[module] = obj
		mods = append(mods, mod)
	}
	return mods
}

func (self *Runtime) RegisterEventHandler(event string, callback EventHandler) {
	list, ok := self.eventHandlers[event]
	if !ok {
		list = []EventHandler{}
	}
	self.eventHandlers[event] = append(list, callback)
}

func (self *Runtime) EventHandlers(event string) []EventHandler {
	list, ok := self.eventHandlers[event]
	if !ok {
		return nil
	}
	out := make([]EventHandler, len(list))
	copy(out, list)
	return out
}

func (self *Runtime) SendEvent(event string, params []interface{}) ([]interface{}, error) {
	ret := []interface{}{}
	cbs, ok := self.eventHandlers[event]
	if ok {
		for i := 0; i < len(cbs); i++ {
			cb := cbs[i]
			ret = append(ret, cb(params))
		}
	}
	return ret, nil
}

func (self *Runtime) RequestExit(result string) {
	if self.runResult == "" {
		logger.Infof("runtime: request exit -> %s", result)
		self.runResult = result
	} else {
		logger.Infof("runtime: request exit ignored, existing result=%s new=%s", self.runResult, result)
	}
	self.reactor.End()
}

func (self *Runtime) ExitResult() string {
	return self.runResult
}

func (self *Runtime) InvokeShutdown(msg interface{}) interface{} {
	if self.inShutdownState {
		return nil
	}
	logger.Errorf("Transition to shutdown state: %s", msg)
	self.inShutdownState = true
	self.SetState(strings.Join([]string{msg.(string), MessageShutdown}, ""))
	for _, cb := range self.eventHandlers["project:shutdown"] {
		err := cb(nil)
		if err != nil {
			logger.Error("Exception during shutdown handler")
		}
		logger.Debug("Reactor garbage collection: ", self.reactor.Get_gc_stats())
	}
	return nil
}

func (self *Runtime) Run() string {
	systime := float64(time.Now().UnixNano()) / 1000000000
	monotime := self.reactor.Monotonic()
	logger.Infof("Start printer at %s (%.1f %.1f)",
		time.Now().String(), systime, monotime)
	err := self.reactor.Run()
	if err != nil {
		msg := "Unhandled exception during run"
		logger.Error(msg)
		self.reactor.Register_callback(self.InvokeShutdown, constants.NOW)
		err = self.reactor.Run()
		if err != nil {
			logger.Error("Repeat unhandled exception during run")
			self.runResult = "error_exit"
		}
	}
	runResult := self.runResult
	logger.Infof("runtime: reactor loop ended with result=%q", runResult)
	if runResult == "firmware_restart" {
		logger.Infof("runtime: dispatching project:firmware_restart")
		_, err := self.SendEvent("project:firmware_restart", nil)
		if err != nil {
			logger.Error("Unhandled exception during post run")
			return runResult
		}
	}
	logger.Infof("runtime: dispatching project:disconnect")
	_, err = self.SendEvent("project:disconnect", nil)
	if err != nil {
		logger.Error("Unhandled exception during post run")
		return runResult
	}
	return runResult
}
