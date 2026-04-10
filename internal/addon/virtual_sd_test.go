package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestVirtualSDGetFileListIncludesRecursiveSupportedFiles(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "root.gco"), []byte("G1 X1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(root.gco) failed: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tempDir, "sub"), 0o755); err != nil {
		t.Fatalf("Mkdir(sub) failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "sub", "Nested.GCODE"), []byte("G1 X2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(Nested.GCODE) failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "ignore.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatalf("WriteFile(ignore.txt) failed: %v", err)
	}

	vsd := NewVirtualSD(tempDir)
	entries, err := vsd.GetFileList(true)
	if err != nil {
		t.Fatalf("GetFileList(true) failed: %v", err)
	}

	got := []string{entries[0].Path, entries[1].Path}
	want := []string{"root.gco", filepath.Join("sub", "nested.gcode")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetFileList(true) paths = %v, want %v", got, want)
	}
}

func TestVirtualSDLoadFileSupportsSubdirectoriesAndExtensions(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tempDir, "plates"), 0o755); err != nil {
		t.Fatalf("Mkdir(plates) failed: %v", err)
	}
	filePath := filepath.Join(tempDir, "plates", "demo.g")
	if err := os.WriteFile(filePath, []byte("G1 X10\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(demo.g) failed: %v", err)
	}

	vsd := NewVirtualSD(tempDir)
	selected, err := vsd.LoadFile("plates/demo.g")
	if err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}
	if selected != "plates/demo.g" {
		t.Fatalf("LoadFile() selected = %q, want %q", selected, "plates/demo.g")
	}
	if got := vsd.FilePath(); got != filePath {
		t.Fatalf("FilePath() = %q, want %q", got, filePath)
	}
	if vsd.FileSize == 0 {
		t.Fatalf("FileSize should be populated")
	}

	if _, err := vsd.LoadFile("plates/demo.txt"); err == nil {
		t.Fatalf("LoadFile() expected invalid extension error")
	}
}

func TestVirtualSDReadLinesPreservesPartialInput(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "demo.gcode")
	if err := os.WriteFile(filePath, []byte("G1 X1\nG1 Y2\nPART"), 0o644); err != nil {
		t.Fatalf("WriteFile(demo.gcode) failed: %v", err)
	}

	vsd := NewVirtualSD(tempDir)
	if _, err := vsd.LoadFile("demo.gcode"); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	lines, partial, eof, err := vsd.ReadLines("")
	if err != nil {
		t.Fatalf("ReadLines(first) failed: %v", err)
	}
	if eof {
		t.Fatalf("ReadLines(first) unexpectedly hit EOF")
	}
	if got, want := lines, []string{"G1 Y2", "G1 X1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadLines(first) lines = %v, want %v", got, want)
	}
	if partial != "PART" {
		t.Fatalf("ReadLines(first) partial = %q, want %q", partial, "PART")
	}

	lines, partial, eof, err = vsd.ReadLines(partial)
	if err != nil {
		t.Fatalf("ReadLines(second) failed: %v", err)
	}
	if !eof {
		t.Fatalf("ReadLines(second) expected EOF")
	}
	if len(lines) != 0 || partial != "PART" {
		t.Fatalf("ReadLines(second) = lines:%v partial:%q", lines, partial)
	}
	if vsd.CurrentFile != nil {
		t.Fatalf("ReadLines(second) should close current file at EOF")
	}
}
