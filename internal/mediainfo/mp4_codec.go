package mediainfo

import "encoding/binary"

type SampleInfo struct {
	Format      string
	Fields      []Field
	SampleCount uint64
	SampleBytes uint64
}

func parseStsdForSample(buf []byte) (SampleInfo, bool) {
	if len(buf) < 16 {
		return SampleInfo{}, false
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	offset := 8
	for i := 0; i < int(count); i++ {
		if offset+8 > len(buf) {
			return SampleInfo{}, false
		}
		size := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
		if size < 8 || offset+size > len(buf) {
			return SampleInfo{}, false
		}
		entry := buf[offset : offset+size]
		typ := string(entry[4:8])
		format := mapMP4SampleEntry(typ)
		info := SampleInfo{Format: format}
		if isVideoSampleEntry(typ) {
			info.Fields = append(info.Fields, parseVisualSampleEntry(entry)...)
		}
		if isAudioSampleEntry(typ) {
			info.Fields = append(info.Fields, parseAudioSampleEntry(entry)...)
		}
		if info.Format != "" || len(info.Fields) > 0 {
			return info, true
		}
		offset += size
	}
	return SampleInfo{}, false
}

func mapMP4SampleEntry(sample string) string {
	switch sample {
	case "avc1", "avc3":
		return "AVC"
	case "hvc1", "hev1":
		return "HEVC"
	case "mp4v":
		return "MPEG-4 Visual"
	case "mp4a":
		return "AAC"
	case "ac-3", "ac-4":
		return "AC-3"
	case "ec-3":
		return "E-AC-3"
	case "alac":
		return "ALAC"
	case "flac":
		return "FLAC"
	case "Opus", "opus":
		return "Opus"
	case "mp4s":
		return "MPEG-4 Systems"
	case "tx3g":
		return "Text"
	case "wvtt":
		return "WebVTT"
	default:
		return ""
	}
}

func isVideoSampleEntry(sample string) bool {
	switch sample {
	case "avc1", "avc3", "hvc1", "hev1", "mp4v":
		return true
	default:
		return false
	}
}

func isAudioSampleEntry(sample string) bool {
	switch sample {
	case "mp4a", "ac-3", "ec-3", "alac", "flac", "opus":
		return true
	default:
		return false
	}
}

func parseVisualSampleEntry(entry []byte) []Field {
	if len(entry) < 36 {
		return nil
	}
	width := binary.BigEndian.Uint16(entry[32:34])
	height := binary.BigEndian.Uint16(entry[34:36])
	fields := []Field{}
	if width > 0 {
		fields = append(fields, Field{Name: "Width", Value: formatPixels(uint64(width))})
	}
	if height > 0 {
		fields = append(fields, Field{Name: "Height", Value: formatPixels(uint64(height))})
	}
	return fields
}

func parseAudioSampleEntry(entry []byte) []Field {
	if len(entry) < 36 {
		return nil
	}
	channels := binary.BigEndian.Uint16(entry[24:26])
	sampleRate := binary.BigEndian.Uint32(entry[32:36])
	fields := []Field{}
	if channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(channels))})
		if layout := channelLayout(uint64(channels)); layout != "" {
			fields = append(fields, Field{Name: "Channel layout", Value: layout})
		}
	}
	if sampleRate > 0 {
		rate := float64(sampleRate) / 65536
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(rate)})
	}
	return fields
}
