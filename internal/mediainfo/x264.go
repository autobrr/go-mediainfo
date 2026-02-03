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
	if strings.HasPrefix(s, "x264 - ") {
		rest := strings.TrimPrefix(s, "x264 - ")
		parts := strings.SplitN(rest, " - ", 2)
		if len(parts) > 0 {
			writingLib = "x264 " + strings.TrimSpace(parts[0])
		}
	}

	encoding := ""
	if idx := strings.Index(s, "options:"); idx != -1 {
		opts := strings.TrimSpace(s[idx+len("options:"):])
		if opts != "" {
			tokens := strings.Fields(opts)
			encoding = strings.Join(tokens, " / ")
		}
	}

	return writingLib, encoding
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
