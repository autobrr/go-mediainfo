package mediainfo

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
)

const maxSniffBytes = 4096

func DetectFormat(header []byte, filename string) string {
	if len(header) == 0 {
		return "Unknown"
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".ifo" || ext == ".bup" {
		return "DVD Video"
	}
	if ext == ".vob" {
		return "MPEG-PS"
	}
	if ext == ".m2ts" || ext == ".mts" || ext == ".m2t" {
		return "BDAV"
	}

	if bytes.HasPrefix(header, []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "Matroska"
	}
	if format := sniffMP4Format(header); format != "" {
		return format
	}
	if len(header) >= 12 {
		sig := string(header[0:4])
		if sig == "RIFF" {
			form := string(header[8:12])
			if form == "AVI " {
				return "AVI"
			}
			if form == "WAVE" {
				return "Wave"
			}
		}
		if sig == "FORM" && string(header[8:12]) == "AIFF" {
			return "AIFF"
		}
	}
	if bytes.HasPrefix(header, []byte("fLaC")) {
		return "FLAC"
	}
	if bytes.HasPrefix(header, []byte("OggS")) {
		return "Ogg"
	}
	if bytes.HasPrefix(header, []byte("ID3")) {
		return "MPEG Audio"
	}
	if isMP3Frame(header) {
		return "MPEG Audio"
	}
	if isMPEGTS(header) {
		return "MPEG-TS"
	}
	if isBDAV(header) {
		return "BDAV"
	}
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xB3}) {
		return "MPEG Video"
	}
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xBA}) {
		return "MPEG-PS"
	}

	return "Unknown"
}

// sniffMP4Format reuses the atom-header parser without requiring the atom body
// to fit in the sniff window. Only complete padding atoms are skipped.
func sniffMP4Format(header []byte) string {
	reader := bytes.NewReader(header)
	for offset := int64(0); offset+8 <= reader.Size(); {
		size, kind, headerSize, ok := readMP4BoxHeader(reader, offset, math.MaxInt64)
		if !ok {
			return ""
		}
		switch kind {
		case "ftyp":
			if offset+headerSize+4 > reader.Size() {
				return ""
			}
			if string(header[offset+headerSize:offset+headerSize+4]) == "qt  " {
				return "QuickTime"
			}
			return "MPEG-4"
		case "moov", "mdat":
			return "QuickTime"
		case "free", "skip", "wide":
			if size > reader.Size()-offset {
				return ""
			}
			offset += size
		default:
			return ""
		}
	}
	return ""
}

func isMP3Frame(header []byte) bool {
	if len(header) < 2 {
		return false
	}
	return header[0] == 0xFF && (header[1]&0xE0) == 0xE0
}

func isMPEGTS(header []byte) bool {
	if len(header) < 376+1 {
		return false
	}
	return header[0] == 0x47 && header[188] == 0x47 && header[376] == 0x47
}

func isBDAV(header []byte) bool {
	// BDAV/M2TS packets are 192 bytes: 4-byte timestamp + 188-byte TS packet.
	if len(header) < 388+1 {
		return false
	}
	return header[4] == 0x47 && header[196] == 0x47 && header[388] == 0x47
}
