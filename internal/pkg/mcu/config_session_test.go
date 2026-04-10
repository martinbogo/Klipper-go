package mcu

import "testing"

func TestDefaultFileoutputConfigSnapshot(t *testing.T) {
	snapshot := DefaultFileoutputConfigSnapshot()
	if snapshot == nil || snapshot.IsConfig || snapshot.CRC != 0 || snapshot.IsShutdown || snapshot.MoveCount != 500 {
		t.Fatalf("unexpected fileoutput snapshot %#v", snapshot)
	}
}

func TestParseConfigSnapshot(t *testing.T) {
	snapshot := ParseConfigSnapshot(map[string]interface{}{
		"is_config":   int64(1),
		"crc":         int64(42),
		"is_shutdown": int64(0),
		"move_count":  int64(123),
	})
	if snapshot == nil || !snapshot.IsConfig || snapshot.CRC != 42 || snapshot.IsShutdown || snapshot.MoveCount != 123 {
		t.Fatalf("unexpected parsed snapshot %#v", snapshot)
	}
}

func TestEvaluateConfigQuery(t *testing.T) {
	localShutdown := EvaluateConfigQuery(&ConfigSnapshot{}, true, "boom", "mcu", true)
	if localShutdown.ErrorMessage == "" {
		t.Fatalf("expected local shutdown to produce an error")
	}
	needsClear := EvaluateConfigQuery(&ConfigSnapshot{IsShutdown: true}, false, "", "mcu", true)
	if !needsClear.NeedsClearShutdown || needsClear.ErrorMessage != "" {
		t.Fatalf("expected clear-shutdown request, got %#v", needsClear)
	}
	noClear := EvaluateConfigQuery(&ConfigSnapshot{IsShutdown: true}, false, "", "mcu", false)
	if noClear.ErrorMessage == "" {
		t.Fatalf("expected shutdown without clear command to error")
	}
}

func TestEvaluateClearedShutdownSnapshot(t *testing.T) {
	if EvaluateClearedShutdownSnapshot(&ConfigSnapshot{IsShutdown: true}, "mcu") == "" {
		t.Fatalf("expected stuck shutdown to error")
	}
	if EvaluateClearedShutdownSnapshot(&ConfigSnapshot{IsShutdown: false}, "mcu") != "" {
		t.Fatalf("did not expect non-shutdown snapshot to error")
	}
}

func TestBuildConnectDecision(t *testing.T) {
	nilDecision := BuildConnectDecision(nil, "command", "start", "mcu")
	if nilDecision.ReturnError == "" {
		t.Fatalf("expected nil snapshot to return an error decision")
	}
	unconfigured := BuildConnectDecision(&ConfigSnapshot{IsConfig: false}, "rpi_usb", "start", "mcu")
	if !unconfigured.NeedsPreConfigReset || !unconfigured.SendConfig || !unconfigured.NeedsRequery {
		t.Fatalf("unexpected unconfigured decision %#v", unconfigured)
	}
	configured := BuildConnectDecision(&ConfigSnapshot{IsConfig: true, CRC: 77}, "command", "start", "mcu")
	if !configured.SendConfig || !configured.UsePrevCRC || configured.PrevCRC != 77 {
		t.Fatalf("unexpected configured decision %#v", configured)
	}
	firmwareRestart := BuildConnectDecision(&ConfigSnapshot{IsConfig: true}, "command", "firmware_restart", "mcu")
	if firmwareRestart.PanicMessage == "" {
		t.Fatalf("expected firmware restart to panic")
	}
}

func TestValidateConfiguredSnapshot(t *testing.T) {
	if ValidateConfiguredSnapshot(nil, false, 0, "mcu") == "" {
		t.Fatalf("expected nil snapshot to fail for non-fileoutput")
	}
	if ValidateConfiguredSnapshot(&ConfigSnapshot{IsConfig: false}, false, 0, "mcu") == "" {
		t.Fatalf("expected unconfigured snapshot to fail for non-fileoutput")
	}
	if ValidateConfiguredSnapshot(&ConfigSnapshot{IsConfig: true, MoveCount: 2}, false, 3, "mcu") == "" {
		t.Fatalf("expected too-few-moves validation failure")
	}
	if ValidateConfiguredSnapshot(&ConfigSnapshot{IsConfig: true, MoveCount: 3}, false, 3, "mcu") != "" {
		t.Fatalf("did not expect valid snapshot to fail")
	}
}
