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

func TestApplyMatroskaAudioProbes_TrueHDAtmosKeepsSevenOneLayout(t *testing.T) {
	info := &MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "ID", Value: "2"},
			{Name: "Format", Value: "TrueHD"},
			{Name: "Format/Info", Value: "Dolby TrueHD"},
			{Name: "Channel(s)", Value: "8 channels"},
			{Name: "Default", Value: "Yes"},
		},
		JSON: map[string]string{"Channels": "8"},
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
