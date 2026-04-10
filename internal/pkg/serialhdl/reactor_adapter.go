package serialhdl

import reactorpkg "goklipper/internal/pkg/reactor"

type reactorTimerAdapter struct {
	timer *reactorpkg.ReactorTimer
}

type reactorCompletionAdapter struct {
	completion *reactorpkg.ReactorCompletion
}

func (self *reactorCompletionAdapter) Wait(waketime float64, waketimeResult interface{}) interface{} {
	return self.completion.Wait(waketime, waketimeResult)
}

func (self *reactorCompletionAdapter) Complete(result interface{}) {
	self.completion.Complete(result)
}

func (self *reactorCompletionAdapter) CreatedAtUnix() int64 {
	return self.completion.CreatedAtUnix()
}

type reactorAdapter struct {
	reactor reactorpkg.IReactor
}

func NewReactorAdapter(reactor reactorpkg.IReactor) Reactor {
	return &reactorAdapter{reactor: reactor}
}

func (self *reactorAdapter) Monotonic() float64 {
	return self.reactor.Monotonic()
}

func (self *reactorAdapter) Pause(waketime float64) float64 {
	return self.reactor.Pause(waketime)
}

func (self *reactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) Timer {
	return &reactorTimerAdapter{timer: self.reactor.Register_timer(callback, waketime)}
}

func (self *reactorAdapter) UpdateTimer(timer Timer, waketime float64) {
	self.reactor.Update_timer(timer.(*reactorTimerAdapter).timer, waketime)
}

func (self *reactorAdapter) RegisterCallback(callback func(interface{}) interface{}, waketime float64) Completion {
	return &reactorCompletionAdapter{completion: self.reactor.Register_callback(callback, waketime)}
}

func (self *reactorAdapter) NewCompletion() Completion {
	return &reactorCompletionAdapter{completion: self.reactor.Completion()}
}

func (self *reactorAdapter) AsyncComplete(completion Completion, result map[string]interface{}) {
	self.reactor.Async_complete(completion.(*reactorCompletionAdapter).completion, result)
}

func (self *reactorAdapter) IsRunning() bool {
	switch reactor := self.reactor.(type) {
	case *reactorpkg.EPollReactor:
		return reactor.IsRunning()
	case *reactorpkg.PollReactor:
		return reactor.IsRunning()
	case *reactorpkg.SelectReactor:
		return reactor.IsRunning()
	default:
		return true
	}
}