package mediainfo

import (
	"bytes"
	"testing"
)

func TestConsumeAC3RetainsPartialValidSyncframe(t *testing.T) {
	header := []byte{0x0B, 0x77, 0x00, 0x00, 0x00, 0x30, 0x00}
	entry := &tsStream{format: "AC-3"}
	consumeAC3(entry, append([]byte{0xAA, 0xBB}, header...), false, false, false)

	if !bytes.Equal(entry.audioBuffer, header) {
		t.Fatalf("audioBuffer = % X, want retained header % X", entry.audioBuffer, header)
	}
}

func TestNormalizeBDAVDTSDuration_UsesVideoDurationForCollapsedSample(t *testing.T) {
	got := normalizeBDAVDTSDuration(0.010, 28.458, true, "DTS")
	if got != 28.458 {
		t.Fatalf("normalizeBDAVDTSDuration()=%v, want %v", got, 28.458)
	}
}

func TestNormalizeBDAVDTSDuration_DoesNotOverrideNormalDuration(t *testing.T) {
	got := normalizeBDAVDTSDuration(28.417, 28.458, true, "DTS")
	if got != 28.417 {
		t.Fatalf("normalizeBDAVDTSDuration()=%v, want %v", got, 28.417)
	}
}

func TestNormalizeBDAVDTSDuration_DoesNotOverrideNonDTS(t *testing.T) {
	got := normalizeBDAVDTSDuration(0.010, 28.458, true, "AC-3")
	if got != 0.010 {
		t.Fatalf("normalizeBDAVDTSDuration()=%v, want %v", got, 0.010)
	}
}
