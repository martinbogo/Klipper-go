package tmc

import "testing"

type fakeUARTMuxActivator struct {
	activations [][]int64
}

func (self *fakeUARTMuxActivator) Activate(instanceID []int64) {
	self.activations = append(self.activations, append([]int64(nil), instanceID...))
}

func TestUARTMutexCacheReusesEntry(t *testing.T) {
	cache := NewUARTMutexCache()
	factoryCalls := 0
	first := cache.Lookup("mcu", func() interface{} {
		factoryCalls++
		return "mutex"
	})
	second := cache.Lookup("mcu", func() interface{} {
		factoryCalls++
		return "other"
	})

	if first != "mutex" || second != "mutex" {
		t.Fatalf("expected cached mutex, got first=%#v second=%#v", first, second)
	}
	if factoryCalls != 1 {
		t.Fatalf("expected factory to be called once, got %d", factoryCalls)
	}
}

func TestUARTSharedResourceRuntimeRejectsMismatchedPins(t *testing.T) {
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", nil, nil, nil)
	if _, err := runtime.RegisterInstance("rx1", "tx0", nil, 0); err == nil {
		t.Fatal("expected mismatched pins error")
	}
}

func TestUARTSharedResourceRuntimeValidatesMuxPinsAndUniqueness(t *testing.T) {
	mux := &fakeUARTMuxActivator{}
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", "mcu0", []interface{}{"p1", "p2"}, mux)
	selectPins := []UARTSelectPin{{Owner: "mcu0", Pin: "p1", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: true}}

	instanceID, err := runtime.RegisterInstance("rx0", "tx0", selectPins, 2)
	if err != nil {
		t.Fatalf("RegisterInstance returned error: %v", err)
	}
	if len(instanceID) != 2 || instanceID[0] != 1 || instanceID[1] != 0 {
		t.Fatalf("unexpected instance id %#v", instanceID)
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", selectPins, 2); err == nil {
		t.Fatal("expected duplicate instance error")
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", []UARTSelectPin{{Owner: "mcu1", Pin: "p1", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: false}}, 3); err == nil {
		t.Fatal("expected foreign mux owner error")
	}
	if _, err := runtime.RegisterInstance("rx0", "tx0", []UARTSelectPin{{Owner: "mcu0", Pin: "other", Invert: false}, {Owner: "mcu0", Pin: "p2", Invert: false}}, 4); err == nil {
		t.Fatal("expected mismatched mux pins error")
	}
}

func TestUARTSharedResourceRuntimePrepareTransferActivatesMux(t *testing.T) {
	mux := &fakeUARTMuxActivator{}
	runtime := NewUARTSharedResourceRuntime("rx0", "tx0", "mcu0", []interface{}{"p1"}, mux)
	runtime.PrepareTransfer([]int64{1})
	if len(mux.activations) != 1 {
		t.Fatalf("expected one activation, got %d", len(mux.activations))
	}
	if len(mux.activations[0]) != 1 || mux.activations[0][0] != 1 {
		t.Fatalf("unexpected activation payload %#v", mux.activations[0])
	}
}