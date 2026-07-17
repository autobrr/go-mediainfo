package mediainfo

import (
	"math"
	"testing"
)

func TestEstimateOggVideoBitRate(t *testing.T) {
	const (
		fileSize   = int64(107335712)
		frameCount = 40522
		frameRate  = 30
	)
	duration := 59567421.0 / 44100
	bitRate, ok := estimateOggVideoBitRate(fileSize, duration, []int64{128000})
	if !ok {
		t.Fatal("estimateOggVideoBitRate() = not available")
	}
	if got := int64(math.Round(bitRate)); got != 473683 {
		t.Fatalf("bit rate = %d, want 473683", got)
	}
	streamSize := int64(math.Round(bitRate / 8 * frameCount / frameRate))
	if streamSize != 79977407 {
		t.Fatalf("stream size = %d, want 79977407", streamSize)
	}
}

func TestOggTheoraFrameCount(t *testing.T) {
	const (
		shift      = 6
		frameCount = 40522
	)
	granule := uint64(40500<<shift | 22)
	if got := oggTheoraFrameCount(granule, shift); got != frameCount {
		t.Fatalf("oggTheoraFrameCount() = %d, want %d", got, frameCount)
	}
}

func TestCanonicalOggVideoOmitsSyntheticSampledDimensions(t *testing.T) {
	stream := canonicalOggVideoStream(&oggLogicalStream{
		serial:       1,
		width:        400,
		height:       300,
		frameRateNum: 30,
		frameRateDen: 1,
	}, 1, 30)
	if !stream.JSONSkipComputed {
		t.Fatal("canonical Ogg video did not retain the skip-computed policy")
	}
	for _, name := range []fieldName{"Sampled_Width", "Sampled_Height"} {
		if _, found := canonicalSeedValue(stream, name); found {
			t.Fatalf("canonical Ogg video contains synthetic %s", name)
		}
	}
}
