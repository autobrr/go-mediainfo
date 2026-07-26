package mediainfo

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestTrueHDChannelsEveryAssignmentBit(t *testing.T) {
	wantWeights := [...]int{2, 1, 1, 2, 2, 2, 2, 1, 1, 2, 2, 1, 1}
	if trueHDChannelCountPerBit != wantWeights {
		t.Fatalf("trueHDChannelCountPerBit=%v; want %v", trueHDChannelCountPerBit, wantWeights)
	}
	tests := []struct {
		channelMap uint16
		channels   uint64
		layout     string
	}{
		{channelMap: 1 << 0, channels: 2, layout: "L R"},
		{channelMap: 1 << 1, channels: 1, layout: "C"},
		{channelMap: 1 << 2, channels: 1, layout: "LFE"},
		{channelMap: 1 << 3, channels: 2, layout: "Ls Rs"},
		{channelMap: 1 << 4, channels: 2, layout: "Tfl Tfr"},
		{channelMap: 1 << 5, channels: 2, layout: "Lsc Rsc"},
		{channelMap: 1 << 6, channels: 2, layout: "Lb Rb"},
		{channelMap: 1 << 7, channels: 1, layout: "Cb"},
		{channelMap: 1 << 8, channels: 1, layout: "Tc"},
		{channelMap: 1 << 9, channels: 2, layout: "Lsd Rsd"},
		{channelMap: 1 << 10, channels: 2, layout: "Lw Rw"},
		{channelMap: 1 << 11, channels: 1, layout: "Tfc"},
		{channelMap: 1 << 12, channels: 1, layout: "LFE2"},
	}
	for _, test := range tests {
		t.Run(test.layout, func(t *testing.T) {
			if got := trueHDChannels(test.channelMap); got != test.channels {
				t.Errorf("trueHDChannels(%#x) = %d; want %d", test.channelMap, got, test.channels)
			}
			if got := trueHDChannelLayout(test.channelMap); got != test.layout {
				t.Errorf("trueHDChannelLayout(%#x) = %q; want %q", test.channelMap, got, test.layout)
			}
		})
	}
}

func TestTrueHDChannelsRepresentativePresentations(t *testing.T) {
	tests := []struct {
		name       string
		channelMap uint16
		channels   uint64
	}{
		{name: "mono", channelMap: 0x0002, channels: 1},
		{name: "stereo", channelMap: 0x0001, channels: 2},
		{name: "5.1", channelMap: 0x000F, channels: 6},
		{name: "7.1", channelMap: 0x004F, channels: 8},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := trueHDChannels(test.channelMap); got != test.channels {
				t.Fatalf("trueHDChannels(%#x) = %d; want %d", test.channelMap, got, test.channels)
			}
		})
	}
}

func TestMatroskaTrueHDTrackUIDThreeKeepsOwnFacts(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(6))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDAudioBitDepth, encodeMatroskaUint(20))...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(3))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_TRUEHD"))...)
	payload = append(payload, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1_234_567))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("TrueHD TrackEntry did not parse")
	}
	streams := []Stream{stream}
	general := Stream{Kind: StreamGeneral}
	applyMatroskaWriterRules("", &general, streams)

	for key, want := range map[fieldName]string{
		"UniqueID": "3",
		"BitRate":  "1234567",
		"BitDepth": "20",
	} {
		got, found := canonicalSeedValue(streams[0], key)
		if !found || got != want {
			t.Fatalf("%s = %q, %v; want %q from this track", key, got, found, want)
		}
	}
}

