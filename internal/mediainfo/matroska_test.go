package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMatroskaAVCQuickProbePacketBudget(t *testing.T) {
	if matroskaAVCQuickProbePackets != 300 {
		t.Fatalf("matroskaAVCQuickProbePackets=%d; want MediaInfo-bounded 300-packet search", matroskaAVCQuickProbePackets)
	}
}

func TestParseMatroskaTracks(t *testing.T) {
	buf := buildMatroskaSample()
	info, ok := parseMatroska(buf)
	if !ok {
		t.Fatalf("expected matroska info")
	}
	if !info.Container.HasDuration() {
		t.Fatalf("expected duration")
	}
	if len(info.Tracks) != 1 {
		t.Fatalf("expected 1 track")
	}
	if info.Tracks[0].Kind != StreamVideo {
		t.Fatalf("expected video track")
	}
	if findField(info.Tracks[0].Fields, "Width") == "" {
		t.Fatalf("missing width")
	}
	if findField(info.Tracks[0].Fields, "Frame rate") == "" {
		t.Fatalf("missing frame rate")
	}
	if findField(info.Tracks[0].Fields, "Bit rate") == "" {
		t.Fatalf("missing bit rate")
	}
}

func TestParseMatroskaStatsTags(t *testing.T) {
	tagsPayload := buildMatroskaTagForStats(123)
	encoders, settings, langs, stats, _, _ := parseMatroskaTags(tagsPayload, "", "")
	if got := encoders[123]; got != "Lavf60.3.100" {
		t.Fatalf("unexpected encoder: %q", got)
	}
	if got := settings[123]; got != "" {
		t.Fatalf("unexpected settings: %q", got)
	}
	if got := langs[123]; got != "" {
		t.Fatalf("unexpected language: %q", got)
	}
	entry := stats[123]
	if !entry.trusted {
		t.Fatalf("expected trusted stats")
	}
	if !entry.hasDataBytes || entry.dataBytes != 1048576 {
		t.Fatalf("unexpected bytes: %+v", entry)
	}
	if !entry.hasFrameCount || entry.frameCount != 1200 {
		t.Fatalf("unexpected frame count: %+v", entry)
	}
	if !entry.hasDuration || entry.durationSeconds != 50 {
		t.Fatalf("unexpected duration: %+v", entry)
	}
	if !entry.hasBitRate || entry.bitRate != 166000 {
		t.Fatalf("unexpected bitrate: %+v", entry)
	}
}

func TestParseMatroskaGeneralTags(t *testing.T) {
	body := append(buildMatroskaSimpleTag("IMDB", " tt32612507 "), buildMatroskaSimpleTag("TMDB", "movie/1304313")...)
	body = append(body, buildMatroskaSimpleTag("TVDB2", "movies/19799")...)
	targets := buildMatroskaElement(mkvIDTagTargets, nil)
	tagsPayload := buildMatroskaElement(mkvIDTag, append(targets, body...))

	_, _, _, _, generalTags, _ := parseMatroskaTags(tagsPayload, "", "")
	if got := generalTags["IMDB"]; got != "tt32612507" {
		t.Fatalf("IMDB = %q, want tt32612507", got)
	}
	if got := generalTags["TMDB"]; got != "movie/1304313" {
		t.Fatalf("TMDB = %q, want movie/1304313", got)
	}
	if got := generalTags["TVDB2"]; got != "movies/19799" {
		t.Fatalf("TVDB2 = %q, want movies/19799", got)
	}
}

func TestParseMatroskaTrackSourceTags(t *testing.T) {
	body := append(buildMatroskaSimpleTag("SOURCE", "UHD Blu-ray"), buildMatroskaSimpleTag("SOURCE_ID", "001011")...)
	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(123)))
	tagsPayload := buildMatroskaElement(mkvIDTag, append(targets, body...))

	_, _, _, stats, _, _ := parseMatroskaTags(tagsPayload, "", "")
	got := stats[123]
	if len(got.extras) != 1 || got.extras[0] != (jsonKV{Key: "SOURCE", Val: "UHD Blu-ray"}) {
		t.Fatalf("extras = %#v", got.extras)
	}
	if !got.hasSourceID || got.sourceID != 0x1011 {
		t.Fatalf("Source ID = %d, present=%v", got.sourceID, got.hasSourceID)
	}
}

func TestApplyMatroskaTrackSourceTags(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Structured("UniqueID", "123")
	info := MatroskaInfo{Tracks: []Stream{builder.Snapshot(canonicalStreamPolicy{})}}

	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		source: "UHD Blu-ray", hasSource: true,
		sourceID: 0x1011, hasSourceID: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	stream := info.Tracks[0]
	if got := stream.JSON["OriginalSourceMedium_ID"]; got != "4113" {
		t.Fatalf("OriginalSourceMedium_ID = %q", got)
	}
	if got := stream.JSONRaw["extra"]; got != `{"OriginalSourceMedium":"Blu-ray","Source":"UHD Blu-ray"}` {
		t.Fatalf("extra = %s", got)
	}
}

func TestParseMatroskaChapterDisplayPrefersIETF(t *testing.T) {
	payload := append(buildMatroskaElement(mkvIDChapString, []byte("Chapter 01")), buildMatroskaElement(mkvIDChapLanguage, []byte("eng"))...)
	payload = append(payload, buildMatroskaElement(mkvIDChapLanguageIETF, []byte("en-US"))...)

	name, lang := parseMatroskaChapterDisplay(payload)
	if name != "Chapter 01" || lang != "en-US" {
		t.Fatalf("display = %q, %q", name, lang)
	}
}

func TestParseMatroskaTrackLanguageIETFUndDoesNotFallback(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, []byte{0x01})
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{0x11})...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("S_TEXT/UTF8"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguage, []byte("eng"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguageIETF, []byte("und"))...)

	stream, ok := parseMatroskaTrackEntry(payload, 0, 0)
	if !ok {
		t.Fatal("expected valid Matroska text track")
	}
	if got := findField(stream.Fields, "Language"); got != "" {
		t.Fatalf("display Language = %q, want omitted", got)
	}
	if got := stream.JSON["Language"]; got != "" {
		t.Fatalf("structured Language = %q, want omitted", got)
	}
	if got, found := canonicalSeedValue(stream, "Language"); found || got != "" {
		t.Fatalf("canonical Language = %q, found=%v; want omitted", got, found)
	}
}

func TestParseMatroskaTrackAccessibilityFlags(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, []byte{0x01})
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{0x11})...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("S_TEXT/UTF8"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagHearingImpaired, []byte{0x01})...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagOriginal, []byte{0x01})...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagCommentary, []byte{0x01})...)

	stream, ok := parseMatroskaTrackEntry(payload, 0, 0)
	if !ok {
		t.Fatal("expected valid Matroska text track")
	}
	if got := stream.JSON["ServiceKind"]; got != "HI / O / C" {
		t.Fatalf("ServiceKind = %q, want %q", got, "HI / O / C")
	}
}

func TestParseMatroskaFLACCodecPrivate(t *testing.T) {
	codecPrivate := []byte{
		0x10, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x00, 0x21, 0x0C, 0x0B, 0xB8, 0x02, 0xF0, 0x0E, 0x5B,
		0x65, 0x40, 0x86, 0x4D, 0x55, 0xF0, 0x03, 0x14, 0x3D, 0x8B, 0xAD, 0x47, 0xD3, 0xB9, 0x97, 0xFA,
		0xE6, 0x4C,
	}
	payload := buildMatroskaElement(mkvIDTrackNumber, []byte{0x01})
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, []byte{0x02})...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_FLAC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)

	stream, ok := parseMatroskaTrackEntry(payload, 124.501, 3)
	if !ok {
		t.Fatal("expected valid Matroska FLAC track")
	}
	for key, want := range map[string]string{
		"BitDepth":          "16",
		"BitDepth_Detected": "16",
		"BitRate_Mode":      "VBR",
		"SamplesPerFrame":   "4096",
		"SamplingCount":     "5976048",
		"FrameCount":        "1459",
		"FrameRate":         "11.719",
	} {
		if got := stream.JSON[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := stream.JSONRaw["extra"]; got != `{"MD5_Unencoded":"864D55F003143D8BAD47D3B997FAE64C"}` {
		t.Fatalf("extra = %q", got)
	}
}

func TestMatroskaFLACDetectedBitDepthUsesDeclaredStreamInfo(t *testing.T) {
	for _, digest := range []string{
		"00112233445566778899AABBCCDDEEFF",
		"BAB396FCA9481C0BF8CB5717065C8FF8",
	} {
		info := flacStreamInfo{bitsPerSample: 24, md5: digest}
		if got := matroskaFLACDetectedBitDepth(info); got != "24" {
			t.Fatalf("digest %q detected bit depth = %q, want declared 24", digest, got)
		}
	}
}

func TestMatroskaFLACDetectedBitDepthUsesVorbisValidBits(t *testing.T) {
	info := flacStreamInfo{bitsPerSample: 24, detectedBits: 21}
	if got := matroskaFLACDetectedBitDepth(info); got != "21" {
		t.Fatalf("detected bit depth = %q, want 21", got)
	}
}

func TestApplyMatroskaEncodersAddsFLACLibraryComponents(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "FLAC", "Format", "FLAC")
	builder.Structured("UniqueID", "42")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.Fields = []Field{{Name: "Format", Value: "FLAC"}}
	streams := []Stream{stream}

	applyMatroskaEncoders(streams, map[uint64]string{42: "reference libFLAC 1.3.4 20220220"}, nil)
	refreshCanonicalCompatibilitySnapshot(&streams[0])

	for key, want := range map[string]string{
		"Encoded_Library":         "reference libFLAC 1.3.4 20220220",
		"Encoded_Library_Name":    "libFLAC",
		"Encoded_Library_Version": "1.3.4",
		"Encoded_Library_Date":    "2022-02-20",
	} {
		if got := streams[0].JSON[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestApplyMatroskaEncodersNormalizesExistingLavcFLAC(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "FLAC", "Format", "FLAC")
	builder.Fill("Channels", "2", "Channel(s)", "2 channels")
	builder.Fill("Encoded_Library", "Lavc58.91.100 flac", "Writing library", "Lavc58.91.100 flac")
	builder.Structured("BitDepth_Detected", "16")
	builder.Structured("UniqueID", "42")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.Fields = []Field{
		{Name: "Format", Value: "FLAC"},
		{Name: "Channel(s)", Value: "2 channels"},
		{Name: "Writing library", Value: "Lavc58.91.100 flac"},
	}
	streams := []Stream{stream}

	normalizeMatroskaLavcFLAC(&streams[0], "Lavc58.91.100 flac")
	if got := matroskaStreamScalar(streams[0], "BitDepth_Detected"); got != "" {
		t.Fatalf("BitDepth_Detected = %q; want omitted", got)
	}
	if got := matroskaStreamScalar(streams[0], "ChannelLayout"); got != "L R" {
		t.Fatalf("ChannelLayout = %q; want L R", got)
	}
	if got := matroskaStreamScalar(streams[0], "ChannelPositions"); got != "Front: L R" {
		t.Fatalf("ChannelPositions = %q; want Front: L R", got)
	}
}

func TestMatroskaPostNormalizationRefreshesAACCompatibilitySnapshot(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "AAC", "Format", "AAC")
	builder.Structured("Format_Settings_PS", "No (Explicit)")
	builder.Structured("BitRate", "128000")
	info := MatroskaInfo{Tracks: []Stream{builder.Snapshot(canonicalStreamPolicy{})}}

	finalizeMatroskaDeferredFacts(&info)
	normalizeMatroskaAACExplicitPS(info.Tracks)
	refreshMatroskaCompatibilitySnapshots(&info)

	if _, exists := info.Tracks[0].JSON["Format_Settings_PS"]; exists {
		t.Fatalf("stale Format_Settings_PS survived canonical cleanup: %#v", info.Tracks[0].JSON)
	}
	if got := info.Tracks[0].JSON["BitRate"]; got != "128000" {
		t.Fatalf("BitRate = %q, want 128000", got)
	}
}

func TestMatroskaPostNormalizationRefreshesFLACCompatibilitySnapshot(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "FLAC", "Format", "FLAC")
	builder.Fill("Channels", "2", "Channel(s)", "2 channels")
	builder.Fill("Encoded_Library", "Lavc58.91.100 flac", "Writing library", "Lavc58.91.100 flac")
	builder.Structured("BitDepth_Detected", "16")
	builder.Structured("FrameCount", "10")
	builder.Structured("FrameRate", "10.000")
	builder.Structured("SamplesPerFrame", "4096")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	info := MatroskaInfo{Tracks: []Stream{stream}}

	finalizeMatroskaDeferredFacts(&info)
	normalizeMatroskaLavcFLAC(&info.Tracks[0], "Lavc58.91.100 flac")
	clearCanonicalSeedField(&info.Tracks[0], "FrameCount", "")
	clearCanonicalSeedField(&info.Tracks[0], "FrameRate", "")
	clearCanonicalSeedField(&info.Tracks[0], "SamplesPerFrame", "")
	refreshMatroskaCompatibilitySnapshots(&info)

	for _, key := range []string{"BitDepth_Detected", "FrameCount", "FrameRate", "SamplesPerFrame"} {
		if _, exists := info.Tracks[0].JSON[key]; exists {
			t.Fatalf("stale %s survived canonical cleanup: %#v", key, info.Tracks[0].JSON)
		}
	}
	for key, want := range map[string]string{
		"ChannelLayout":    "L R",
		"ChannelPositions": "Front: L R",
	} {
		if got := info.Tracks[0].JSON[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestMatroskaAC3StereoExtraOmitsInapplicableMixFields(t *testing.T) {
	probe := &matroskaAudioProbe{format: "AC-3"}
	info := ac3Info{
		acmod: 2, dialnorm: -23, hasDialnorm: true,
		dialnormCount: 2, dialnormSum: 2, dialnormMin: -31, dialnormMax: -23,
		hasCmixlev: true, cmixlevDB: -3,
		hasSurmixlev: true, surmixlevDB: -3,
		hasDmixmod: true, dmixmod: "Lt/Rt",
	}
	fields := matroskaAC3CanonicalExtraFields(probe, info, false, "")
	for _, field := range fields {
		switch field.Key {
		case "cmixlev", "surmixlev", "dmixmod", "dialnorm_Maximum":
			t.Fatalf("inapplicable stereo field emitted: %s=%s", field.Key, field.Val)
		}
	}
}

func TestApplyMatroskaTagStats(t *testing.T) {
	info := MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamVideo,
				Fields: []Field{
					{Name: "Format", Value: "AVC"},
					{Name: "ID", Value: "1"},
					{Name: "Width", Value: "1920 pixels"},
					{Name: "Height", Value: "1080 pixels"},
					{Name: "Frame rate", Value: "24.000 FPS"},
				},
				JSON: map[string]string{"UniqueID": "123"},
			},
		},
	}
	seedMatroskaLegacyTestStream(&info.Tracks[0])
	complete := applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{
		123: {
			trusted:         true,
			dataBytes:       1048576,
			hasDataBytes:    true,
			frameCount:      1200,
			hasFrameCount:   true,
			durationSeconds: 50,
			hasDuration:     true,
			bitRate:         166000,
			hasBitRate:      true,
		},
	}, 2*1048576)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if !complete {
		t.Fatalf("expected complete stats coverage")
	}
	if findField(info.Tracks[0].Fields, "Stream size") == "" {
		t.Fatalf("expected stream size")
	}
	if findField(info.Tracks[0].Fields, "Duration") == "" {
		t.Fatalf("expected duration")
	}
	if findField(info.Tracks[0].Fields, "Bit rate") == "" {
		t.Fatalf("expected bitrate")
	}
	if info.Tracks[0].JSON["FrameCount"] != "1200" {
		t.Fatalf("unexpected frame count json: %#v", info.Tracks[0].JSON)
	}
}

