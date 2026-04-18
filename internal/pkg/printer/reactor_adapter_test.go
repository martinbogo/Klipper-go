package printer

import (
	"testing"
)

type fakeBridgeTimer struct{}

type fakeBridgeCompletion struct{}

type fakeBridgeReactor struct {
	timer                 *fakeBridgeTimer
	completion            *fakeBridgeCompletion
	asyncCompletion       *fakeBridgeCompletion
	asyncResult           map[string]interface{}
	updatedTimer          *fakeBridgeTimer
	unregisteredTimer     *fakeBridgeTimer
	registerTimerEvent    float64
	registerCallbackEvent float64
	registerAsyncEvent    float64
	lastPaused            float64
	ended                 bool
	runCalled             bool
	callbackValue         interface{}
	asyncValue            interface{}
	timerValue            float64
}

func (self *fakeBridgeReactor) Run() error {
	self.runCalled = true
	return nil
}

func (self *fakeBridgeReactor) Monotonic() float64 { return 9.5 }

func (self *fakeBridgeReactor) End() { self.ended = true }

func (self *fakeBridgeReactor) Register_timer(callback func(float64) float64, waketime float64) *fakeBridgeTimer {
	self.registerTimerEvent = waketime
	self.timerValue = callback(2.25)
	if self.timer == nil {
		self.timer = &fakeBridgeTimer{}
	}
	return self.timer
}

func (self *fakeBridgeReactor) Unregister_timer(timer *fakeBridgeTimer) {
	self.unregisteredTimer = timer
}

func (self *fakeBridgeReactor) Update_timer(timer *fakeBridgeTimer, waketime float64) {
	self.updatedTimer = timer
	self.registerTimerEvent = waketime
}

func (self *fakeBridgeReactor) Register_callback(callback func(interface{}) interface{}, eventtime float64) *fakeBridgeCompletion {
	self.registerCallbackEvent = eventtime
	self.callbackValue = callback("sync")
	if self.completion == nil {
		self.completion = &fakeBridgeCompletion{}
	}
	return self.completion
}

func (self *fakeBridgeReactor) Register_async_callback(callback func(interface{}) interface{}, waketime float64) {
	self.registerAsyncEvent = waketime
	self.asyncValue = callback("async")
}

func (self *fakeBridgeReactor) Get_gc_stats() [3]float64 { return [3]float64{1, 2, 3} }

func (self *fakeBridgeReactor) Completion() *fakeBridgeCompletion {
	if self.completion == nil {
		self.completion = &fakeBridgeCompletion{}
	}
	return self.completion
}

func (self *fakeBridgeReactor) Pause(waketime float64) float64 {
	self.lastPaused = waketime
	return waketime + 1
}

func (self *fakeBridgeReactor) Async_complete(completion *fakeBridgeCompletion, result map[string]interface{}) {
	self.asyncCompletion = completion
	self.asyncResult = result
}

