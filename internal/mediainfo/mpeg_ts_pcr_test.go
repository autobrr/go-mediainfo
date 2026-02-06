package mediainfo

import "testing"

func TestParsePCR27(t *testing.T) {
	// Minimal TS packet with adaptation field containing a PCR.
	packet := make([]byte, 188)
	packet[3] = 0x30 // adaptation=3 (adapt+payload)
	packet[4] = 7    // adaptation_field_length
	packet[5] = 0x10 // PCR_flag

	var (
		base uint64 = 0x1ABCDEFFF // <= 33 bits
		ext  uint64 = 0x12A       // <= 9 bits
	)

	// Encode PCR base/ext into bytes 6..11.
	packet[6] = byte(base >> 25)
	packet[7] = byte(base >> 17)
	packet[8] = byte(base >> 9)
	packet[9] = byte(base >> 1)
	packet[10] = byte((base&1)<<7) | 0x7E | byte((ext>>8)&1) // reserved bits ignored by parser
	packet[11] = byte(ext & 0xFF)

	got, ok := parsePCR27(packet)
	if !ok {
		t.Fatalf("parsePCR27: ok=false")
	}
	want := base*300 + ext
	if got != want {
		t.Fatalf("parsePCR27: got=%d want=%d", got, want)
	}
}

