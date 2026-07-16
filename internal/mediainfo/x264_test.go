package mediainfo

import "testing"

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
