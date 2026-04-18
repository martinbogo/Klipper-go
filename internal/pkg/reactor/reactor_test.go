package reactor

import (
	"testing"
	"time"
)

func TestReactorCompletionCompleteNilMarksDone(t *testing.T) {
	reactor := NewSelectReactor(false)
	completion := NewReactorCompletion(reactor)

	if completion.Test() {
		t.Fatalf("completion unexpectedly marked done before Complete")
	}

	completion.Complete(nil)

	if !completion.Test() {
		t.Fatalf("completion not marked done after Complete(nil)")
	}

	got := completion.Wait(reactor.Monotonic()+0.01, "timeout")
	if got != nil {
		t.Fatalf("Wait() = %#v, want nil after Complete(nil)", got)
	}
}

func TestSelectReactorSysPauseUsesRequestedDelay(t *testing.T) {
	reactor := NewSelectReactor(false)
	start := time.Now()
	target := reactor.Monotonic() + 0.02

	reactor._sys_pause(target)

	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Fatalf("_sys_pause() elapsed = %v, want at least about 20ms", elapsed)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("_sys_pause() elapsed = %v, want well below 250ms", elapsed)
	}
}

func TestSelectReactorSetFdWakeTracksWritableHandlers(t *testing.T) {
	reactor := NewSelectReactor(false)
	handler := NewReactorFileHandler(123, nil, nil)

	reactor.Set_fd_wake(handler, false, true)

	if reactor._write_fds.Len() != 1 {
		t.Fatalf("write fd count = %d, want 1", reactor._write_fds.Len())
	}
	if reactor._write_fds.Front().Value != handler {
		t.Fatalf("stored write handler = %#v, want %#v", reactor._write_fds.Front().Value, handler)
	}

	reactor.Set_fd_wake(handler, false, false)
	if reactor._write_fds.Len() != 0 {
		t.Fatalf("write fd count after disable = %d, want 0", reactor._write_fds.Len())
	}
}

func TestEPollReactorNestedCallbackCompletionWait(t *testing.T) {
	reactor := NewEPollReactor(false)
	go func() {
		_ = reactor.Run()
	}()

	defer reactor.End()
	time.Sleep(20 * time.Millisecond)

	outer := reactor.Register_callback(func(interface{}) interface{} {
		inner := reactor.Register_callback(func(interface{}) interface{} {
			return "inner-ok"
		}, 0)
		return inner.Wait(reactor.Monotonic()+0.2, "timeout")
	}, 0)

	got := outer.Wait(reactor.Monotonic()+0.5, "outer-timeout")
	if got != "inner-ok" {
		t.Fatalf("nested callback wait = %#v, want %q", got, "inner-ok")
	}
}
