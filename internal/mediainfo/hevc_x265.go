package mediainfo

import (
	"encoding/binary"
	"strings"
)

// x265 embeds its writing library + encoding settings into an HEVC SEI
// user_data_unregistered message (payloadType 5). MediaInfoLib dispatches purely on
// the high 8 bytes of the 16-byte UUID; for x265 that is 0x2CA2DE09B51747DB. This
// mirrors File_Hevc::sei_message_user_data_unregistered_x265 so the emitted
// Encoded_Library / _Name / _Version / _Settings fields match MediaInfo 1:1.
const (
	x265UserDataUUIDHi  uint64 = 0x2CA2DE09B51747DB
	atemeUserDataUUIDHi uint64 = 0x427FCC9BB8924821
)

// parseHEVCUserDataUnregistered handles a payloadType==5 SEI message. payload is the
// raw message body: a 16-byte UUID followed by the (optionally NUL-terminated) text.
func parseHEVCUserDataUnregistered(payload []byte, info *hevcHDRInfo) {
	if info == nil || len(payload) < 16 {
		return
	}
	switch binary.BigEndian.Uint64(payload[0:8]) {
	case atemeUserDataUUIDHi:
		body := strings.TrimSpace(strings.TrimRight(string(payload[16:]), "\x00"))
		if strings.HasPrefix(body, "ATEME Titan KFE ") {
			info.encoderLibrary = body
			info.encoderName = "ATEME Titan KFE"
			info.encoderVersion = strings.TrimSpace(strings.TrimPrefix(body, info.encoderName))
		}
	case x265UserDataUUIDHi:
		parseX265InfoString(payload[16:], info)
	}
}

// parseX265InfoString parses the x265 info text, e.g.
//
//	x265 (build 216) - 4.2+1-e444744:[Mac OS X][clang 21.0.0][64 bit] 8bit+10bit+12bit - H.265/HEVC codec - Copyright ... - http://x265.org - options: cpuid=98 frame-threads=3 ...
//
// producing the writing library "x265 <version>" (space form, matching findX264Info)
// and the encoding settings string. Segments are delimited by " - ": segment 0 yields
// the name, segment 1 the version, the "options:" segment the settings; all other
// segments (codec name, copyright, URL) are discarded.
func parseX265InfoString(data []byte, info *hevcHDRInfo) {
	if info.x265Seen || len(data) == 0 {
		return
	}
	// MediaInfo treats the payload as text and tolerates a single trailing NUL; any
	// earlier NUL means it is not a text payload, so the message is ignored.
	s := string(data)
	if z := strings.IndexByte(s, 0x00); z >= 0 {
		if z+1 != len(data) {
			return
		}
		s = s[:z]
	}

	var encodedLibrary string
	var settings string
	loop := 0
	for pos := 0; pos < len(s); loop++ {
		segEnd := len(s)
		if idx := strings.Index(s[pos:], " - "); idx >= 0 {
			segEnd = pos + idx
		}
		switch {
		case strings.HasPrefix(s[pos:], "options: "):
			settings = parseX265Options(s[pos:])
		case loop == 0:
			value := trimBytesBelow(s[pos:segEnd], 0x30)
			if sp := strings.IndexByte(value, ' '); sp >= 0 {
				value = value[:sp]
			}
			encodedLibrary = value
		case loop == 1 && strings.HasPrefix(encodedLibrary, "x265"):
			value := s[pos:segEnd]
			if idx := strings.Index(value, " 8bpp"); idx >= 0 {
				value = value[:idx]
			}
			encodedLibrary += " - " + value
		}
		pos = segEnd
		if pos+3 <= len(s) {
			pos += 3 // skip the " - " separator
		}
	}

	if !strings.HasPrefix(encodedLibrary, "x265") {
		return
	}
	info.x265Seen = true
	info.x265Library = strings.Replace(encodedLibrary, "x265 - ", "x265 ", 1)
	info.x265Settings = settings
}

// parseX265Options tokenizes the "options: ..." segment. Following MediaInfo, the
// leading "options:" token is dropped, as are tokens that are redundant with other
// reported fields: any token starting with a digit (e.g. a positional resolution),
// "fps=", or "bitdepth=". Remaining tokens are joined with " / ".
func parseX265Options(segment string) string {
	var b strings.Builder
	for _, token := range strings.Fields(segment) {
		if token == "options:" {
			continue
		}
		if token[0] >= '0' && token[0] <= '9' {
			continue
		}
		if strings.HasPrefix(token, "fps=") || strings.HasPrefix(token, "bitdepth=") {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" / ")
		}
		b.WriteString(token)
	}
	return b.String()
}

// trimBytesBelow trims bytes with value < threshold from both ends of s.
func trimBytesBelow(s string, threshold byte) string {
	start := 0
	for start < len(s) && s[start] < threshold {
		start++
	}
	end := len(s)
	for end > start && s[end-1] < threshold {
		end--
	}
	return s[start:end]
}