func TestApplyMatroskaBareTagDurationCorrection(t *testing.T) {
	tests := []struct {
		name         string
		delay        string
		tagDuration  float64
		hasDuration  bool
		bareDuration bool
		wantDuration string
		wantFrames   string
	}{
		{name: "valid subtraction", delay: "3", tagDuration: 10.125, hasDuration: true, bareDuration: true, wantDuration: "7.125000", wantFrames: "171"},
		{name: "equal duration", delay: "10", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "20.000", wantFrames: "480"},
		{name: "delay exceeds duration", delay: "11", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "20.000", wantFrames: "480"},
		{name: "rounded frame count is zero", delay: "9.99", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "20.000", wantFrames: "480"},
		{name: "missing delay", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "10.000000", wantFrames: "240"},
		{name: "zero delay", delay: "0", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "10.000000", wantFrames: "240"},
		{name: "negative delay", delay: "-1", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "11.000000", wantFrames: "264"},
		{name: "malformed delay", delay: "invalid", tagDuration: 10, hasDuration: true, bareDuration: true, wantDuration: "10.000000", wantFrames: "240"},
		{name: "missing tag duration", delay: "3", tagDuration: 10, wantDuration: "20.000", wantFrames: "480"},
		{name: "statistics duration", delay: "3", tagDuration: 10, hasDuration: true, wantDuration: "20.000", wantFrames: "480"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := MatroskaInfo{
				General: []Field{{Name: "Writing application", Value: "Lavf60.3.100"}},
				Tracks: []Stream{{
					Kind:   StreamVideo,
					Fields: []Field{{Name: "Frame rate", Value: "24.000 FPS"}},
					JSON: map[string]string{
						"UniqueID":   "123",
						"Delay":      tt.delay,
						"Duration":   "20.000",
						"FrameCount": "480",
					},
				}},
				tagStats: map[uint64]matroskaTagStats{123: {
					durationSeconds: tt.tagDuration,
					durationPrec:    6,
					hasDuration:     tt.hasDuration,
					bareDuration:    tt.bareDuration,
				}},
			}
			seedMatroskaLegacyTestStream(&info.Tracks[0])

			applyMatroskaBareTagDurationCorrection(&info)
			refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

			if got := info.Tracks[0].JSON["Duration"]; got != tt.wantDuration {
				t.Errorf("Duration = %q, want %q", got, tt.wantDuration)
			}
			if got := info.Tracks[0].JSON["FrameCount"]; got != tt.wantFrames {
				t.Errorf("FrameCount = %q, want %q", got, tt.wantFrames)
			}
		})
	}
}

func TestFindMatroskaSeekPositionsDeduplicatesInFirstSeenOrder(t *testing.T) {
	seekHead := buildMatroskaElement(mkvIDSeekHead, append(
		buildMatroskaSeekEntryForTest(mkvIDAttachments, 40),
		append(
			buildMatroskaSeekEntryForTest(mkvIDAttachments, 40),
			buildMatroskaSeekEntryForTest(mkvIDAttachments, 90)...,
		)...,
	))
	secondSeekHead := buildMatroskaElement(mkvIDSeekHead, append(
		buildMatroskaSeekEntryForTest(mkvIDAttachments, 90),
		buildMatroskaSeekEntryForTest(mkvIDAttachments, 140)...,
	))
	buf := append([]byte{0}, append(seekHead, secondSeekHead...)...)

	positions := findMatroskaSeekPositions(buf, 1, mkvIDAttachments)
	want := []uint64{40, 90, 140}
	if len(positions) != len(want) {
		t.Fatalf("positions = %v, want %v", positions, want)
	}
	for i := range want {
		if positions[i] != want[i] {
			t.Fatalf("positions = %v, want %v", positions, want)
		}
	}
}

func TestFindMatroskaSeekPositionsPreservesEmptyAndMalformedSemantics(t *testing.T) {
	malformedEntry := buildMatroskaElement(mkvIDSeek, buildMatroskaElement(mkvIDSeekID, buildMatroskaID(mkvIDAttachments)))
	seekHead := buildMatroskaElement(mkvIDSeekHead, malformedEntry)
	buf := append([]byte{0}, seekHead...)

	if positions := findMatroskaSeekPositions(buf, 1, mkvIDAttachments); len(positions) != 0 {
		t.Fatalf("positions = %v, want none", positions)
	}
}

func TestMatroskaOversizedChildSizesDoNotNarrowBeforeBoundsCheck(t *testing.T) {
	oversized := uint64(1) << 32
	seekHead := append(buildMatroskaID(mkvIDSeekHead), buildMatroskaSize(oversized)...)
	if positions := findMatroskaSeekPositions(append([]byte{0}, seekHead...), 1, mkvIDAttachments); len(positions) != 0 {
		t.Fatalf("oversized SeekHead returned positions %v", positions)
	}

	attachedFile := append(buildMatroskaID(mkvIDAttachedFile), buildMatroskaSize(oversized)...)
	if attachments := parseMatroskaAttachments(attachedFile); len(attachments) != 0 {
		t.Fatalf("oversized AttachedFile returned attachments %#v", attachments)
	}
}

func TestParseMatroskaDuplicateAttachmentSeekPositionRendersNameOnce(t *testing.T) {
	const attachmentPosition = uint64(mkvMaxScan + 128)
	seekEntries := append(
		buildMatroskaSeekEntryForTest(mkvIDAttachments, attachmentPosition),
		buildMatroskaSeekEntryForTest(mkvIDAttachments, attachmentPosition)...,
	)
	segment := append(buildMatroskaInfo(), buildMatroskaTracks()...)
	segment = append(segment, buildMatroskaElement(mkvIDSeekHead, seekEntries)...)
	if uint64(len(segment)) >= attachmentPosition {
		t.Fatalf("metadata length %d exceeds attachment position %d", len(segment), attachmentPosition)
	}
	segment = append(segment, make([]byte, int(attachmentPosition)-len(segment))...)
	attachedFile := buildMatroskaElement(mkvIDFileName, []byte("cover.jpg"))
	attachedFile = append(attachedFile, buildMatroskaElement(mkvIDFileMimeType, []byte("image/jpeg"))...)
	attachedFile = append(attachedFile, buildMatroskaElement(mkvIDFileData, []byte{0xFF, 0xD8, 0xFF})...)
	segment = append(segment, buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, attachedFile))...)
	file := append(buildMatroskaID(mkvIDSegment), 0xFF)
	file = append(file, segment...)

	info, ok := ParseMatroskaWithOptions(bytes.NewReader(file), int64(len(file)), defaultAnalyzeOptions())
	if !ok {
		t.Fatal("expected Matroska parse to succeed")
	}
	if len(info.attachments) != 1 || info.attachments[0] != "cover.jpg" {
		t.Fatalf("attachments = %v, want [cover.jpg]", info.attachments)
	}
}

func TestParseMatroskaAttachmentFallbackWithoutSeekHead(t *testing.T) {
	segment := append(buildMatroskaInfo(), buildMatroskaTracks()...)
	attachmentStart := int(mkvMaxScan) - 32
	if len(segment) >= attachmentStart {
		t.Fatalf("metadata length %d exceeds fallback position", len(segment))
	}
	segment = append(segment, make([]byte, attachmentStart-len(segment))...)
	png := minimalPNG(16, 9, 2)
	attachedFile := buildMatroskaElement(mkvIDFileName, []byte("cover.png"))
	attachedFile = append(attachedFile, buildMatroskaElement(mkvIDFileMimeType, []byte("image/png"))...)
	attachedFile = append(attachedFile, buildMatroskaElement(mkvIDFileData, png)...)
	segment = append(segment, buildMatroskaElement(mkvIDAttachments, buildMatroskaElement(mkvIDAttachedFile, attachedFile))...)
	segment = append(segment, make([]byte, 128)...)
	file := append(buildMatroskaID(mkvIDSegment), 0xFF)
	file = append(file, segment...)

	info, ok := ParseMatroskaWithOptions(bytes.NewReader(file), int64(len(file)), defaultAnalyzeOptions())
	if !ok {
		t.Fatal("expected Matroska parse to succeed")
	}
	if len(info.attachments) != 1 || info.attachments[0] != "cover.png" || len(info.attachmentInfo) != 1 {
		t.Fatalf("fallback attachments = names:%v info:%#v", info.attachments, info.attachmentInfo)
	}
	stream, ok := matroskaAttachmentImageStream(info.attachmentInfo[0])
	if !ok || stream.JSON["Width"] != "16" || stream.JSON["Height"] != "9" {
		t.Fatalf("fallback attachment metadata = ok:%v stream:%+v", ok, stream)
	}
}

