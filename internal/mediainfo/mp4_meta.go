package mediainfo

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const mp4EpochDelta = 2082844800 // seconds between 1904-01-01 and 1970-01-01

func formatMP4UTCTime(mp4Seconds uint64) string {
	if mp4Seconds == 0 || mp4Seconds < mp4EpochDelta {
		return ""
	}
	utc := time.Unix(int64(mp4Seconds-mp4EpochDelta), 0).UTC()
	return utc.Format("2006-01-02 15:04:05 UTC")
}

func parseMvhdMeta(payload []byte) (float64, uint32, uint64, uint64, bool) {
	if len(payload) < 20 {
		return 0, 0, 0, 0, false
	}
	version := payload[0]
	switch version {
	case 0:
		if len(payload) < 20 {
			return 0, 0, 0, 0, false
		}
		creation := uint64(binary.BigEndian.Uint32(payload[4:8]))
		modified := uint64(binary.BigEndian.Uint32(payload[8:12]))
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		if timescale == 0 {
			return 0, 0, 0, 0, false
		}
		return float64(duration) / float64(timescale), timescale, creation, modified, true
	case 1:
		if len(payload) < 32 {
			return 0, 0, 0, 0, false
		}
		creation := binary.BigEndian.Uint64(payload[4:12])
		modified := binary.BigEndian.Uint64(payload[12:20])
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		if timescale == 0 {
			return 0, 0, 0, 0, false
		}
		return float64(duration) / float64(timescale), timescale, creation, modified, true
	default:
		return 0, 0, 0, 0, false
	}
}

