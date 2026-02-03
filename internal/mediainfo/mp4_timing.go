package mediainfo

import "encoding/binary"

func mergeSampleInfo(a, b SampleInfo) SampleInfo {
	info := a
	if b.Format != "" {
		info.Format = b.Format
	}
	if len(b.Fields) > 0 {
		info.Fields = append(info.Fields, b.Fields...)
	}
	if b.SampleCount > 0 {
		info.SampleCount = b.SampleCount
	}
	if b.SampleBytes > 0 {
		info.SampleBytes = b.SampleBytes
	}
	if b.Width > 0 {
		info.Width = b.Width
	}
	if b.Height > 0 {
		info.Height = b.Height
	}
	return info
}

func parseStts(payload []byte) (uint64, bool) {
	if len(payload) < 8 {
		return 0, false
	}
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	for i := 0; i < int(entryCount); i++ {
		if offset+8 > len(payload) {
			break
		}
		sampleCount := binary.BigEndian.Uint32(payload[offset : offset+4])
		total += uint64(sampleCount)
		offset += 8
	}
	if total == 0 {
		return 0, false
	}
	return total, true
}

func parseMdhd(payload []byte) (float64, bool) {
	if len(payload) < 24 {
		return 0, false
	}
	version := payload[0]
	if version == 0 {
		if len(payload) < 24 {
			return 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		if timescale == 0 {
			return 0, false
		}
		return float64(duration) / float64(timescale), true
	}
	if version == 1 {
		if len(payload) < 36 {
			return 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		if timescale == 0 {
			return 0, false
		}
		return float64(duration) / float64(timescale), true
	}
	return 0, false
}
