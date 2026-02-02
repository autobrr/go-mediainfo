package mediainfo

import "encoding/binary"

func parseStsz(payload []byte) (uint64, bool) {
	if len(payload) < 12 {
		return 0, false
	}
	sampleSize := binary.BigEndian.Uint32(payload[4:8])
	sampleCount := binary.BigEndian.Uint32(payload[8:12])
	if sampleCount == 0 {
		return 0, false
	}
	if sampleSize > 0 {
		return uint64(sampleSize) * uint64(sampleCount), true
	}
	offset := 12
	var total uint64
	for i := 0; i < int(sampleCount); i++ {
		if offset+4 > len(payload) {
			break
		}
		size := binary.BigEndian.Uint32(payload[offset : offset+4])
		total += uint64(size)
		offset += 4
	}
	if total == 0 {
		return 0, false
	}
	return total, true
}
