package mediainfo

import (
	"strconv"
	"strings"
)

func findX264Info(data []byte) (string, string) {
	idx := strings.Index(string(data), "x264 - core")
	if idx == -1 {
		return "", ""
	}
	s := string(data[idx:])
	end := strings.IndexByte(s, 0)
	if end != -1 {
		s = s[:end]
	}

	writingLib := ""
	if after, ok := strings.CutPrefix(s, "x264 - "); ok {
		rest := after
		parts := strings.SplitN(rest, " - ", 2)
		if len(parts) > 0 {
			version := strings.TrimLeft(parts[0], " \t\r\n")
			writingLib = "x264 " + version
		}
	}

	encoding := ""
	if _, after, ok := strings.Cut(s, "options:"); ok {
		opts := strings.TrimSpace(after)
		if opts != "" {
			tokens := strings.Fields(opts)
			encoding = strings.Join(tokens, " / ")
		}
	}

	return writingLib, encoding
}

// findX264InfoAnnexB extracts x264's unregistered SEI payload without mixing
// adjacent encoded bytes into the null-terminated options string.
func findX264InfoAnnexB(data []byte) (string, string) {
	var writingLibrary, encoding string
	scanAnnexBNALs(data, func(nal []byte) bool {
		if len(nal) == 0 || nal[0]&0x1F != 6 {
			return true
		}
		rbsp := nalToRBSP(nal)
		for pos := 0; pos < len(rbsp); {
			payloadType := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadType += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadType += int(rbsp[pos])
			pos++

			payloadSize := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadSize += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadSize += int(rbsp[pos])
			pos++
			if payloadSize > len(rbsp)-pos {
				break
			}
			if payloadType == 5 && payloadSize > 16 {
				writingLibrary, encoding = findX264Info(rbsp[pos+16 : pos+payloadSize])
				if writingLibrary != "" {
					return false
				}
			}
			pos += payloadSize
		}
		return true
	})
	return writingLibrary, encoding
}

// findLastX264Info returns the final framed x264 unregistered-SEI payload in an
// Annex-B sample. Markers in slices or other NAL types are intentionally ignored.
func findLastX264Info(data []byte) (string, string) {
	var writingLibrary, encoding string
	scanAnnexBNALs(data, func(nal []byte) bool {
		if len(nal) == 0 || nal[0]&0x1F != 6 {
			return true
		}
		rbsp := nalToRBSP(nal)
		for pos := 0; pos < len(rbsp); {
			payloadType := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadType += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadType += int(rbsp[pos])
			pos++
			payloadSize := 0
			for pos < len(rbsp) && rbsp[pos] == 0xFF {
				payloadSize += 255
				pos++
			}
			if pos >= len(rbsp) {
				break
			}
			payloadSize += int(rbsp[pos])
			pos++
			if payloadSize > len(rbsp)-pos {
				break
			}
			if payloadType == 5 && payloadSize > 16 {
				if library, settings := findX264Info(rbsp[pos+16 : pos+payloadSize]); library != "" || settings != "" {
					writingLibrary, encoding = library, settings
				}
			}
			pos += payloadSize
		}
		return true
	})
	return writingLibrary, encoding
}

// findLastX264InfoLengthPrefixed validates MP4 AVC NAL framing before reading
// x264 user_data_unregistered SEI. This keeps non-SEI marker bytes inert while
// retaining the complete SEI payload rather than the abbreviated Annex-B probe.
func findLastX264InfoLengthPrefixed(data []byte, lengthSize int) (string, string) {
	if lengthSize < 1 || lengthSize > 4 {
		return "", ""
	}
	var writingLibrary, encoding string
	for pos := 0; pos+lengthSize <= len(data); {
		size := 0
		for i := 0; i < lengthSize; i++ {
			size = size<<8 | int(data[pos+i])
		}
		pos += lengthSize
		if size <= 0 || size > len(data)-pos {
			break
		}
		nal := data[pos : pos+size]
		pos += size
		if len(nal) == 0 || nal[0]&0x1F != 6 {
			continue
		}
		rbsp := nalToRBSP(nal)
		for seiPos := 0; seiPos < len(rbsp); {
			payloadType := 0
			for seiPos < len(rbsp) && rbsp[seiPos] == 0xFF {
				payloadType += 255
				seiPos++
			}
			if seiPos >= len(rbsp) {
				break
			}
			payloadType += int(rbsp[seiPos])
			seiPos++
			payloadSize := 0
			for seiPos < len(rbsp) && rbsp[seiPos] == 0xFF {
				payloadSize += 255
				seiPos++
			}
			if seiPos >= len(rbsp) {
				break
			}
			payloadSize += int(rbsp[seiPos])
			seiPos++
			if payloadSize > len(rbsp)-seiPos {
				break
			}
			if payloadType == 5 && payloadSize > 16 {
				if library, settings := findX264Info(rbsp[seiPos+16 : seiPos+payloadSize]); library != "" || settings != "" {
					writingLibrary, encoding = library, settings
				}
			}
			seiPos += payloadSize
		}
	}
	return writingLibrary, encoding
}

func findX264Bitrate(encoding string) (float64, bool) {
	idx := strings.Index(encoding, "bitrate=")
	if idx == -1 {
		return 0, false
	}
	start := idx + len("bitrate=")
	end := start
	for end < len(encoding) {
		ch := encoding[end]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			end++
			continue
		}
		break
	}
	if end == start {
		return 0, false
	}
	value, err := strconv.ParseFloat(encoding[start:end], 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value * 1000, true
}

func findX264ParamKbps(encoding string, key string) (float64, bool) {
	idx := strings.Index(encoding, key+"=")
	if idx == -1 {
		return 0, false
	}
	start := idx + len(key) + 1
	end := start
	for end < len(encoding) {
		ch := encoding[end]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			end++
			continue
		}
		break
	}
	if end == start {
		return 0, false
	}
	value, err := strconv.ParseFloat(encoding[start:end], 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func findX264VbvMaxrate(encoding string) (float64, bool) {
	return findX264ParamKbps(encoding, "vbv_maxrate")
}

func findX264VbvBufsize(encoding string) (float64, bool) {
	return findX264ParamKbps(encoding, "vbv_bufsize")
}

func findX264ParamInt(encoding string, key string) (int, bool) {
	idx := strings.Index(encoding, key+"=")
	if idx == -1 {
		return 0, false
	}
	start := idx + len(key) + 1
	end := start
	for end < len(encoding) {
		ch := encoding[end]
		if ch >= '0' && ch <= '9' {
			end++
			continue
		}
		break
	}
	if end == start {
		return 0, false
	}
	value, err := strconv.Atoi(encoding[start:end])
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func findX264Keyint(encoding string) (int, bool) {
	value, ok := findX264ParamInt(encoding, "keyint")
	if !ok || value <= 0 {
		return 0, false
	}
	return value, true
}

func findX264Bframes(encoding string) (int, bool) {
	return findX264ParamInt(encoding, "bframes")
}