func TestApplyMatroskaTagStatsKeepsContainerCBR(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "E-AC-3"},
			{Name: "ID", Value: "1"},
			{Name: "Bit rate mode", Value: "Constant"},
			{Name: "Bit rate", Value: "768 kb/s"},
		},
		JSON: map[string]string{
			"UniqueID":   "123",
			"BitRate":    "768000",
			"StreamSize": "371521536",
			"Duration":   "3870.019000000",
		},
	}}}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, bitRate: 767999, hasBitRate: true,
		dataBytes: 371521536, hasDataBytes: true,
		durationSeconds: 3870.019, hasDuration: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["BitRate"]; got != "768000" {
		t.Fatalf("BitRate = %q, want 768000", got)
	}
}

func TestApplyMatroskaTagStatsRoundsTextDurationToNanoseconds(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind:   StreamText,
		Fields: []Field{{Name: "ID", Value: "1"}},
		JSON:   map[string]string{"UniqueID": "123"},
	}}}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, durationSeconds: 8313.166, hasDuration: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["Duration"]; got != "8313.166000000" {
		t.Fatalf("Duration = %q, want 8313.166000000", got)
	}
}

func TestApplyMatroskaTagStatsUpdatesFLACSampleCounts(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "FLAC"},
			{Name: "Sampling rate", Value: "48.0 kHz"},
		},
		JSON: map[string]string{
			"UniqueID":        "123",
			"SamplesPerFrame": "4096",
		},
	}}}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, durationSeconds: 124.501, hasDuration: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["SamplingCount"]; got != "5976048" {
		t.Fatalf("SamplingCount = %q", got)
	}
	if got := info.Tracks[0].JSON["FrameCount"]; got != "1459" {
		t.Fatalf("FrameCount = %q", got)
	}
}

func TestApplyMatroskaTagStatsDerivesOpusCadenceFromTrustedCounts(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "Opus"},
			{Name: "ID", Value: "1"},
			{Name: "Sampling rate", Value: "48.0 kHz"},
		},
		JSON: map[string]string{"UniqueID": "123", "SamplesPerFrame": "960"},
	}}}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, durationSeconds: 10, hasDuration: true, frameCount: 400, hasFrameCount: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	json := info.Tracks[0].JSON
	if json["SamplingCount"] != "480000" || json["FrameCount"] != "400" || json["FrameRate"] != "40.000" || json["SamplesPerFrame"] != "1200" {
		t.Fatalf("inconsistent Opus cadence: %#v", json)
	}
}

func TestApplyMatroskaTagStatsNormalizesTrustedAACBitRate(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "AAC LC"},
			{Name: "ID", Value: "1"},
		},
		JSON: map[string]string{"UniqueID": "123", "BitRate": "192000"},
	}}}

	seedMatroskaLegacyTestStream(&info.Tracks[0])
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, bitRate: 191999, hasBitRate: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["BitRate"]; got != "192000" {
		t.Fatalf("AAC BitRate = %q, want normalized 192000", got)
	}
}

func TestApplyMatroskaTagStatsUsesTrustedAACBitRateRegardlessOfTrackUID(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "AAC LC"},
			{Name: "ID", Value: "1"},
			{Name: "Bit rate mode", Value: "Constant"},
			{Name: "Bit rate", Value: "256 kb/s"},
		},
		JSON: map[string]string{"UniqueID": "18163320629618101418", "BitRate": "256000"},
	}}}
	seedMatroskaLegacyTestStream(&info.Tracks[0])

	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{18163320629618101418: {
		trusted: true, bitRate: 251111, hasBitRate: true,
	}}, 0)

	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
	if got := info.Tracks[0].JSON["BitRate"]; got != "251111" {
		t.Fatalf("trusted AAC BitRate = %q, want 251111", got)
	}
}

func TestApplyMatroskaTagStatsPrefersDeclaredAACBitRate(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{{
		Kind: StreamAudio,
		Fields: []Field{
			{Name: "Format", Value: "AAC LC"},
			{Name: "ID", Value: "1"},
		},
		JSON: map[string]string{"UniqueID": "123", "BitRate": "189375"},
	}}}
	seedMatroskaLegacyTestStream(&info.Tracks[0])
	info.Tracks[0].matroskaDeferredFacts = &matroskaDeferredFacts{}
	info.Tracks[0].matroskaDeferredFacts.Set("BitRate", "192000")

	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, bitRate: 189375, hasBitRate: true,
	}}, 0)
	refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])

	if got := info.Tracks[0].JSON["BitRate"]; got != "192000" {
		t.Fatalf("AAC BitRate = %q, want declared 192000", got)
	}
}

func TestHasValidDTSLBRHeaderRequiresAlignmentAndHeaderType(t *testing.T) {
	valid := []byte{0, 0, 0, 0, 0x0A, 0x80, 0x19, 0x21, 0x01}
	if !hasValidDTSLBRHeader(valid) {
		t.Fatal("aligned sync-only DTS LBR header was rejected")
	}
	valid[len(valid)-1] = 0x02
	if !hasValidDTSLBRHeader(valid) {
		t.Fatal("aligned decoder-init DTS LBR header was rejected")
	}

	for name, payload := range map[string][]byte{
		"unaligned": {0, 0x0A, 0x80, 0x19, 0x21, 0x01},
		"reserved":  {0x0A, 0x80, 0x19, 0x21, 0x03},
		"truncated": {0x0A, 0x80, 0x19, 0x21},
	} {
		t.Run(name, func(t *testing.T) {
			if hasValidDTSLBRHeader(payload) {
				t.Fatalf("invalid DTS LBR payload accepted: % X", payload)
			}
		})
	}
}

func TestApplyMatroskaMP3ProbeUsesPerStreamEvidence(t *testing.T) {
	base := matroskaAudioProbe{mp3: mp3HeaderInfo{
		bitrateKbps: 128, sampleRate: 48000, channels: 2, channelMode: 0x01, versionID: 0x03,
	}}
	plain := Stream{Kind: StreamAudio, JSON: map[string]string{"Duration": "10.000"}}
	seedMatroskaLegacyTestStream(&plain)
	applyMatroskaMP3Probe(&plain, &base)
	refreshCanonicalCompatibilitySnapshot(&plain)
	if got := plain.JSON["Format_Settings_ModeExtension"]; got != "" {
		t.Fatalf("plain joint-stereo mode extension = %q, want empty", got)
	}

	msProbe := base
	msProbe.mp3.modeExt = 0x02
	msProbe.mp3Library = "LAME3.98r"
	msProbe.mp3AudioFrameSeen = true
	ms := Stream{Kind: StreamAudio, JSON: map[string]string{"Duration": "10.000"}}
	seedMatroskaLegacyTestStream(&ms)
	applyMatroskaMP3Probe(&ms, &msProbe)
	refreshCanonicalCompatibilitySnapshot(&ms)
	if got := ms.JSON["Format_Settings_ModeExtension"]; got != "MS Stereo" {
		t.Fatalf("MS joint-stereo mode extension = %q", got)
	}
	if plain.JSON["Duration"] != ms.JSON["Duration"] || plain.JSON["FrameCount"] != ms.JSON["FrameCount"] {
		t.Fatalf("unrelated stream count changed timing: plain=%#v ms=%#v", plain.JSON, ms.JSON)
	}
}

func TestApplyMatroskaAudioProbesKeepsMPEGTracksIndependent(t *testing.T) {
	info := MatroskaInfo{Tracks: []Stream{
		{Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: "1"}}, JSON: map[string]string{"Duration": "10.000"}},
		{Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: "2"}}, JSON: map[string]string{"Duration": "20.000"}},
	}}
	probes := map[uint64]*matroskaAudioProbe{
		1: {format: "MPEG Audio", ok: true, mp3: mp3HeaderInfo{bitrateKbps: 128, sampleRate: 48000, channels: 2, channelMode: 1, versionID: 3}},
		2: {format: "MPEG Audio", ok: true, mp3: mp3HeaderInfo{bitrateKbps: 192, sampleRate: 48000, channels: 2, channelMode: 1, modeExt: 2, versionID: 3}, mp3Library: "LAME3.100", mp3AudioFrameSeen: true, mp3FrameCount: 100},
	}
	for index := range info.Tracks {
		seedMatroskaLegacyTestStream(&info.Tracks[index])
	}

	applyMatroskaAudioProbes(&info, probes)
	for index := range info.Tracks {
		refreshCanonicalCompatibilitySnapshot(&info.Tracks[index])
	}

	if got := info.Tracks[0].JSON["Format_Settings_ModeExtension"]; got != "" {
		t.Fatalf("track 1 inherited mode extension %q", got)
	}
	if got := info.Tracks[1].JSON["Format_Settings_ModeExtension"]; got != "MS Stereo" {
		t.Fatalf("track 2 mode extension = %q", got)
	}
	if info.Tracks[0].JSON["Duration"] == info.Tracks[1].JSON["Duration"] {
		t.Fatalf("independent track timing collapsed: %#v %#v", info.Tracks[0].JSON, info.Tracks[1].JSON)
	}
}

func TestProbeMatroskaMP3UsesInfoFrameAndFollowingAudioFrame(t *testing.T) {
	const frameSize = 192
	infoFrame := make([]byte, frameSize)
	copy(infoFrame, []byte{0xFF, 0xFB, 0x54, 0x60})
	copy(infoFrame[36:], "Info")
	binary.BigEndian.PutUint32(infoFrame[40:], 0x0003)
	binary.BigEndian.PutUint32(infoFrame[44:], 100)
	binary.BigEndian.PutUint32(infoFrame[48:], 100*frameSize)

	probe := &matroskaAudioProbe{format: "MPEG Audio"}
	probes := map[uint64]*matroskaAudioProbe{1: probe}
	probeMatroskaAudio(probes, 1, infoFrame, 1, frameSize, true)
	if !probe.ok || !probe.collect || probe.mp3FrameCount != 100 || probe.mp3PayloadBytes != 99*frameSize {
		t.Fatalf("Info frame evidence not retained: %+v", probe)
	}

	nextFrame := make([]byte, frameSize)
	copy(nextFrame, []byte{0xFF, 0xFB, 0x54, 0x60})
	probeMatroskaAudio(probes, 1, nextFrame, 1, frameSize, true)
	if !probe.collect || probe.mp3.modeExt != 0x02 {
		t.Fatalf("following audio frame evidence not applied: %+v", probe)
	}
	for range 511 {
		probeMatroskaAudio(probes, 1, nextFrame, 1, frameSize, true)
	}
	if probe.collect || probe.mp3FramesObserved != 512 {
		t.Fatalf("bounded MP3 mode sampling did not complete: %+v", probe)
	}

	stream := Stream{Kind: StreamAudio, JSON: map[string]string{"Duration": "3.000"}}
	seedMatroskaLegacyTestStream(&stream)
	applyMatroskaMP3Probe(&stream, probe)
	refreshCanonicalCompatibilitySnapshot(&stream)
	if stream.JSON["FrameCount"] != "100" || stream.JSON["Duration"] != "2.400" || stream.JSON["StreamSize"] != "19008" || stream.JSON["Format_Settings_ModeExtension"] != "MS Stereo" {
		t.Fatalf("Info-derived MP3 metadata mismatch: %#v", stream.JSON)
	}

	mixedProbe := &matroskaAudioProbe{format: "MPEG Audio"}
	mixedProbes := map[uint64]*matroskaAudioProbe{1: mixedProbe}
	probeMatroskaAudio(mixedProbes, 1, infoFrame, 1, frameSize, true)
	nonMSFrame := append([]byte(nil), nextFrame...)
	nonMSFrame[3] = 0x40
	for range 19 {
		probeMatroskaAudio(mixedProbes, 1, nonMSFrame, 1, frameSize, true)
	}
	probeMatroskaAudio(mixedProbes, 1, nextFrame, 1, frameSize, true)
	mixedStream := Stream{Kind: StreamAudio, JSON: map[string]string{"Duration": "3.000"}}
	seedMatroskaLegacyTestStream(&mixedStream)
	applyMatroskaMP3Probe(&mixedStream, mixedProbe)
	refreshCanonicalCompatibilitySnapshot(&mixedStream)
	if got := mixedStream.JSON["Format_Settings_ModeExtension"]; got != "" {
		t.Fatalf("mixed Info/audio mode extension = %q; want omitted", got)
	}
}

