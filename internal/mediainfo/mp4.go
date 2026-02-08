package mediainfo

import (
	"encoding/binary"
	"io"
)

const maxMoovSize = int64(16 << 20)

type MP4Track struct {
	ID              uint32
	Kind            StreamKind
	Format          string
	Fields          []Field
	JSON            map[string]string
	SampleCount     uint64
	SampleBytes     uint64
	SampleDelta     uint32
	LastSampleDelta uint32
	DurationSeconds float64
	EditDuration    float64
	EditMediaTime   int64
	Default         bool
	AlternateGroup  uint16
	Timescale       uint32
	Width           uint64
	Height          uint64
}

type MP4Info struct {
	Container      ContainerInfo
	General        []Field
	Tracks         []MP4Track
	MovieTimescale uint32
}

func ParseMP4(r io.ReaderAt, size int64) (MP4Info, bool) {
	info := MP4Info{}
	var offset int64
	for offset+8 <= size {
		boxSize, boxType, headerSize, ok := readMP4BoxHeader(r, offset, size)
		if !ok || boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "ftyp" {
			payload := make([]byte, boxSize-headerSize)
			if _, err := r.ReadAt(payload, dataOffset); err == nil || err == io.EOF {
				if fields := parseFtyp(payload); len(fields) > 0 {
					info.General = append(info.General, fields...)
				}
			}
		}
		if boxType == "moov" {
			moovSize := boxSize - headerSize
			if moovSize > maxMoovSize {
				return MP4Info{}, false
			}
			buf := make([]byte, moovSize)
			if _, err := r.ReadAt(buf, dataOffset); err != nil && err != io.EOF {
				return MP4Info{}, false
			}
			if moovInfo, ok := parseMoov(buf); ok {
				if len(info.General) > 0 {
					moovInfo.General = append(info.General, moovInfo.General...)
				}
				return moovInfo, true
			}
		}
		offset += boxSize
	}
	return MP4Info{}, false
}

func readMP4BoxHeader(r io.ReaderAt, offset, fileSize int64) (boxSize int64, boxType string, headerSize int64, ok bool) {
	var header [8]byte
	if _, err := r.ReadAt(header[:], offset); err != nil {
		return 0, "", 0, false
	}

	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType = string(header[4:8])
	if size32 == 0 {
		return fileSize - offset, boxType, 8, true
	}
	if size32 == 1 {
		var larger [8]byte
		if _, err := r.ReadAt(larger[:], offset+8); err != nil {
			return 0, "", 0, false
		}
		size64 := binary.BigEndian.Uint64(larger[:])
		if size64 < 16 {
			return 0, "", 0, false
		}
		return int64(size64), boxType, 16, true
	}
	if size32 < 8 {
		return 0, "", 0, false
	}
	return int64(size32), boxType, 8, true
}

func parseMoov(buf []byte) (MP4Info, bool) {
	var offset int64
	info := MP4Info{}
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "mvhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, timescale, ok := parseMvhd(payload); ok {
				info.Container.DurationSeconds = duration
				info.MovieTimescale = timescale
			}
		}
		if boxType == "udta" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if app := parseMP4WritingApp(payload); app != "" {
				info.General = append(info.General, Field{Name: "Writing application", Value: app})
			}
		}
		if boxType == "trak" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if track, ok := parseTrak(payload, info.MovieTimescale); ok {
				info.Tracks = append(info.Tracks, track)
			}
		}
		offset += boxSize
	}
	if info.Container.HasDuration() || len(info.Tracks) > 0 {
		return info, true
	}
	return MP4Info{}, false
}

func readMP4BoxHeaderFrom(buf []byte, offset int64) (boxSize int64, boxType string, headerSize int64) {
	if offset+8 > int64(len(buf)) {
		return 0, "", 0
	}
	size32 := binary.BigEndian.Uint32(buf[offset : offset+4])
	boxType = string(buf[offset+4 : offset+8])
	if size32 == 0 {
		return int64(len(buf)) - offset, boxType, 8
	}
	if size32 == 1 {
		if offset+16 > int64(len(buf)) {
			return 0, "", 0
		}
		size64 := binary.BigEndian.Uint64(buf[offset+8 : offset+16])
		return int64(size64), boxType, 16
	}
	return int64(size32), boxType, 8
}

