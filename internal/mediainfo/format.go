package mediainfo

import (
	"bytes"
	"io"
	"math"
	"path/filepath"
	"strings"
)

const maxSniffBytes = 4096

// DetectFormat identifies media from a bounded header and filename. AnalyzeFile
// also checks file atom boundaries when MPEG and MP4 signatures overlap.
func DetectFormat(header []byte, filename string) string {
	return detectFormat(header, filename, nil, 0)
}

func detectFormat(header []byte, filename string, reader io.ReaderAt, size int64) string {
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
	if isMPEGTS(header) {
		return "MPEG-TS"
	}
	if format := sniffMP4Format(header); format != "" {
		// A complete padding chain or an explicit brand is stronger than a
		// coincidental MPEG signature in the first atom's size or payload.
		if kind := string(header[4:8]); kind == "moov" || kind == "mdat" {
			if mpeg := sniffMPEGFormat(header); mpeg != "" && !preferMP4AtomChain(reader, size) {
				return mpeg
			}
		}
		return format
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

// preferMP4AtomChain skips payloads at their declared boundaries. The header-read
// and movie limits retain the existing MP4 choice when more proof needs more IO.
func preferMP4AtomChain(reader io.ReaderAt, size int64) bool {
	if reader == nil {
		return false
	}
	var offset int64
	for atoms := 0; atoms < maxSniffBytes/16 && offset+8 <= size; atoms++ {
		length, kind, header, ok := readMP4BoxHeader(reader, offset, size)
		if !ok {
			return false
		}
		if kind == "moov" {
			if offset > 0 || length-header > maxMoovSize {
				return true
			}
		}
		offset += length
	}
	return offset+8 <= size
}

// sniffMPEGFormat distinguishes a real MPEG packet/frame sequence from an MP4
// atom size that happens to contain the same leading signature bytes.
func sniffMPEGFormat(header []byte) string {
	if len(header) >= 18 && bytes.HasPrefix(header, []byte{0, 0, 1, 0xBA}) &&
		header[4]&0xC4 == 0x44 && header[6]&4 == 4 && header[8]&4 == 4 &&
		header[9]&1 == 1 && header[12]&3 == 3 && header[13]&0xF8 == 0xF8 {
		end := 14 + int(header[13]&7)
		if end+4 <= len(header) && bytes.HasPrefix(header[end:], []byte{0, 0, 1}) && isPESStreamID(header[end+3]) {
			return "MPEG-PS"
		}
	}
	if first, ok := parseMPEGAudioHeader(header); ok {
		length := mpegAudioFrameLengthBytes(first)
		if first.layerID == 0x02 {
			// Layer II has 1152 samples in every MPEG version.
			length = 144000 * first.bitrateKbps / first.sampleRate
			if first.padding {
				length++
			}
		}
		if length > 0 && length+4 <= len(header) {
			if next, ok := parseMPEGAudioHeader(header[length:]); ok && next.versionID == first.versionID && next.layerID == first.layerID && next.sampleRate == first.sampleRate {
				return "MPEG Audio"
			}
		}
	}
	return ""
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
			if size < headerSize+4 || offset+headerSize+4 > reader.Size() {
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