func TestMatroskaTagCompletenessIsPerTrack(t *testing.T) {
	track11 := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&track11, "UniqueID", "11", "Unique ID", "11")
	track12 := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&track12, "UniqueID", "12", "Unique ID", "12")
	replaceCanonicalSeedFill(&track12, "Language", "en", "Language", "English")
	tracks := []Stream{track11, track12}
	if matroskaHasCompleteTagLanguages(tracks, map[uint64]string{}) {
		t.Fatal("missing language for track 11 reported complete")
	}
	if !matroskaHasCompleteTagLanguages(tracks, map[uint64]string{11: "fr"}) {
		t.Fatal("per-track language coverage reported incomplete")
	}

	existing := map[uint64]matroskaTagStats{11: {
		trusted: true, dataBytes: 100, hasDataBytes: true,
	}}
	candidate := map[uint64]matroskaTagStats{
		11: {trusted: true, durationSeconds: 1, hasDuration: true},
		12: {trusted: true, dataBytes: 100, hasDataBytes: true, bitRate: 800000, hasBitRate: true},
	}
	if !matroskaHasCompleteCombinedTagStats(tracks, existing, candidate) {
		t.Fatal("complementary per-track stats reported incomplete")
	}
	delete(candidate, 12)
	if matroskaHasCompleteCombinedTagStats(tracks, existing, candidate) {
		t.Fatal("missing stats for track 12 reported complete")
	}
}

func TestParseMatroskaTagsCarriesTagLanguage(t *testing.T) {
	tag := buildMatroskaLanguageTag(7, "jpn")
	_, _, langs, _, _, _ := parseMatroskaTags(tag, "", "")
	if got := langs[7]; got != "jpn" {
		t.Fatalf("TagLanguage = %q, want jpn", got)
	}
}

func TestMergeMatroskaTailTrackTagsPreservesHeadKeys(t *testing.T) {
	encoders := mergeMatroskaTagEncoders(map[uint64]string{1: "x264 core 164"}, map[uint64]string{2: "x265 4.1"})
	settings := mergeMatroskaTagValues(map[uint64]string{1: "cabac=1"}, map[uint64]string{2: "crf=18"})
	languages := mergeMatroskaTagValues(map[uint64]string{1: "eng"}, map[uint64]string{2: "jpn"})
	for uid, want := range map[uint64]string{1: "x264 core 164", 2: "x265 4.1"} {
		if encoders[uid] != want {
			t.Fatalf("encoder[%d] = %q, want %q", uid, encoders[uid], want)
		}
	}
	if settings[1] != "cabac=1" || settings[2] != "crf=18" || languages[1] != "eng" || languages[2] != "jpn" {
		t.Fatalf("tail map merge dropped head keys: settings=%v languages=%v", settings, languages)
	}
}

func TestParseMatroskaLanguageOnlyTagsFromHeadAndTailWindows(t *testing.T) {
	for _, tt := range []struct {
		name     string
		position int
	}{
		{name: "head", position: 8 << 20},
		{name: "tail", position: 33 << 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			generalBody := buildMatroskaElement(mkvIDTagTargets, nil)
			generalBody = append(generalBody, buildMatroskaSimpleTag("WINDOW_GENERAL", tt.name)...)
			trackBody := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(7)))
			trackBody = append(trackBody, buildMatroskaSimpleTag("WINDOW_TRACK", tt.name)...)
			tags := append(buildMatroskaLanguageTag(7, "jpn"), buildMatroskaElement(mkvIDTag, generalBody)...)
			tags = append(tags, buildMatroskaElement(mkvIDTag, trackBody)...)
			segment := append(buildMatroskaInfo(), buildMatroskaAudioTracks(7)...)
			if len(segment) >= tt.position {
				t.Fatalf("metadata length %d exceeds tag position", len(segment))
			}
			segment = append(segment, make([]byte, tt.position-len(segment))...)
			segment = append(segment, buildMatroskaElement(mkvIDTags, tags)...)
			file := append(buildMatroskaID(mkvIDSegment), 0xFF)
			file = append(file, segment...)

			info, ok := ParseMatroskaWithOptions(bytes.NewReader(file), int64(len(file)), defaultAnalyzeOptions())
			if !ok || len(info.Tracks) != 1 {
				t.Fatalf("parse failed: ok=%v tracks=%d", ok, len(info.Tracks))
			}
			refreshCanonicalCompatibilitySnapshot(&info.Tracks[0])
			if got := info.Tracks[0].JSON["Language"]; got == "" {
				t.Fatal("language-only Tags fallback did not reach audio output")
			}
			if got := info.scopedTags.general.get("WINDOW_GENERAL"); got != tt.name {
				t.Fatalf("General fallback tag = %q, want %q", got, tt.name)
			}
			node := canonicalSeedStructuredNode(&info.Tracks[0], "extra")
			if node == nil {
				t.Fatal("track canonical extra is missing")
			}
			var extra map[string]string
			if err := json.Unmarshal([]byte(structuredNodeText(*node)), &extra); err != nil {
				t.Fatalf("track extra is invalid: %v", err)
			}
			if got := extra["WINDOW_TRACK"]; got != tt.name {
				t.Fatalf("Track fallback tag = %q, want %q: %#v", got, tt.name, extra)
			}
		})
	}
}

func TestParseMatroskaSeekHeadScopedTagSurvivesHeadFallback(t *testing.T) {
	const tagPosition = 18 << 20
	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(7)))
	tag := append([]byte(nil), targets...)
	tag = append(tag, buildMatroskaSimpleTag("SEEK_ONLY", "retained")...)
	tags := buildMatroskaElement(mkvIDTags, buildMatroskaElement(mkvIDTag, tag))
	segment := append(buildMatroskaInfo(), buildMatroskaAudioTracks(7)...)
	segment = append(segment, buildMatroskaElement(mkvIDSeekHead, buildMatroskaSeekEntryForTest(mkvIDTags, tagPosition))...)
	if len(segment) >= tagPosition {
		t.Fatalf("metadata length %d exceeds tag position", len(segment))
	}
	segment = append(segment, make([]byte, tagPosition-len(segment))...)
	segment = append(segment, tags...)
	segment = append(segment, make([]byte, (20<<20)-len(segment))...)
	file := append(buildMatroskaID(mkvIDSegment), 0xFF)
	file = append(file, segment...)

	info, ok := ParseMatroskaWithOptions(bytes.NewReader(file), int64(len(file)), defaultAnalyzeOptions())
	if !ok || len(info.Tracks) != 1 {
		t.Fatalf("parse failed: ok=%v tracks=%d", ok, len(info.Tracks))
	}
	node := canonicalSeedStructuredNode(&info.Tracks[0], "extra")
	if node == nil {
		t.Fatal("track canonical extra is missing")
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(structuredNodeText(*node)), &extra); err != nil {
		t.Fatalf("track extra is invalid: %v", err)
	}
	if got := extra["SEEK_ONLY"]; got != "retained" {
		t.Fatalf("SeekHead scoped tag = %q, want retained: %#v", got, extra)
	}
}

func TestParseMatroskaTailScanIncludesMissingPerTrackEncoderSettings(t *testing.T) {
	headTags := append(buildMatroskaTagForStats(7), buildMatroskaEncoderTag(0, "mkvmerge v82.0.0", "")...)
	segment := append(buildMatroskaInfo(), buildMatroskaVideoTrackWithUID(7)...)
	segment = append(segment, buildMatroskaElement(mkvIDTags, headTags)...)
	const tailPosition = 33 << 20
	if len(segment) >= tailPosition {
		t.Fatalf("metadata length %d exceeds tail position", len(segment))
	}
	segment = append(segment, make([]byte, tailPosition-len(segment))...)
	tailTags := buildMatroskaEncoderTag(7, "x264 core 164", "cabac=1 / ref=4")
	segment = append(segment, buildMatroskaElement(mkvIDTags, tailTags)...)
	file := append(buildMatroskaID(mkvIDSegment), 0xFF)
	file = append(file, segment...)

	info, ok := ParseMatroskaWithOptions(bytes.NewReader(file), int64(len(file)), defaultAnalyzeOptions())
	if !ok || len(info.Tracks) != 1 {
		t.Fatalf("parse failed: ok=%v tracks=%d", ok, len(info.Tracks))
	}
	if got := findField(info.Tracks[0].Fields, "Writing library"); !strings.Contains(got, "x264") {
		t.Fatalf("tail encoder = %q, want x264", got)
	}
	if got := findField(info.Tracks[0].Fields, "Encoding settings"); got != "cabac=1 / ref=4" {
		t.Fatalf("tail encoding settings = %q", got)
	}
}

func TestParseMatroskaRejectsShortPreciseTagRead(t *testing.T) {
	const tagPosition = 18 << 20
	segment := append(buildMatroskaInfo(), buildMatroskaAudioTracks(7)...)
	segment = append(segment, buildMatroskaElement(mkvIDSeekHead, buildMatroskaSeekEntryForTest(mkvIDTags, tagPosition))...)
	if len(segment) >= tagPosition {
		t.Fatalf("metadata length %d exceeds tag position", len(segment))
	}
	segment = append(segment, make([]byte, tagPosition-len(segment))...)
	segment = append(segment, buildMatroskaElement(mkvIDTags, buildMatroskaLanguageTag(7, "jpn"))...)
	segment = append(segment, make([]byte, (20<<20)-len(segment))...)
	file := append(buildMatroskaID(mkvIDSegment), 0xFF)
	file = append(file, segment...)
	reader := shortAtReader{ReaderAt: bytes.NewReader(file), offset: int64(len(buildMatroskaID(mkvIDSegment))+1) + tagPosition}

	info, ok := ParseMatroskaWithOptions(reader, int64(len(file)), defaultAnalyzeOptions())
	if !ok || len(info.Tracks) != 1 {
		t.Fatalf("parse failed: ok=%v tracks=%d", ok, len(info.Tracks))
	}
	if got := info.Tracks[0].JSON["Language"]; got != "" {
		t.Fatalf("partial zero-padded Tags published language %q", got)
	}
}

func TestParseMatroskaTagStatsWithoutDate(t *testing.T) {
	stats, ok := parseMatroskaTagStats(map[string]string{
		"_STATISTICS_TAGS":        "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES",
		"_STATISTICS_WRITING_APP": "mkvmerge v94.0 ('Initiate') 64-bit",
		"BPS":                     "5913898",
		"DURATION":                "00:42:01.080000000",
		"NUMBER_OF_FRAMES":        "63027",
		"NUMBER_OF_BYTES":         "1863676305",
	}, "", "")
	if !ok || !stats.trusted {
		t.Fatalf("expected trusted stats, got: %+v", stats)
	}
	if !stats.hasDataBytes || !stats.hasDuration || !stats.hasFrameCount || !stats.hasBitRate {
		t.Fatalf("missing parsed stats: %+v", stats)
	}
}

func TestParseMatroskaTagStatsBareDurationIsAuthoritative(t *testing.T) {
	bare, ok := parseMatroskaTagStats(map[string]string{
		"DURATION": "00:42:01.080000000",
	}, "", "")
	if !ok || !bare.hasDuration || !bare.trusted || !bare.bareDuration {
		t.Fatalf("bare DURATION not parsed: ok=%v stats=%+v", ok, bare)
	}
	if !matroskaTagStatsAreAuthoritative(&MatroskaInfo{}, bare) {
		t.Fatal("valid per-track DURATION was not authoritative")
	}

	listed, ok := parseMatroskaTagStats(map[string]string{
		"_STATISTICS_TAGS": "DURATION",
		"DURATION":         "00:42:01.080000000",
	}, "", "")
	if !ok || !listed.trusted || listed.bareDuration || !listed.hasDuration || listed.durationSeconds <= 0 {
		t.Fatalf("listed DURATION did not produce trusted statistics: ok=%v stats=%+v", ok, listed)
	}
	if !matroskaTagStatsAreAuthoritative(&MatroskaInfo{}, listed) {
		t.Fatal("listed statistics were not authoritative")
	}
}

