package serialhdl

type Timer interface{}

type Completion interface {
	Wait(waketime float64, waketimeResult interface{}) interface{}
	Complete(result interface{})
	CreatedAtUnix() int64
}

type Reactor interface {
	Monotonic() float64
	Pause(waketime float64) float64
	RegisterTimer(callback func(float64) float64, waketime float64) Timer
	UpdateTimer(timer Timer, waketime float64)
	RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion
	NewCompletion() Completion
	AsyncComplete(completion Completion, result map[string]interface{})
	IsRunning() bool
}
