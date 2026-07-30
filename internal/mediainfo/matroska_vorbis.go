package mediainfo

import (
	"encoding/binary"
	"regexp"
	"strings"
)

// matroskaVorbisInfo holds identification-header bitrates and selected comment
// metadata exposed by MediaInfo.
type matroskaVorbisInfo struct {
	maximumBitRate int64
	nominalBitRate int64
	minimumBitRate int64
	vendor         string
	encoder        string
	applicationURL string
}

// parseMatroskaVorbisPrivate decodes Matroska-laced Vorbis identification and
// comment headers. A valid identification header is sufficient for success.
func parseMatroskaVorbisPrivate(data []byte) (matroskaVorbisInfo, bool) {
	if len(data) < 3 || data[0] != 2 {
		return matroskaVorbisInfo{}, false
	}
	pos := 1
	readLaceSize := func() (int, bool) {
		total := 0
		for pos < len(data) {
			value := int(data[pos])
			pos++
			total += value
			if value != 0xff {
				return total, true
			}
		}
		return 0, false
	}
	identSize, ok := readLaceSize()
	if !ok {
		return matroskaVorbisInfo{}, false
	}
	commentSize, ok := readLaceSize()
	if !ok || identSize < 30 || pos+identSize+commentSize > len(data) {
		return matroskaVorbisInfo{}, false
	}
	ident := data[pos : pos+identSize]
	comment := data[pos+identSize : pos+identSize+commentSize]
	if len(ident) < 30 || ident[0] != 1 || string(ident[1:7]) != "vorbis" {
		return matroskaVorbisInfo{}, false
	}
	info := matroskaVorbisInfo{
		maximumBitRate: int64(int32(binary.LittleEndian.Uint32(ident[16:20]))),
		nominalBitRate: int64(int32(binary.LittleEndian.Uint32(ident[20:24]))),
		minimumBitRate: int64(int32(binary.LittleEndian.Uint32(ident[24:28]))),
	}
	if len(comment) < 11 || comment[0] != 3 || string(comment[1:7]) != "vorbis" {
		return info, true
	}
	offset := 7
	vendorLength := int(binary.LittleEndian.Uint32(comment[offset : offset+4]))
	offset += 4
	if vendorLength < 0 || offset+vendorLength > len(comment) {
		return info, true
	}
	info.vendor = string(comment[offset : offset+vendorLength])
	offset += vendorLength
	if offset+4 > len(comment) {
		return info, true
	}
	count := int(binary.LittleEndian.Uint32(comment[offset : offset+4]))
	offset += 4
	for range count {
		if offset+4 > len(comment) {
			break
		}
		length := int(binary.LittleEndian.Uint32(comment[offset : offset+4]))
		offset += 4
		if length < 0 || offset+length > len(comment) {
			break
		}
		entry := string(comment[offset : offset+length])
		offset += length
		name, value, found := strings.Cut(entry, "=")
		if !found || value == "" {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(name)) {
		case "ENCODER", "ENCODED_BY", "ENCODED-USING", "ENCODED_USING":
			if info.encoder == "" {
				info.encoder = strings.TrimSpace(value)
			}
		case "ENCODER_URL", "ENCODED_USING_URL", "ENCODED-USING-URL":
			if info.applicationURL == "" {
				info.applicationURL = strings.TrimSpace(value)
			}
		}
	}
	if info.applicationURL == "" && strings.Contains(strings.ToLower(info.encoder), "besweet") {
		info.applicationURL = "http://DSPguru.doom9.org"
	}
	return info, true
}

// splitMatroskaVorbisLibrary normalizes known aoTuV and libVorbis vendor
// strings into MediaInfo library components.
func splitMatroskaVorbisLibrary(value string) (name, version, date string) {
	lower := strings.ToLower(value)
	dateDigits := ""
	if match := regexp.MustCompile(`\b(\d{8})\b`).FindStringSubmatch(value); len(match) > 1 {
		dateDigits = match[1]
		date = dateDigits[:4] + "-" + dateDigits[4:6] + "-" + dateDigits[6:]
	}
	switch {
	case strings.Contains(lower, "aotuv"):
		name = "aoTuV"
		if match := regexp.MustCompile(`(?i)aotuv\s+([^\[]*)`).FindStringSubmatch(value); len(match) > 1 {
			version = match[1]
		}
		if strings.TrimSpace(version) == "" {
			version = dateDigits
		}
	case strings.Contains(lower, "libvorbis"):
		name = "libVorbis"
		if strings.Contains(lower, "xiph.org libvorbis i 20020717") {
			version = "1.0"
		} else if match := regexp.MustCompile(`(?i)xiph\.org libvorbis i (\d{8}) (\([^)]*\))`).FindStringSubmatch(value); len(match) > 2 {
			version = match[2]
			date = match[1] + " " + match[2]
		}
	}
	return name, version, date
}