func TestParseMatroskaTrackEntryHeaderStripping(t *testing.T) {
	compression := buildMatroskaElement(mkvIDContentCompression,
		append(
			buildMatroskaElement(mkvIDContentCompAlgo, encodeMatroskaUint(3)),
			buildMatroskaElement(mkvIDContentCompSettings, []byte{0x0B, 0x77})...,
		),
	)
	encoding := buildMatroskaElement(mkvIDContentEncoding, compression)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_AC3"))...)
	entry = append(entry, buildMatroskaElement(mkvIDContentEncodings, encoding)...)

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Muxing mode"); got != "Header stripping" {
		t.Fatalf("unexpected muxing mode: %q", got)
	}
	if !bytes.Equal(stream.mkvHeaderStripBytes, []byte{0x0B, 0x77}) {
		t.Fatalf("unexpected header strip bytes: %#v", stream.mkvHeaderStripBytes)
	}
}

func TestParseMatroskaTrackEntryNonHeaderCompression(t *testing.T) {
	compression := buildMatroskaElement(mkvIDContentCompression,
		append(
			buildMatroskaElement(mkvIDContentCompAlgo, encodeMatroskaUint(0)),
			buildMatroskaElement(mkvIDContentCompSettings, []byte{0x01})...,
		),
	)
	encoding := buildMatroskaElement(mkvIDContentEncoding, compression)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_AC3"))...)
	entry = append(entry, buildMatroskaElement(mkvIDContentEncodings, encoding)...)

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Muxing mode"); got != "" {
		t.Fatalf("unexpected muxing mode: %q", got)
	}
	if len(stream.mkvHeaderStripBytes) != 0 {
		t.Fatalf("unexpected header strip bytes: %#v", stream.mkvHeaderStripBytes)
	}
}

func TestParseMatroskaTrackEntryZlibSubtitle(t *testing.T) {
	compression := buildMatroskaElement(mkvIDContentCompression,
		nil,
	)
	encoding := buildMatroskaElement(mkvIDContentEncoding, compression)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(17)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("S_HDMV/PGS"))...)
	entry = append(entry, buildMatroskaElement(mkvIDContentEncodings, encoding)...)

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
	if !ok {
		t.Fatal("expected parsed stream")
	}
	if got := stream.JSON["MuxingMode"]; got != "zlib" {
		t.Fatalf("MuxingMode = %q, want zlib", got)
	}
}

func TestParseMatroskaTrackEntryKeepsPixelDimensionsWithDisplaySize(t *testing.T) {
	videoPayload := make([]byte, 0, 32)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(1920))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(1038))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDDisplayWidth, encodeMatroskaUint(320))...)
	videoPayload = append(videoPayload, buildMatroskaElement(mkvIDDisplayHeight, encodeMatroskaUint(173))...)
	video := buildMatroskaElement(mkvIDTrackVideo, videoPayload)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	entry = append(entry, video...)

	stream, ok := parseMatroskaTrackEntry(entry, 0, 3)
	if !ok {
		t.Fatalf("expected parsed stream")
	}
	if got := findField(stream.Fields, "Width"); got != "1 920 pixels" {
		t.Fatalf("Width=%q", got)
	}
	if got := findField(stream.Fields, "Height"); got != "1 038 pixels" {
		t.Fatalf("Height=%q", got)
	}
	if got := findField(stream.Fields, "Display aspect ratio"); got != "1.85:1" {
		t.Fatalf("Display aspect ratio=%q", got)
	}
}

func TestParseMatroskaInfoDateUTC(t *testing.T) {
	base := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	target := time.Date(2012, time.November, 28, 15, 41, 23, 0, time.UTC)
	delta := target.Sub(base).Nanoseconds()
	datePayload := make([]byte, 8)
	binary.BigEndian.PutUint64(datePayload, uint64(delta))

	infoBuf := append(buildMatroskaElement(mkvIDTimecodeScale, []byte{0x0F, 0x42, 0x40}),
		buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0x00, 0x00})...)
	infoBuf = append(infoBuf, buildMatroskaElement(mkvIDDateUTC, datePayload)...)

	seg, ok := parseMatroskaInfo(infoBuf)
	if !ok {
		t.Fatalf("expected parsed segment info")
	}
	if got := findField(seg.Fields, "Encoded date"); got != "2012-11-28 15:41:23 UTC" {
		t.Fatalf("unexpected encoded date: %q", got)
	}
}

func TestApplyMatroskaAudioProbesSkipsDialnormTextFields(t *testing.T) {
	info := &MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamAudio,
				Fields: []Field{
					{Name: "Format", Value: "AC-3"},
					{Name: "ID", Value: "1"},
				},
				JSON: map[string]string{},
			},
		},
	}
	probes := map[uint64]*matroskaAudioProbe{
		1: {
			format: "AC-3",
			ok:     true,
			info: ac3Info{
				channels:      6,
				layout:        "L R C LFE Ls Rs",
				sampleRate:    48000,
				frameRate:     31.25,
				spf:           1536,
				bitRateKbps:   640,
				hasDialnorm:   true,
				dialnorm:      -18,
				hasCompr:      true,
				comprDB:       -1.16,
				hasComprField: true,
				comprFieldDB:  -1.16,
			},
		},
	}

	applyMatroskaAudioProbes(info, probes)
	stream := info.Tracks[0]
	if got := findField(stream.Fields, "Dialog Normalization"); got != "" {
		t.Fatalf("expected no dialog normalization field, got %q", got)
	}
	if got := findField(stream.Fields, "compr"); got != "" {
		t.Fatalf("expected no compr field, got %q", got)
	}
	if got := findField(stream.Fields, "dialnorm_Average"); got != "" {
		t.Fatalf("expected no dialnorm_Average field, got %q", got)
	}
}

func TestShouldApplyMatroskaClusterStats(t *testing.T) {
	nonEmptyTags := map[uint64]matroskaTagStats{
		1: {trusted: true, hasDataBytes: true, dataBytes: 42},
	}
	tests := []struct {
		name       string
		parseSpeed float64
		size       int64
		tagStats   map[uint64]matroskaTagStats
		complete   bool
		want       bool
	}{
		{
			name:       "full parse speed always applies",
			parseSpeed: 1,
			size:       mkvMaxScan * 10,
			tagStats:   nonEmptyTags,
			complete:   true,
			want:       false,
		},
		{
			name:       "default parse speed applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan,
			tagStats:   nil,
			complete:   false,
			want:       false,
		},
		{
			name:       "large file with no tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nil,
			complete:   false,
			want:       false,
		},
		{
			name:       "large file with some tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nonEmptyTags,
			complete:   false,
			want:       false,
		},
		{
			name:       "complete tag stats applies",
			parseSpeed: 0.5,
			size:       mkvMaxScan + 1,
			tagStats:   nonEmptyTags,
			complete:   true,
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldApplyMatroskaClusterStats(tc.parseSpeed, tc.size, tc.tagStats, tc.complete)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestVideoProbeNeedsSample(t *testing.T) {
	probe := &matroskaVideoProbe{}
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected empty probe to not need samples")
	}

	probe.codec = "AVC"
	if !videoProbeNeedsSample(probe) {
		t.Fatalf("expected AVC probe to need samples")
	}
	probe.exhausted = true
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected exhausted probe to stop sampling")
	}
	probe.exhausted = false
	probe.codec = "HEVC"
	probe.hdrInfo = hevcHDRInfo{
		hasMastering: true,
		maxCLL:       1000,
		maxFALL:      400,
		hdr10Plus:    true,
	}
	if !videoProbeNeedsSample(probe) {
		t.Fatalf("expected HDR-complete HEVC probe without x265 SEI to keep sampling")
	}
	probe.hdrInfo.x265Seen = true
	if videoProbeNeedsSample(probe) {
		t.Fatalf("expected HDR-complete HEVC probe with x265 SEI to stop sampling")
	}
}

func TestProbeMatroskaVideo_HEVCContinuesAfterHDRForX265SEI(t *testing.T) {
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}
	probes := map[uint64]*matroskaVideoProbe{1: probe}

	probeMatroskaVideo(probes, 1, buildHEVCX265LengthPrefixedSample(t))

	if !probe.hdrInfo.x265Seen {
		t.Fatalf("expected later x265 SEI to be parsed after HDR completion")
	}
	if probe.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("x265Library = %q, want %q", probe.hdrInfo.x265Library, "x265 9.9")
	}
	if probe.hdrInfo.x265Settings != "wpp / me=0" {
		t.Fatalf("x265Settings = %q, want %q", probe.hdrInfo.x265Settings, "wpp / me=0")
	}
}

func TestH264LengthPrefixedToAnnexB(t *testing.T) {
	payload := []byte{0, 0, 0, 2, 0x65, 0x88, 0, 0, 0, 2, 0x41, 0x80}
	want := []byte{0, 0, 0, 1, 0x65, 0x88, 0, 0, 0, 1, 0x41, 0x80}
	if got := h264LengthPrefixedToAnnexB(payload, 4); !bytes.Equal(got, want) {
		t.Fatalf("Annex B = %x, want %x", got, want)
	}
}

func TestParseH264PicTimingTimeCode(t *testing.T) {
	var bits []byte
	appendBits := func(value uint64, width int) {
		for bit := width - 1; bit >= 0; bit-- {
			bits = append(bits, byte(value>>bit&1))
		}
	}
	appendBits(0, 4)  // pic_struct
	appendBits(1, 1)  // clock_timestamp_flag
	appendBits(0, 2)  // ct_type
	appendBits(0, 1)  // nuit_field_based_flag
	appendBits(0, 5)  // counting_type
	appendBits(1, 1)  // full_timestamp_flag
	appendBits(1, 1)  // discontinuity_flag; a discontinuous complete clock remains usable
	appendBits(0, 1)  // cnt_dropped_flag
	appendBits(12, 8) // n_frames
	appendBits(5, 6)  // seconds
	appendBits(4, 6)  // minutes
	appendBits(3, 5)  // hours
	payload := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		payload[i/8] |= bit << (7 - i%8)
	}

	got, ok := parseH264PicTimingTimeCode(payload, h264SPSInfo{PicStructPresent: true})
	if !ok || got != "03:04:05:12" {
		t.Fatalf("time code = %q, ok=%v", got, ok)
	}
}

func TestParseH264PicTimingTimeCodeRequiresHours(t *testing.T) {
	var bits []byte
	appendBits := func(value uint64, width int) {
		for bit := width - 1; bit >= 0; bit-- {
			bits = append(bits, byte(value>>bit&1))
		}
	}
	appendBits(0, 4)  // pic_struct
	appendBits(1, 1)  // clock_timestamp_flag
	appendBits(0, 2)  // ct_type
	appendBits(0, 1)  // nuit_field_based_flag
	appendBits(0, 5)  // counting_type
	appendBits(0, 1)  // full_timestamp_flag
	appendBits(0, 1)  // discontinuity_flag
	appendBits(0, 1)  // cnt_dropped_flag
	appendBits(7, 8)  // n_frames
	appendBits(1, 1)  // seconds_flag
	appendBits(54, 6) // seconds
	appendBits(1, 1)  // minutes_flag
	appendBits(39, 6) // minutes
	appendBits(0, 1)  // hours_flag
	payload := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		payload[i/8] |= bit << (7 - i%8)
	}

	if got, ok := parseH264PicTimingTimeCode(payload, h264SPSInfo{PicStructPresent: true}); ok || got != "" {
		t.Fatalf("incomplete time code = %q, ok=%v", got, ok)
	}
}

func TestStandardMatroskaH264GOPLength(t *testing.T) {
	if !standardMatroskaH264GOPLength(24) || standardMatroskaH264GOPLength(23) {
		t.Fatal("unexpected GOP length classification")
	}
}

func TestMatroskaH264GOPNeedsExplicitRate(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "FrameRate", "24.000", "Frame rate", "24.000 FPS")
	if matroskaH264GOPNeedsExplicitRate(stream, 24) {
		t.Fatal("exact one-second GOP should be implicit")
	}
	replaceCanonicalSeedFill(&stream, "FrameRate", "23.976", "Frame rate", "23.976 FPS")
	if !matroskaH264GOPNeedsExplicitRate(stream, 24) {
		t.Fatal("fractional-rate GOP should be explicit")
	}
}

