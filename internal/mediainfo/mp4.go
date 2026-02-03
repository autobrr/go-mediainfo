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
	SampleCount     uint64
	SampleBytes     uint64
	DurationSeconds float64
	Width           uint64
	Height          uint64
}

type MP4Info struct {
	Container ContainerInfo
	General   []Field
	Tracks    []MP4Track
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
	header := make([]byte, 8)
	if _, err := r.ReadAt(header, offset); err != nil {
		return 0, "", 0, false
	}

	size32 := binary.BigEndian.Uint32(header[0:4])
	boxType = string(header[4:8])
	if size32 == 0 {
		return fileSize - offset, boxType, 8, true
	}
	if size32 == 1 {
		larger := make([]byte, 8)
		if _, err := r.ReadAt(larger, offset+8); err != nil {
			return 0, "", 0, false
		}
		size64 := binary.BigEndian.Uint64(larger)
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
			if duration, ok := parseMvhd(payload); ok {
				info.Container.DurationSeconds = duration
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
			if track, ok := parseTrak(payload); ok {
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
	end := offset + length
	if end > int64(len(buf)) {
		end = int64(len(buf))
	}
	if offset > end {
		return nil
	}
	return buf[offset:end]
}

func parseMvhd(payload []byte) (float64, bool) {
	if len(payload) < 20 {
		return 0, false
	}
	version := payload[0]
	if version == 0 {
		if len(payload) < 20 {
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
		if len(payload) < 32 {
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

func parseTrak(buf []byte) (MP4Track, bool) {
	var offset int64
	var trackID uint32
	for offset+8 <= int64(len(buf)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(buf, offset)
		if boxSize <= 0 {
			break
		}
		dataOffset := offset + headerSize
		if boxType == "tkhd" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if id, ok := parseTkhd(payload); ok {
				trackID = id
			}
		}
		if boxType == "mdia" {
			payload := sliceBox(buf, dataOffset, boxSize-headerSize)
			if track, ok := parseMdia(payload); ok {
				if trackID > 0 {
					track.ID = trackID
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
			if duration, ok := parseMdhd(payload); ok {
				trackDuration = duration
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
		SampleCount:     sampleInfo.SampleCount,
		SampleBytes:     sampleInfo.SampleBytes,
		DurationSeconds: trackDuration,
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

func parseTkhd(payload []byte) (uint32, bool) {
	if len(payload) < 20 {
		return 0, false
	}
	version := payload[0]
	if version == 0 {
		if len(payload) < 20 {
			return 0, false
		}
		return binary.BigEndian.Uint32(payload[12:16]), true
	}
	if version == 1 {
		if len(payload) < 32 {
			return 0, false
		}
		return binary.BigEndian.Uint32(payload[20:24]), true
	}
	return 0, false
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
			if count, ok := parseStts(payload); ok {
				info.SampleCount = count
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
