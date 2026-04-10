package filament

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"goklipper/common/logger"
	"goklipper/common/utils/reflects"
	ace_v2 "goklipper/internal/pkg/filament/ace_v2"
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
	self.dev.Close()
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

	logger.Infof("ACE proxy evaluating cmd=%s", cmd)

	var frame []byte
	var err error

	switch cmd {
	case "feed":
		handler := ace_v2.NewV2ProtoHandler(1)
		frame, err = handler.SerializeFeedOrRollback(50, 50)
	case "rollback":
		handler := ace_v2.NewV2ProtoHandler(1)
		frame, err = handler.SerializeFeedOrRollback(-50, 50)
	case "set_rfid":
		enable, _ := req["enable"].(bool)
		handler := ace_v2.NewV2ProtoHandler(1)
		frame, err = handler.SerializeSetRfidEnable(enable)
	case "drying":
		params, ok := req["params"].(map[string]interface{})
		if ok {
			var temp, dur int

			if t, ok := params["temp"].(float64); ok {
				temp = int(t)
			} else if t, ok := params["temp"].(int); ok {
				temp = t
			}

			if d, ok := params["duration"].(float64); ok {
				dur = int(d)
			} else if d, ok := params["duration"].(int); ok {
				dur = d
			}

			handler := ace_v2.NewV2ProtoHandler(1)
			frame, err = handler.SerializeDrying(uint8(temp), uint8(dur/60))
		}
	case "drying_stop":
		handler := ace_v2.NewV2ProtoHandler(1)
		frame, err = handler.SerializeDrying(0, 0)
	}

	if err == nil && frame != nil {
		logger.Infof("ACE proxy wrote proto frame for cmd=%s", cmd)
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
		if err != nil {
			logger.Errorf("ACE proxy serialization error: %v", err)
		}
		logger.Infof("ACE proxy passing back to V1 natively: %v", req)
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
			logger.Infof("ACE queue pumped writer payload: %v", task.(aceRequestInfo).request)
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
		text_buffer := append(self.read_buffer, raw_bytes[:n]...)
		i := bytes.IndexByte(text_buffer, FrameEnd)
		if i >= 0 {
			buffer = text_buffer
			self.read_buffer = []byte{}
		} else {
			self.read_buffer = append(self.read_buffer, raw_bytes[:n]...)
			return nil
		}
	} else {
		if (eventtime - self.send_time) > 2 {
			return fmt.Errorf(RespondTimeoutError)
		}
		return nil
	}

	if len(buffer) < MinFrameSize {
		return nil
	}

	if buffer[0] != FrameStart1 || buffer[1] != FrameStart2 {
		logger.Error("Invalid data from ACE PRO (head bytes)", string(buffer))
		return nil
	}

	payload_len := uint16(buffer[2]) | uint16(buffer[3])<<8
	if len(buffer) < int(payload_len+MinFrameSize) {
		logger.Errorf("Invalid data from ACE PRO (len) {%d} {%d} %v", payload_len, len(buffer), string(buffer))
		return nil
	}

	if buffer[len(buffer)-1] != FrameEnd {
		logger.Errorf("Invalid data from ACE PRO (end bytes)", string(buffer))
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
			ret = ace_v2.TranslateV2ResponseToJSON(unwrappedV2)
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
		logger.Warnf("Response missing 'id', ignoring: %v", ret)
		return nil
	}

	var id int
	switch v := idVal.(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	default:
		logger.Warnf("Response 'id' is not a number, ignoring: %v", ret)
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
	return int(self.dev.GetFd())
}

func (self *AceCommun) Is_send_queue_empty() bool {
	return self.queue.Is_empty()
}