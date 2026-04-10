package motion

import "testing"

func TestManualStepperCoreQueueMoveAndDwell(t *testing.T) {
	core := NewManualStepperCore()
	if core.Trapq() == nil {
		t.Fatal("expected trapq to be allocated")
	}
	if got := core.SyncPrintTime(2.5); got != 0 {
		t.Fatalf("SyncPrintTime() delay = %v, want 0", got)
	}
	if got := core.NextCmdTime(); got != 2.5 {
		t.Fatalf("NextCmdTime() = %v, want 2.5", got)
	}
	endTime := core.QueueMove(0, 10, 5, 1)
	if endTime <= 2.5 {
		t.Fatalf("QueueMove() endTime = %v, want > 2.5", endTime)
	}
	if got := core.SyncPrintTime(1.0); got <= 0 {
		t.Fatalf("SyncPrintTime() delay after queued move = %v, want > 0", got)
	}
	beforeDwell := core.NextCmdTime()
	core.Dwell(0.75)
	if got := core.NextCmdTime(); got <= beforeDwell {
		t.Fatalf("Dwell() nextCmdTime = %v, want > %v", got, beforeDwell)
	}
}