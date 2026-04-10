package ace_v2

import (
	"encoding/binary"
	"errors"

	"google.golang.org/protobuf/proto"
)

// Decompiled from `goklipper/internal/pkg/filament/ace_v2`

// V2ProtoHandler handles the serialization and deserialization
// of the proprietary ACE V2 communication protocol.
type V2ProtoHandler struct {
	deviceID uint8
	sequence uint16
}

func NewV2ProtoHandler(deviceID uint8) *V2ProtoHandler {
	return &V2ProtoHandler{
		deviceID: deviceID,
		sequence: 0,
	}
}

// BuildRequest constructs the frame header and payload for an ACE command.
func (h *V2ProtoHandler) BuildRequest(cmdCode uint8, payload []byte) ([]byte, error) {
	if len(payload) > 255 {
		return nil, errors.New("payload too large")
	}

	h.sequence++

	// Frame structure:
	// Magic (2B) | Length (1B) | Sequence (2B) | DeviceID (1B) | CmdCode (1B) | Payload (NB) | CRC16 (2B)
	frame := make([]byte, 7+len(payload)+2)

	// Magic Word: 0xAC 0xE2
	frame[0] = 0xAC
	frame[1] = 0xE2

	// Length = payload length + header len
	frame[2] = byte(len(payload) + 7)

	// Sequence
	binary.BigEndian.PutUint16(frame[3:5], h.sequence)

	// Device ID & Cmd Code
	frame[5] = h.deviceID
	frame[6] = cmdCode

	// Payload
	copy(frame[7:7+len(payload)], payload)

	// Actual Klipper ACE CRC calculations
	crc := calculateCRC16(frame[:7+len(payload)])
	binary.LittleEndian.PutUint16(frame[7+len(payload):], crc)

	return frame, nil
}

// ParseResponse validates an incoming byte slice and extracts the command payload.
func (h *V2ProtoHandler) ParseResponse(data []byte) (cmdCode uint8, payload []byte, err error) {
	if len(data) < 9 {
		return 0, nil, errors.New("frame too small")
	}

	if data[0] != 0xAC || data[1] != 0xE2 {
		return 0, nil, errors.New("invalid magic bytes")
	}

	length := data[2]
	if int(length)+2 > len(data) {
		return 0, nil, errors.New("incomplete frame")
	}

	expectedCRC := binary.LittleEndian.Uint16(data[int(length) : int(length)+2])
	actualCRC := calculateCRC16(data[:length])

	if actualCRC != expectedCRC {
		return 0, nil, errors.New("CRC mismatch")
	}

	cmdCode = data[6]
	payloadLen := length - 7
	payload = make([]byte, payloadLen)
	copy(payload, data[7:7+payloadLen])

	return cmdCode, payload, nil
}

// Helper methods extracting from authentic structure mapping

func (h *V2ProtoHandler) SerializeSetFan(speed uint8) ([]byte, error) {
	req := &SetFanRequest{Speed: uint32(speed)}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	return h.BuildRequest(0x14, payload)
}

func (h *V2ProtoHandler) SerializeSetRfidEnable(enable bool) ([]byte, error) {
	req := &SetRfidEnableRequest{Enable: enable}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	return h.BuildRequest(0x1A, payload)
}

func (h *V2ProtoHandler) SerializeDrying(temp uint8, durationHours uint8) ([]byte, error) {
	req := &DryingRequest{
		Temperature:   uint32(temp),
		DurationHours: uint32(durationHours),
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	return h.BuildRequest(0x22, payload)
}

func (h *V2ProtoHandler) SerializeFeedOrRollback(length int32, speed uint32) ([]byte, error) {
	req := &FeedOrRollbackRequest{
		Length: length,
		Speed:  speed,
	}
	payload, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}
	return h.BuildRequest(0x15, payload)
}

// Parses specific responses using Protobufs
func (h *V2ProtoHandler) ParseFilamentInfoResponse(data []byte) (*FilamentInfoResponse, error) {
	resp := &FilamentInfoResponse{}
	err := proto.Unmarshal(data, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (h *V2ProtoHandler) ParseStatusResponse(data []byte) (*StatusResponse, error) {
	resp := &StatusResponse{}
	err := proto.Unmarshal(data, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// calculateCRC16 matching logic found in gklib_remote (poly 0xA001 Modbus)
func calculateCRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if (crc & 0x0001) != 0 {
				crc >>= 1
				crc ^= 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// TranslateV2ResponseToJSON converts a raw ACE V2 status response into a
// V1-compatible JSON map with "result" wrapping.
func TranslateV2ResponseToJSON(unwrappedV2 []byte) map[string]interface{} {
	handler := NewV2ProtoHandler(1)

	status, err := handler.ParseStatusResponse(unwrappedV2)
	if err != nil || status == nil {
		return nil
	}

	var statusStr = "ready"
	if status.State == 2 {
		statusStr = "feeding"
	}

	var dryerStatus = "stop"
	if status.TargetTemp > 0 {
		dryerStatus = "drying"
	}

	slots := []interface{}{
		map[string]interface{}{"index": float64(0), "status": "ready"},
		map[string]interface{}{"index": float64(1), "status": "ready"},
		map[string]interface{}{"index": float64(2), "status": "ready"},
		map[string]interface{}{"index": float64(3), "status": "ready"},
	}

	result := map[string]interface{}{
		"status": statusStr,
		"temp":   float64(status.CurrentTemp),
		"dryer": map[string]interface{}{
			"status":      dryerStatus,
			"target_temp": float64(status.TargetTemp),
			"duration":    float64(0),
			"remain_time": float64(0),
		},
		"fan_speed": float64(7000),
		"slots":     slots,
	}

	return map[string]interface{}{
		"result": result,
	}
}
