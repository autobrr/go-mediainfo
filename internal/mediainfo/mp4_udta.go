package mediainfo

import "encoding/binary"

func parseMP4WritingApp(udta []byte) string {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return ""
	}
	meta = meta[4:]
	ilst, ok := findMP4Box(meta, "ilst")
	if !ok {
		return ""
	}
	tool, ok := findMP4Box(ilst, "\xa9too")
	if !ok {
		return ""
	}
	data, ok := findMP4Box(tool, "data")
	if !ok || len(data) <= 8 {
		return ""
	}
	return string(data[8:])
}

func findMP4Box(buf []byte, boxType string) ([]byte, bool) {
	pos := 0
	for pos+8 <= len(buf) {
		size := int(binary.BigEndian.Uint32(buf[pos : pos+4]))
		if size < 8 || pos+size > len(buf) {
			return nil, false
		}
		typ := string(buf[pos+4 : pos+8])
		if typ == boxType {
			return buf[pos+8 : pos+size], true
		}
		pos += size
	}
	return nil, false
}
