package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBDAVOverallBitRateMaximumUsesDiscIndexVersion(t *testing.T) {
	bdmv := filepath.Join(t.TempDir(), "BDMV")
	streamDir := filepath.Join(bdmv, "STREAM")
	if err := os.MkdirAll(streamDir, 0o755); err != nil {
		t.Fatal(err)
	}
	streamPath := filepath.Join(streamDir, "00001.m2ts")
	if err := os.WriteFile(filepath.Join(bdmv, "index.bdmv"), []byte("INDX0300"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := bdavOverallBitRateMaximum(streamPath, false, false, 1, 1, 1); got != "109000000" {
		t.Fatalf("UHD maximum = %q, want 109000000", got)
	}
	if err := os.WriteFile(filepath.Join(bdmv, "index.bdmv"), []byte("INDX0200"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := bdavOverallBitRateMaximum(streamPath, true, true, 1, 1, 0); got != "48000000" {
		t.Fatalf("HD maximum = %q, want 48000000", got)
	}
}

func TestShouldApplyBDAVSizingAllowsVideoOnlyHEVC(t *testing.T) {
	if !shouldApplyBDAVSizing("HEVC", 0, 0) {
		t.Fatal("video-only HEVC should be sized")
	}
	if shouldApplyBDAVSizing("HEVC", 2, 1) {
		t.Fatal("HEVC with partially sized audio should not be sized")
	}
	if !shouldApplyBDAVSizing("HEVC", 2, 2) {
		t.Fatal("HEVC with fully sized audio should be sized")
	}
}

func TestNormalizeBDAVTextDurationTruncatesMilliseconds(t *testing.T) {
	const ticks = uint64(2_649_959)
	if got := normalizeBDAVTextDuration(float64(ticks)/90000, ticks); got != 29.443 {
		t.Fatalf("duration = %.3f, want 29.443", got)
	}
}

func TestOverallBitRateValueUsesIntegerMilliseconds(t *testing.T) {
	got, ok := overallBitRateValue(1000, 0.3336)
	if !ok || got != "23952" {
		t.Fatalf("overallBitRateValue() = %q, %v; want 23952, true", got, ok)
	}
	if got, ok := overallBitRateValue(1000, 0); ok || got != "" {
		t.Fatalf("overallBitRateValue(zero duration) = %q, %v; want empty, false", got, ok)
	}
}

func TestRemainingStreamSizeValueRequiresValidPayloadSum(t *testing.T) {
	got, ok := remainingStreamSizeValue(1000, 750)
	if !ok || got != "250" {
		t.Fatalf("remainingStreamSizeValue() = %q, %v; want 250, true", got, ok)
	}
	for _, sum := range []int64{0, 1001} {
		if got, ok := remainingStreamSizeValue(1000, sum); ok || got != "" {
			t.Fatalf("remainingStreamSizeValue(1000, %d) = %q, %v; want empty, false", sum, got, ok)
		}
	}
}

func TestSumCanonicalStreamSizesIgnoresLegacySnapshots(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("StreamSize", "125")
	direct := builder.Snapshot(canonicalStreamPolicy{})
	legacy := Stream{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "Stream size", Value: "250 Bytes"}},
		JSON:   map[string]string{"StreamSize": "250"},
	}
	if got := sumCanonicalStreamSizes([]Stream{direct, legacy}); got != 125 {
		t.Fatalf("sumCanonicalStreamSizes() = %d; want 125", got)
	}
}

func TestOverallBitRateModeForKindPrefersVariableAcrossAudioStreams(t *testing.T) {
	constant := newCanonicalStreamBuilder(StreamAudio)
	constant.DirectStructured("BitRate_Mode", "CBR")
	variable := newCanonicalStreamBuilder(StreamAudio)
	variable.DirectStructured("BitRate_Mode", "VBR")
	streams := []Stream{constant.Snapshot(canonicalStreamPolicy{}), variable.Snapshot(canonicalStreamPolicy{})}

	if got := overallBitRateModeForKind(streams, StreamAudio); got != "Variable" {
		t.Fatalf("overallBitRateModeForKind() = %q, want %q", got, "Variable")
	}
}

func TestOverallBitRateModeForKindRejectsSnapshotOnlyMode(t *testing.T) {
	stream := Stream{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "Bit rate mode", Value: "Variable"}},
		JSON:   map[string]string{"BitRate_Mode": "VBR"},
	}
	if got := overallBitRateModeForKind([]Stream{stream}, StreamAudio); got != "" {
		t.Fatalf("overallBitRateModeForKind() = %q; want no canonical mode", got)
	}
}

func TestOverallBitRateModeForKindUsesMigratedCanonicalOverride(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("BitRate_Mode", "Constant", "Bit rate mode", "Constant")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.Fields = []Field{{Name: "Bit rate mode", Value: "Constant"}}
	replaceCanonicalSeedProjection(&stream, "BitRate_Mode", "Variable", "VBR", "Bit rate mode", "Variable")

	if got := overallBitRateModeForKind([]Stream{stream}, StreamAudio); got != "Variable" {
		t.Fatalf("overallBitRateModeForKind() = %q, want Variable", got)
	}
}

func TestMatroskaTextBitRateModeForKindIgnoresStructuredOnlyMode(t *testing.T) {
	structuredOnly := newCanonicalStreamBuilder(StreamVideo)
	structuredOnly.Structured("BitRate_Mode", "VBR")
	textVisible := newCanonicalStreamBuilder(StreamAudio)
	textVisible.Fill("BitRate_Mode", "Variable", "Bit rate mode", "Variable")
	streams := []Stream{
		structuredOnly.Snapshot(canonicalStreamPolicy{}),
		textVisible.Snapshot(canonicalStreamPolicy{}),
	}

	if got := matroskaTextBitRateModeForKind(streams, StreamVideo); got != "" {
		t.Fatalf("structured-only text mode = %q; want empty", got)
	}
	if got := matroskaTextBitRateModeForKind(streams, StreamAudio); got != "Variable" {
		t.Fatalf("text-visible mode = %q; want Variable", got)
	}
}