func TestParseTrueHDFrameAtmosMajorSync(t *testing.T) {
	frame := []byte{
		0x61, 0xC8, 0xFF, 0xD5,
		0xF8, 0x72, 0x6F, 0xBA, 0x00, 0x17, 0x80, 0x4F,
		0xB7, 0x52, 0x10, 0x00, 0x00, 0x00, 0x8A, 0xAD,
		0x43, 0xFC, 0x00, 0x00, 0x7E, 0xEF, 0xE3, 0x07,
		0xE3, 0x01, 0x1F, 0xC6, 0xBC, 0x00, 0x5C, 0xBD,
	}

	info, ok := parseTrueHDFrame(frame)
	if !ok {
		t.Fatal("parseTrueHDFrame returned ok=false")
	}
	if !info.atmos {
		t.Fatal("expected TrueHD Atmos presentation")
	}
	if info.sampleRate != 48000 {
		t.Fatalf("sampleRate=%d want 48000", info.sampleRate)
	}
	if info.samplesPerFrame != 40 {
		t.Fatalf("samplesPerFrame=%d want 40", info.samplesPerFrame)
	}
	if info.maxBitRate != 8199000 {
		t.Fatalf("maxBitRate=%d want 8199000", info.maxBitRate)
	}
	if info.channelMap != 0x4F {
		t.Fatalf("channelMap=%#x want 0x4f", info.channelMap)
	}
	if got := trueHDChannels(info.channelMap); got != 8 {
		t.Fatalf("channels=%d want 8", got)
	}
	if got := trueHDChannelPositions(info.channelMap); got != "Front: L C R, Side: L R, Back: L R, LFE" {
		t.Fatalf("positions=%q", got)
	}
	if got := trueHDChannelLayout(info.channelMap); got != "L R C LFE Ls Rs Lb Rb" {
		t.Fatalf("layout=%q", got)
	}
	if info.dynamicObjects != 11 {
		t.Fatalf("dynamicObjects=%d want 11", info.dynamicObjects)
	}
	if !info.hasDynamicObjects {
		t.Fatal("parsed Atmos object count is not marked present")
	}
	atmos, ok := trueHDAtmosPresentationInfo(info)
	if !ok {
		t.Fatal("expected Atmos presentation details")
	}
	if atmos.additionalFeatures != "16-ch" {
		t.Fatalf("additionalFeatures=%q want 16-ch", atmos.additionalFeatures)
	}
	if atmos.dynamicObjects != 11 {
		t.Fatalf("dynamicObjects=%d want 11", atmos.dynamicObjects)
	}
	if atmos.bedChannelCount != 1 || atmos.bedChannelConfig != "LFE" {
		t.Fatalf("bed=%d/%q want 1/LFE", atmos.bedChannelCount, atmos.bedChannelConfig)
	}
}

func TestParseTrueHDProgramAssignmentBedAndObjects(t *testing.T) {
	data := make([]byte, 8)
	bw := ac3BitWriter{buf: data}
	bw.writeBits(0, 1)     // not dynamic-object-only
	bw.writeBits(0x5, 4)   // bed + dynamic objects
	bw.writeBits(0, 1)     // b_bed_object_chan_distribute
	bw.writeBits(0, 1)     // one bed instance
	bw.writeBits(0, 1)     // not LFE-only
	bw.writeBits(0, 1)     // nonstandard channel assignment
	bw.writeBits(0x3F, 17) // L R C LFE Ls Rs
	bw.writeBits(3, 5)     // four dynamic objects

	br := ac3BitReader{data: data}
	info := trueHDInfo{dynamicObjects: 16, hasDynamicObjects: true}
	if !parseTrueHDProgramAssignment(&br, &info) {
		t.Fatal("program_assignment parse failed")
	}
	if info.dynamicObjects != 4 || !info.hasDynamicObjects {
		t.Fatalf("dynamic objects = %d, present=%v", info.dynamicObjects, info.hasDynamicObjects)
	}
	if info.atmosBedChannels != 6 || info.atmosBedLayout != "L R C LFE Ls Rs" {
		t.Fatalf("bed = %d/%q", info.atmosBedChannels, info.atmosBedLayout)
	}
}

