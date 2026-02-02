package mediainfo

import (
	"encoding/binary"
	"io"
	"math"
)

const (
	mkvIDSegment       = 0x18538067
	mkvIDInfo          = 0x1549A966
	mkvIDTimecodeScale = 0x2AD7B1
	mkvIDDuration      = 0x4489
	mkvIDTracks        = 0x1654AE6B
	mkvIDTrackEntry    = 0xAE
	mkvIDTrackType     = 0x83
	mkvIDCodecID       = 0x86
	mkvMaxScan         = int64(4 << 20)
)

type MatroskaInfo struct {
	Container ContainerInfo
	Tracks    []Stream
}

func ParseMatroska(r io.ReaderAt, size int64) (MatroskaInfo, bool) {
	scanSize := size
	if scanSize > mkvMaxScan {
		scanSize = mkvMaxScan
	}
	if scanSize <= 0 {
		return MatroskaInfo{}, false
	}

	buf := make([]byte, scanSize)
	if _, err := r.ReadAt(buf, 0); err != nil && err != io.EOF {
		return MatroskaInfo{}, false
	}

	info, ok := parseMatroska(buf)
	if !ok {
		return MatroskaInfo{}, false
	}
	return info, true
}

func parseMatroska(buf []byte) (MatroskaInfo, bool) {
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDSegment {
			if info, ok := parseMatroskaSegment(buf[dataStart:dataEnd]); ok {
				return info, true
			}
		}
		pos = dataEnd
	}
	return MatroskaInfo{}, false
}

func parseMatroskaSegment(buf []byte) (MatroskaInfo, bool) {
	info := MatroskaInfo{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDInfo {
			if duration, ok := parseMatroskaInfo(buf[dataStart:dataEnd]); ok {
				info.Container.DurationSeconds = duration
			}
		}
		if id == mkvIDTracks {
			if tracks, ok := parseMatroskaTracks(buf[dataStart:dataEnd]); ok {
				info.Tracks = append(info.Tracks, tracks...)
			}
		}
		pos = dataEnd
	}
	if info.Container.HasDuration() || len(info.Tracks) > 0 {
		return info, true
	}
	return MatroskaInfo{}, false
}

func parseMatroskaInfo(buf []byte) (float64, bool) {
	timecodeScale := uint64(1000000)
	var durationValue float64
	var hasDuration bool

	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		payload := buf[dataStart:dataEnd]
		switch id {
		case mkvIDTimecodeScale:
			if value, ok := readUnsigned(payload); ok {
				timecodeScale = value
			}
		case mkvIDDuration:
			if value, ok := readFloat(payload); ok {
				durationValue = value
				hasDuration = true
			}
		}
		pos = dataEnd
	}

	if !hasDuration {
		return 0, false
	}
	seconds := durationValue * float64(timecodeScale) / 1e9
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

func parseMatroskaTracks(buf []byte) ([]Stream, bool) {
	entries := []Stream{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackEntry {
			if stream, ok := parseMatroskaTrackEntry(buf[dataStart:dataEnd]); ok {
				entries = append(entries, stream)
			}
		}
		pos = dataEnd
	}
	return entries, len(entries) > 0
}

func parseMatroskaTrackEntry(buf []byte) (Stream, bool) {
	pos := 0
	var trackType uint64
	var codecID string
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackType {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackType = value
			}
		}
		if id == mkvIDCodecID {
			codecID = string(buf[dataStart:dataEnd])
		}
		pos = dataEnd
	}
	kind, format := mapMatroskaCodecID(codecID, trackType)
	if kind == "" {
		return Stream{}, false
	}
	return Stream{Kind: kind, Fields: []Field{{Name: "Format", Value: format}}}, true
}

const unknownVintSize = ^uint64(0)

func readVintID(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	var value uint64
	for i := 0; i < length; i++ {
		value = (value << 8) | uint64(buf[pos+i])
	}
	return value, length, true
}

func readVintSize(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	mask := byte(0xFF >> length)
	value := uint64(first & mask)
	for i := 1; i < length; i++ {
		value = (value << 8) | uint64(buf[pos+i])
	}
	if value == (uint64(1)<<(uint(length*7)))-1 {
		return unknownVintSize, length, true
	}
	return value, length, true
}

func vintLength(first byte) int {
	for i := 0; i < 8; i++ {
		if first&(1<<(7-uint(i))) != 0 {
			return i + 1
		}
	}
	return 0
}

func readUnsigned(buf []byte) (uint64, bool) {
	if len(buf) == 0 || len(buf) > 8 {
		return 0, false
	}
	var value uint64
	for _, b := range buf {
		value = (value << 8) | uint64(b)
	}
	return value, true
}

func readFloat(buf []byte) (float64, bool) {
	if len(buf) == 4 {
		bits := binary.BigEndian.Uint32(buf)
		return float64(math.Float32frombits(bits)), true
	}
	if len(buf) == 8 {
		bits := binary.BigEndian.Uint64(buf)
		return math.Float64frombits(bits), true
	}
	return 0, false
}