func TestScanMatroskaClusters_HEVCReadsLateX265AfterHDRComplete(t *testing.T) {
	cluster := mkvClusterWithSimpleBlock(mkvBlockNoLace(buildHEVCX265LengthPrefixedSample(t)))
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: matroskaHEVCQuickProbePackets,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, map[uint64]*matroskaVideoProbe{1: probe}, false, false, 0.5, 1, nil, nil)

	if !probe.hdrInfo.x265Seen {
		t.Fatalf("expected scanner to read later x265 SEI after HDR completion")
	}
	if probe.packetCount != 1 {
		t.Fatalf("packetCount = %d, want 1", probe.packetCount)
	}
	if probe.hdrInfo.x265Library != "x265 9.9" {
		t.Fatalf("x265Library = %q, want x265 9.9", probe.hdrInfo.x265Library)
	}
}

func TestScanMatroskaClusters_HEVCStopsAtPacketCapWithoutX265(t *testing.T) {
	cluster := mkvClusterWithSimpleBlocks(
		mkvBlockNoLace(buildHEVCNonX265LengthPrefixedSample()),
		mkvBlockNoLace(buildHEVCNonX265LengthPrefixedSample()),
	)
	probe := &matroskaVideoProbe{
		codec:         "HEVC",
		nalLengthSize: 4,
		targetPackets: 1,
		hdrInfo: hevcHDRInfo{
			hasMastering: true,
			maxCLL:       1000,
			maxFALL:      400,
			hdr10Plus:    true,
		},
	}
	videoProbes := map[uint64]*matroskaVideoProbe{1: probe}

	scanMatroskaClusters(bytes.NewReader(cluster), 0, int64(len(cluster)), 1000000, nil, videoProbes, false, false, 1, 1, nil, nil)

	if !probe.exhausted {
		t.Fatalf("expected HEVC probe to exhaust at packet cap")
	}
	if probe.packetCount != 1 {
		t.Fatalf("packetCount = %d, want 1", probe.packetCount)
	}
	if !matroskaProbesComplete(nil, videoProbes) {
		t.Fatalf("expected exhausted non-x265 HEVC probe to be complete")
	}
}

func TestApplyMatroskaVideoProbes_X265SEIOverridesContainerEncoder(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	replaceCanonicalSeedFill(&stream, "Encoded_Library", "HandBrake 1.7.0", "Writing library", "HandBrake 1.7.0")
	replaceCanonicalSeedFill(&stream, "Encoded_Library_Settings", "container settings", "Encoding settings", "container settings")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	probes := map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			x265Library:  "x265 9.9",
			x265Settings: "wpp / me=0",
			x265Seen:     true,
		},
	}}

	applyMatroskaVideoProbes(&info, probes)

	stream = info.Tracks[0]
	if got := matroskaStreamDisplay(stream, "Writing library"); got != "x265 9.9" {
		t.Fatalf("Writing library = %q, want x265 9.9", got)
	}
	if got := matroskaStreamDisplay(stream, "Encoding settings"); got != "wpp / me=0" {
		t.Fatalf("Encoding settings = %q, want wpp / me=0", got)
	}
}

func TestApplyMatroskaInBandH264SPSOverridesStaleCadence(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	for name, value := range map[fieldName]string{
		"Duration":                "10000",
		"FrameCount":              "250",
		"FrameRate":               "25.000",
		"FrameRate_Mode":          "CFR",
		"FrameRate_Mode_Original": "VFR",
		"ScanType":                "Progressive",
	} {
		replaceCanonicalSeedFill(&stream, name, value, "", "")
	}
	applyMatroskaInBandH264SPS(&stream, h264SPSInfo{
		HasScanType:       true,
		MBAFF:             true,
		HasFixedFrameRate: true,
		FixedFrameRate:    true,
		FrameRate:         25,
	}, true)
	if got := matroskaStreamScalar(stream, "ScanType"); got != "MBAFF" {
		t.Fatalf("ScanType = %q", got)
	}
	if got := matroskaStreamScalar(stream, "ScanOrder"); got != "TFF" {
		t.Fatalf("ScanOrder = %q", got)
	}
	if got := matroskaStreamScalar(stream, "FrameRate_Mode"); got != "VFR" {
		t.Fatalf("FrameRate_Mode = %q", got)
	}
	if got := matroskaStreamScalar(stream, "FrameRate_Original"); got != "25.000" {
		t.Fatalf("FrameRate_Original = %q", got)
	}
	for _, name := range []fieldName{"Duration", "FrameCount", "FrameRate", "FrameRate_Mode_Original"} {
		if got := matroskaStreamScalar(stream, name); got != "" {
			t.Fatalf("%s = %q; want cleared", name, got)
		}
	}
}

func TestApplyMatroskaAVCFieldCadenceUsesObservedRateRatio(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	for name, value := range map[fieldName]string{
		"FrameRate":     "50.000",
		"FrameRate_Num": "25",
		"FrameRate_Den": "1",
	} {
		replaceCanonicalSeedFill(&stream, name, value, "", "")
	}
	applyMatroskaAVCFieldCadence(&stream, h264SPSInfo{FrameRate: 25})
	for name, want := range map[fieldName]string{
		"FrameRate_Mode":     "VFR",
		"FrameRate":          "50.000",
		"FrameRate_Original": "25.000",
		"FrameRate_Num":      "50",
		"FrameRate_Den":      "1",
	} {
		if got := matroskaStreamScalar(stream, name); got != want {
			t.Fatalf("%s = %q; want %q", name, got, want)
		}
	}
	if got := matroskaStreamDisplay(stream, "Frame rate"); got != "50.000 FPS" {
		t.Fatalf("Frame rate display = %q; want 50.000 FPS", got)
	}
}

func TestApplyMatroskaVideoProbes_DolbyVisionWithStaticHDR10(t *testing.T) {
	stream := Stream{Kind: StreamVideo,
		mkvHasDolbyVision: true,
		mkvDolbyVision: dolbyVisionConfig{
			versionMajor:    1,
			profile:         8,
			level:           6,
			rpuPresent:      true,
			blPresent:       true,
			compatibilityID: 1,
		}}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	probes := map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			hasMastering:          true,
			masteringLuminanceMin: 0.005,
			masteringLuminanceMax: 1000,
			maxCLL:                705,
			maxFALL:               144,
		},
	}}

	applyMatroskaVideoProbes(&info, probes)

	if got, _ := canonicalSeedValue(info.Tracks[0], "HDR_Format"); got != "Dolby Vision / SMPTE ST 2086" {
		t.Fatalf("HDR_Format = %q", got)
	}
	if got, _ := canonicalSeedValue(info.Tracks[0], "HDR_Format_Compression"); got != "None / " {
		t.Fatalf("HDR_Format_Compression = %q", got)
	}
	if got, _ := canonicalSeedValue(info.Tracks[0], "MasteringDisplay_Luminance_Min"); got != "0.0050" {
		t.Fatalf("MasteringDisplay_Luminance_Min = %q", got)
	}
	if got, _ := canonicalSeedValue(info.Tracks[0], "MasteringDisplay_Luminance_Max"); got != "1000" {
		t.Fatalf("MasteringDisplay_Luminance_Max = %q", got)
	}
	if got, _ := canonicalSeedValue(info.Tracks[0], "MaxCLL"); got != "705" {
		t.Fatalf("MaxCLL = %q", got)
	}
	if got, _ := canonicalSeedValue(info.Tracks[0], "MaxFALL"); got != "144" {
		t.Fatalf("MaxFALL = %q", got)
	}
}

func TestApplyMatroskaVideoProbes_DolbyVisionWithoutSecondaryHDR(t *testing.T) {
	stream := Stream{
		Kind:              StreamVideo,
		mkvHasDolbyVision: true,
		mkvDolbyVision: dolbyVisionConfig{
			versionMajor: 1, profile: 8, level: 6,
			rpuPresent: true, blPresent: true, compatibilityID: 1,
		},
	}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "HEVC"}})

	if got := matroskaStreamScalar(info.Tracks[0], "HDR_Format_Compression"); got != "None" {
		t.Fatalf("HDR_Format_Compression = %q; want None", got)
	}
	if got := matroskaStreamScalar(info.Tracks[0], "HDR_Format_Compatibility"); got != "" {
		t.Fatalf("HDR_Format_Compatibility = %q; want omitted", got)
	}
	if got := matroskaStreamDisplay(info.Tracks[0], "HDR format"); got != "Dolby Vision, Version 1.0, Profile 8, dvhe.08.06, BL+RPU, no metadata compression" {
		t.Fatalf("HDR format = %q", got)
	}
}

func TestApplyMatroskaVideoProbes_HDR10PlusVersionZero(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			hdr10Plus: true, hdr10PlusVersion: 0, hdr10PlusToneMapping: true,
		},
	}})

	if got := matroskaStreamScalar(info.Tracks[0], "HDR_Format_Version"); got != "0" {
		t.Fatalf("HDR_Format_Version = %q; want 0", got)
	}
	if got := matroskaStreamScalar(info.Tracks[0], "HDR_Format_Compatibility"); got != "" {
		t.Fatalf("HDR_Format_Compatibility = %q; want omitted", got)
	}
	if got := matroskaStreamDisplay(info.Tracks[0], "HDR format"); got != "SMPTE ST 2094 App 4, Version 0" {
		t.Fatalf("HDR format = %q", got)
	}
}

func TestMatroskaDolbyVisionDuplicateSignalsStayParsed(t *testing.T) {
	bits := make([]byte, 3)
	writer := bitWriter{b: bits}
	writer.writeBits(5, 7)
	writer.writeBits(3, 6)
	writer.writeBits(1, 1)
	writer.writeBits(0, 1)
	writer.writeBits(1, 1)
	writer.writeBits(0, 4)
	writer.writeBits(0, 2)
	payload := append([]byte{1, 0}, bits...)
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box, uint32(len(box)))
	copy(box[4:8], "dvcC")
	copy(box[8:], payload)
	configs := parseDolbyVisionConfigsFromPrivate(append(append([]byte{}, box...), box...))
	if len(configs) != 2 {
		t.Fatalf("Dolby Vision configs = %d; want 2", len(configs))
	}

	stream := Stream{
		Kind:                StreamVideo,
		mkvHasDolbyVision:   true,
		mkvDolbyVision:      configs[0],
		mkvDolbyVisionCount: len(configs),
	}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "HEVC"}})
	for name, want := range map[fieldName]string{
		"HDR_Format":             "Dolby Vision / Dolby Vision",
		"HDR_Format_Profile":     "dvhe.05 / dvhe.05",
		"HDR_Format_Level":       "03 / 03",
		"HDR_Format_Compression": "None / None",
		"colour_range":           "Full",
	} {
		if got := matroskaStreamScalar(info.Tracks[0], name); got != want {
			t.Fatalf("%s = %q; want %q", name, got, want)
		}
	}
}

func TestMatroskaProbesCompleteRequiresParsedAudio(t *testing.T) {
	audio := map[uint64]*matroskaAudioProbe{
		1: {format: "AC-3"},
	}
	if matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected unparsed audio probe to be incomplete")
	}

	audio[1].ok = true
	if !matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected parsed AC-3 audio probe to be complete")
	}

	audio[1] = &matroskaAudioProbe{format: "E-AC-3", ok: true, collect: true}
	if matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected collecting E-AC-3 probe to be incomplete")
	}

	audio[1].collect = false
	if !matroskaProbesComplete(audio, nil) {
		t.Fatalf("expected non-collecting E-AC-3 probe to be complete")
	}
}

func TestMergeMatroskaAttachmentNamesPreservesInitialAndMultiplicity(t *testing.T) {
	got := mergeMatroskaAttachmentNames([]string{"initial.txt", "cover.png"}, []string{"cover.png", "cover.png", "later.txt"})
	want := []string{"initial.txt", "cover.png", "cover.png", "later.txt"}
	if strings.Join(got, " / ") != strings.Join(want, " / ") {
		t.Fatalf("Attachments output = %q, want %q", strings.Join(got, " / "), strings.Join(want, " / "))
	}
}

func TestMatroskaTagsHaveDataKeepsEmptyPreciseReadFallbackEligible(t *testing.T) {
	if matroskaTagsHaveData(nil, nil, nil, map[uint64]matroskaTagStats{}, nil) {
		t.Fatal("empty precise Tags read must remain fallback-eligible")
	}
	if !matroskaTagsHaveData(nil, nil, nil, map[uint64]matroskaTagStats{7: {durationSeconds: 1}}, nil) {
		t.Fatal("parsed Tags data must suppress redundant fallback")
	}
}

