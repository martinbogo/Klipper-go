package motion

import "testing"

func TestLegacyRailStateTracksRangeAndHomingInfo(t *testing.T) {
	state := NewLegacyRailState(1.5, 9.5, LegacyRailHomingInfo{
		Speed:             25,
		PositionEndstop:   7,
		RetractSpeed:      9,
		RetractDist:       2,
		PositiveDir:       true,
		SecondHomingSpeed: 4,
	})

	if minPos, maxPos := state.Range(); minPos != 1.5 || maxPos != 9.5 {
		t.Fatalf("unexpected range: %v..%v", minPos, maxPos)
	}

	info := state.HomingInfo()
	if info.Speed != 25 || info.PositionEndstop != 7 || !info.PositiveDir {
		t.Fatalf("unexpected homing info: %#v", info)
	}

	info.Speed = 99
	if state.HomingInfo().Speed != 25 {
		t.Fatalf("expected homing info copy to be independent")
	}

	state.SetHomingInfo(LegacyRailHomingInfo{Speed: 11, PositionEndstop: 3})
	if got := state.HomingInfo().Speed; got != 11 {
		t.Fatalf("expected updated speed 11, got %v", got)
	}
	if got := state.HomingInfo().PositionEndstop; got != 3 {
		t.Fatalf("expected updated position_endstop 3, got %v", got)
	}

	if minPos, maxPos := state.Get_range(); minPos != 1.5 || maxPos != 9.5 {
		t.Fatalf("unexpected legacy range: %v..%v", minPos, maxPos)
	}
	legacyInfo := state.Get_homing_info()
	if legacyInfo.Speed != 11 || legacyInfo.PositionEndstop != 3 {
		t.Fatalf("unexpected legacy homing info: %#v", legacyInfo)
	}
	state.Set_homing_info(&LegacyRailHomingInfo{Speed: 5, PositionEndstop: 2})
	if got := state.HomingInfo().Speed; got != 5 {
		t.Fatalf("expected updated legacy speed 5, got %v", got)
	}
}
