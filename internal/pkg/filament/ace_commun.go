package filament

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"goklipper/common/logger"
	"goklipper/common/utils/reflects"
	"goklipper/internal/pkg/queue"
	serialpkg "goklipper/internal/pkg/serialhdl"

	"github.com/tarm/serial"
)

const (
	RespondTimeoutError = "Respond timeout with the ACE PRO"
	UnableToCommunError = "Unable to communicate with the ACE PRO"
	OpenRemoteDevError  = "Unable to open remote dev"
	OpenSerialDevError  = "Unable to open serial port"
	NotFoundSerialError = "Not found serial port"
)

type AceCommun struct {
	name         string
	dev          serialpkg.SerialDeviceBase
	baud         int
	is_connected bool
	request_id   int
	send_time    float64
	queue        *queue.Queue
	read_buffer  []byte
	callback_map map[int]func(map[string]interface{})
	IsV2         bool
}

type aceRequestInfo struct {
	request  map[string]interface{}
	callback func(map[string]interface{})
}

func NewAceCommunication(name string, baud int) *AceCommun {
	self := new(AceCommun)
	self.is_connected = false
	self.dev = nil
	self.name = name
	self.baud = baud
	self.send_time = 0.
	self.read_buffer = nil
	self.callback_map = map[int]func(resp map[string]interface{}){}
	self.request_id = 0
	return self
}

