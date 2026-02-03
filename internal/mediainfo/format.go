package mediainfo

import (
	"bytes"
	"path/filepath"
	"strings"
)

const maxSniffBytes = 4096

func DetectFormat(header []byte, filename string) string {
	if len(header) == 0 {
		return "Unknown"
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".ifo" {
		return "DVD Video"
	}
	if ext == ".vob" {
		return "MPEG-PS"
	}

	if bytes.HasPrefix(header, []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "Matroska"
	}
	if len(header) >= 12 {
		if string(header[4:8]) == "ftyp" {
			brand := string(header[8:12])
			if brand == "qt  " {
				return "QuickTime"
			}
			return "MPEG-4"
		}
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
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xB3}) {
		return "MPEG Video"
	}
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xBA}) {
		return "MPEG-PS"
	}

	return "Unknown"
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
