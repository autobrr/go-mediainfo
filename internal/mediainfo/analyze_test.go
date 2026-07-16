package mediainfo

import "testing"

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
	replaceCanonicalSeedLegacyProjection(&stream, "BitRate_Mode", "Variable", "VBR", "Bit rate mode", "Variable")

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
