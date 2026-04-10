package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOtaBeginUpdateValidatesTarget(t *testing.T) {
	ota := NewOtaWithVersionFile("nozzle_mcu", filepath.Join(t.TempDir(), "version"))

	version, err := ota.BeginUpdate("/tmp/firmware_v1.2.3_20240101.bin")
	if err != nil {
		t.Fatalf("BeginUpdate() unexpected error: %v", err)
	}
	if version != "\"v1.2.3\"" {
		t.Fatalf("BeginUpdate() version = %q, want %q", version, "\"v1.2.3\"")
	}

	if _, err := ota.BeginUpdate("/tmp/mcu_firmware_v1.2.3_20240101.bin"); err == nil {
		t.Fatalf("BeginUpdate() expected target validation error")
	}
}

func TestOtaPendingVersionUsesSharedJournalPerMCU(t *testing.T) {
	versionFile := filepath.Join(t.TempDir(), "version")
	mcu := NewOtaWithVersionFile("mcu", versionFile)
	nozzle := NewOtaWithVersionFile("nozzle_mcu", versionFile)

	if err := mcu.StorePendingVersion("\"v1.2.3\""); err != nil {
		t.Fatalf("StorePendingVersion(mcu) failed: %v", err)
	}
	if err := nozzle.StorePendingVersion("\"v2.0.0\""); err != nil {
		t.Fatalf("StorePendingVersion(nozzle) failed: %v", err)
	}

	if got := mcu.PendingVersion(); got != "\"v1.2.3\"" {
		t.Fatalf("PendingVersion(mcu) = %q, want %q", got, "\"v1.2.3\"")
	}
	if got := nozzle.PendingVersion(); got != "\"v2.0.0\"" {
		t.Fatalf("PendingVersion(nozzle) = %q, want %q", got, "\"v2.0.0\"")
	}

	if err := mcu.ClearPendingVersion(); err != nil {
		t.Fatalf("ClearPendingVersion(mcu) failed: %v", err)
	}
	if got := nozzle.PendingVersion(); got != "\"v2.0.0\"" {
		t.Fatalf("PendingVersion(nozzle) after mcu clear = %q, want %q", got, "\"v2.0.0\"")
	}

	content, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("ReadFile(versionFile) failed: %v", err)
	}
	if string(content) != "nozzle_mcu:\"v2.0.0\"\n" {
		t.Fatalf("shared version journal = %q", string(content))
	}
}

func TestOtaHandleReadyClearsOnlyMatchingMCU(t *testing.T) {
	versionFile := filepath.Join(t.TempDir(), "version")
	mcu := NewOtaWithVersionFile("mcu", versionFile)
	nozzle := NewOtaWithVersionFile("nozzle_mcu", versionFile)

	if err := mcu.StorePendingVersion("\"v1.2.3\""); err != nil {
		t.Fatalf("StorePendingVersion(mcu) failed: %v", err)
	}
	if err := nozzle.StorePendingVersion("\"v2.0.0\""); err != nil {
		t.Fatalf("StorePendingVersion(nozzle) failed: %v", err)
	}

	if err := mcu.HandleReady("\"v1.2.3\""); err != nil {
		t.Fatalf("HandleReady(success) error: %v", err)
	}
	if mcu.State() != "update_success" {
		t.Fatalf("HandleReady(success) state = %q", mcu.State())
	}
	if got := nozzle.PendingVersion(); got != "\"v2.0.0\"" {
		t.Fatalf("PendingVersion(nozzle) after success = %q, want %q", got, "\"v2.0.0\"")
	}

	if err := nozzle.HandleReady("\"v9.9.9\""); err == nil {
		t.Fatalf("HandleReady(failure) expected error")
	}
	if nozzle.State() != "update_failed" {
		t.Fatalf("HandleReady(failure) state = %q", nozzle.State())
	}
}

func TestOtaBuildTransferChunkKeepsFinalPartialChunk(t *testing.T) {
	tempDir := t.TempDir()
	firmwarePath := filepath.Join(tempDir, "firmware_v1.2.3_20240101.bin")
	if err := os.WriteFile(firmwarePath, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile(firmware) failed: %v", err)
	}

	ota := NewOtaWithVersionFile("mcu", filepath.Join(tempDir, "version"))
	if _, err := ota.BeginUpdate(firmwarePath); err != nil {
		t.Fatalf("BeginUpdate() failed: %v", err)
	}

	chunk, err := ota.BuildTransferChunk(0, 10)
	if err != nil {
		t.Fatalf("BuildTransferChunk(first) error: %v", err)
	}
	if chunk.Finished {
		t.Fatalf("BuildTransferChunk(first) unexpectedly marked finished")
	}
	if chunk.NextOffset != 3 {
		t.Fatalf("BuildTransferChunk(first) next offset = %d, want 3", chunk.NextOffset)
	}
	if got, want := chunk.Data, []int64{97, 98, 99}; !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildTransferChunk(first) data = %v, want %v", got, want)
	}
	if ota.State() != "writing" {
		t.Fatalf("BuildTransferChunk(first) state = %q, want %q", ota.State(), "writing")
	}

	chunk, err = ota.BuildTransferChunk(chunk.NextOffset, 10)
	if err != nil {
		t.Fatalf("BuildTransferChunk(second) error: %v", err)
	}
	if !chunk.Finished {
		t.Fatalf("BuildTransferChunk(second) expected finished chunk")
	}
	if chunk.NextOffset != 3 || len(chunk.Data) != 0 {
		t.Fatalf("BuildTransferChunk(second) = %+v", chunk)
	}
	if ota.State() != "transfer_finish" {
		t.Fatalf("BuildTransferChunk(second) state = %q, want %q", ota.State(), "transfer_finish")
	}
}

func TestOtaValidateVersionStoresPendingVersionAndParsesParts(t *testing.T) {
	versionFile := filepath.Join(t.TempDir(), "version")
	ota := NewOtaWithVersionFile("mcu", versionFile)

	parts, err := ota.ValidateVersion("\"v1.2.3\"", "\"v1.0.0\"", 123, 456)
	if err != nil {
		t.Fatalf("ValidateVersion() unexpected error: %v", err)
	}
	if got, want := parts, []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidateVersion() parts = %v, want %v", got, want)
	}
	if got := ota.PendingVersion(); got != "\"v1.2.3\"" {
		t.Fatalf("PendingVersion() = %q, want %q", got, "\"v1.2.3\"")
	}

	if _, err := ota.ValidateVersion("\"v1.2.3\"", "\"v1.2.3\"", 123, 999); err == nil {
		t.Fatalf("ValidateVersion() expected same-version rejection")
	}
	if _, err := ota.ValidateVersion("\"v1.2.3\"", "\"v1.0.0\"", 123, 123); err == nil {
		t.Fatalf("ValidateVersion() expected crc-match rejection")
	}
}
