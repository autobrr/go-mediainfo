package mediainfo

import (
	"encoding/binary"
	"testing"
)

func TestFindX264InfoAnnexBUsesSEIPayloadBoundary(t *testing.T) {
	text := []byte("x264 - core 164 r3108 - H.264/MPEG-4 AVC codec - options: cabac=1 trellis=2\x00")
	payloadSize := 16 + len(text)
	nal := make([]byte, 0, payloadSize+12)
	nal = append(nal, 0, 0, 1, 0x06, 0x05, byte(payloadSize))
	nal = append(nal, make([]byte, 16)...)
	nal = append(nal, text...)
	nal = append(nal, 0x80, 0, 0, 1, 0x65, 0xFF)

	library, settings := findX264InfoAnnexB(nal)
	if library != "x264 core 164 r3108" {
		t.Fatalf("library=%q", library)
	}
	if settings != "cabac=1 / trellis=2" {
		t.Fatalf("settings=%q", settings)
	}
}

func TestFindLastX264InfoLengthPrefixedValidatesNALType(t *testing.T) {
	frame := func(nal []byte) []byte {
		framed := make([]byte, 4, len(nal)+4)
		binary.BigEndian.PutUint32(framed, uint32(len(nal)))
		return append(framed, nal...)
	}
	sei := func(crf string) []byte {
		text := []byte("x264 - core 164 r3108 - H.264/MPEG-4 AVC codec - options: cabac=1 crf=" + crf + "\x00")
		payloadSize := 16 + len(text)
		nal := []byte{0x06, 0x05, byte(payloadSize)}
		nal = append(nal, make([]byte, 16)...)
		nal = append(nal, text...)
		return append(nal, 0x80)
	}

	sample := append(frame(sei("35.0")), frame(sei("16.0"))...)
	library, settings := findLastX264InfoLengthPrefixed(sample, 4)
	if library != "x264 core 164 r3108" || settings != "cabac=1 / crf=16.0" {
		t.Fatalf("valid SEI = library %q settings %q", library, settings)
	}

	forged := frame([]byte("\x65x264 - core 164 - options: crf=1.0\x00"))
	if library, settings := findLastX264InfoLengthPrefixed(forged, 4); library != "" || settings != "" {
		t.Fatalf("non-SEI marker accepted: library %q settings %q", library, settings)
	}

	malformed := frame(sei("18.0"))
	binary.BigEndian.PutUint32(malformed, uint32(len(malformed)))
	if library, settings := findLastX264InfoLengthPrefixed(malformed, 4); library != "" || settings != "" {
		t.Fatalf("malformed length accepted: library %q settings %q", library, settings)
	}
}
