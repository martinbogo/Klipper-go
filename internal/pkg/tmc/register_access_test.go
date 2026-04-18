package tmc

import "testing"

type fakeRegisterLocker struct {
	lockCount   int
	unlockCount int
}

func (self *fakeRegisterLocker) Lock() {
	self.lockCount++
}

func (self *fakeRegisterLocker) Unlock() {
	self.unlockCount++
}

type fakeRegisterRuntime struct {
	getCalls int
	setCalls int
	lastReg  string
	lastVal  int64
	getValue int64
	getErr   error
	setErr   error
}

func (self *fakeRegisterRuntime) GetRegister(regName string) (int64, error) {
	self.getCalls++
	self.lastReg = regName
	return self.getValue, self.getErr
}

func (self *fakeRegisterRuntime) SetRegister(regName string, val int64, printTime *float64) error {
	self.setCalls++
	self.lastReg = regName
	self.lastVal = val
	return self.setErr
}

func TestLockedRegisterAccessSerializesCalls(t *testing.T) {
	locker := &fakeRegisterLocker{}
	runtime := &fakeRegisterRuntime{getValue: 42}
	access := NewLockedRegisterAccess(NewFieldHelper(TMC2209Fields, TMC2208SignedFields, TMC2209FieldFormatters, nil), runtime, locker)

	if _, err := access.Get_register("DRV_STATUS"); err != nil {
		t.Fatalf("expected register read to succeed, got %v", err)
	}
	if err := access.Set_register("GCONF", 7, nil); err != nil {
		t.Fatalf("expected register write to succeed, got %v", err)
	}
	if runtime.getCalls != 1 || runtime.setCalls != 1 {
		t.Fatalf("expected one get and one set, got get=%d set=%d", runtime.getCalls, runtime.setCalls)
	}
	if locker.lockCount != 2 || locker.unlockCount != 2 {
		t.Fatalf("expected lock/unlock around each call, got lock=%d unlock=%d", locker.lockCount, locker.unlockCount)
	}
}