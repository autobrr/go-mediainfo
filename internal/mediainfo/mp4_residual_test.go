package mediainfo

import (
	"encoding/binary"
	"fmt"
	"math"
	"path/filepath"
	"testing"
)

func TestParseMP4TimingRetainsIntegerDurations(t *testing.T) {
	payload := make([]byte, 24)
	binary.BigEndian.PutUint32(payload[4:8], 2)
	binary.BigEndian.PutUint32(payload[8:12], 3)
	binary.BigEndian.PutUint32(payload[12:16], 1000)
	binary.BigEndian.PutUint32(payload[16:20], 2)
	binary.BigEndian.PutUint32(payload[20:24], 1001)

	count, duration, firstDelta, lastDelta, ok, variable := parseStts(payload)
	if !ok || !variable {
		t.Fatalf("parseStts ok=%v variable=%v, want true/true", ok, variable)
	}
	if count != 5 || duration != 5002 || firstDelta != 1000 || lastDelta != 1001 {
		t.Fatalf("parseStts = count %d duration %d deltas %d/%d", count, duration, firstDelta, lastDelta)
	}

	mdhd := make([]byte, 24)
	binary.BigEndian.PutUint32(mdhd[12:16], 29970)
	binary.BigEndian.PutUint32(mdhd[16:20], 23832976)
	seconds, ticks, timescale, _, ok := parseMdhdMeta(mdhd)
	if !ok || ticks != 23832976 || timescale != 29970 {
		t.Fatalf("parseMdhdMeta = %.9f, %d/%d, ok=%v", seconds, ticks, timescale, ok)
	}
}

func TestParseTkhdRetainsDurationTicks(t *testing.T) {
	v0 := make([]byte, 36)
	binary.BigEndian.PutUint32(v0[12:16], 7)
	binary.BigEndian.PutUint32(v0[20:24], 38171647)
	info, ok := parseTkhd(v0)
	if !ok || info.ID != 7 || info.DurationTicks != 38171647 {
		t.Fatalf("v0 tkhd = %+v, ok=%v", info, ok)
	}
	binary.BigEndian.PutUint32(v0[20:24], ^uint32(0))
	info, ok = parseTkhd(v0)
	if !ok || info.DurationTicks != 0 {
		t.Fatalf("indefinite v0 tkhd duration = %d, ok=%v", info.DurationTicks, ok)
	}

	v1 := make([]byte, 48)
	v1[0] = 1
	binary.BigEndian.PutUint32(v1[20:24], 9)
	binary.BigEndian.PutUint64(v1[28:36], 508831323)
	info, ok = parseTkhd(v1)
	if !ok || info.ID != 9 || info.DurationTicks != 508831323 {
		t.Fatalf("v1 tkhd = %+v, ok=%v", info, ok)
	}
}

func TestMP4PresentationAndMediaHeaderDurations(t *testing.T) {
	track := MP4Track{
		DurationSeconds:     float64(23832976) / 29970,
		Timescale:           29970,
		mediaDurationTicks:  23832976,
		sampleDurationTicks: 23833000,
		trackDurationTicks:  38171647,
		movieTimescale:      48000,
	}
	if got := formatJSONSeconds(mp4PresentationDurationSeconds(track)); got != "795.243" {
		t.Fatalf("presentation duration = %q, want 795.243", got)
	}
	if got := formatJSONSeconds(mp4SampleDurationSeconds(track)); got != "795.229" {
		t.Fatalf("sample duration = %q, want 795.229", got)
	}
	if !mp4HasDistinctSampleDuration(track) || !mp4ShouldExposeMediaHeaderDuration(track) {
		t.Fatal("expected distinct sample and media-header durations")
	}
	if got := mp4RoundedDurationMilliseconds(6377.0 + 1.0/12.0); got != 6377084 {
		t.Fatalf("float32 mdhd duration = %d, want 6377084", got)
	}
}

func TestParseElstRetainsSignedEntriesAndRejectsTruncation(t *testing.T) {
	want := []mp4EditEntry{
		{duration: 1_000, mediaTime: -1, rate: 0x00010000},
		{duration: 3_000, mediaTime: 1_024, rate: 0x00010000},
	}
	for _, version := range []byte{0, 1} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			payload := buildMP4ElstPayload(version, want)
			got := parseElst(payload)
			if len(got) != len(want) {
				t.Fatalf("entries = %d; want %d", len(got), len(want))
			}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("entry %d = %+v; want %+v", index, got[index], want[index])
				}
			}
			for cut := range len(payload) {
				if parsed := parseElst(payload[:cut]); len(parsed) != 0 {
					t.Fatalf("accepted %d-byte truncated elst: %+v", cut, parsed)
				}
			}
		})
	}
}

