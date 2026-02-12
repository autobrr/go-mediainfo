package mediainfo

import "testing"

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