func TestReactorAdapterForwardsCalls(t *testing.T) {
	rawTimer := &struct{}{}
	rawCompletion := &struct{}{}
	asyncResult := map[string]interface{}{}
	statsValue := [3]float64{1, 2, 3}

	var runCalled bool
	var endCalled bool
	var registerCallbackEventtime float64
	var registerAsyncEventtime float64
	var registerTimerEventtime float64
	var updateTimerEventtime float64
	var pauseEventtime float64
	var forwardedCallbackValue interface{}
	var forwardedAsyncValue interface{}
	var forwardedTimerResult float64
	var forwardedRegisterCallback float64
	var forwardedRegisterAsync float64
	var asyncCompletion interface{}

	adapter := NewReactorAdapter(ReactorAdapterOptions{
		Run: func() error {
			runCalled = true
			return nil
		},
		Monotonic: func() float64 {
			return 42.5
		},
		End: func() {
			endCalled = true
		},
		RegisterCallback: func(callback func(interface{}) interface{}, eventtime float64) {
			registerCallbackEventtime = eventtime
			if eventtime == 3.5 {
				forwardedCallbackValue = callback("sync-value")
				return
			}
			forwardedCallbackValue = callback(23.5)
		},
		RegisterAsyncCallback: func(callback func(interface{}) interface{}, eventtime float64) {
			registerAsyncEventtime = eventtime
			if eventtime == 4.5 {
				forwardedAsyncValue = callback("async-value")
				return
			}
			forwardedAsyncValue = callback(19.75)
		},
		GetGCStats: func() [3]float64 {
			return statsValue
		},
		RegisterTimer: func(callback func(float64) float64, eventtime float64) interface{} {
			registerTimerEventtime = eventtime
			forwardedTimerResult = callback(11.25)
			return rawTimer
		},
		UpdateTimer: func(timer interface{}, eventtime float64) {
			if timer != rawTimer {
				t.Fatalf("UpdateTimer timer = %v, want %v", timer, rawTimer)
			}
			updateTimerEventtime = eventtime
		},
		Pause: func(eventtime float64) float64 {
			pauseEventtime = eventtime
			return eventtime + 1
		},
		Completion: func() interface{} {
			return rawCompletion
		},
		AsyncComplete: func(completion interface{}, result map[string]interface{}) {
			asyncCompletion = completion
			asyncResult = result
		},
	})

	if err := adapter.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !runCalled {
		t.Fatal("Run() did not forward")
	}
	if got := adapter.Monotonic(); got != 42.5 {
		t.Fatalf("Monotonic() = %v, want 42.5", got)
	}
	adapter.End()
	if !endCalled {
		t.Fatal("End() did not forward")
	}

	adapter.Register_callback(func(value interface{}) interface{} {
		if value != "sync-value" {
			t.Fatalf("Register_callback value = %v, want sync-value", value)
		}
		return "sync-result"
	}, 3.5)
	if registerCallbackEventtime != 3.5 {
		t.Fatalf("Register_callback eventtime = %v, want 3.5", registerCallbackEventtime)
	}
	if forwardedCallbackValue != "sync-result" {
		t.Fatalf("Register_callback return = %v, want sync-result", forwardedCallbackValue)
	}

	adapter.Register_async_callback(func(value interface{}) interface{} {
		if value != "async-value" {
			t.Fatalf("Register_async_callback value = %v, want async-value", value)
		}
		return "async-result"
	}, 4.5)
	if registerAsyncEventtime != 4.5 {
		t.Fatalf("Register_async_callback eventtime = %v, want 4.5", registerAsyncEventtime)
	}
	if forwardedAsyncValue != "async-result" {
		t.Fatalf("Register_async_callback return = %v, want async-result", forwardedAsyncValue)
	}

	if got := adapter.Get_gc_stats(); got != statsValue {
		t.Fatalf("Get_gc_stats() = %v, want %v", got, statsValue)
	}

	handle := adapter.RegisterTimer(func(eventtime float64) float64 {
		return eventtime + 0.5
	}, 5.5)
	if registerTimerEventtime != 5.5 {
		t.Fatalf("RegisterTimer eventtime = %v, want 5.5", registerTimerEventtime)
	}
	if forwardedTimerResult != 11.75 {
		t.Fatalf("RegisterTimer callback result = %v, want 11.75", forwardedTimerResult)
	}
	handle.Update(6.5)
	if updateTimerEventtime != 6.5 {
		t.Fatalf("TimerHandle.Update eventtime = %v, want 6.5", updateTimerEventtime)
	}

	adapter.RegisterAsyncCallback(func(eventtime float64) {
		forwardedRegisterAsync = eventtime
	})
	if registerAsyncEventtime != 0 {
		t.Fatalf("RegisterAsyncCallback eventtime = %v, want 0", registerAsyncEventtime)
	}
	if forwardedRegisterAsync != 19.75 {
		t.Fatalf("RegisterAsyncCallback callback value = %v, want 19.75", forwardedRegisterAsync)
	}

	adapter.RegisterCallback(func(eventtime float64) {
		forwardedRegisterCallback = eventtime
	}, 7.5)
	if registerCallbackEventtime != 7.5 {
		t.Fatalf("RegisterCallback eventtime = %v, want 7.5", registerCallbackEventtime)
	}
	if forwardedRegisterCallback != 23.5 {
		t.Fatalf("RegisterCallback callback value = %v, want 23.5", forwardedRegisterCallback)
	}

	if got := adapter.Pause(8.5); got != 9.5 {
		t.Fatalf("Pause() = %v, want 9.5", got)
	}
	if pauseEventtime != 8.5 {
		t.Fatalf("Pause eventtime = %v, want 8.5", pauseEventtime)
	}
	if got := adapter.Completion(); got != rawCompletion {
		t.Fatalf("Completion() = %v, want %v", got, rawCompletion)
	}

	adapter.AsyncComplete(rawCompletion, map[string]interface{}{"ok": true})
	if asyncCompletion != rawCompletion {
		t.Fatalf("AsyncComplete completion = %v, want %v", asyncCompletion, rawCompletion)
	}
	if ok, _ := asyncResult["ok"].(bool); !ok {
		t.Fatalf("AsyncComplete result = %v, want ok=true", asyncResult)
	}
}