func TestSummarizeMP4EditListBoundaries(t *testing.T) {
	tests := []struct {
		name           string
		entries        []mp4EditEntry
		movieTimescale uint32
		wantDuration   float64
		wantMediaTime  int64
	}{
		{
			name: "positive media time",
			entries: []mp4EditEntry{
				{duration: 3_000, mediaTime: 1_024, rate: 0x00010000},
			},
			movieTimescale: 1_000,
			wantDuration:   3,
			wantMediaTime:  1_024,
		},
		{
			name: "leading empty edit",
			entries: []mp4EditEntry{
				{duration: 1_000, mediaTime: -1, rate: 0x00010000},
				{duration: 3_000, mediaTime: 0, rate: 0x00010000},
			},
			movieTimescale: 1_000,
			wantDuration:   3,
		},
		{
			name: "multiple media edits do not select a source offset",
			entries: []mp4EditEntry{
				{duration: 1_000, mediaTime: 0, rate: 0x00010000},
				{duration: 2_000, mediaTime: 2_000, rate: 0x00010000},
				{duration: 3_000, mediaTime: 4_000, rate: 0x00010000},
			},
			movieTimescale: 1_000,
			wantDuration:   6,
		},
		{
			name: "non-unit media rate unsupported",
			entries: []mp4EditEntry{
				{duration: 3_000, mediaTime: 1_024, rate: 0x00008000},
			},
			movieTimescale: 1_000,
		},
		{
			name: "zero duration",
			entries: []mp4EditEntry{
				{duration: 0, mediaTime: 1_024, rate: 0x00010000},
			},
			movieTimescale: 1_000,
		},
		{
			name: "zero timescale",
			entries: []mp4EditEntry{
				{duration: 3_000, mediaTime: 1_024, rate: 0x00010000},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			duration, mediaTime := summarizeMP4EditList(test.entries, test.movieTimescale)
			if duration != test.wantDuration || mediaTime != test.wantMediaTime {
				t.Fatalf("summary = %.9f/%d; want %.9f/%d", duration, mediaTime, test.wantDuration, test.wantMediaTime)
			}
		})
	}
}

func TestMP4EditListSourceDelayForms(t *testing.T) {
	positive := MP4Track{
		Timescale:          48_000,
		trackDurationTicks: 3_000,
		movieTimescale:     1_000,
		editList: []mp4EditEntry{
			{duration: 3_000, mediaTime: 1_024, rate: 0x00010000},
		},
	}
	if got := mp4EditSourceDelaySeconds(positive); math.Abs(got-(-float64(1_024)/48_000)) > 1e-12 {
		t.Fatalf("positive-media source delay = %.12f", got)
	}
	if got := mp4PresentationDurationSeconds(positive); got != 3 {
		t.Fatalf("positive-media presentation duration = %v; want 3", got)
	}

	empty := MP4Track{
		Timescale:          48_000,
		trackDurationTicks: 4_000,
		movieTimescale:     1_000,
		editList: []mp4EditEntry{
			{duration: 1_000, mediaTime: -1, rate: 0x00010000},
			{duration: 3_000, mediaTime: 0, rate: 0x00010000},
		},
	}
	if got := mp4EditSourceDelaySeconds(empty); got != 1 {
		t.Fatalf("empty-edit source delay = %v; want 1", got)
	}
	if got := mp4PresentationDurationSeconds(empty); got != 3 {
		t.Fatalf("empty-edit presentation duration = %v; want 3", got)
	}

	for _, track := range []MP4Track{
		{
			Timescale: 48_000, trackDurationTicks: 3_000, movieTimescale: 1_000,
			editList: []mp4EditEntry{{duration: 3_000, mediaTime: 1_024, rate: 0x00008000}},
		},
		{
			Timescale: 48_000, trackDurationTicks: 6_000, movieTimescale: 1_000,
			editList: []mp4EditEntry{
				{duration: 2_000, mediaTime: 0, rate: 0x00010000},
				{duration: 2_000, mediaTime: 2_000, rate: 0x00010000},
				{duration: 2_000, mediaTime: 4_000, rate: 0x00010000},
			},
		},
		{
			Timescale: 48_000, trackDurationTicks: 3_000, movieTimescale: 1_000,
			editList: []mp4EditEntry{{duration: 0, mediaTime: 1_024, rate: 0x00010000}},
		},
	} {
		if got := mp4EditSourceDelaySeconds(track); got != 0 {
			t.Fatalf("unsupported edit source delay = %v; want 0", got)
		}
	}
}

