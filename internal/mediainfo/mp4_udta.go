package mediainfo

import (
	"encoding/binary"
	"strings"
)

func parseMP4WritingApp(udta []byte) string {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return ""
	}
	meta = mp4MetaChildren(meta)
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

func parseMP4Description(udta []byte) string {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return ""
	}
	meta = mp4MetaChildren(meta)
	ilst, ok := findMP4Box(meta, "ilst")
	if !ok {
		return ""
	}
	desc, ok := findMP4Box(ilst, "desc")
	if !ok {
		return ""
	}
	data, ok := findMP4Box(desc, "data")
	if !ok || len(data) <= 8 {
		return ""
	}
	return string(data[8:])
}

// parseMP4UserMetadata decodes common iTunes ilst text atoms into General
// metadata fields.
func parseMP4UserMetadata(udta []byte, budget *embeddedAssetBudget) []Field {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return nil
	}
	ilst, ok := findMP4Box(mp4MetaChildren(meta), "ilst")
	if !ok {
		return nil
	}
	tags := []struct {
		atom  string
		label string
	}{
		{atom: "\xa9nam", label: "Title"},
		{atom: "\xa9alb", label: "Album"},
		{atom: "\xa9cmt", label: "Comment"},
		{atom: "tvsh", label: "Collection"},
	}
	fields := make([]Field, 0, len(tags))
	for _, tag := range tags {
		item, found := findMP4Box(ilst, tag.atom)
		if !found {
			continue
		}
		data, found := findMP4Box(item, "data")
		if !found || len(data) <= 8 {
			continue
		}
		value, valid := decodeMP4MetadataValue(data)
		// Retain legacy untyped iTunes strings already accepted by this parser.
		if binary.BigEndian.Uint32(data[:4]) == 0 {
			value, valid = strings.TrimRight(string(data[8:]), "\x00"), true
		}
		if !valid {
			continue
		}
		if tag.atom == "\xa9nam" {
			value = strings.TrimSpace(value)
		}
		if value != "" && budget.reserveString(uint64(len(value)), embeddedAssetMaxStringBytes) == embeddedAssetAccepted {
			fields = append(fields, Field{Name: tag.label, Value: value})
		}
	}
	return fields
}

// parseMP4TrackName decodes a QuickTime trak/udta name atom.
func parseMP4TrackName(udta []byte) string {
	payload, ok := findMP4Box(udta, "name")
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(payload), "\x00"))
}

// parseMP4UnknownUserMetadata reports recognized ilst atoms whose data kind is
// unsupported by MediaInfo's text projection.
func parseMP4UnknownUserMetadata(udta []byte) []jsonKV {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return nil
	}
	ilst, ok := findMP4Box(mp4MetaChildren(meta), "ilst")
	if !ok {
		return nil
	}
	cover, ok := findMP4Box(ilst, "covr")
	if !ok {
		return nil
	}
	data, ok := findMP4Box(cover, "data")
	if !ok || len(data) < 8 {
		return nil
	}
	kind := binary.BigEndian.Uint32(data[:4]) & 0x00FFFFFF
	if kind == 13 || kind == 14 || kind == 27 && validMP4BMPCover(data[8:]) {
		return nil
	}
	return []jsonKV{{Key: "covr", Val: "Unknown kind of value!"}}
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

// validMP4BMPCover checks only the fixed BMP headers without retaining pixels.
func validMP4BMPCover(data []byte) bool {
	if len(data) < 26 || string(data[:2]) != "BM" {
		return false
	}
	size := uint64(binary.LittleEndian.Uint32(data[2:6]))
	offset := uint64(binary.LittleEndian.Uint32(data[10:14]))
	dib := uint64(binary.LittleEndian.Uint32(data[14:18]))
	if size != uint64(len(data)) || offset >= size || dib > size-14 || offset < 14+dib {
		return false
	}
	var width, height int64
	var planes, bits uint16
	if dib == 12 {
		width = int64(binary.LittleEndian.Uint16(data[18:20]))
		height = int64(binary.LittleEndian.Uint16(data[20:22]))
		planes = binary.LittleEndian.Uint16(data[22:24])
		bits = binary.LittleEndian.Uint16(data[24:26])
	} else if dib >= 40 && len(data) >= 54 {
		width = int64(int32(binary.LittleEndian.Uint32(data[18:22])))
		height = int64(int32(binary.LittleEndian.Uint32(data[22:26])))
		planes = binary.LittleEndian.Uint16(data[26:28])
		bits = binary.LittleEndian.Uint16(data[28:30])
	} else {
		return false
	}
	return width > 0 && height != 0 && planes == 1 && (bits == 1 || bits == 4 || bits == 8 || bits == 16 || bits == 24 || bits == 32)
}
