package mediainfo

import (
	"encoding/binary"
	"math"
)

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
	if b.sampleDurationTicks > 0 {
		info.sampleDurationTicks = b.sampleDurationTicks
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

func parseStts(payload []byte) (uint64, uint64, uint32, uint32, bool, bool) {
	if len(payload) < 8 {
		return 0, 0, 0, 0, false, false
	}
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	var duration uint64
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
		duration += uint64(sampleCount) * uint64(sampleDelta)
		offset += 8
	}
	if total == 0 {
		return 0, 0, 0, 0, false, false
	}
	if entryCount > 1 {
		variable = true
	}
	return total, duration, firstDelta, lastDelta, true, variable
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

// mp4PresentationDurationSeconds returns the track-header presentation span,
// falling back to edit-list and media-header durations for incomplete files.
func mp4PresentationDurationSeconds(track MP4Track) float64 {
	if track.trackDurationTicks > 0 && track.movieTimescale > 0 {
		if len(track.editList) == 2 &&
			track.editList[0].mediaTime == -1 &&
			track.editList[0].rate == 0x00010000 &&
			track.editList[1].mediaTime >= 0 &&
			track.editList[1].rate == 0x00010000 &&
			track.editList[0].duration <= ^uint64(0)-track.editList[1].duration &&
			track.editList[0].duration+track.editList[1].duration == track.trackDurationTicks {
			return float64(track.editList[1].duration) / float64(track.movieTimescale)
		}
		return float64(track.trackDurationTicks) / float64(track.movieTimescale)
	}
	if track.EditDuration > 0 {
		return track.EditDuration
	}
	return track.DurationSeconds
}

// mp4EditSourceDelaySeconds returns the source-timeline offset for the two
// edit-list forms handled by MediaInfoLib v26.05: one unit-rate media edit, or
// one leading empty unit-rate edit followed by media.
func mp4EditSourceDelaySeconds(track MP4Track) float64 {
	switch len(track.editList) {
	case 1:
		entry := track.editList[0]
		if entry.duration == 0 || entry.mediaTime <= 0 || entry.rate != 0x00010000 || track.Timescale == 0 {
			return 0
		}
		return -float64(entry.mediaTime) / float64(track.Timescale)
	case 2:
		empty, media := track.editList[0], track.editList[1]
		if empty.mediaTime != -1 || empty.duration == 0 || empty.rate != 0x00010000 ||
			media.mediaTime < 0 || media.duration == 0 || media.rate != 0x00010000 ||
			track.movieTimescale == 0 || empty.duration > ^uint64(0)-media.duration {
			return 0
		}
		if track.trackDurationTicks > 0 && empty.duration+media.duration != track.trackDurationTicks {
			return 0
		}
		return float64(empty.duration) / float64(track.movieTimescale)
	default:
		return 0
	}
}

// mp4SampleDurationSeconds returns the complete decode span represented by
// the stts table.
func mp4SampleDurationSeconds(track MP4Track) float64 {
	if track.sampleDurationTicks == 0 || track.Timescale == 0 {
		return 0
	}
	return float64(track.sampleDurationTicks) / float64(track.Timescale)
}

// mp4HasDistinctSampleDuration reports whether the sample-table duration lies
// outside MediaInfo's one movie-timescale-tick track-header tolerance.
func mp4HasDistinctSampleDuration(track MP4Track) bool {
	if track.trackDurationTicks == 0 || track.movieTimescale == 0 {
		return false
	}
	sampleDuration := mp4SampleDurationSeconds(track)
	if sampleDuration == 0 {
		return false
	}
	lowerTicks := track.trackDurationTicks
	if lowerTicks > 0 {
		lowerTicks--
	}
	upperTicks := track.trackDurationTicks + 1
	lower := float64(lowerTicks) / float64(track.movieTimescale)
	upper := float64(upperTicks) / float64(track.movieTimescale)
	return sampleDuration < lower || sampleDuration > upper
}

// mp4FrameRate returns the decode-timeline rate rather than deriving it from
// a possibly edited presentation duration.
func mp4FrameRate(track MP4Track, fallbackDuration float64) float64 {
	if !track.VariableDeltas && track.SampleDelta > 0 && track.Timescale > 0 {
		return float64(track.Timescale) / float64(track.SampleDelta)
	}
	if sampleDuration := mp4SampleDurationSeconds(track); sampleDuration > 0 && track.SampleCount > 0 {
		return float64(track.SampleCount) / sampleDuration
	}
	if fallbackDuration > 0 && track.SampleCount > 0 {
		return float64(track.SampleCount) / fallbackDuration
	}
	return 0
}

// rationalizeMP4FrameRate retains exact common NTSC ratios while preserving
// decimal-timescale rates such as 29970/1000.
func rationalizeMP4FrameRate(track MP4Track, rate float64) (int, int) {
	if rate <= 0 {
		return 0, 0
	}
	if track.VariableDeltas {
		return rationalizeFrameRate(rate)
	}
	common := []struct {
		numerator   int
		denominator int
	}{
		{numerator: 24000, denominator: 1001},
		{numerator: 30000, denominator: 1001},
		{numerator: 60000, denominator: 1001},
	}
	for _, candidate := range common {
		value := float64(candidate.numerator) / float64(candidate.denominator)
		if math.Abs(rate-value) < 0.000001 {
			return candidate.numerator, candidate.denominator
		}
	}
	if integer := math.Round(rate); math.Abs(rate-integer) < 0.000001 {
		return int(integer), 1
	}
	return int(math.Round(rate * 1000)), 1000
}

// mp4VideoBitRate derives the stream rate from frame count and MediaInfo's
// three-decimal displayed frame rate.
func mp4VideoBitRate(track MP4Track, frameRate float64) float64 {
	if track.VariableDeltas || track.SampleBytes == 0 || track.SampleCount == 0 || frameRate <= 0 {
		return 0
	}
	displayedRate := math.Round(frameRate*1000) / 1000
	if displayedRate <= 0 {
		return 0
	}
	return float64(track.SampleBytes) * 8 * displayedRate / float64(track.SampleCount)
}

// mp4RoundedDurationMilliseconds reproduces MediaInfo's float32-backed
// duration fill used by mdhd_Duration.
func mp4RoundedDurationMilliseconds(seconds float64) int64 {
	return int64(math.Round(float64(float32(seconds * 1000))))
}

// mp4ShouldExposeMediaHeaderDuration reports whether stts and mdhd carry
// distinct integer durations.
func mp4ShouldExposeMediaHeaderDuration(track MP4Track) bool {
	return track.mediaDurationTicks > 0 && track.sampleDurationTicks > 0 &&
		track.mediaDurationTicks != track.sampleDurationTicks
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
