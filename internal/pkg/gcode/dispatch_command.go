package gcode

import (
	"fmt"
	printerpkg "goklipper/internal/pkg/printer"
)

// DispatchCommand is the reusable parsed G-code command envelope used by the
// project dispatcher and modules. It keeps the raw command metadata, typed
// accessors, ack state, and response callbacks together in one runtime value.
type DispatchCommand struct {
	CommandParams
	Need_ack     bool
	Respond_info func(msg string, log bool)
	Respond_raw  func(msg string)
}

var _ printerpkg.Command = (*DispatchCommand)(nil)

func NewDispatchCommand(
	respondInfo func(string, bool),
	respondRaw func(string),
	command string,
	commandline string,
	params map[string]string,
	needAck bool,
) *DispatchCommand {
	return &DispatchCommand{
		CommandParams: CommandParams{
			Command:     command,
			Commandline: commandline,
			Params:      params,
		},
		Need_ack:     needAck,
		Respond_info: respondInfo,
		Respond_raw:  respondRaw,
	}
}

func (self *DispatchCommand) Ack(msg string) bool {
	if self.Need_ack == false {
		return false
	}
	ok_msg := "ok"
	if msg != "" {
		ok_msg = fmt.Sprintf("ok %s", msg)
	}
	self.Respond_raw(ok_msg)
	self.Need_ack = false
	return true
}

func (self *DispatchCommand) String(name string, defaultValue string) string {
	return self.Get(name, defaultValue, nil, nil, nil, nil, nil)
}

func (self *DispatchCommand) Float(name string, defaultValue float64) float64 {
	return self.Get_float(name, defaultValue, nil, nil, nil, nil)
}

func (self *DispatchCommand) Int(name string, defaultValue int, minValue *int, maxValue *int) int {
	return self.Get_int(name, defaultValue, minValue, maxValue)
}

func (self *DispatchCommand) Parameters() map[string]string {
	return self.Params
}

func (self *DispatchCommand) Get_command_parameters() map[string]string {
	return self.Params
}

func (self *DispatchCommand) Get_commandline() string {
	return self.Commandline
}

func (self *DispatchCommand) RawParameters() string {
	return self.CommandParams.GetRawCommandParameters()
}

func (self *DispatchCommand) RespondInfo(msg string, log bool) {
	self.Respond_info(msg, log)
}

func (self *DispatchCommand) RespondRaw(msg string) {
	self.Respond_raw(msg)
}