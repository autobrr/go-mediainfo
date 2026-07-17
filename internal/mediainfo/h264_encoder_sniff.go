package mediainfo

import (
	"bytes"
	"strings"
)

func findH264WritingLibrary(data []byte) string {
	// MediaInfo sometimes derives Encoded_Library from encoder strings embedded in the bitstream (SEI user data).
	// Keep this conservative: only return well-known, explicit markers.
	const zencoder = "Zencoder Video Encoding System"
	if bytes.Contains(data, []byte(zencoder)) {
		return zencoder
	}
	if lib := findBiliBiliH264Encoder(data); lib != "" {
		return lib
	}
	return ""
}

func findBiliBiliH264Encoder(data []byte) string {
	const marker = "BiliBili H264 Encoder"
	idx := bytes.Index(data, []byte(marker))
	if idx == -1 {
		return ""
	}
	end := idx
	// Scan forward for a short, printable, NUL-terminated ASCII string.
	for end < len(data) && end-idx < 128 {
		ch := data[end]
		if ch == 0 {
			break
		}
		if ch < 0x20 || ch > 0x7E {
			break
		}
		end++
	}
	s := strings.TrimSpace(string(data[idx:end]))
	if s == marker {
		return ""
	}
	return s
}
