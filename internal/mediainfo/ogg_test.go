package mediainfo

import (
	"bytes"
	"encoding/binary"
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

func TestParseOggBoundsLogicalSerialState(t *testing.T) {
	exact := buildOggSerialFixture(maxOggLogicalStreams)
	if _, streams, _, _, _, ok := parseOgg(bytes.NewReader(exact), int64(len(exact))); !ok || len(streams) != 1 {
		t.Fatalf("exact logical-stream limit = ok %v, streams %d; want one recognized stream", ok, len(streams))
	}

	over := buildOggSerialFixture(maxOggLogicalStreams + 1)
	if _, streams, _, _, _, ok := parseOgg(bytes.NewReader(over), int64(len(over))); !ok || len(streams) != 1 {
		t.Fatalf("limit+1 parse = ok %v, streams %d; want retained recognized stream", ok, len(streams))
	}
}

func TestParseOggRepeatedSerialsDoNotConsumeSlots(t *testing.T) {
	data := buildOggPageForTest(7, 0x02, 48_000, vorbisIdentificationPacketForTest())
	for page := range maxOggLogicalStreams + 32 {
		headerType := byte(0)
		if page == maxOggLogicalStreams+31 {
			headerType = 0x04
		}
		data = append(data, buildOggPageForTest(7, headerType, uint64(48_000+page), nil)...)
	}
	// Reuse after EOS is still the same serial identity and must not allocate a
	// second logical-stream slot.
	data = append(data, buildOggPageForTest(7, 0x02, 96_000, nil)...)

	if _, streams, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data))); !ok || len(streams) != 1 {
		t.Fatalf("repeated serial parse = ok %v, streams %d", ok, len(streams))
	}
}

func TestParseOggMalformedPageNearLogicalStreamLimit(t *testing.T) {
	data := buildOggSerialFixture(maxOggLogicalStreams)
	truncated := make([]byte, 27)
	copy(truncated, "OggS")
	truncated[26] = 1
	data = append(data, truncated...)
	if _, _, _, _, _, ok := parseOgg(bytes.NewReader(data), int64(len(data))); ok {
		t.Fatal("truncated page near logical-stream limit was accepted")
	}
}

func TestParseOggLogicalStreamLimitHasBoundedAllocations(t *testing.T) {
	data := buildOggSerialFixture(maxOggLogicalStreams + 1)
	allocations := testing.AllocsPerRun(5, func() {
		_, _, _, _, _, _ = parseOgg(bytes.NewReader(data), int64(len(data)))
	})
	const allocationsPerSerialBudget = 12
	if maximum := float64(maxOggLogicalStreams*allocationsPerSerialBudget + 256); allocations > maximum {
		t.Fatalf("logical-stream limit allocations = %.0f, want <= %.0f", allocations, maximum)
	}
}

func BenchmarkParseOggLogicalStreamLimit(b *testing.B) {
	data := buildOggSerialFixture(maxOggLogicalStreams)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for range b.N {
		_, _, _, _, _, _ = parseOgg(bytes.NewReader(data), int64(len(data)))
	}
}

func buildOggSerialFixture(count int) []byte {
	data := buildOggPageForTest(1, 0x02, 48_000, vorbisIdentificationPacketForTest())
	for serial := uint32(2); serial <= uint32(count); serial++ {
		data = append(data, buildOggPageForTest(serial, 0x02, 0, nil)...)
	}
	return data
}

func buildOggPageForTest(serial uint32, headerType byte, granule uint64, body []byte) []byte {
	if len(body) > 255 {
		panic("single-segment Ogg test body is too large")
	}
	segmentCount := byte(0)
	if len(body) > 0 {
		segmentCount = 1
	}
	page := make([]byte, 27+int(segmentCount), 27+int(segmentCount)+len(body))
	copy(page, "OggS")
	page[5] = headerType
	binary.LittleEndian.PutUint64(page[6:14], granule)
	binary.LittleEndian.PutUint32(page[14:18], serial)
	page[26] = segmentCount
	if segmentCount != 0 {
		page[27] = byte(len(body))
	}
	return append(page, body...)
}

func vorbisIdentificationPacketForTest() []byte {
	packet := make([]byte, 30)
	packet[0] = 0x01
	copy(packet[1:7], "vorbis")
	packet[11] = 2
	binary.LittleEndian.PutUint32(packet[12:16], 48_000)
	binary.LittleEndian.PutUint32(packet[20:24], 128_000)
	return packet
}
