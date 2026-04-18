package printer

type ReactorSource[Timer any, Completion any] interface {
	Run() error
	Monotonic() float64
	End()
	Register_callback(func(interface{}) interface{}, float64) Completion
	Register_async_callback(func(interface{}) interface{}, float64)
	Get_gc_stats() [3]float64
	Register_timer(func(float64) float64, float64) Timer
	Update_timer(Timer, float64)
	Unregister_timer(Timer)
	Pause(float64) float64
	Completion() Completion
	Async_complete(Completion, map[string]interface{})
}

type ReactorAdapterOptions struct {
	Run                   func() error
	Monotonic             func() float64
	End                   func()
	RegisterCallback      func(func(interface{}) interface{}, float64)
	RegisterAsyncCallback func(func(interface{}) interface{}, float64)
	GetGCStats            func() [3]float64
	RegisterTimer         func(func(float64) float64, float64) interface{}
	UpdateTimer           func(interface{}, float64)
	UnregisterTimer       func(interface{})
	Pause                 func(float64) float64
	Completion            func() interface{}
	AsyncComplete         func(interface{}, map[string]interface{})
}

type ReactorAdapter struct {
	opts         ReactorAdapterOptions
	timerAdapter *TimerReactorAdapter
}

var _ Reactor = (*ReactorAdapter)(nil)
var _ ModuleReactor = (*ReactorAdapter)(nil)

type timerHandleAdapter struct {
	update func(interface{}, float64)
	timer  interface{}
}

var _ TimerHandle = (*timerHandleAdapter)(nil)

func NewReactorAdapter(opts ReactorAdapterOptions) *ReactorAdapter {
	return &ReactorAdapter{opts: opts}
}

func NewReactorAdapterFrom[Timer any, Completion any](reactor ReactorSource[Timer, Completion]) *ReactorAdapter {
	return NewReactorAdapter(ReactorAdapterOptions{
		Run:       reactor.Run,
		Monotonic: reactor.Monotonic,
		End:       reactor.End,
		RegisterCallback: func(callback func(interface{}) interface{}, eventtime float64) {
			reactor.Register_callback(callback, eventtime)
		},
		RegisterAsyncCallback: func(callback func(interface{}) interface{}, waketime float64) {
			reactor.Register_async_callback(callback, waketime)
		},
		GetGCStats: reactor.Get_gc_stats,
		RegisterTimer: func(callback func(float64) float64, waketime float64) interface{} {
			return reactor.Register_timer(callback, waketime)
		},
		UpdateTimer: func(timer interface{}, waketime float64) {
			typed, ok := timer.(Timer)
			if !ok {
				panic("unsupported reactor timer type")
			}
			reactor.Update_timer(typed, waketime)
		},
		UnregisterTimer: func(timer interface{}) {
			if timer == nil {
				return
			}
			typed, ok := timer.(Timer)
			if !ok {
				panic("unsupported reactor timer type")
			}
			reactor.Unregister_timer(typed)
		},
		Pause: reactor.Pause,
		Completion: func() interface{} {
			return reactor.Completion()
		},
		AsyncComplete: func(completion interface{}, result map[string]interface{}) {
			typed, ok := completion.(Completion)
			if !ok {
				panic("unsupported reactor completion type")
			}
			reactor.Async_complete(typed, result)
		},
	})
}

func (self *ReactorAdapter) Run() error {
	return self.opts.Run()
}

func (self *ReactorAdapter) Monotonic() float64 {
	return self.opts.Monotonic()
}

func (self *ReactorAdapter) End() {
	self.opts.End()
}

func (self *ReactorAdapter) Register_callback(callback func(interface{}) interface{}, eventtime float64) {
	self.opts.RegisterCallback(callback, eventtime)
}

func (self *ReactorAdapter) Register_async_callback(callback func(argv interface{}) interface{}, waketime float64) {
	self.opts.RegisterAsyncCallback(callback, waketime)
}

func (self *ReactorAdapter) Get_gc_stats() [3]float64 {
	return self.opts.GetGCStats()
}

func (self *ReactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) TimerHandle {
	return &timerHandleAdapter{update: self.opts.UpdateTimer, timer: self.opts.RegisterTimer(callback, waketime)}
}

func (self *ReactorAdapter) RegisterAsyncCallback(callback func(float64)) {
	self.opts.RegisterAsyncCallback(func(argv interface{}) interface{} {
		callback(argv.(float64))
		return nil
	}, 0)
}

func (self *ReactorAdapter) RegisterCallback(callback func(float64), waketime float64) {
	self.opts.RegisterCallback(func(argv interface{}) interface{} {
		callback(argv.(float64))
		return nil
	}, waketime)
}

func (self *ReactorAdapter) Pause(waketime float64) float64 {
	return self.opts.Pause(waketime)
}

func (self *ReactorAdapter) Completion() interface{} {
	return self.opts.Completion()
}

func (self *ReactorAdapter) AsyncComplete(completion interface{}, result map[string]interface{}) {
	self.opts.AsyncComplete(completion, result)
}

func (self *ReactorAdapter) TimerAdapter() *TimerReactorAdapter {
	if self.timerAdapter == nil {
		self.timerAdapter = NewTimerReactorAdapter(TimerReactorAdapterOptions{
			Monotonic:       self.opts.Monotonic,
			RegisterTimer:   self.opts.RegisterTimer,
			UnregisterTimer: self.opts.UnregisterTimer,
		})
	}
	return self.timerAdapter
}

func (self *timerHandleAdapter) Update(waketime float64) {
	self.update(self.timer, waketime)
}

type TimerReactorAdapterOptions struct {
	Monotonic       func() float64
	RegisterTimer   func(func(float64) float64, float64) interface{}
	UnregisterTimer func(interface{})
}

type TimerReactorAdapter struct {
	opts TimerReactorAdapterOptions
}

func NewTimerReactorAdapter(opts TimerReactorAdapterOptions) *TimerReactorAdapter {
	return &TimerReactorAdapter{opts: opts}
}

func NewTimerReactorAdapterFrom[Timer any, Completion any](reactor ReactorSource[Timer, Completion]) *TimerReactorAdapter {
	return NewTimerReactorAdapter(TimerReactorAdapterOptions{
		Monotonic: reactor.Monotonic,
		RegisterTimer: func(callback func(float64) float64, waketime float64) interface{} {
			return reactor.Register_timer(callback, waketime)
		},
		UnregisterTimer: func(timer interface{}) {
			if timer == nil {
				return
			}
			typed, ok := timer.(Timer)
			if !ok {
				panic("unsupported reactor timer type")
			}
			reactor.Unregister_timer(typed)
		},
	})
}

func (self *TimerReactorAdapter) Monotonic() float64 {
	return self.opts.Monotonic()
}

func (self *TimerReactorAdapter) RegisterTimer(callback func(float64) float64, waketime float64) interface{} {
	return self.opts.RegisterTimer(callback, waketime)
}

func (self *TimerReactorAdapter) UnregisterTimer(timer interface{}) {
	self.opts.UnregisterTimer(timer)
}