func TestMP4UneditedTrackKindsKeepPresentationTiming(t *testing.T) {
	for _, kind := range []StreamKind{StreamAudio, StreamVideo, StreamText, StreamMenu} {
		t.Run(string(kind), func(t *testing.T) {
			track := MP4Track{
				Kind:               kind,
				DurationSeconds:    4,
				trackDurationTicks: 4_000,
				movieTimescale:     1_000,
			}
			if got := mp4PresentationDurationSeconds(track); got != 4 {
				t.Fatalf("duration = %v; want 4", got)
			}
			if got := mp4EditSourceDelaySeconds(track); got != 0 {
				t.Fatalf("source delay = %v; want 0", got)
			}
		})
	}
}

func TestMP4PositiveAudioEditRetainsSourceTimeline(t *testing.T) {
	report, err := AnalyzeFile(filepath.Join("samples", "sample.mp4"))
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	var audio *Stream
	for index := range report.Streams {
		if report.Streams[index].Kind == StreamAudio {
			audio = &report.Streams[index]
			break
		}
	}
	if audio == nil {
		t.Fatal("audio stream not found")
	}

	media := buildJSONMedia(report)
	var audioFields []jsonKV
	for _, track := range media.Tracks {
		if jsonFieldValue(track.Fields, "@type") == string(StreamAudio) {
			audioFields = track.Fields
			break
		}
	}
	if audioFields == nil {
		t.Fatal("projected audio stream not found")
	}

	want := map[string]string{
		"Source_Duration":           "4.021",
		"Source_Duration_LastFrame": "-0.011",
		"FrameCount":                "188",
		"Source_FrameCount":         "189",
		"StreamSize":                "48301",
		"Source_StreamSize":         "48544",
	}
	for key, expected := range want {
		if got := jsonFieldValue(audioFields, key); got != expected {
			t.Errorf("%s = %q; want %q", key, got, expected)
		}
	}

	extra := canonicalSeedStructuredNode(audio, "extra")
	if extra == nil || extra.Kind != structuredObject {
		t.Fatal("audio extra object not found")
	}
	extraValues := make(map[string]string, len(extra.Object))
	for _, member := range extra.Object {
		extraValues[member.Key] = member.Value.Text
	}
	if got := extraValues["Source_Delay"]; got != "-21" {
		t.Errorf("Source_Delay = %q, want -21", got)
	}
	if got := extraValues["Source_Delay_Source"]; got != "Container" {
		t.Errorf("Source_Delay_Source = %q, want Container", got)
	}
}

func buildMP4ElstPayload(version byte, entries []mp4EditEntry) []byte {
	entrySize := 12
	if version == 1 {
		entrySize = 20
	}
	payload := make([]byte, 8+entrySize*len(entries))
	payload[0] = version
	binary.BigEndian.PutUint32(payload[4:8], uint32(len(entries)))
	offset := 8
	for _, entry := range entries {
		if version == 1 {
			binary.BigEndian.PutUint64(payload[offset:offset+8], entry.duration)
			binary.BigEndian.PutUint64(payload[offset+8:offset+16], uint64(entry.mediaTime))
			binary.BigEndian.PutUint32(payload[offset+16:offset+20], uint32(entry.rate))
			offset += 20
			continue
		}
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(entry.duration))
		binary.BigEndian.PutUint32(payload[offset+4:offset+8], uint32(entry.mediaTime))
		binary.BigEndian.PutUint32(payload[offset+8:offset+12], uint32(entry.rate))
		offset += 12
	}
	return payload
}