func sliceBox(buf []byte, offset, length int64) []byte {
	if offset < 0 || length < 0 {
		return nil
	}
	end := min(offset+length, int64(len(buf)))
	if offset > end {
		return nil
	}
	return buf[offset:end]
}

func parseMvhd(payload []byte) (float64, uint32, bool) {
	return parseMP4Duration(payload, 20, 32)
}

func parseTrak(buf []byte, movieTimescale uint32) (MP4Track, bool) {
	var offset int64
	var tkhdInfo tkhdInfo
	var hasTkhd bool
	var editDuration float64
	var editMediaTime int64
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "tkhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if info, ok := parseTkhd(payload); ok {
				tkhdInfo = info
				hasTkhd = true
			}
		}
		if boxType == "edts" && movieTimescale > 0 {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, mediaTime := parseEdts(payload, movieTimescale); duration > 0 {
				editDuration = duration
				editMediaTime = mediaTime
			}
		}
		if boxType == "mdia" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if track, ok := parseMdia(payload); ok {
				if hasTkhd && tkhdInfo.ID > 0 {
					track.ID = tkhdInfo.ID
				}
				if editDuration > 0 {
					track.EditDuration = editDuration
					track.EditMediaTime = editMediaTime
				}
				if hasTkhd {
					track.Default = tkhdInfo.Default
					track.AlternateGroup = tkhdInfo.AlternateGroup
				}
				return track, true
			}
		}
		offset += boxSize
	}
	return MP4Track{}, false
}

func parseMdia(buf []byte) (MP4Track, bool) {
	var offset int64
	var handler string
	var sampleInfo SampleInfo
	var trackDuration float64
	var trackTimescale uint32
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "hdlr" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			handler = parseHdlr(payload)
		}
		if boxType == "mdhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if duration, timescale, ok := parseMdhd(payload); ok {
				trackDuration = duration
				trackTimescale = timescale
			}
		}
		if boxType == "minf" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if info, ok := parseMinfSample(payload); ok {
				sampleInfo = info
			}
		}
		offset += boxSize
	}
	if handler == "" {
		return MP4Track{}, false
	}
	kind, format := mapHandlerType(handler)
	if kind == "" {
		return MP4Track{}, false
	}
	if sampleInfo.Format != "" {
		format = sampleInfo.Format
	}
	return MP4Track{
		Kind:            kind,
		Format:          format,
		Fields:          sampleInfo.Fields,
		JSON:            sampleInfo.JSON,
		SampleCount:     sampleInfo.SampleCount,
		SampleBytes:     sampleInfo.SampleBytes,
		SampleDelta:     sampleInfo.SampleDelta,
		LastSampleDelta: sampleInfo.LastSampleDelta,
		DurationSeconds: trackDuration,
		Timescale:       trackTimescale,
		Width:           sampleInfo.Width,
		Height:          sampleInfo.Height,
	}, true
}

func parseHdlr(payload []byte) string {
	if len(payload) < 20 {
		return ""
	}
	return string(payload[8:12])
}

type tkhdInfo struct {
	ID             uint32
	Default        bool
	AlternateGroup uint16
}

func parseTkhd(payload []byte) (tkhdInfo, bool) {
	if len(payload) < 20 {
		return tkhdInfo{}, false
	}
	version := payload[0]
	flags := uint32(payload[1])<<16 | uint32(payload[2])<<8 | uint32(payload[3])
	if version == 0 {
		if len(payload) < 36 {
			return tkhdInfo{}, false
		}
		id := binary.BigEndian.Uint32(payload[12:16])
		alternateGroup := binary.BigEndian.Uint16(payload[34:36])
		return tkhdInfo{
			ID:             id,
			Default:        flags&0x000001 != 0,
			AlternateGroup: alternateGroup,
		}, true
	}
	if version == 1 {
		if len(payload) < 48 {
			return tkhdInfo{}, false
		}
		id := binary.BigEndian.Uint32(payload[20:24])
		alternateGroup := binary.BigEndian.Uint16(payload[46:48])
		return tkhdInfo{
			ID:             id,
			Default:        flags&0x000001 != 0,
			AlternateGroup: alternateGroup,
		}, true
	}
	return tkhdInfo{}, false
}

