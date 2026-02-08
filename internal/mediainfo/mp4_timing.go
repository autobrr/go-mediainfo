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
	if len(b.JSON) > 0 {
		if info.JSON == nil {
			info.JSON = map[string]string{}
		}
		for k, v := range b.JSON {
			info.JSON[k] = v
		}
	}
	if b.SampleCount > 0 {
		info.SampleCount = b.SampleCount
	}
	if b.SampleBytes > 0 {
		info.SampleBytes = b.SampleBytes
	}
	if len(b.SampleSizeHead) > 0 {
		info.SampleSizeHead = b.SampleSizeHead
	}
	if len(b.SampleSizeTail) > 0 {
		info.SampleSizeTail = b.SampleSizeTail
	}
	if b.SampleDelta > 0 {
		info.SampleDelta = b.SampleDelta
	}
	if b.LastSampleDelta > 0 {
		info.LastSampleDelta = b.LastSampleDelta
	}
	if b.Width > 0 {
		info.Width = b.Width
	}
	if b.Height > 0 {
		info.Height = b.Height
	}
	return info
}

func parseStts(payload []byte) (uint64, uint32, uint32, bool) {
	if len(payload) < 8 {
		return 0, 0, 0, false
	}
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	var firstDelta uint32
	var lastDelta uint32
	for i := 0; i < int(entryCount); i++ {
		if offset+8 > len(payload) {
			break
		}
		sampleCount := binary.BigEndian.Uint32(payload[offset : offset+4])
		sampleDelta := binary.BigEndian.Uint32(payload[offset+4 : offset+8])
		if i == 0 {
			firstDelta = sampleDelta
		}
		lastDelta = sampleDelta
		total += uint64(sampleCount)
		offset += 8
	}
	if total == 0 {
		return 0, 0, 0, false
	}
	return total, firstDelta, lastDelta, true
}

func parseMdhd(payload []byte) (float64, uint32, bool) {
	return parseMP4Duration(payload, 24, 36)
}

func parseMP4Duration(payload []byte, minV0 int, minV1 int) (float64, uint32, bool) {
	if len(payload) < minV0 {
		return 0, 0, false
	}
	version := payload[0]
	switch version {
	case 0:
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		if timescale == 0 {
			return 0, 0, false
		}
		return float64(duration) / float64(timescale), timescale, true
	case 1:
		if len(payload) < minV1 {
			return 0, 0, false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		if timescale == 0 {
			return 0, 0, false
		}
		return float64(duration) / float64(timescale), timescale, true
	default:
		return 0, 0, false
	}
}