func TestAppendMatroskaAttachmentUniqueUsesPayloadIdentity(t *testing.T) {
	first := matroskaAttachment{name: "cover.png", mime: "image/png", data: minimalPNG(16, 9, 2), complete: true}
	first.size = int64(len(first.data))
	second := matroskaAttachment{name: "COVER.PNG", mime: "image/png", data: minimalPNG(16, 9, 6), size: first.size, complete: true}

	attachments := appendMatroskaAttachmentUnique(nil, first)
	attachments = appendMatroskaAttachmentUnique(attachments, second)
	if len(attachments) != 2 {
		t.Fatalf("same-name/same-size distinct images collapsed: %#v", attachments)
	}
	for i, attachment := range attachments {
		stream, ok := matroskaAttachmentImageStream(attachment)
		if !ok || stream.JSON["Width"] != "16" || stream.JSON["Height"] != "9" {
			t.Fatalf("image %d metadata missing: ok=%v stream=%+v", i, ok, stream)
		}
	}
}

func TestAppendMatroskaAttachmentUniqueReplacesBoundedPrefix(t *testing.T) {
	fullData := minimalPNG(16, 9, 2)
	partial := matroskaAttachment{name: "cover.png", mime: "image/png", data: append([]byte(nil), fullData[:24]...), size: int64(len(fullData))}
	full := matroskaAttachment{name: "cover.png", mime: "image/png", data: fullData, size: int64(len(fullData)), complete: true}
	attachments := appendMatroskaAttachmentUnique([]matroskaAttachment{partial}, full)
	if len(attachments) != 1 || !attachments[0].complete || !bytes.Equal(attachments[0].data, fullData) {
		t.Fatalf("partial attachment was not replaced by full payload: %#v", attachments)
	}
}

func TestParseMatroskaSegmentIgnoresPartialChapters(t *testing.T) {
	display := buildMatroskaElement(mkvIDChapString, []byte("Chapter 1"))
	atom := buildMatroskaElement(mkvIDChapterAtom, append(buildMatroskaElement(mkvIDChapterTimeStart, []byte{0}), buildMatroskaElement(mkvIDChapterDisplay, display)...))
	chapters := buildMatroskaElement(mkvIDChapters, buildMatroskaElement(mkvIDEditionEntry, atom))
	base := buildMatroskaInfo()

	full, ok := parseMatroskaSegment(append(append([]byte{}, base...), chapters...))
	if !ok || !matroskaHasMenu(full.Tracks) {
		t.Fatalf("complete chapters did not produce Menu: ok=%v tracks=%+v", ok, full.Tracks)
	}
	partial, ok := parseMatroskaSegment(append(append([]byte{}, base...), chapters[:len(chapters)-1]...))
	if !ok || matroskaHasMenu(partial.Tracks) {
		t.Fatalf("partial chapters were treated as complete: ok=%v tracks=%+v", ok, partial.Tracks)
	}
}

func TestRestoreMatroskaRetainedFieldsKeepsChapterEditionsSeparate(t *testing.T) {
	first := appendMatroskaChapterMenus(nil, [][]matroskaChapter{{{startMs: 0, name: "First"}}})[0]
	second := appendMatroskaChapterMenus(nil, [][]matroskaChapter{{{startMs: 97, name: "Second"}}})[0]
	streams := []Stream{first, second}
	general := Stream{Kind: StreamGeneral}

	restoreMatroskaRetainedFields(&general, streams, "", matroskaRetainedGeneralPresence{})

	firstExtra := canonicalSeedStructuredNode(&streams[0], "extra")
	if firstExtra == nil || len(firstExtra.Object) != 1 || firstExtra.Object[0].Key != "_00_00_00_000" {
		t.Fatalf("first edition was contaminated: %#v", firstExtra)
	}
	secondExtra := canonicalSeedStructuredNode(&streams[1], "extra")
	if secondExtra == nil || len(secondExtra.Object) != 1 || secondExtra.Object[0].Key != "_00_00_00_097" {
		t.Fatalf("second edition changed: %#v", secondExtra)
	}
}

func TestParseHEVCTimeCodeRequiresCompleteClock(t *testing.T) {
	partial := make([]byte, 8)
	w := bitWriter{b: partial}
	w.writeBits(1, 2) // num_clock_ts
	w.writeBits(1, 1) // clock_timestamp_flag
	w.writeBits(0, 1) // units_field_based_flag
	w.writeBits(0, 5) // counting_type
	w.writeBits(0, 1) // full_timestamp_flag
	w.writeBits(0, 1) // discontinuity_flag
	w.writeBits(0, 1) // cnt_dropped_flag
	w.writeBits(12, 9)
	w.writeBits(0, 1) // seconds_flag: partial clock
	var info hevcHDRInfo
	parseHEVCTimeCode(partial, &info)
	if info.timeCode != "" {
		t.Fatalf("partial clock emitted as %q", info.timeCode)
	}

	complete := make([]byte, 8)
	w = bitWriter{b: complete}
	w.writeBits(1, 2)
	w.writeBits(1, 1)
	w.writeBits(0, 1)
	w.writeBits(0, 5)
	w.writeBits(1, 1) // full_timestamp_flag
	w.writeBits(0, 1)
	w.writeBits(0, 1)
	w.writeBits(12, 9)
	w.writeBits(3, 6)
	w.writeBits(2, 6)
	w.writeBits(1, 5)
	parseHEVCTimeCode(complete, &info)
	if info.timeCode != "01:02:03:12" {
		t.Fatalf("complete clock = %q", info.timeCode)
	}
}

func TestParsePNGAttachmentRejectsOverflowingChunkLength(t *testing.T) {
	data := make([]byte, 33)
	copy(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	binary.BigEndian.PutUint32(data[8:12], ^uint32(0))
	copy(data[12:16], "IHDR")
	if _, _, _, _, ok := parsePNGAttachment(data); ok {
		t.Fatal("overflowing PNG chunk length was accepted")
	}
}

func TestApplyMatroskaVideoProbesMasteringPrimariesSource(t *testing.T) {
	containerPrimaries := "R: x=0.680000 y=0.320000, G: x=0.265000 y=0.690000, B: x=0.150000 y=0.060000, White point: x=0.312700 y=0.329000"
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	replaceCanonicalSeedFill(&stream, "MasteringDisplay_ColorPrimaries", containerPrimaries, "Mastering display color primaries", containerPrimaries)
	info := MatroskaInfo{Tracks: []Stream{stream}}
	probes := map[uint64]*matroskaVideoProbe{1: {codec: "HEVC", hdrInfo: hevcHDRInfo{masteringPrimaries: "BT.2020"}}}
	applyMatroskaVideoProbes(&info, probes)
	primaries, _ := canonicalSeedValue(info.Tracks[0], "MasteringDisplay_ColorPrimaries")
	source, _ := canonicalSeedValue(info.Tracks[0], "MasteringDisplay_ColorPrimaries_Source")
	if primaries != containerPrimaries || source != "Container" {
		t.Fatalf("container primaries provenance mismatch: primaries=%q source=%q", primaries, source)
	}
}

func TestApplyMatroskaVideoProbesAFD8PreservesIncompatibleGeometry(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	replaceCanonicalSeedFill(&stream, "Width", "1920", "Width", "1 920 pixels")
	replaceCanonicalSeedFill(&stream, "Height", "1080", "Height", "1 080 pixels")
	replaceCanonicalSeedFill(&stream, "PixelAspectRatio", "1.000", "Pixel aspect ratio", "1.000")
	replaceCanonicalSeedFill(&stream, "DisplayAspectRatio", "1.778", "Display aspect ratio", "16:9")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "AVC", activeFormat: 8}})
	activeFormat, _ := canonicalSeedValue(info.Tracks[0], "ActiveFormatDescription")
	pixelAspectRatio, _ := canonicalSeedValue(info.Tracks[0], "PixelAspectRatio")
	displayAspectRatio, _ := canonicalSeedValue(info.Tracks[0], "DisplayAspectRatio")
	if activeFormat != "8" || pixelAspectRatio != "1.000" || displayAspectRatio != "1.778" {
		t.Fatalf("AFD 8 corrupted incompatible geometry: AFD=%q PAR=%q DAR=%q", activeFormat, pixelAspectRatio, displayAspectRatio)
	}

	stream = Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&stream, "ID", "1", "ID", "1")
	replaceCanonicalSeedFill(&stream, "Width", "2350", "Width", "2 350 pixels")
	replaceCanonicalSeedFill(&stream, "Height", "1000", "Height", "1 000 pixels")
	info.Tracks[0] = stream
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {codec: "AVC", activeFormat: 8}})
	pixelAspectRatio, _ = canonicalSeedValue(info.Tracks[0], "PixelAspectRatio")
	displayAspectRatio, _ = canonicalSeedValue(info.Tracks[0], "DisplayAspectRatio")
	if pixelAspectRatio != "0.999" || displayAspectRatio != "2.350" {
		t.Fatalf("cinema geometry did not retain parity override: PAR=%q DAR=%q", pixelAspectRatio, displayAspectRatio)
	}
}

func TestApplyMatroskaMPEG2ProbeEmitsDropFrameFlag(t *testing.T) {
	for _, drop := range []bool{false, true} {
		stream := Stream{Kind: StreamVideo, JSON: map[string]string{}}
		value := drop
		parser := mpeg2VideoParser{info: mpeg2VideoInfo{Version: "2", Width: 720, Height: 480, FrameRate: 30000.0 / 1001.0, TimeCode: "00:01:00:00", TimeCodeSource: "Group of pictures header", GOPDropFrame: &value}}
		applyMatroskaMPEG2Probe(&stream, &parser)
		want := "No"
		if drop {
			want = "Yes"
		}
		got, found := canonicalSeedValue(stream, "Delay_Original_DropFrame")
		if !found || got != want {
			t.Fatalf("drop=%v emitted %q, want %q", drop, got, want)
		}
	}
}

func TestApplyMatroskaMPEG2ProbeProjectsParsedStandard(t *testing.T) {
	stream := Stream{Kind: StreamVideo, JSON: map[string]string{}}
	parser := mpeg2VideoParser{info: mpeg2VideoInfo{Version: "2", Width: 1920, Standard: "Component"}}
	applyMatroskaMPEG2Probe(&stream, &parser)
	if got, found := canonicalSeedValue(stream, "Standard"); !found || got != "Component" {
		t.Fatalf("Standard = %q, found=%v; want Component", got, found)
	}
}

func TestApplyMatroskaMPEG2ProbeSuppressesMixedPictureStructure(t *testing.T) {
	for _, test := range []struct {
		name              string
		progressiveFrames int
		want              bool
	}{
		{name: "interlaced only", want: true},
		{name: "mixed pictures", progressiveFrames: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stream := Stream{Kind: StreamVideo, JSON: map[string]string{}}
			parser := mpeg2VideoParser{
				info: mpeg2VideoInfo{
					Version:          "2",
					Width:            720,
					Height:           576,
					ScanType:         "Interlaced",
					PictureStructure: "Frame",
				},
				progressiveFrames: test.progressiveFrames,
			}
			applyMatroskaMPEG2Probe(&stream, &parser)
			got, found := canonicalSeedValue(stream, "Format_Settings_PictureStructure")
			if found != test.want {
				t.Fatalf("picture structure found = %v, want %v (value %q)", found, test.want, got)
			}
			if found && got != "Frame" {
				t.Fatalf("picture structure = %q, want Frame", got)
			}
		})
	}
}