func TestParseTrueHDFrameWithoutAtmosFlag(t *testing.T) {
	frame := []byte{
		0xF8, 0x72, 0x6F, 0xBA, 0x00, 0x17, 0x80, 0x4F,
		0xB7, 0x52, 0x10, 0x00, 0x00, 0x00, 0x8A, 0xAD,
		0x43, 0x7C, 0x00, 0x00, 0x7E, 0xEF, 0xE3, 0x07,
		0xE3, 0x01, 0x1F, 0xC6, 0xBC, 0x00, 0x5C, 0xBD,
	}

	info, ok := parseTrueHDFrame(frame)
	if !ok {
		t.Fatal("parseTrueHDFrame returned ok=false")
	}
	if info.atmos {
		t.Fatal("did not expect Atmos when substream_info high bit is clear")
	}
	if _, ok := trueHDAtmosPresentationInfo(info); ok {
		t.Fatal("did not expect Atmos presentation details")
	}
}

func TestParseTrueHDFrame96KHzWithoutAtmos(t *testing.T) {
	frame := []byte{
		0x22, 0x16, 0xFF, 0xB0,
		0xF8, 0x72, 0x6F, 0xBA, 0x10, 0x17, 0x80, 0x4F,
		0xB7, 0x52, 0x00, 0x00, 0x00, 0x00, 0x87, 0x50,
		0x30, 0x7C, 0x00, 0x00, 0x4A, 0xEF, 0xE3, 0x07,
		0xE3, 0x00, 0xE5, 0xED,
	}

	info, ok := parseTrueHDFrame(frame)
	if !ok {
		t.Fatal("parseTrueHDFrame returned ok=false")
	}
	if info.atmos {
		t.Fatal("did not expect Atmos")
	}
	if info.sampleRate != 96000 || info.samplesPerFrame != 80 {
		t.Fatalf("rate=%d/%d want 96000/80", info.sampleRate, info.samplesPerFrame)
	}
}

func TestApplyMatroskaAudioProbes_TrueHDAtmosWithoutExtensionOmitsObjectCounts(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "ID", Value: "2"}},
		canonicalSeed: matroskaTrueHDCanonicalSeed(matroskaTrueHDCanonicalFacts{
			trackNumber: 2, audioChannels: 8, audioSampleRate: 48_000, defaultValue: true,
		}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "TrueHD",
			truehd: trueHDInfo{
				atmos:           true,
				sampleRate:      48000,
				samplesPerFrame: 40,
				maxBitRate:      8199000,
			},
			ok: true,
		},
	}

	applyMatroskaAudioProbes(info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Format"); got != "MLP FBA 16-ch" {
		t.Fatalf("Format=%q want MLP FBA 16-ch", got)
	}
	if got := findField(stream.Fields, "Channel layout"); got != "L R C LFE Ls Rs Lb Rb" {
		t.Fatalf("Channel layout=%q want standard 7.1", got)
	}
	if got := stream.JSON["ChannelLayout"]; got != "L R C LFE Ls Rs Lb Rb" {
		t.Fatalf("JSON ChannelLayout=%q want standard 7.1", got)
	}
	if got := stream.JSON["Format_AdditionalFeatures"]; got != "16-ch" {
		t.Fatalf("Format_AdditionalFeatures=%q want 16-ch", got)
	}
	if got := stream.JSONRaw["extra"]; got != "" {
		t.Fatalf("fabricated Atmos object metadata: %s", got)
	}
	for _, field := range []string{"Number of dynamic objects", "Bed channel count", "Bed channel configuration"} {
		if got := findField(stream.Fields, field); got != "" {
			t.Fatalf("%s=%q without parsed extension", field, got)
		}
	}
}

