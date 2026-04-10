package addon

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	OTAUpgradingStart = 1 << iota
	OTAUpgradingEraseing
	OTAUpgradingErased
	OTAUpgradingWriting
	OTAUpgradingWrited
	OTAUpgradingFinish
	OTAUpgradingError
	OTAUpgradingReserved
)

const (
	ECODCheckErr = 20 + iota
	ECODWriteErr
	ECWriteErr
	ECEraseErr
	ECCRCErr
)

const (
	FWFileName         = `^(?:(?:mcu|nozzle)_)?firmware_v(\d{1,3}\.\d{1,3}\.\d{1,3})_(\d{8})\.bin$`
	DefaultVersionFile = "/tmp/version"
)

type TransferChunk struct {
	NextOffset int
	Data       []int64
	Finished   bool
}

type Ota struct {
	mcuName         string
	state           string
	progress        float64
	fwUpdatePath    string
	checkReg        *regexp.Regexp
	isTimeout       bool
	localInfo       map[string]interface{}
	versionFilePath string
}

func NewOta(mcuName string) *Ota {
	return NewOtaWithVersionFile(mcuName, DefaultVersionFile)
}

func NewOtaWithVersionFile(mcuName string, versionFilePath string) *Ota {
	return &Ota{
		mcuName:         mcuName,
		state:           "standby",
		progress:        0,
		fwUpdatePath:    "",
		checkReg:        regexp.MustCompile(FWFileName),
		isTimeout:       false,
		versionFilePath: versionFilePath,
	}
}

func (self *Ota) CheckRegexp() *regexp.Regexp {
	return self.checkReg
}

func (self *Ota) State() string {
	return self.state
}

func (self *Ota) Progress() float64 {
	return self.progress
}

func (self *Ota) UpdatePath() string {
	return self.fwUpdatePath
}

func (self *Ota) IsTimeout() bool {
	return self.isTimeout
}

func (self *Ota) SetTimeout(isTimeout bool) {
	self.isTimeout = isTimeout
}

func (self *Ota) LocalInfo() map[string]interface{} {
	return self.localInfo
}

func (self *Ota) SetLocalInfo(localInfo map[string]interface{}) {
	self.localInfo = localInfo
}

func (self *Ota) SetState(state string) {
	self.state = state
}

func (self *Ota) MarkDownloadStarted() {
	self.state = "upgrading_download"
	self.progress = 0.01
}

func (self *Ota) ShouldTimeout() bool {
	return self.state == "eraseing" || self.state == "writing" || self.state == "transfer_finish"
}

func (self *Ota) BeginUpdate(updatePath string) (string, error) {
	base := filepath.Base(updatePath)
	match := self.checkReg.FindStringSubmatch(base)
	if match == nil {
		return "", fmt.Errorf("firmware file format error")
	}

	mcuPrefix := strings.Split(self.mcuName, "_")[0]
	filePrefix := strings.Split(base, "_")[0]
	if !strings.Contains(base, mcuPrefix) && filePrefix != "firmware" {
		return "", fmt.Errorf("The firmware does not support this mcu")
	}

	self.fwUpdatePath = updatePath
	self.state = "ota start"
	self.progress = 0
	self.isTimeout = false
	return "\"" + strings.ToLower("v"+match[1]) + "\"", nil
}

func (self *Ota) FirmwareCRC() (uint32, error) {
	otaFile, err := os.OpenFile(self.fwUpdatePath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		self.state = "open file error"
		return 0, err
	}
	defer otaFile.Close()

	byteArr, err := io.ReadAll(otaFile)
	if err != nil {
		self.state = "read file error"
		return 0, err
	}
	if len(byteArr) < 9 {
		self.state = "read file error"
		return 0, fmt.Errorf("firmware file too small")
	}

	byteArr = byteArr[:len(byteArr)-9]
	return crc32.ChecksumIEEE(byteArr) & 0xffffffff, nil
}

func (self *Ota) BuildTransferChunk(offset, count int) (*TransferChunk, error) {
	f, err := os.Open(self.fwUpdatePath)
	if err != nil {
		self.state = "open file error"
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		self.state = "read file error"
		return nil, err
	}

	bytesRead := make([]byte, count)
	n, readErr := f.Read(bytesRead)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		self.state = "read file error"
		return nil, readErr
	}

	fs, err := f.Stat()
	if err != nil {
		self.state = "file stat error"
		return nil, err
	}

	if n == 0 && errors.Is(readErr, io.EOF) {
		self.state = "transfer_finish"
		self.progress = 1
		return &TransferChunk{NextOffset: offset, Data: []int64{}, Finished: true}, nil
	}

	data := make([]int64, 0, n)
	for _, d := range bytesRead[:n] {
		data = append(data, int64(d))
	}

	nextOffset := offset + n
	self.state = "writing"
	if fs.Size() <= 0 {
		self.progress = 1
	} else {
		self.progress = math.Round(float64(nextOffset)/float64(fs.Size())*100) / 100
	}

	return &TransferChunk{NextOffset: nextOffset, Data: data, Finished: false}, nil
}

