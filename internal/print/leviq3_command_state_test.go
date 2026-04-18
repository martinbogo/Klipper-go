package print

import "testing"

func TestLeviQ3CommandStateTracksOpenAndCancellation(t *testing.T) {
	state := NewLeviQ3CommandState()
	if state.IsOpen() {
		t.Fatal("new command state should start closed")
	}

	state.Begin()
	if !state.IsOpen() {
		t.Fatal("command state should be open after Begin")
	}
	if cancelled, reason := state.CancelledState(); cancelled || reason != "" {
		t.Fatalf("unexpected initial cancellation state: cancelled=%t reason=%q", cancelled, reason)
	}

	reason := state.NoteCancellation("printer stopped")
	if reason != "printer stopped" {
		t.Fatalf("NoteCancellation() = %q, want %q", reason, "printer stopped")
	}
	if cancelled, gotReason := state.CancelledState(); !cancelled || gotReason != "printer stopped" {
		t.Fatalf("unexpected cancellation state: cancelled=%t reason=%q", cancelled, gotReason)
	}

	state.Finish()
	if state.IsOpen() {
		t.Fatal("command state should be closed after Finish")
	}

	state.Begin()
	if cancelled, gotReason := state.CancelledState(); cancelled || gotReason != "" {
		t.Fatalf("Begin() should clear cancellation state, got cancelled=%t reason=%q", cancelled, gotReason)
	}
}

func TestLeviQ3CommandStateEnsureActive(t *testing.T) {
	state := NewLeviQ3CommandState()
	if err := state.EnsureActive("run", false); err != nil {
		t.Fatalf("EnsureActive() unexpected error: %v", err)
	}
	if err := state.EnsureActive("run", true); err == nil || err.Error() != "run: printer shutdown" {
		t.Fatalf("EnsureActive() shutdown error = %v", err)
	}

	state.NoteCancellation("")
	if err := state.EnsureActive("probe", false); err == nil || err.Error() != "probe: leviq3 cancelled" {
		t.Fatalf("EnsureActive() cancellation error = %v", err)
	}
	if err := state.EnsureActive("", false); err == nil || err.Error() != "leviq3 cancelled" {
		t.Fatalf("EnsureActive() empty-stage error = %v", err)
	}
}