func TestTimerReactorAdapterForwardsCalls(t *testing.T) {
	rawTimer := &struct{}{}
	var registerEventtime float64
	var unregisterTimer interface{}
	var callbackResult float64

	adapter := NewTimerReactorAdapter(TimerReactorAdapterOptions{
		Monotonic: func() float64 {
			return 13.25
		},
		RegisterTimer: func(callback func(float64) float64, eventtime float64) interface{} {
			registerEventtime = eventtime
			callbackResult = callback(2.5)
			return rawTimer
		},
		UnregisterTimer: func(timer interface{}) {
			unregisterTimer = timer
		},
	})

	if got := adapter.Monotonic(); got != 13.25 {
		t.Fatalf("Monotonic() = %v, want 13.25", got)
	}
	if got := adapter.RegisterTimer(func(eventtime float64) float64 { return eventtime + 4 }, 6.75); got != rawTimer {
		t.Fatalf("RegisterTimer() = %v, want %v", got, rawTimer)
	}
	if registerEventtime != 6.75 {
		t.Fatalf("RegisterTimer eventtime = %v, want 6.75", registerEventtime)
	}
	if callbackResult != 6.5 {
		t.Fatalf("RegisterTimer callback result = %v, want 6.5", callbackResult)
	}
	adapter.UnregisterTimer(rawTimer)
	if unregisterTimer != rawTimer {
		t.Fatalf("UnregisterTimer timer = %v, want %v", unregisterTimer, rawTimer)
	}
}

func TestReactorAdapterForBridgesTypedReactorMethods(t *testing.T) {
	reactor := &fakeBridgeReactor{}
	adapter := NewReactorAdapterFrom[*fakeBridgeTimer, *fakeBridgeCompletion](reactor)

	if err := adapter.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reactor.runCalled {
		t.Fatal("expected Run to be forwarded")
	}
	if got := adapter.Monotonic(); got != 9.5 {
		t.Fatalf("Monotonic() = %v, want 9.5", got)
	}
	adapter.Register_callback(func(value interface{}) interface{} {
		if value != "sync" {
			t.Fatalf("Register_callback value = %v, want sync", value)
		}
		return "sync-result"
	}, 3.5)
	if reactor.registerCallbackEvent != 3.5 || reactor.callbackValue != "sync-result" {
		t.Fatalf("expected callback forwarding, got event=%v value=%v", reactor.registerCallbackEvent, reactor.callbackValue)
	}
	adapter.Register_async_callback(func(value interface{}) interface{} {
		if value != "async" {
			t.Fatalf("Register_async_callback value = %v, want async", value)
		}
		return "async-result"
	}, 4.5)
	if reactor.registerAsyncEvent != 4.5 || reactor.asyncValue != "async-result" {
		t.Fatalf("expected async forwarding, got event=%v value=%v", reactor.registerAsyncEvent, reactor.asyncValue)
	}
	handle := adapter.RegisterTimer(func(eventtime float64) float64 { return eventtime + 0.75 }, 5.5)
	if reactor.registerTimerEvent != 5.5 {
		t.Fatalf("RegisterTimer eventtime = %v, want 5.5", reactor.registerTimerEvent)
	}
	if reactor.timerValue != 3 {
		t.Fatalf("RegisterTimer callback result = %v, want 3", reactor.timerValue)
	}
	handle.Update(6.5)
	if reactor.updatedTimer != reactor.timer {
		t.Fatalf("Update timer = %v, want %v", reactor.updatedTimer, reactor.timer)
	}
	if reactor.registerTimerEvent != 6.5 {
		t.Fatalf("Update timer eventtime = %v, want 6.5", reactor.registerTimerEvent)
	}
	if got := adapter.Completion(); got != reactor.completion {
		t.Fatalf("Completion() = %v, want %v", got, reactor.completion)
	}
	adapter.AsyncComplete(reactor.completion, map[string]interface{}{"ok": true})
	if reactor.asyncCompletion != reactor.completion {
		t.Fatalf("AsyncComplete completion = %v, want %v", reactor.asyncCompletion, reactor.completion)
	}
	if ok, _ := reactor.asyncResult["ok"].(bool); !ok {
		t.Fatalf("AsyncComplete result = %v, want ok=true", reactor.asyncResult)
	}
	if got := adapter.Pause(7.5); got != 8.5 {
		t.Fatalf("Pause() = %v, want 8.5", got)
	}
	adapter.End()
	if !reactor.ended {
		t.Fatal("expected End to be forwarded")
	}

	timerAdapter := NewTimerReactorAdapterFrom[*fakeBridgeTimer, *fakeBridgeCompletion](reactor)
	if got := timerAdapter.Monotonic(); got != 9.5 {
		t.Fatalf("Timer adapter Monotonic() = %v, want 9.5", got)
	}
	registered := timerAdapter.RegisterTimer(func(eventtime float64) float64 { return eventtime + 1 }, 8.25)
	if registered != reactor.timer {
		t.Fatalf("Timer adapter RegisterTimer() = %v, want %v", registered, reactor.timer)
	}
	timerAdapter.UnregisterTimer(registered)
	if reactor.unregisteredTimer != reactor.timer {
		t.Fatalf("Timer adapter UnregisterTimer() = %v, want %v", reactor.unregisteredTimer, reactor.timer)
	}
	timerAdapter.UnregisterTimer(nil)
}

