package mediainfo

import "testing"

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
	if info.dynamicObjects != 11 {
		t.Fatalf("dynamicObjects=%d want 11", info.dynamicObjects)
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

func TestApplyMatroskaAudioProbes_TrueHDAtmosKeepsSevenOneLayout(t *testing.T) {
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
	refreshCanonicalLegacySnapshot(&info.Tracks[0])
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
	if got := stream.JSONRaw["extra"]; got != `{"NumberOfDynamicObjects":"11","BedChannelCount":"1","BedChannelConfiguration":"LFE"}` {
		t.Fatalf("extra=%s", got)
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
	refreshCanonicalLegacySnapshot(&info.Tracks[0])
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
	refreshCanonicalLegacySnapshot(&info.Tracks[0])
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
	refreshCanonicalLegacySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["FrameCount"]; got != "8856498" {
		t.Fatalf("FrameCount=%q want trusted Statistics Tags value", got)
	}
}
