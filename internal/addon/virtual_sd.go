package addon

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goklipper/common/utils/maths"
	"goklipper/internal/pkg/util"
)

var validGCodeExts = map[string]struct{}{
	"g":     {},
	"gco":   {},
	"gcode": {},
}

type FileEntry struct {
	Path string
	Size int64
}

type VirtualSD struct {
	SdcardDirname     string
	CurrentFile       *os.File
	FileSize          int64
	MustPauseWork     bool
	CmdFromSD         bool
	NextFilePosition  int
	FilePosition      int
}

func NewVirtualSD(sdcardDirname string) *VirtualSD {
	return &VirtualSD{SdcardDirname: sdcardDirname}
}

func HasValidGCodeExtension(filename string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	_, ok := validGCodeExts[ext]
	return ok
}

func (self *VirtualSD) GetFileList(checkSubdirs bool) ([]FileEntry, error) {
	if checkSubdirs {
		entries := []FileEntry{}
		if err := filepath.Walk(self.SdcardDirname, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !HasValidGCodeExtension(info.Name()) {
				return nil
			}
			relPath, err := filepath.Rel(self.SdcardDirname, path)
			if err != nil {
				return err
			}
			entries = append(entries, FileEntry{Path: strings.ToLower(relPath), Size: info.Size()})
			return nil
		}); err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Path < entries[j].Path
		})
		return entries, nil
	}

	dirs, err := os.ReadDir(self.SdcardDirname)
	if err != nil {
		return nil, err
	}

	entries := []FileEntry{}
	for _, dir := range dirs {
		if strings.HasPrefix(dir.Name(), ".") || dir.IsDir() || !HasValidGCodeExtension(dir.Name()) {
			continue
		}
		info, err := dir.Info()
		if err != nil {
			return nil, err
		}
		entries = append(entries, FileEntry{Path: dir.Name(), Size: info.Size()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Path) < strings.ToLower(entries[j].Path)
	})
	return entries, nil
}

func (self *VirtualSD) GetStatus(isActive bool) map[string]interface{} {
	return map[string]interface{}{
		"file_path":     self.FilePath(),
		"progress":      maths.Round(self.Progress(), 4),
		"is_active":     isActive,
		"file_position": self.FilePosition,
		"file_size":     self.FileSize,
	}
}

func (self *VirtualSD) FilePath() string {
	if self.CurrentFile != nil {
		return self.CurrentFile.Name()
	}
	return ""
}

func (self *VirtualSD) Progress() float64 {
	if self.FileSize != 0 {
		return float64(self.FilePosition) / float64(self.FileSize)
	}
	return 0.
}

func (self *VirtualSD) Reset() {
	self.CloseCurrentFile()
	self.FilePosition = 0
	self.FileSize = 0
	self.NextFilePosition = 0
	self.MustPauseWork = false
	self.CmdFromSD = false
}

func (self *VirtualSD) CloseCurrentFile() error {
	if self.CurrentFile == nil {
		return nil
	}
	err := self.CurrentFile.Close()
	self.CurrentFile = nil
	return err
}

func (self *VirtualSD) LoadFile(filename string) (string, error) {
	filename = strings.Trim(filename, "\"")
	if !HasValidGCodeExtension(filename) {
		return "", fmt.Errorf("Gcode file extension incorrect")
	}

	file := filepath.Join(self.SdcardDirname, strings.TrimPrefix(filename, "/"))
	info, err := os.Stat(file)
	if err != nil {
		return "", fmt.Errorf("Unable to open file: %s", file)
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("Gcode file is empty")
	}

	opened, err := os.Open(file)
	if err != nil {
		return "", err
	}
	if err := self.CloseCurrentFile(); err != nil {
		_ = opened.Close()
		return "", err
	}

	self.CurrentFile = opened
	self.FilePosition = 0
	self.NextFilePosition = 0
	self.FileSize = info.Size()
	return filename, nil
}

func (self *VirtualSD) SeekToFilePosition() error {
	if self.CurrentFile == nil {
		return nil
	}
	_, err := self.CurrentFile.Seek(int64(self.FilePosition), io.SeekStart)
	return err
}

func (self *VirtualSD) PreviewWindow(lookback, lookahead int) (float64, string, string, error) {
	if self.CurrentFile == nil {
		return 0, "", "", nil
	}
	readpos := math.Max(float64(self.FilePosition)-float64(lookback), 0)
	readcount := float64(self.FilePosition) - readpos
	if _, err := self.CurrentFile.Seek(int64(readpos), io.SeekStart); err != nil {
		return readpos, "", "", err
	}
	data := make([]byte, int(readcount)+lookahead)
	n, err := self.CurrentFile.Read(data)
	if err != nil && err != io.EOF {
		return readpos, "", "", err
	}
	data = data[:n]
	pivot := int(readcount)
	if pivot > len(data) {
		pivot = len(data)
	}
	return readpos, string(data[:pivot]), string(data[pivot:]), nil
}

func (self *VirtualSD) ReadLines(partialInput string) ([]string, string, bool, error) {
	if self.CurrentFile == nil {
		return nil, partialInput, true, nil
	}

	data := make([]byte, 8192)
	l, err := self.CurrentFile.Read(data)
	if err != nil && err != io.EOF {
		return nil, partialInput, false, err
	}
	if l <= 0 {
		if closeErr := self.CloseCurrentFile(); closeErr != nil {
			return nil, partialInput, true, closeErr
		}
		return nil, partialInput, true, nil
	}

	lines := strings.Split(string(data[:l]), "\n")
	lines[0] = partialInput + lines[0]
	partialInput = lines[len(lines)-1]
	lines = append([]string{}, lines[:len(lines)-1]...)
	lines = append([]string{}, util.Reverse(lines)...)
	return lines, partialInput, false, nil
}

func (self *VirtualSD) AdvanceLine(line string) (string, int) {
	nextFilePosition := self.FilePosition + len(line) + 1
	self.NextFilePosition = nextFilePosition
	return strings.TrimRight(line, "\r"), nextFilePosition
}

func (self *VirtualSD) CommitFilePosition() {
	self.FilePosition = self.NextFilePosition
}

func (self *VirtualSD) SeekToNextPosition() error {
	if self.CurrentFile == nil {
		return nil
	}
	_, err := self.CurrentFile.Seek(int64(self.FilePosition), io.SeekStart)
	return err
}