func TestReactorAdapterSingleInstanceCanServeRuntimeAndModuleInterfaces(t *testing.T) {
	reactor := &fakeBridgeReactor{}
	adapter := NewReactorAdapterFrom[*fakeBridgeTimer, *fakeBridgeCompletion](reactor)

	var runtime Reactor = adapter
	var module ModuleReactor = adapter

	runtimeAdapter, ok := runtime.(*ReactorAdapter)
	if !ok {
		t.Fatalf("runtime interface backed by %T, want *ReactorAdapter", runtime)
	}
	moduleAdapter, ok := module.(*ReactorAdapter)
	if !ok {
		t.Fatalf("module interface backed by %T, want *ReactorAdapter", module)
	}
	if runtimeAdapter != moduleAdapter {
		t.Fatal("expected runtime and module interfaces to share one adapter instance")
	}

	if got := runtime.Monotonic(); got != 9.5 {
		t.Fatalf("runtime Monotonic() = %v, want 9.5", got)
	}
	if got := module.Monotonic(); got != 9.5 {
		t.Fatalf("module Monotonic() = %v, want 9.5", got)
	}

	handle := module.RegisterTimer(func(eventtime float64) float64 {
		return eventtime + 2
	}, 4.25)
	if reactor.registerTimerEvent != 4.25 {
		t.Fatalf("module RegisterTimer eventtime = %v, want 4.25", reactor.registerTimerEvent)
	}
	handle.Update(5.25)
	if reactor.registerTimerEvent != 5.25 {
		t.Fatalf("shared timer Update eventtime = %v, want 5.25", reactor.registerTimerEvent)
	}
}

func TestReactorAdapterReusesSingleTimerAdapterView(t *testing.T) {
	reactor := &fakeBridgeReactor{}
	adapter := NewReactorAdapterFrom[*fakeBridgeTimer, *fakeBridgeCompletion](reactor)

	first := adapter.TimerAdapter()
	second := adapter.TimerAdapter()
	if first != second {
		t.Fatal("expected cached timer adapter view")
	}
	if got := first.Monotonic(); got != 9.5 {
		t.Fatalf("TimerAdapter Monotonic() = %v, want 9.5", got)
	}
	registered := first.RegisterTimer(func(eventtime float64) float64 { return eventtime + 1.5 }, 6.25)
	if registered != reactor.timer {
		t.Fatalf("TimerAdapter RegisterTimer() = %v, want %v", registered, reactor.timer)
	}
	first.UnregisterTimer(registered)
	if reactor.unregisteredTimer != reactor.timer {
		t.Fatalf("TimerAdapter UnregisterTimer() = %v, want %v", reactor.unregisteredTimer, reactor.timer)
	}
}
