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

func parseMP4Description(udta []byte) string {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return ""
	}
	meta = meta[4:]
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
func parseMP4UserMetadata(udta []byte) []Field {
	meta, ok := findMP4Box(udta, "meta")
	if !ok || len(meta) <= 4 {
		return nil
	}
	ilst, ok := findMP4Box(meta[4:], "ilst")
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
		value := strings.TrimRight(string(data[8:]), "\x00")
		if value != "" {
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
	ilst, ok := findMP4Box(meta[4:], "ilst")
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
	if kind == 13 || kind == 14 {
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