func (self *AceCommun) Check_ace_port_exit(fileName string) bool {
	info, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (self *AceCommun) Connect() error {
	if strings.HasPrefix(self.name, "tcp@") {
		dev, err := serialpkg.NewRemoteDev(self.name)
		if err != nil {
			logger.Errorf("%s %s: %s", OpenRemoteDevError, self.name, err)
			return fmt.Errorf("%s %s: %s", OpenRemoteDevError, self.name, err)
		}
		self.dev = dev
	} else {
		if !self.Check_ace_port_exit(self.name) {
			return fmt.Errorf("%s %s ", NotFoundSerialError, self.name)
		}

		cfg := &serial.Config{Name: self.name, Baud: self.baud, ReadTimeout: time.Microsecond * 900}
		port, err := serial.OpenPort(cfg)
		if err != nil {
			logger.Errorf("%s %s: %s", OpenSerialDevError, self.name, err)
			return fmt.Errorf("%s %s: %s", OpenSerialDevError, self.name, err)
		}

		f := reflects.GetPrivateFieldValue(port, "f").(*os.File)
		serialpkg.PrepareACESerialPort(f, self.name, self.baud)

		self.dev = serialpkg.NewSerialDev(port, cfg, f)
	}
	self.is_connected = true
	self.queue = queue.NewQueue()
	return nil
}

func (self *AceCommun) Disconnect() {
	if self.dev != nil {
		self.dev.Close()
	}
	self.is_connected = false
	self.queue = nil
	self.dev = nil
	self.send_time = 0.
	self.read_buffer = nil
	self.request_id = 0
	self.callback_map = map[int]func(map[string]interface{}){}
}

func (self *AceCommun) Is_connected() bool {
	return self.is_connected
}

func (self *AceCommun) sendRequestV1(req map[string]interface{}) error {
	if _, ok := req["id"]; !ok {
		req["id"] = self.request_id
		self.request_id++
	}
	var buf []byte
	buf = append(buf, FrameStart1)
	buf = append(buf, FrameStart2)

	bts, _ := json.Marshal(req)
	size := len(bts)

	buf = append(buf, byte(size))
	buf = append(buf, byte(size>>8))

	crc := CalcCRC(bts)

	buf = append(buf, bts...)
	buf = append(buf, byte(crc))
	buf = append(buf, byte(crc>>8))
	buf = append(buf, FrameEnd)
	fd := int(self.dev.GetFd())
	_, err := syscall.Write(fd, buf)

	return err
}

func (self *AceCommun) sendRequest(req map[string]interface{}) error {
	if !self.IsV2 {
		return self.sendRequestV1(req)
	}
	cmd, ok := req["cmd"].(string)
	if !ok {
		cmd, _ = req["method"].(string)
	}

	var frame []byte
	var err error

	switch cmd {
	case "feed":
		frame, err = BuildACEV2RequestFrame(cmd, req)
	case "rollback":
		frame, err = BuildACEV2RequestFrame(cmd, req)
	case "set_rfid":
		frame, err = BuildACEV2RequestFrame(cmd, req)
	case "drying":
		frame, err = BuildACEV2RequestFrame(cmd, req)
	case "drying_stop":
		frame, err = BuildACEV2RequestFrame(cmd, req)
	}

	if err == nil && frame != nil {
		fd := int(self.dev.GetFd())
		_, _ = syscall.Write(fd, frame)

		if idVal, ok := req["id"]; ok {
			if id, ok := idVal.(int); ok {
				if cb, ok := self.callback_map[id]; ok {
					delete(self.callback_map, id)
					cb(map[string]interface{}{
						"id":     float64(id),
						"result": map[string]interface{}{"success": true},
					})
				}
			}
		}
	} else {
		return self.sendRequestV1(req)
	}

	return nil
}

func (self *AceCommun) Writer(eventtime float64) {
	if !self.queue.Is_empty() {
		task := self.queue.Get_nowait()
		if task != nil {
			id := self.request_id
			self.request_id += 1
			self.callback_map[id] = task.(aceRequestInfo).callback
			task.(aceRequestInfo).request["id"] = id
			self.sendRequest(task.(aceRequestInfo).request)
			self.send_time = eventtime
		}
	}
}

func (self *AceCommun) Reader(eventtime float64) error {
	buffer := []byte{}
	raw_bytes := make([]byte, 4096)
	fd := int(self.dev.GetFd())
	n, err := syscall.Read(fd, raw_bytes)
	if err != nil {
		return fmt.Errorf("%s %v", UnableToCommunError, err)
	}

	if n > 0 && len(raw_bytes) > 0 {
		self.read_buffer = append(self.read_buffer, raw_bytes[:n]...)
	} else if len(self.read_buffer) == 0 {
		if (eventtime - self.send_time) > 2 {
			return fmt.Errorf(RespondTimeoutError)
		}
		return nil
	}

	for len(self.read_buffer) >= MinFrameSize {
		if self.read_buffer[0] != FrameStart1 || self.read_buffer[1] != FrameStart2 {
			self.read_buffer = self.read_buffer[1:]
			continue
		}

		payload_len := uint16(self.read_buffer[2]) | uint16(self.read_buffer[3])<<8
		expected_len := int(payload_len) + MinFrameSize

		// If payload_len is unreasonably large, it's likely a sync issue; skip the start bytes
		if expected_len > 4096 {
			self.read_buffer = self.read_buffer[1:]
			continue
		}

		if len(self.read_buffer) >= expected_len {
			buffer = self.read_buffer[:expected_len]
			self.read_buffer = append([]byte(nil), self.read_buffer[expected_len:]...)
			break
		} else {
			break
		}
	}

	if len(buffer) == 0 {
		if (eventtime - self.send_time) > 2 {
			return fmt.Errorf(RespondTimeoutError)
		}
		return nil
	}

	payload_len := uint16(buffer[2]) | uint16(buffer[3])<<8
	if buffer[len(buffer)-1] != FrameEnd {
		logger.Errorf("Invalid data from ACE PRO (end bytes: got %x)", buffer[len(buffer)-1])
		return nil
	}

	payload := buffer[4 : 4+payload_len]
	crc := binary.LittleEndian.Uint16(buffer[4+payload_len : 4+payload_len+2])

	if len(buffer) < int(4+payload_len+2+1) {
		logger.Errorf("Invalid data from ACE PRO (len) {%d} {%d} {%d} %v", payload_len, len(buffer), crc, string(buffer))
		return nil
	}

	if crc != CalcCRC(payload) {
		logger.Error("Invalid data from ACE PRO (CRC)")
		return nil
	}

	var ret map[string]interface{}
	if self.IsV2 {
		unwrappedV2, errV2 := NewV2ProtoHandler().ParseResponse(payload)
		if errV2 == nil && len(unwrappedV2) > 0 {
			ret = TranslateACEV2ResponseToJSON(unwrappedV2)
			if ret != nil {
				for pendingId := range self.callback_map {
					ret["id"] = float64(pendingId)
					break
				}
			}
		}
	}

	if ret == nil {
		err := json.Unmarshal(payload, &ret)
		if err != nil {
			logger.Errorf("json error: %v, payload: %v", err, payload)
			return nil
		}
	}

	idVal, ok := ret["id"]
	if !ok || idVal == nil {
		return nil
	}

	var id int
	switch v := idVal.(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	default:
		return nil
	}

	if cb, ok := self.callback_map[id]; ok {
		delete(self.callback_map, id)
		cb(ret)
	}
	return nil
}

func (self *AceCommun) Push_send_queue(request map[string]interface{}, callback func(map[string]interface{})) {
	self.queue.Put_nowait(aceRequestInfo{request, callback})
}

func (self *AceCommun) Name() string {
	return self.name
}

func (self *AceCommun) Fd() int {
	if self.dev == nil {
		return -1
	}
	return int(self.dev.GetFd())
}

func (self *AceCommun) Is_send_queue_empty() bool {
	if self.queue == nil {
		return true
	}
	return self.queue.Is_empty()
}
