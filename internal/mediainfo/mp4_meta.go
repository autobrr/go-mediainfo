package mediainfo

import (
	"encoding/binary"
	"strconv"
	"strings"
	"time"
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

func parseMdhdMeta(payload []byte) (float64, uint32, string, bool) {
	if len(payload) < 24 {
		return 0, 0, "", false
	}
	version := payload[0]
	switch version {
	case 0:
		if len(payload) < 24 {
			return 0, 0, "", false
		}
		timescale := binary.BigEndian.Uint32(payload[12:16])
		duration := binary.BigEndian.Uint32(payload[16:20])
		lang := decodeMP4Language(binary.BigEndian.Uint16(payload[20:22]))
		if timescale == 0 {
			return 0, 0, "", false
		}
		return float64(duration) / float64(timescale), timescale, lang, true
	case 1:
		if len(payload) < 36 {
			return 0, 0, "", false
		}
		timescale := binary.BigEndian.Uint32(payload[20:24])
		duration := binary.BigEndian.Uint64(payload[24:32])
		lang := decodeMP4Language(binary.BigEndian.Uint16(payload[32:34]))
		if timescale == 0 {
			return 0, 0, "", false
		}
		return float64(duration) / float64(timescale), timescale, lang, true
	default:
		return 0, 0, "", false
	}
}

func decodeMP4Language(code uint16) string {
	// ISO 639-2/T packed: 1 bit pad + 3x5-bit values, each plus 0x60.
	a := byte(((code >> 10) & 0x1F) + 0x60)
	b := byte(((code >> 5) & 0x1F) + 0x60)
	c := byte((code & 0x1F) + 0x60)
	if a < 'a' || a > 'z' || b < 'a' || b > 'z' || c < 'a' || c > 'z' {
		return ""
	}
	return string([]byte{a, b, c})
}

func parseHdlrName(payload []byte) string {
	// FullBox + pre-defined + handler_type + reserved(12) => 24 bytes.
	if len(payload) <= 24 {
		return ""
	}
	name := payload[24:]
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