func TestMP4FrameRateRatioAndBitRateAccounting(t *testing.T) {
	constant := MP4Track{Timescale: 29970, SampleDelta: 1000, SampleCount: 23833, SampleBytes: 488479041}
	rate := mp4FrameRate(constant, 0)
	numerator, denominator := rationalizeMP4FrameRate(constant, rate)
	if numerator != 29970 || denominator != 1000 {
		t.Fatalf("constant ratio = %d/%d, want 29970/1000", numerator, denominator)
	}

	variable := MP4Track{
		Timescale:           16000,
		SampleCount:         39473,
		sampleDurationTicks: 21073318,
		VariableDeltas:      true,
		SampleBytes:         23188175,
	}
	rate = mp4FrameRate(variable, 0)
	numerator, denominator = rationalizeMP4FrameRate(variable, rate)
	if numerator != 30000 || denominator != 1001 {
		t.Fatalf("variable ratio = %d/%d, want 30000/1001", numerator, denominator)
	}
	if got := mp4VideoBitRate(variable, rate); got != 0 {
		t.Fatalf("variable bitrate override = %v, want 0", got)
	}

	tests := []struct {
		track MP4Track
		rate  float64
		want  int64
	}{
		{track: constant, rate: 29.970, want: 4914100},
		{track: MP4Track{SampleCount: 4030, SampleBytes: 155572479}, rate: 23.976, want: 7404478},
		{track: MP4Track{SampleCount: 153048, SampleBytes: 12762247707}, rate: 24, want: 16010347},
		{track: MP4Track{SampleCount: 72016, SampleBytes: 2694272241}, rate: 25, want: 7482427},
	}
	for _, test := range tests {
		if got := int64(math.Round(mp4VideoBitRate(test.track, test.rate))); got != test.want {
			t.Errorf("bitrate = %d, want %d", got, test.want)
		}
	}
}

func TestMP4CanonicalDisplayAspectRatioKeepsContainerValue(t *testing.T) {
	facts := &mp4StructuredFacts{}
	facts.Set("DisplayAspectRatio", "1.367")
	seed := canonicalMP4VisualSampleSeed(
		[]Field{{Name: "Display aspect ratio", Value: "4:3"}},
		facts,
		mp4VisualCanonicalFacts{sampleType: "avc1", width: 656, height: 480},
	)
	stream := Stream{Kind: StreamVideo, canonicalSeed: seed}
	if got, _ := canonicalSeedValue(stream, "DisplayAspectRatio"); got != "1.367" {
		t.Fatalf("structured DAR = %q, want 1.367", got)
	}
	if got, _ := canonicalSeedTextValue(stream, "Display aspect ratio"); got != "4:3" {
		t.Fatalf("display DAR = %q, want 4:3", got)
	}
}

func TestMP4DolbyVisionRawTextUsesFullDescriptors(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Text("Codec configuration box", "hvcC")
	track := MP4Track{
		hasDolbyVision:         true,
		hevcContainerMastering: true,
		dolbyVision: dolbyVisionConfig{
			versionMajor:    1,
			profile:         8,
			level:           6,
			rpuPresent:      true,
			blPresent:       true,
			compatibilityID: 6,
		},
	}
	applyMP4HEVCProbe(builder, hevcHDRInfo{hasMastering: true}, track)
	stream := builder.Snapshot(canonicalStreamPolicy{})
	wantHDR := formatDolbyVisionHDR(track.dolbyVision) +
		" / SMPTE ST 2086, Version HDR10, HDR10 compatible" +
		" / SMPTE ST 2086, Version HDR10, HDR10 compatible"
	if got, _ := canonicalSeedTextValue(stream, "HDR format"); got != wantHDR {
		t.Fatalf("HDR display = %q, want %q", got, wantHDR)
	}
	if got, _ := canonicalSeedTextValue(stream, "Codec configuration box"); got != "hvcC+dvvC" {
		t.Fatalf("configuration display = %q, want hvcC+dvvC", got)
	}
}

func TestRawTextMP4AspectRatioClassification(t *testing.T) {
	if got := rawTextValue(StreamVideo, "DisplayAspectRatio/String", "1.328", map[string]string{"CodecID": "vp09", "DisplayAspectRatio": "1.328"}, 0); got != "4:3" {
		t.Fatalf("near-4:3 display = %q, want 4:3", got)
	}
	if got := rawTextValue(StreamVideo, "DisplayAspectRatio/String", "1.85:1", map[string]string{"CodecID": "hvc1", "DisplayAspectRatio": "1.850"}, 0); got != "1.85:1" {
		t.Fatalf("cinema display = %q, want 1.85:1", got)
	}
}