func (self *Ota) HandleStatus(status, errCode int) {
	switch status {
	case OTAUpgradingFinish:
		self.state = "upgrading_restart"
	case OTAUpgradingStart:
		self.state = "start"
	case OTAUpgradingEraseing:
		self.state = "eraseing"
	case OTAUpgradingErased:
		self.state = "erase finish"
	case OTAUpgradingWriting:
		self.state = "writing"
	case OTAUpgradingWrited:
		self.state = "write finish"
	case OTAUpgradingError:
		switch errCode {
		case ECCRCErr:
			self.state = "CRC check error"
		case ECODCheckErr:
			self.state = "ota data check error"
		case ECODWriteErr:
			self.state = "ota data write error"
		case ECWriteErr:
			self.state = "flash write error"
		case ECEraseErr:
			self.state = "flash erase error"
		}
	}
}

func (self *Ota) GetStatus(version interface{}) map[string]interface{} {
	return map[string]interface{}{
		"state":    self.state,
		"progress": self.progress,
		"version":  version,
	}
}

func (self *Ota) ValidateVersion(newVersion string, currentVersion string, crc32 uint32, localCRC int64) ([]int, error) {
	if currentVersion == newVersion {
		if err := self.StorePendingVersion(newVersion); err != nil {
			return nil, err
		}
		return nil, errors.New("version match consistenly,not need upgrad")
	}

	if int64(crc32) == localCRC {
		if err := self.StorePendingVersion(newVersion); err != nil {
			return nil, err
		}
		return nil, errors.New("CRC32 match consistenly,not need upgrad")
	}

	if err := self.StorePendingVersion(newVersion); err != nil {
		return nil, err
	}
	return parseVersionParts(newVersion)
}

func (self *Ota) PendingVersion() string {
	lines, err := self.readVersionEntries()
	if err != nil {
		return ""
	}

	for _, line := range lines {
		split := strings.SplitN(line, ":", 2)
		if len(split) == 2 && split[0] == self.mcuName {
			return split[1]
		}
	}
	return ""
}

func (self *Ota) StorePendingVersion(newVersion string) error {
	lines, err := self.readVersionEntries()
	if err != nil {
		return err
	}

	entry := self.mcuName + ":" + newVersion
	updated := make([]string, 0, len(lines)+1)
	replaced := false
	for _, line := range lines {
		split := strings.SplitN(line, ":", 2)
		if len(split) == 2 && split[0] == self.mcuName {
			if !replaced {
				updated = append(updated, entry)
				replaced = true
			}
			continue
		}
		updated = append(updated, line)
	}
	if !replaced {
		updated = append(updated, entry)
	}
	return self.writeVersionEntries(updated)
}

func (self *Ota) ClearPendingVersion() error {
	lines, err := self.readVersionEntries()
	if err != nil {
		return err
	}

	updated := make([]string, 0, len(lines))
	for _, line := range lines {
		split := strings.SplitN(line, ":", 2)
		if len(split) == 2 && split[0] == self.mcuName {
			continue
		}
		updated = append(updated, line)
	}
	return self.writeVersionEntries(updated)
}

func (self *Ota) HandleReady(currentVersion string) error {
	updateVersion := self.PendingVersion()
	if updateVersion == "" || currentVersion == "" {
		return nil
	}
	if updateVersion == currentVersion {
		return self.UpdateState("update_success")
	}
	return self.UpdateState("update_failed")
}

func (self *Ota) UpdateState(stats string) error {
	self.state = stats
	if err := self.ClearPendingVersion(); err != nil {
		return err
	}
	self.fwUpdatePath = ""
	if self.state == "update_success" {
		return nil
	}
	return fmt.Errorf("ota firmware update failed")
}

func (self *Ota) readVersionEntries() ([]string, error) {
	data, err := os.ReadFile(self.versionFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func (self *Ota) writeVersionEntries(lines []string) error {
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	return os.WriteFile(self.versionFilePath, []byte(content), 0o644)
}

func parseVersionParts(newVersion string) ([]int, error) {
	newVersion = strings.ReplaceAll(newVersion, "\"", "")
	versionStrArr := strings.Split(newVersion, "v")
	if len(versionStrArr) < 2 {
		return nil, fmt.Errorf("invalid version %q", newVersion)
	}
	versionStrArr = strings.Split(versionStrArr[1], ".")
	if len(versionStrArr) != 3 {
		return nil, fmt.Errorf("invalid version %q", newVersion)
	}

	v1, err := strconv.Atoi(versionStrArr[0])
	if err != nil {
		return nil, err
	}
	v2, err := strconv.Atoi(versionStrArr[1])
	if err != nil {
		return nil, err
	}
	v3, err := strconv.Atoi(versionStrArr[2])
	if err != nil {
		return nil, err
	}
	return []int{v1, v2, v3}, nil
}