func TestApplyMatroskaAudioProbes_TrueHDNonAtmosKeepsMatroskaLayout(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "ID", Value: "2"}},
		canonicalSeed: matroskaTrueHDCanonicalSeed(matroskaTrueHDCanonicalFacts{
			trackNumber: 2, audioChannels: 6, audioSampleRate: 44_100,
			segmentDuration: 10, durationPrec: 9,
		}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "TrueHD",
			truehd: trueHDInfo{
				sampleRate:      44100,
				samplesPerFrame: 40,
				maxBitRate:      8199000,
			},
			ok: true,
		},
	}

	applyMatroskaAudioProbes(info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Channel layout"); got != "L R C LFE Ls Rs" {
		t.Fatalf("Channel layout=%q want Matroska layout", got)
	}
	if got := stream.JSON["ChannelLayout"]; got != "L R C LFE Ls Rs" {
		t.Fatalf("JSON ChannelLayout=%q want Matroska layout", got)
	}
	if got := stream.JSON["ChannelPositions"]; got != "Front: L C R, Side: L R, LFE" {
		t.Fatalf("JSON ChannelPositions=%q want Matroska positions", got)
	}
	if got := findField(stream.Fields, "Maximum bit rate"); got != "8 199 kb/s" {
		t.Fatalf("Maximum bit rate=%q want 8 199 kb/s", got)
	}
	if got := findField(stream.Fields, "Frame rate"); got != "1102.500 FPS (40 SPF)" {
		t.Fatalf("Frame rate=%q want 1102.500 FPS (40 SPF)", got)
	}
	if got := stream.JSON["FrameRate"]; got != "1102.500" {
		t.Fatalf("FrameRate=%q want 1102.500", got)
	}
	if got := stream.JSON["FrameRate_Num"]; got != "44100" {
		t.Fatalf("FrameRate_Num=%q want 44100", got)
	}
	if got := stream.JSON["FrameRate_Den"]; got != "40" {
		t.Fatalf("FrameRate_Den=%q want 40", got)
	}
}

func TestApplyMatroskaAudioProbes_TrueHDNonAtmosEightChannelLayout(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "ID", Value: "2"}},
		canonicalSeed: matroskaTrueHDCanonicalSeed(matroskaTrueHDCanonicalFacts{
			trackNumber: 2, audioChannels: 8, audioSampleRate: 96_000,
			segmentDuration: 10, durationPrec: 9,
		}),
	}}}
	probes := map[uint64]*matroskaAudioProbe{
		2: {
			format: "TrueHD",
			truehd: trueHDInfo{sampleRate: 96000, samplesPerFrame: 80},
			ok:     true,
		},
	}

	applyMatroskaAudioProbes(info, probes)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	stream := info.Tracks[0]

	if got := findField(stream.Fields, "Format"); got != "MLP FBA" {
		t.Fatalf("Format=%q want MLP FBA", got)
	}
	if got := findField(stream.Fields, "Commercial name"); got != "Dolby TrueHD" {
		t.Fatalf("Commercial name=%q want Dolby TrueHD", got)
	}
	if got := stream.JSON["ChannelLayout"]; got != "L R C LFE Ls Rs Lb Rb" {
		t.Fatalf("ChannelLayout=%q want standard 7.1", got)
	}
	if got := stream.JSON["ChannelPositions"]; got != "Front: L C R, Side: L R, Back: L R, LFE" {
		t.Fatalf("ChannelPositions=%q want standard 7.1 positions", got)
	}
}

func TestApplyMatroskaTagStatsPreservesTrueHDFrameCount(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "2"},
			{Name: "Format", Value: "TrueHD"},
		},
		JSON: map[string]string{"UniqueID": "17"},
	}}}
	tags := map[uint64]matroskaTagStats{
		17: {
			trusted:       true,
			hasFrameCount: true,
			frameCount:    8856498,
		},
	}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(info, tags, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["FrameCount"]; got != "8856498" {
		t.Fatalf("FrameCount=%q want trusted Statistics Tags value", got)
	}
}

func TestApplyMatroskaTagStatsPreservesTrueHDBitRate(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "3"},
			{Name: "Format", Value: "MLP FBA"},
		},
		JSON: map[string]string{
			"UniqueID":   "3",
			"Duration":   "248.948708333",
			"StreamSize": "78964338",
		},
	}}}
	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(info, map[uint64]matroskaTagStats{3: {
		trusted: true, hasBitRate: true, bitRate: 2537536,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if got := info.Tracks[0].JSON["BitRate"]; got != "2537536" {
		t.Fatalf("BitRate=%q want trusted TrueHD Statistics Tags value", got)
	}
}
