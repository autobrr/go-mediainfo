package mediainfo

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestFindX264InfoAnnexBUsesSEIPayloadBoundary(t *testing.T) {
	text := []byte("x264 - core 164 r3108 - H.264/MPEG-4 AVC codec - options: cabac=1 trellis=2\x00")
	payloadSize := 16 + len(text)
	nal := make([]byte, 0, payloadSize+12)
	nal = append(nal, 0, 0, 1, 0x06, 0x05, byte(payloadSize))
	uuid := append(x264SEIUUIDHigh[:], make([]byte, 8)...)
	nal = append(nal, uuid...)
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

func TestFindX264InfoPreservesVersionPaddingWithoutOptions(t *testing.T) {
	library, settings := findX264Info([]byte("x264 - core 164 r3107 encoded by tool          \x00"))
	if library != "x264 core 164 r3107 encoded by tool          " || settings != "" {
		t.Fatalf("library=%q settings=%q", library, settings)
	}
	name, version := splitEncodedLibrary("x264 - " + strings.TrimPrefix(library, "x264 "))
	if name != "x264" || version != "core 164 r3107 encoded by tool          " {
		t.Fatalf("split library = (%q, %q)", name, version)
	}
}

func TestFindX264InfoPreservesVersionPaddingBeforeOptions(t *testing.T) {
	library, settings := findX264Info([]byte("x264 - core 164 r3107 encoded by tool          - options: cabac=1\x00"))
	if library != "x264 core 164 r3107 encoded by tool         " || settings != "cabac=1" {
		t.Fatalf("library=%q settings=%q", library, settings)
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
		uuid := append(x264SEIUUIDHigh[:], make([]byte, 8)...)
		nal = append(nal, uuid...)
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

func TestFindX264InfoSEIPayloadRejectsUnknownUUIDAndEmbeddedNull(t *testing.T) {
	text := []byte("x264 - core 164 - options: crf=18.0")
	unknown := append(make([]byte, 16), text...)
	if library, settings := findX264InfoSEIPayload(unknown); library != "" || settings != "" {
		t.Fatalf("unknown UUID accepted: library %q settings %q", library, settings)
	}
	malformed := append(append(append([]byte{}, x264SEIUUIDHigh[:]...), make([]byte, 8)...), text...)
	malformed = append(malformed, 0, 'x')
	if library, settings := findX264InfoSEIPayload(malformed); library != "" || settings != "" {
		t.Fatalf("embedded null accepted: library %q settings %q", library, settings)
	}
}

func TestFindX264InfoSEIPayloadAcceptsGenericEncoderUnderRecognizedUUID(t *testing.T) {
	payload := append(append([]byte{}, x264SEIUUIDHigh[:]...), make([]byte, 8)...)
	payload = append(payload, []byte("Zencoder Video Encoding System\x00")...)
	library, settings := findX264InfoSEIPayload(payload)
	if library != "Zencoder Video Encoding System" || settings != "" {
		t.Fatalf("generic encoder = %q, %q", library, settings)
	}
}

func TestFindLastX264InfoLengthPrefixedRejectsZeroTrailingSEI(t *testing.T) {
	text := []byte("x264 - core 84 - options: bitrate=20000\x00")
	payloadSize := 16 + len(text)
	nal := make([]byte, 0, 3+payloadSize+1)
	nal = append(nal, 0x06, 0x05, byte(payloadSize))
	nal = append(nal, x264SEIUUIDHigh[:]...)
	nal = append(nal, make([]byte, 8)...)
	nal = append(nal, text...)
	nal = append(nal, 0x00)
	framed := make([]byte, 4, len(nal)+4)
	binary.BigEndian.PutUint32(framed, uint32(len(nal)))
	framed = append(framed, nal...)
	if library, settings := findLastX264InfoLengthPrefixed(framed, 4); library != "" || settings != "" {
		t.Fatalf("zero-trailing SEI accepted: library %q settings %q", library, settings)
	}
}
