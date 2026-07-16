package mediainfo

import (
	"math"
	"testing"
)

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

func TestNormalizeBDAVContainerDuration(t *testing.T) {
	const (
		fallback = 5312.429928
		size     = int64(19509374976)
		bitrate  = 29379197.994762
	)
	want := float64(size*8) / bitrate
	if got := normalizeBDAVContainerDuration(fallback, size, bitrate, true); math.Abs(got-want) > 1e-9 {
		t.Fatalf("normalizeBDAVContainerDuration()=%0.12f want=%0.12f", got, want)
	}
	if got := normalizeBDAVContainerDuration(fallback, size, bitrate, false); got != fallback {
		t.Fatalf("non-BDAV duration=%0.12f want=%0.12f", got, fallback)
	}
}