func parseMdhdMeta(payload []byte) (float64, uint64, uint32, string, bool) {
	if len(payload) < 24 {
		return 0, 0, 0, "", false
	}
	version := payload[0]
	switch version {
	case 0:
		if len(payload) < 24 {
			return 0, 0, 0, "", false
		}
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		lang := decodeMP4Language(binary.BigEndian.Uint16(payload[20:22]))
		if timescale == 0 {
			return 0, 0, 0, "", false
		}
		return float64(duration) / float64(timescale), uint64(duration), timescale, lang, true
	case 1:
		if len(payload) < 36 {
			return 0, 0, 0, "", false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		lang := decodeMP4Language(binary.BigEndian.Uint16(payload[32:34]))
		if timescale == 0 {
			return 0, 0, 0, "", false
		}
		return float64(duration) / float64(timescale), duration, timescale, lang, true
	default:
		return 0, 0, 0, "", false
	}
}

// macintoshLanguages uses explicit QuickTime numeric codes, including the gap
// before Welsh (128): https://developer.apple.com/documentation/quicktime-file-format/language_code_values
// Reserved entries are absent. A compact-table offset would misdecode code 128.
var macintoshLanguages = map[uint16]string{
	0: "en", 1: "fr", 2: "de", 3: "it", 4: "nl", 5: "sv", 6: "es", 7: "da", 8: "pt", 9: "no",
	10: "he", 11: "ja", 12: "ar", 13: "fi", 14: "el", 15: "is", 16: "mt", 17: "tr", 18: "hr", 19: "zh-tw",
	20: "ur", 21: "hi", 22: "th", 23: "ko", 24: "lt", 25: "pl", 26: "hu", 27: "et", 28: "lv", 29: "smi",
	30: "fo", 31: "fa", 32: "ru", 33: "zh-cn", 34: "nl-be", 35: "ga", 36: "sq", 37: "ro", 38: "cs", 39: "sk",
	40: "sl", 41: "yi", 42: "sr", 43: "mk", 44: "bg", 45: "uk", 46: "be", 47: "uz", 48: "kk", 49: "az",
	50: "az", 51: "hy", 52: "ka", 53: "mo", 54: "ky", 55: "tg", 56: "tk", 57: "mn-cn", 58: "mn", 59: "ps",
	60: "ku", 61: "ks", 62: "sd", 63: "bo", 64: "ne", 65: "sa", 66: "mr", 67: "bn", 68: "as", 69: "gu",
	70: "pa", 71: "or", 72: "ml", 73: "kn", 74: "ta", 75: "te", 76: "si", 77: "my", 78: "km", 79: "lo",
	80: "vi", 81: "id", 82: "tl", 83: "ms", 84: "ms-bn", 85: "am", 87: "om", 88: "so", 89: "sw",
	90: "rw", 91: "rn", 92: "ny", 93: "mg", 94: "eo",
	128: "cy", 129: "eu", 130: "ca", 131: "la", 132: "qu", 133: "gn", 134: "ay", 135: "tt", 136: "ug", 137: "dz", 138: "jv",
}

func decodeMP4Language(code uint16) string {
	if code < 0x400 {
		return macintoshLanguages[code]
	}
	if code == 0x7fff || code&0x8000 != 0 {
		return ""
	}
	// ISO 639-2/T packed: 1 bit pad + 3x5-bit values, each plus 0x60.
	a := byte(((code >> 10) & 0x1F) + 0x60)
	b := byte(((code >> 5) & 0x1F) + 0x60)
	c := byte((code & 0x1F) + 0x60)
	if a < 'a' || a > 'z' || b < 'a' || b > 'z' || c < 'a' || c > 'z' {
		return ""
	}
	return string([]byte{a, b, c})
}

// parseMP4ExtendedLanguage accepts bounded version-0 UTF-8 strings. A malformed
// box contributes no value, leaving the track's mdhd or prior valid elng intact.
func parseMP4ExtendedLanguage(payload []byte, budget *embeddedAssetBudget) string {
	if len(payload) < 5 || payload[0] != 0 || int64(len(payload)-4) > embeddedAssetMaxNameBytes {
		return ""
	}
	value := payload[4:]
	if pos := bytes.IndexByte(value, 0); pos >= 0 {
		for _, b := range value[pos:] {
			if b != 0 {
				return ""
			}
		}
		value = value[:pos]
	}
	if len(value) == 0 || !utf8.Valid(value) {
		return ""
	}
	if budget.reserveString(uint64(len(value)), embeddedAssetMaxNameBytes) != embeddedAssetAccepted {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func parseHdlrName(payload []byte) string {
	// FullBox + pre-defined + handler_type + reserved(12) => 24 bytes.
	if len(payload) <= 24 {
		return ""
	}
	name := payload[24:]
	// QuickTime-style handlers prefix the name with an 8-bit Pascal length.
	if len(name) > 1 && int(name[0]) <= len(name)-1 && name[0] < 0x20 {
		name = name[1 : 1+int(name[0])]
	}
	if idx := bytesIndexByte(name, 0); idx >= 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(string(name))
}

func bytesIndexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

func parseMP4Chpl(udta []byte) []mp4Chapter {
	var offset int64
	for offset+8 <= int64(len(udta)) {
		boxSize, boxType, headerSize := readMP4BoxHeaderFrom(udta, offset)
		if boxSize <= 0 {
			break
		}
		if boxType == "chpl" {
			payload := sliceBox(udta, offset+headerSize, boxSize-headerSize)
			return parseMP4ChplPayload(payload)
		}
		offset += boxSize
	}
	return nil
}

func parseMP4ChplPayload(payload []byte) []mp4Chapter {
	// Observed layout:
	// version(1) flags(3) reserved(4) count(1) [uint64 time(1e7 ticks) uint8 len bytes title]...
	if len(payload) < 9 {
		return nil
	}
	count := int(payload[8])
	pos := 9
	out := make([]mp4Chapter, 0, count)
	for i := 0; i < count; i++ {
		if pos+9 > len(payload) {
			break
		}
		ticks := binary.BigEndian.Uint64(payload[pos : pos+8])
		pos += 8
		nameLen := int(payload[pos])
		pos++
		if nameLen < 0 || pos+nameLen > len(payload) {
			break
		}
		title := strings.TrimRight(string(payload[pos:pos+nameLen]), "\x00")
		pos += nameLen
		// Chapter ticks are in 1e7 units per second; ms = ticks / 10000.
		ms := int64(ticks / 10000)
		out = append(out, mp4Chapter{startMs: ms, title: title})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatMP4ChapterTimeKey(startMs int64) string {
	if startMs < 0 {
		startMs = 0
	}
	totalSeconds := startMs / 1000
	ms := startMs % 1000
	hh := totalSeconds / 3600
	mm := (totalSeconds % 3600) / 60
	ss := totalSeconds % 60
	return fmt2(hh) + "_" + fmt2(mm) + "_" + fmt2(ss) + "_" + fmt3(ms)
}

func formatMP4ChapterTimeText(startMs int64) string {
	if startMs < 0 {
		startMs = 0
	}
	totalSeconds := startMs / 1000
	ms := startMs % 1000
	hh := totalSeconds / 3600
	mm := (totalSeconds % 3600) / 60
	ss := totalSeconds % 60
	return fmt2(hh) + ":" + fmt2(mm) + ":" + fmt2(ss) + "." + fmt3(ms)
}

func fmt2(v int64) string {
	if v < 10 {
		return "0" + strconv.FormatInt(v, 10)
	}
	return strconv.FormatInt(v, 10)
}

func fmt3(v int64) string {
	if v < 10 {
		return "00" + strconv.FormatInt(v, 10)
	}
	if v < 100 {
		return "0" + strconv.FormatInt(v, 10)
	}
	return strconv.FormatInt(v, 10)
}
