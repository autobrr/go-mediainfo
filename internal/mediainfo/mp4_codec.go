package mediainfo

import "encoding/binary"

func parseStsdForFormat(buf []byte) string {
	if len(buf) < 16 {
		return ""
	}
	count := binary.BigEndian.Uint32(buf[4:8])
	offset := 8
	for i := 0; i < int(count); i++ {
		if offset+8 > len(buf) {
			return ""
		}
		size := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
		if size < 8 || offset+size > len(buf) {
			return ""
		}
		typ := string(buf[offset+4 : offset+8])
		format := mapMP4SampleEntry(typ)
		if format != "" {
			return format
		}
		offset += size
	}
	return ""
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
