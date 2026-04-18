package filament

import (
	"bytes"
	"encoding/binary"
	"errors"
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

// BuildACEV2RequestFrame is intentionally disabled in the default build.
// ACE v1 devices use the JSON path, and importing/generated protobuf-based
// ACE v2 support in the default filament package caused startup panics on
// ARM static builds before hardware detection could decide between v1 and v2.
func BuildACEV2RequestFrame(cmd string, req map[string]interface{}) ([]byte, error) {
	_ = cmd
	_ = req
	return nil, errors.New("ace v2 support is disabled in the default build")
}

// TranslateACEV2ResponseToJSON returns nil in the default build so callers can
// fall back to JSON parsing for ACE v1 devices.
func TranslateACEV2ResponseToJSON(unwrappedV2 []byte) map[string]interface{} {
	_ = unwrappedV2
	return nil
}
