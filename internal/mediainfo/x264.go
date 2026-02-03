package mediainfo

import (
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