func parseEdts(payload []byte, movieTimescale uint32) (float64, int64) {
	if movieTimescale == 0 {
		return 0, 0
	}
	var offset int64
	var duration float64
	var mediaTime int64
	for offset+8 <= int64(len(payload)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(payload, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "elst" {
			elstPayload := sliceBox(payload, dataOffset, boxSize-headerSize)
			if parsedDuration, parsedMediaTime := parseElst(elstPayload, movieTimescale); parsedDuration > 0 {
				duration = parsedDuration
				mediaTime = parsedMediaTime
			}
		}
		offset += boxSize
	}
	return duration, mediaTime
}

func parseElst(payload []byte, movieTimescale uint32) (float64, int64) {
	if len(payload) < 8 || movieTimescale == 0 {
		return 0, 0
	}
	version := payload[0]
	entryCount := binary.BigEndian.Uint32(payload[4:8])
	offset := 8
	var total uint64
	var mediaTime int64
	switch version {
	case 0:
		for i := 0; i < int(entryCount); i++ {
			if offset+12 > len(payload) {
				break
			}
			segmentDuration := binary.BigEndian.Uint32(payload[offset : offset+4])
			mediaTimeValue := int32(binary.BigEndian.Uint32(payload[offset+4 : offset+8]))
			if mediaTimeValue >= 0 && segmentDuration > 0 {
				total += uint64(segmentDuration)
				if mediaTime == 0 {
					mediaTime = int64(mediaTimeValue)
				}
			}
			offset += 12
		}
	case 1:
		for i := 0; i < int(entryCount); i++ {
			if offset+20 > len(payload) {
				break
			}
			segmentDuration := binary.BigEndian.Uint64(payload[offset : offset+8])
			mediaTimeValue := int64(binary.BigEndian.Uint64(payload[offset+8 : offset+16]))
			if mediaTimeValue >= 0 && segmentDuration > 0 {
				total += segmentDuration
				if mediaTime == 0 {
					mediaTime = mediaTimeValue
				}
			}
			offset += 20
		}
	default:
		return 0, 0
	}
	if total == 0 {
		return 0, mediaTime
	}
	return float64(total) / float64(movieTimescale), mediaTime
}

func mapHandlerType(handler string) (StreamKind, string) {
	switch handler {
	case "vide":
		return StreamVideo, "Video"
	case "soun":
		return StreamAudio, "Audio"
	case "text", "sbtl", "subt":
		return StreamText, "Text"
	default:
		return "", ""
	}
}

func parseMinfSample(buf []byte) (SampleInfo, bool) {
	var offset int64
	var info SampleInfo
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "stbl" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if parsed, ok := parseStbl(payload); ok {
				info = mergeSampleInfo(info, parsed)
			}
		}
		offset += boxSize
	}
	if info.Format != "" || len(info.Fields) > 0 || info.SampleCount > 0 {
		return info, true
	}
	return SampleInfo{}, false
}

func parseStbl(buf []byte) (SampleInfo, bool) {
	var offset int64
	info := SampleInfo{}
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "stsd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if parsed, ok := parseStsdForSample(payload); ok {
				info = mergeSampleInfo(info, parsed)
			}
		}
		if boxType == "stts" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if count, sampleDelta, lastDelta, ok := parseStts(payload); ok {
				info.SampleCount = count
				info.SampleDelta = sampleDelta
				info.LastSampleDelta = lastDelta
			}
		}
		if boxType == "stsz" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if total, ok := parseStsz(payload); ok {
				info.SampleBytes = total
			}
		}
		offset += boxSize
	}
	if info.Format != "" || len(info.Fields) > 0 || info.SampleCount > 0 {
		return info, true
	}
	return SampleInfo{}, false
}
