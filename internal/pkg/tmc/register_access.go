package tmc

type RegisterLocker interface {
	Lock()
	Unlock()
}

type registerLockerFuncs struct {
	lock   func()
	unlock func()
}

func NewRegisterLockerFuncs(lock, unlock func()) RegisterLocker {
	return &registerLockerFuncs{lock: lock, unlock: unlock}
}

func (self *registerLockerFuncs) Lock() {
	if self.lock != nil {
		self.lock()
	}
}

func (self *registerLockerFuncs) Unlock() {
	if self.unlock != nil {
		self.unlock()
	}
}

type RegisterRuntime interface {
	GetRegister(regName string) (int64, error)
	SetRegister(regName string, val int64, printTime *float64) error
}

type LockedRegisterAccess struct {
	fields  *FieldHelper
	runtime RegisterRuntime
	mutex   RegisterLocker
}

var _ RegisterAccess = (*LockedRegisterAccess)(nil)

func NewLockedRegisterAccess(fields *FieldHelper, runtime RegisterRuntime, mutex RegisterLocker) *LockedRegisterAccess {
	return &LockedRegisterAccess{fields: fields, runtime: runtime, mutex: mutex}
}

func (self *LockedRegisterAccess) Get_fields() *FieldHelper {
	return self.fields
}

func (self *LockedRegisterAccess) Get_register(regName string) (int64, error) {
	if self.mutex != nil {
		self.mutex.Lock()
		defer self.mutex.Unlock()
	}
	return self.runtime.GetRegister(regName)
}

func (self *LockedRegisterAccess) Set_register(regName string, val int64, printTime *float64) error {
	if self.mutex != nil {
		self.mutex.Lock()
		defer self.mutex.Unlock()
	}
	return self.runtime.SetRegister(regName, val, printTime)
}
