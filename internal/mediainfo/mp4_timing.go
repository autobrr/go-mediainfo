package mediainfo

import "encoding/binary"

// mergeSampleInfo combines independently parsed sample-table facts, preferring
// non-zero scalar values and appending bounded sample-size windows.
func mergeSampleInfo(a, b SampleInfo) SampleInfo {
	info := a
	if b.Format != "" {
		info.Format = b.Format
	}
	if len(b.Fields) > 0 {
		info.Fields = append(info.Fields, b.Fields...)
	}
	if len(b.canonicalSeed) > 0 {
		info.canonicalSeed = append(info.canonicalSeed, b.canonicalSeed...)
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
	if b.VariableDeltas {
		info.VariableDeltas = true
	}
	if b.MinimumSampleDelta > 0 {
		info.MinimumSampleDelta = b.MinimumSampleDelta
	}
	if b.MaximumSampleDelta > 0 {
		info.MaximumSampleDelta = b.MaximumSampleDelta
	}
	if b.FirstChunkOff > 0 {
		info.FirstChunkOff = b.FirstChunkOff
	}
	if b.Width > 0 {
		info.Width = b.Width
	}
	if b.Height > 0 {
		info.Height = b.Height
	}
	if b.SampleEntryType != "" {
		info.SampleEntryType = b.SampleEntryType
	}
	if b.NonEmptySampleCount > 0 {
		info.NonEmptySampleCount = b.NonEmptySampleCount
	}
	if len(b.chunkOffsetsHead) > 0 {
		info.chunkOffsetsHead = append([]uint64(nil), b.chunkOffsetsHead...)
	}
	if len(b.sampleToChunk) > 0 {
		info.sampleToChunk = append([]mp4SampleToChunkEntry(nil), b.sampleToChunk...)
	}
	if b.hevcNALLengthSize > 0 {
		info.hevcNALLengthSize = b.hevcNALLengthSize
	}
	if b.avcNALLengthSize > 0 {
		info.avcNALLengthSize = b.avcNALLengthSize
		info.avcSPS = b.avcSPS
		info.avcParameterSets = append([]byte(nil), b.avcParameterSets...)
	}
	if b.hevcSEI.x265Seen || b.hevcSEI.hasMastering || b.hevcSEI.maxCLL > 0 || b.hevcSEI.maxFALL > 0 {
		info.hevcSEI = b.hevcSEI
	}
	if b.hasDolbyVision {
		info.dolbyVision = b.dolbyVision
		info.hasDolbyVision = true
	}
	if b.hevcContainerMastering {
		info.hevcContainerMastering = true
	}
	if b.hevcContainerCLL {
		info.hevcContainerCLL = true
	}
	if len(b.sampleStartsHead) > 0 {
		info.sampleStartsHead = append([]uint64(nil), b.sampleStartsHead...)
	}
	return info
}

// parseSttsSampleStarts expands a bounded prefix of decode timestamps.
func parseSttsSampleStarts(payload []byte, limit int) []uint64 {
	if len(payload) < 8 || limit <= 0 {
		return nil
	}
	entryCount := int(binary.BigEndian.Uint32(payload[4:8]))
	result := make([]uint64, 0, limit)
	var timestamp uint64
	for index, offset := 0, 8; index < entryCount && offset+8 <= len(payload) && len(result) < limit; index, offset = index+1, offset+8 {
		count := binary.BigEndian.Uint32(payload[offset : offset+4])
		delta := binary.BigEndian.Uint32(payload[offset+4 : offset+8])
		for sample := uint32(0); sample < count && len(result) < limit; sample++ {
			result = append(result, timestamp)
			timestamp += uint64(delta)
		}
	}
	return result
}

func parseStts(payload []byte) (uint64, uint32, uint32, bool, bool) {
	if len(payload) < 8 {
		return 0, 0, 0, false, false
	}
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	var firstDelta uint32
	var lastDelta uint32
	variable := false
	for i := 0; i < int(entryCount); i++ {
		if offset+8 > len(payload) {
			break
		}
		sampleCount := binary.BigEndian.Uint32(payload[offset : offset+4])
		sampleDelta := binary.BigEndian.Uint32(payload[offset+4 : offset+8])
		if i == 0 {
			firstDelta = sampleDelta
		} else if sampleDelta != firstDelta {
			variable = true
		}
		lastDelta = sampleDelta
		total += uint64(sampleCount)
		offset += 8
	}
	if total == 0 {
		return 0, 0, 0, false, false
	}
	if entryCount > 1 {
		variable = true
	}
	return total, firstDelta, lastDelta, true, variable
}

// parseSttsDeltaRange returns the shortest and longest non-zero decode-time
// deltas in an stts table.
func parseSttsDeltaRange(payload []byte) (uint32, uint32) {
	if len(payload) < 8 {
		return 0, 0
	}
	entryCount := int(binary.BigEndian.Uint32(payload[4:8]))
	var minimum uint32
	var maximum uint32
	for index, offset := 0, 8; index < entryCount && offset+8 <= len(payload); index, offset = index+1, offset+8 {
		delta := binary.BigEndian.Uint32(payload[offset+4 : offset+8])
		if delta == 0 {
			continue
		}
		if minimum == 0 || delta < minimum {
			minimum = delta
		}
		if delta > maximum {
			maximum = delta
		}
	}
	return minimum, maximum
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
