package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf16"
)

type id3Picture struct {
	MIME        string
	Description string
	DataHead    []byte
	DataSize    int64
}

type id3v2Data struct {
	Offset   int64
	Text     map[string]string
	Pictures []id3Picture
}

func parseID3v2(file io.ReadSeeker) (id3v2Data, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return id3v2Data{}, false
	}
	var header [10]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		_, _ = file.Seek(0, io.SeekStart)
		return id3v2Data{}, false
	}
	if header[0] != 'I' || header[1] != 'D' || header[2] != '3' {
		_, _ = file.Seek(0, io.SeekStart)
		return id3v2Data{Offset: 0}, true
	}

	ver := header[3]
	if ver != 3 && ver != 4 {
		// Unsupported version; still skip.
		size := synchsafe32(header[6:10])
		offset := int64(10 + size)
		_, _ = file.Seek(offset, io.SeekStart)
		return id3v2Data{Offset: offset}, true
	}

	flags := header[5]
	tagSize := int64(synchsafe32(header[6:10]))
	offset := int64(10) + tagSize

	payload := make([]byte, tagSize)
	if _, err := io.ReadFull(file, payload); err != nil {
		_, _ = file.Seek(offset, io.SeekStart)
		return id3v2Data{Offset: offset}, true
	}

	// Ignore unsynchronization for now; most modern tags don't use it.
	_ = flags

	text := map[string]string{}
	var pics []id3Picture
	rd := payload

	// Skip extended header if present.
	if ver == 3 && (flags&0x40) != 0 && len(rd) >= 4 {
		ext := int(binary.BigEndian.Uint32(rd[0:4]))
		if ext > 0 && ext <= len(rd) {
			rd = rd[ext:]
		}
	}
	if ver == 4 && (flags&0x40) != 0 && len(rd) >= 4 {
		ext := int(synchsafe32(rd[0:4]))
		if ext > 0 && ext <= len(rd) {
			rd = rd[ext:]
		}
	}

	for len(rd) >= 10 {
		if bytes.Equal(rd[0:4], []byte{0, 0, 0, 0}) {
			break
		}
		id := string(rd[0:4])
		var size int
		if ver == 4 {
			size = int(synchsafe32(rd[4:8]))
		} else {
			size = int(binary.BigEndian.Uint32(rd[4:8]))
		}
		if size <= 0 || 10+size > len(rd) {
			break
		}
		flags2 := rd[8:10]
		_ = flags2

		data := rd[10 : 10+size]
		switch id {
		case "TIT2", "TALB", "TPE1", "TPE2", "TENC", "TRCK", "TYER", "TDRC":
			if v := decodeID3Text(data); v != "" {
				text[id] = normalizeID3Multi(v)
			}
		case "APIC":
			if pic, ok := parseID3APIC(data); ok {
				pics = append(pics, pic)
			}
		}
		rd = rd[10+size:]
	}

	_, _ = file.Seek(offset, io.SeekStart)
	return id3v2Data{Offset: offset, Text: text, Pictures: pics}, true
}

func synchsafe32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return (uint32(b[0]&0x7F) << 21) | (uint32(b[1]&0x7F) << 14) | (uint32(b[2]&0x7F) << 7) | uint32(b[3]&0x7F)
}

func decodeID3Text(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	enc := data[0]
	raw := data[1:]
	switch enc {
	case 0x00, 0x03:
		// ISO-8859-1 or UTF-8: for our ASCII-ish tags, treat as bytes.
		s := string(bytes.TrimRight(raw, "\x00"))
		return strings.TrimSpace(s)
	case 0x01, 0x02:
		// UTF-16 with BOM or UTF-16BE.
		if len(raw) < 2 {
			return ""
		}
		be := enc == 0x02
		if enc == 0x01 {
			// BOM.
			if raw[0] == 0xFE && raw[1] == 0xFF {
				be = true
				raw = raw[2:]
			} else if raw[0] == 0xFF && raw[1] == 0xFE {
				be = false
				raw = raw[2:]
			}
		}
		raw = bytes.TrimRight(raw, "\x00")
		if len(raw)%2 == 1 {
			raw = raw[:len(raw)-1]
		}
		u16 := make([]uint16, 0, len(raw)/2)
		for i := 0; i+1 < len(raw); i += 2 {
			if be {
				u16 = append(u16, binary.BigEndian.Uint16(raw[i:i+2]))
			} else {
				u16 = append(u16, binary.LittleEndian.Uint16(raw[i:i+2]))
			}
		}
		s := string(utf16.Decode(u16))
		return strings.TrimSpace(s)
	default:
		return ""
	}
}

func normalizeID3Multi(s string) string {
	// ID3v2.4 often uses NUL separators for multiple values.
	if strings.ContainsRune(s, '\u0000') {
		parts := strings.Split(s, "\x00")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, " / ")
	}
	return strings.TrimSpace(s)
}

func parseID3APIC(data []byte) (id3Picture, bool) {
	if len(data) < 4 {
		return id3Picture{}, false
	}
	enc := data[0]
	rd := data[1:]
	mimeEnd := bytes.IndexByte(rd, 0x00)
	if mimeEnd < 0 {
		return id3Picture{}, false
	}
	mime := string(rd[:mimeEnd])
	rd = rd[mimeEnd+1:]
	if len(rd) < 2 {
		return id3Picture{}, false
	}
	_ = rd[0] // picture type
	rd = rd[1:]

	desc := ""
	if enc == 0x00 || enc == 0x03 {
		if idx := bytes.IndexByte(rd, 0x00); idx >= 0 {
			desc = strings.TrimSpace(string(rd[:idx]))
			rd = rd[idx+1:]
		}
	} else {
		// UTF-16: description ends with 0x00 0x00
		if idx := bytes.Index(rd, []byte{0x00, 0x00}); idx >= 0 {
			desc = decodeID3Text(append([]byte{enc}, rd[:idx]...))
			rd = rd[idx+2:]
		}
	}
	if desc == "" {
		desc = "Image"
	}
	if len(rd) == 0 {
		return id3Picture{}, false
	}
	head := rd
	if len(head) > 64<<10 {
		head = head[:64<<10]
	}
	return id3Picture{
		MIME:        mime,
		Description: desc,
		DataHead:    append([]byte(nil), head...),
		DataSize:    int64(len(rd)),
	}, true
}
