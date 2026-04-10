package filament

// ACE communication frame constants
const (
	FrameStart1  = 0xFF
	FrameStart2  = 0xAA
	FrameEnd     = 0xFE
	MinFrameSize = 7 // start(2) + len(2) + CRC(2) + end(1)
)

// CalcCRC computes the CRC-16 checksum used by ACE communication framing.
func CalcCRC(buf []byte) uint16 {
	var crc uint16 = 0xffff
	for i := 0; i < len(buf); i++ {
		data := uint16(buf[i])
		data ^= crc & 0xff
		data ^= (data & 0x0f) << 4
		crc = ((data << 8) | (crc >> 8)) ^ (data >> 4) ^ (data << 3)
	}
	return crc
}