func minimalPNG(width, height uint32, colorType byte) []byte {
	data := make([]byte, 33)
	copy(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	binary.BigEndian.PutUint32(data[8:12], 13)
	copy(data[12:16], "IHDR")
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	data[24] = 8
	data[25] = colorType
	return data
}

func buildHEVCX265LengthPrefixedSample(t *testing.T) []byte {
	t.Helper()

	uuid := []byte{0x2C, 0xA2, 0xDE, 0x09, 0xB5, 0x17, 0x47, 0xDB, 0xBB, 0x55, 0xA4, 0xFE, 0x7F, 0xC2, 0xFC, 0x4E}
	text := "x265 (build 1) - 9.9 - H.265/HEVC codec - c - u - options: wpp 320 bitdepth=8 fps=2 me=0"
	body := append(append([]byte{}, uuid...), []byte(text)...)
	if len(body) > 254 {
		t.Fatalf("test SEI payload too large for single-byte size: %d", len(body))
	}
	nal := append([]byte{0x4E, 0x01, 0x05, byte(len(body))}, body...)
	return append([]byte{0x00, 0x00, 0x00, byte(len(nal))}, nal...)
}

func buildHEVCNonX265LengthPrefixedSample() []byte {
	nal := []byte{0x40, 0x01, 0x0C, 0x01}
	return append([]byte{0x00, 0x00, 0x00, byte(len(nal))}, nal...)
}

func mkvClusterWithSimpleBlocks(blocks ...[]byte) []byte {
	payload := buildMatroskaElement(mkvIDTimecode, []byte{0x00})
	for _, block := range blocks {
		payload = append(payload, buildMatroskaElement(mkvIDSimpleBlock, block)...)
	}
	return buildMatroskaElement(mkvIDCluster, payload)
}

func buildMatroskaSample() []byte {
	segment := append(
		buildMatroskaInfo(),
		buildMatroskaTracks()...,
	)
	segmentElem := append(buildMatroskaID(mkvIDSegment), buildMatroskaSize(uint64(len(segment)))...)
	segmentElem = append(segmentElem, segment...)
	return segmentElem
}

func TestParseMatroskaSegmentResolvesInfoBeforeTracks(t *testing.T) {
	segment := append(buildMatroskaTracks(), buildMatroskaInfo()...)
	info, ok := parseMatroskaSegment(segment)
	if !ok || len(info.Tracks) != 1 {
		t.Fatalf("segment parse = %v, tracks=%d", ok, len(info.Tracks))
	}
	if duration, found := canonicalSeedValue(info.Tracks[0], "Duration"); !found || duration == "" {
		t.Fatalf("video duration = %q, %v; want Segment Info-derived value", duration, found)
	}
	if frameCount, found := canonicalSeedValue(info.Tracks[0], "FrameCount"); !found || frameCount == "" {
		t.Fatalf("video frame count = %q, %v; want duration-derived value", frameCount, found)
	}
}

func buildMatroskaInfo() []byte {
	info := make([]byte, 0, 32)
	info = append(info, buildMatroskaElement(mkvIDTimecodeScale, []byte{0x0F, 0x42, 0x40})...)
	info = append(info, buildMatroskaElement(mkvIDDuration, []byte{0x41, 0x20, 0x00, 0x00})...)
	return buildMatroskaElement(mkvIDInfo, info)
}

func buildMatroskaTracks() []byte {
	trackEntry := buildMatroskaElement(mkvIDTrackType, []byte{0x01})
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41708333))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1000000))...)
	trackEntry = append(trackEntry, buildMatroskaVideoSettings(1920, 1080)...)
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
}

func buildMatroskaAudioTracks(trackUID uint64) []byte {
	trackEntry := buildMatroskaElement(mkvIDTrackType, []byte{0x02})
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(trackUID))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDCodecID, []byte("A_AAC"))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDTrackLanguage, []byte("und"))...)
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
}

func buildMatroskaVideoTrackWithUID(trackUID uint64) []byte {
	trackEntry := buildMatroskaElement(mkvIDTrackType, []byte{0x01})
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(trackUID))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	trackEntry = append(trackEntry, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41708333))...)
	trackEntry = append(trackEntry, buildMatroskaVideoSettings(1920, 1080)...)
	trackEntry = buildMatroskaElement(mkvIDTrackEntry, trackEntry)
	return buildMatroskaElement(mkvIDTracks, trackEntry)
}

func buildMatroskaTagForStats(trackUID uint64) []byte {
	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(trackUID)))
	body := append([]byte(nil), targets...)
	body = append(body, buildMatroskaSimpleTag("ENCODER", "Lavf60.3.100")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_TAGS", "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_WRITING_APP", "mkvmerge v82.0.0")...)
	body = append(body, buildMatroskaSimpleTag("_STATISTICS_WRITING_DATE_UTC", "2024-01-01 12:00:00")...)
	body = append(body, buildMatroskaSimpleTag("BPS", "166000")...)
	body = append(body, buildMatroskaSimpleTag("DURATION", "00:00:50.000000000")...)
	body = append(body, buildMatroskaSimpleTag("NUMBER_OF_FRAMES", "1200")...)
	body = append(body, buildMatroskaSimpleTag("NUMBER_OF_BYTES", "1048576")...)
	return buildMatroskaElement(mkvIDTag, body)
}

func buildMatroskaEncoderTag(trackUID uint64, encoder, settings string) []byte {
	body := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(trackUID)))
	if encoder != "" {
		body = append(body, buildMatroskaSimpleTag("ENCODER", encoder)...)
	}
	if settings != "" {
		body = append(body, buildMatroskaSimpleTag("ENCODER_SETTINGS", settings)...)
	}
	return buildMatroskaElement(mkvIDTag, body)
}

func buildMatroskaLanguageTag(trackUID uint64, language string) []byte {
	targets := buildMatroskaElement(mkvIDTagTargets, buildMatroskaElement(mkvIDTagTrackUID, encodeMatroskaUint(trackUID)))
	simple := buildMatroskaElement(mkvIDTagName, []byte("LANGUAGE"))
	simple = append(simple, buildMatroskaElement(mkvIDTagString, []byte("1"))...)
	simple = append(simple, buildMatroskaElement(mkvIDTagLanguage, []byte(language))...)
	body := append([]byte(nil), targets...)
	body = append(body, buildMatroskaElement(mkvIDSimpleTag, simple)...)
	return buildMatroskaElement(mkvIDTag, body)
}

func buildMatroskaSimpleTag(name, value string) []byte {
	body := buildMatroskaElement(mkvIDTagName, []byte(name))
	body = append(body, buildMatroskaElement(mkvIDTagString, []byte(value))...)
	return buildMatroskaElement(mkvIDSimpleTag, body)
}

func buildMatroskaVideoSettings(width, height uint64) []byte {
	video := make([]byte, 0, 24)
	video = append(video, buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(width))...)
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(height))...)
	return buildMatroskaElement(mkvIDTrackVideo, video)
}

func encodeMatroskaUint(value uint64) []byte {
	if value <= 0xFF {
		return []byte{byte(value)}
	}
	if value <= 0xFFFF {
		return []byte{byte(value >> 8), byte(value)}
	}
	if value <= 0xFFFFFF {
		return []byte{byte(value >> 16), byte(value >> 8), byte(value)}
	}
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func buildMatroskaElement(id uint64, payload []byte) []byte {
	buf := append(buildMatroskaID(id), buildMatroskaSize(uint64(len(payload)))...)
	buf = append(buf, payload...)
	return buf
}

func buildMatroskaSeekEntryForTest(targetID, position uint64) []byte {
	payload := buildMatroskaElement(mkvIDSeekID, buildMatroskaID(targetID))
	payload = append(payload, buildMatroskaElement(mkvIDSeekPosition, encodeMatroskaUint(position))...)
	return buildMatroskaElement(mkvIDSeek, payload)
}

func buildMatroskaID(id uint64) []byte {
	if id <= 0xFF {
		return []byte{byte(id)}
	}
	if id <= 0xFFFF {
		return []byte{byte(id >> 8), byte(id)}
	}
	if id <= 0xFFFFFF {
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	}
	return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
}

func buildMatroskaSize(size uint64) []byte {
	for length := 1; length <= 8; length++ {
		maxValue := (uint64(1) << (7 * length)) - 2
		if size > maxValue {
			continue
		}
		buf := make([]byte, length)
		value := size
		for i := length - 1; i >= 0; i-- {
			buf[i] = byte(value)
			value >>= 8
		}
		buf[0] |= 1 << (8 - length)
		return buf
	}
	panic("Matroska size exceeds finite VINT range")
}

type shortAtReader struct {
	io.ReaderAt
	offset int64
}

func (r shortAtReader) ReadAt(p []byte, off int64) (int, error) {
	if off != r.offset || len(p) < 2 {
		return r.ReaderAt.ReadAt(p, off)
	}
	n, _ := r.ReaderAt.ReadAt(p[:len(p)/2], off)
	return n, io.EOF
}

func TestMatroskaMPEG2CodecPrivatePrecedesClusterSequenceHeader(t *testing.T) {
	private := makeMPEG2SequenceHeaderWithFlatMatrices(8, 9)
	probe := newMatroskaMPEG2VideoProbe(Stream{mkvCodecPrivate: private})

	cluster := makeMPEG2SequenceHeaderWithFlatMatrices(5, 6)
	cluster = append(cluster, 0x00, 0x00, 0x01, 0xB7)
	probe.mpeg2.consume(cluster)

	want := strings.Repeat("08", 64) + " / " + strings.Repeat("09", 64)
	if got := probe.mpeg2.finalize().MatrixData; got != want {
		t.Fatalf("MatrixData = %q, want CodecPrivate value %q", got, want)
	}
}

func makeMPEG2SequenceHeaderWithFlatMatrices(intra, nonIntra uint32) []byte {
	payload := make([]byte, 136)
	writer := bitWriter{b: payload}
	writer.writeBits(720, 12)
	writer.writeBits(480, 12)
	writer.writeBits(3, 4)
	writer.writeBits(4, 4)
	writer.writeBits(10_000, 18)
	writer.writeBits(1, 1)
	writer.writeBits(112, 10)
	writer.writeBits(0, 1)
	writer.writeBits(1, 1)
	for range 64 {
		writer.writeBits(intra, 8)
	}
	writer.writeBits(1, 1)
	for range 64 {
		writer.writeBits(nonIntra, 8)
	}
	payload = payload[:(writer.bit+7)/8]
	return append([]byte{0x00, 0x00, 0x01, 0xB3}, payload...)
}

func TestParseMatroskaTagStatsDistrustsOlderDate(t *testing.T) {
	stats, ok := parseMatroskaTagStats(map[string]string{
		"_STATISTICS_TAGS":             "BPS DURATION NUMBER_OF_FRAMES NUMBER_OF_BYTES",
		"_STATISTICS_WRITING_APP":      "no_variable_data",
		"_STATISTICS_WRITING_DATE_UTC": "1970-01-01 00:00:00",
		"BPS":                          "7969978",
		"DURATION":                     "00:37:47.268000000",
		"NUMBER_OF_FRAMES":             "54360",
		"NUMBER_OF_BYTES":              "2258759550",
	}, "2010-02-22 21:41:29 UTC", "no_variable_data")
	if !ok || stats.trusted {
		t.Fatalf("expected distrusted stats, got ok=%v stats=%+v", ok, stats)
	}
	want := []jsonKV{
		{Key: "Statistics_Tags_Issue", Val: "no_variable_data 1970-01-01 00:00:00 / no_variable_data 2010-02-22 21:41:29"},
		{Key: "FromStats_BitRate", Val: "7969978"},
		{Key: "FromStats_Duration", Val: "00:37:47.268000000"},
		{Key: "FromStats_FrameCount", Val: "54360"},
		{Key: "FromStats_StreamSize", Val: "2258759550"},
	}
	if len(stats.extras) != len(want) {
		t.Fatalf("extras mismatch: %+v", stats.extras)
	}
	for i, kv := range want {
		if stats.extras[i] != kv {
			t.Fatalf("extras[%d] = %+v, want %+v", i, stats.extras[i], kv)
		}
	}
}

func TestFormatMatroskaDateUTCWrapsLikeMediaInfo(t *testing.T) {
	// mkvmerge "no_variable_data" writes the Unix epoch as a negative offset
	// from the 2001 epoch; MediaInfoLib's unsigned 32-bit decode wraps it.
	unixEpochOffset := uint64(978307200000000000)
	raw := -unixEpochOffset
	if got := formatMatroskaDateUTC(raw); got != "2010-02-22 21:41:29 UTC" {
		t.Fatalf("wrapped date = %q", got)
	}
	if got := formatMatroskaDateUTC(387252489000000000); got != "2013-04-10 02:08:09 UTC" {
		t.Fatalf("normal date = %q", got)
	}
}
