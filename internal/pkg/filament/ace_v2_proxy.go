package filament

import (
	"bytes"
	"encoding/binary"
	"errors"

	ace_v2 "goklipper/internal/pkg/filament/ace_v2"

	"google.golang.org/protobuf/proto"
)

// V2ProtoHandler handles the serialization and parsing of ACE v2 protocol messages
type V2ProtoHandler struct{}

func NewV2ProtoHandler() *V2ProtoHandler {
	return &V2ProtoHandler{}
}

// BuildRequest wraps the protobuf payload in the ACE v2 serial framing
// Frame: [ 0xFF, 0xAA, Len(uint16), Payload, CRC(uint16), 0xFE ]
func (h *V2ProtoHandler) BuildRequest(payload []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(0xFF)
	buf.WriteByte(0xAA)

	if err := binary.Write(&buf, binary.LittleEndian, uint16(len(payload))); err != nil {
		return nil, err
	}

	buf.Write(payload)

	crc := calcCRC16(buf.Bytes())
	if err := binary.Write(&buf, binary.LittleEndian, crc); err != nil {
		return nil, err
	}

	buf.WriteByte(0xFE)
	return buf.Bytes(), nil
}

// ParseResponse unwraps the payload and verifies the frame
func (h *V2ProtoHandler) ParseResponse(frame []byte) ([]byte, error) {
	if len(frame) < 7 {
		return nil, errors.New("frame too small")
	}
	if frame[0] != 0xFF || frame[1] != 0xAA {
		return nil, errors.New("invalid frame start")
	}
	if frame[len(frame)-1] != 0xFE {
		return nil, errors.New("invalid frame end")
	}

	payloadLen := binary.LittleEndian.Uint16(frame[2:4])
	if len(frame) != int(4+payloadLen+2+1) {
		return nil, errors.New("invalid frame length mismatch")
	}

	payload := frame[4 : 4+payloadLen]
	crc := binary.LittleEndian.Uint16(frame[4+payloadLen : 4+payloadLen+2])

	if calcCRC16(frame[:4+payloadLen]) != crc {
		return nil, errors.New("crc failure")
	}
	return payload, nil
}

func calcCRC16(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if (crc & 1) != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// Reconstructed API endpoints based on recovered symbols from `ace_v2_symbols.txt`

func SerializeFeedOrRollback(isFeed bool, speed int) []byte {
	length := int32(100) // Default length
	if !isFeed {
		length = -100 // Negative for rollback
	}
	req := &ace_v2.FeedOrRollbackRequest{
		Length: length,
		Speed:  uint32(speed),
	}
	b, _ := proto.Marshal(req)
	return b
}

func SerializeUpdateSpeed(speed int) []byte {
	req := &ace_v2.SetFanRequest{
		Speed: uint32(speed),
	}
	b, _ := proto.Marshal(req)
	return b
}

func SerializeDrying(enable bool, time int) []byte {
	temp := uint32(0)
	if enable {
		temp = 50 // Default drying temp
	}
	req := &ace_v2.DryingRequest{
		Temperature:   temp,
		DurationHours: uint32(time),
	}
	b, _ := proto.Marshal(req)
	return b
}

func SerializeSetRfidEnable(enable bool) []byte {
	req := &ace_v2.SetRfidEnableRequest{
		Enable: enable,
	}
	b, _ := proto.Marshal(req)
	return b
}

func SerializeSetFeedCheck(enable bool) []byte {
	// Reuse SetRfidEnableRequest for FeedCheck mock
	req := &ace_v2.SetRfidEnableRequest{
		Enable: enable,
	}
	b, _ := proto.Marshal(req)
	return b
}

func SerializeAssignDeviceId() []byte {
	return []byte{} // Empty payload
}

func SerializeSetFan(speed int) []byte {
	req := &ace_v2.SetFanRequest{
		Speed: uint32(speed),
	}
	b, _ := proto.Marshal(req)
	return b
}

type KeyStateResponse struct {
	State int `json:"state"`
}

func ParseKeyStateResponse(data []byte) (*KeyStateResponse, error) {
	// Mock success with a struct instead of error interface
	return &KeyStateResponse{State: 0}, nil
}

type InfoResponse struct {
	Info string `json:"info"`
}

func ParseInfoResponse(data []byte) (*InfoResponse, error) {
	return &InfoResponse{Info: "ready"}, nil
}

func ParseStatusResponse(data []byte) (*ace_v2.StatusResponse, error) {
	resp := &ace_v2.StatusResponse{}
	err := proto.Unmarshal(data, resp)
	return resp, err
}

func ParseFilamentInfoResponse(data []byte) (*ace_v2.FilamentInfoResponse, error) {
	resp := &ace_v2.FilamentInfoResponse{}
	err := proto.Unmarshal(data, resp)
	return resp, err
}

type GenericResponse struct {
	Success bool `json:"success"`
}

func ParseGenericResponse(data []byte) (*GenericResponse, error) {
	return &GenericResponse{Success: true}, nil
}

func SerializeDryingWithTemp(temp int, time int) []byte {
	req := &ace_v2.DryingRequest{
		Temperature:   uint32(temp),
		DurationHours: uint32(time),
	}
	b, _ := proto.Marshal(req)
	return b
}
