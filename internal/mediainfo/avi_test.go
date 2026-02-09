package mediainfo

import (
	"encoding/binary"
	"testing"
)

func TestParseAVIIndex(t *testing.T) {
	streams := []*aviStream{
		{index: 0},
		{index: 1},
	}
	entry := func(id string, size uint32) []byte {
		buf := make([]byte, 16)
		copy(buf[0:4], []byte(id))
		binary.LittleEndian.PutUint32(buf[12:16], size)
		return buf
	}
	data := append(entry("00dc", 1200), entry("01wb", 400)...)
	data = append(data, entry("JUNK", 999)...)

	if ok, _ := parseAVIIndex(data, streams); !ok {
		t.Fatalf("expected index parse to find stream entries")
	}
	if streams[0].bytes != 1200 {
		t.Fatalf("unexpected stream 0 bytes: %d", streams[0].bytes)
	}
	if streams[1].bytes != 400 {
		t.Fatalf("unexpected stream 1 bytes: %d", streams[1].bytes)
	}
}

func TestParseAVIIndexNoEntries(t *testing.T) {
	streams := []*aviStream{{index: 0}}
	if ok, _ := parseAVIIndex([]byte("short"), streams); ok {
		t.Fatalf("expected no index entries")
	}
	if streams[0].bytes != 0 {
		t.Fatalf("unexpected bytes update: %d", streams[0].bytes)
	}
}
