package mediainfo

import (
	"encoding/binary"
	"maps"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestMatroskaTrackIdentifiersRequireCanonicalValues(t *testing.T) {
	snapshotOnly := Stream{
		Fields: []Field{{Name: "ID", Value: "7"}},
		JSON:   map[string]string{"UniqueID": "9"},
	}
	if got := streamTrackNumber(snapshotOnly); got != 0 {
		t.Fatalf("snapshot-only TrackNumber = %d; want 0", got)
	}
	if got := streamTrackUID(snapshotOnly); got != 0 {
		t.Fatalf("snapshot-only TrackUID = %d; want 0", got)
	}

	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.DirectStructured("ID", "7")
	builder.DirectStructured("UniqueID", "9")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	if got := streamTrackNumber(stream); got != 7 {
		t.Fatalf("canonical TrackNumber = %d; want 7", got)
	}
	if got := streamTrackUID(stream); got != 9 {
		t.Fatalf("canonical TrackUID = %d; want 9", got)
	}
}

func TestMatroskaFallbackTypeOrderOmitsXMLChild(t *testing.T) {
	generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
	generalBuilder.DirectStructured("Format", "Matroska")
	generic := Stream{Kind: StreamAudio, canonicalSeed: matroskaFallbackAudioCanonicalSeed(matroskaFallbackAudioCanonicalFacts{format: "Audio", trackNumber: 2})}
	ac3 := Stream{Kind: StreamAudio, canonicalSeed: matroskaAC3CanonicalSeed(matroskaAC3CanonicalFacts{format: "AC-3", trackNumber: 3})}
	streams := []Stream{generic, ac3}
	applyMatroskaFallbackTypeOrderXMLCompatibility(streams)
	if !streams[0].canonicalPolicy.HideTypeOrderXML {
		t.Fatal("generic fallback did not retain XML type-order policy")
	}
	report := Report{Ref: "fallback.mkv", General: generalBuilder.Snapshot(canonicalStreamPolicy{}), Streams: streams}
	attachCanonicalStore(&report)
	if !report.General.reportStore.streams[1].HideTypeOrderXML {
		t.Fatal("field store lost XML type-order policy")
	}
	finalizeFieldStore(report.General.reportStore)
	for _, entry := range report.General.reportStore.streams[1].Fields {
		if firstNonEmpty(entry.StructuredKey, string(entry.Name)) == "@typeorder" && entry.Options.ShowXML {
			t.Fatalf("field store retained XML-visible type order: %#v", entry)
		}
	}
	if output := RenderXML([]Report{report}); strings.Contains(output, "<_typeorder>1</_typeorder>") {
		t.Fatalf("generic fallback leaked XML type-order child: %s", output)
	}
	if output := RenderJSON([]Report{report}); !strings.Contains(output, `"@typeorder":"1"`) {
		t.Fatalf("generic fallback lost JSON type order: %s", output)
	}
}

func TestMatroskaLegacyDurationRequiresCanonicalProjection(t *testing.T) {
	snapshotOnly := Stream{JSON: map[string]string{"Duration": "7.070"}}
	if got := matroskaProjectedDuration(snapshotOnly); got != "" {
		t.Fatalf("snapshot-only compatibility scalar = %q; want empty", got)
	}
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.DirectStructured("Duration", "7070")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	if got := matroskaProjectedDuration(stream); got != "7.070" {
		t.Fatalf("canonical compatibility scalar = %q; want 7.070", got)
	}
}

func TestMatroskaStreamDisplayRequiresCanonicalText(t *testing.T) {
	snapshotOnly := Stream{Fields: []Field{{Name: "Format", Value: "AAC"}}}
	if got := matroskaStreamDisplay(snapshotOnly, "Format"); got != "" {
		t.Fatalf("snapshot-only display = %q; want empty", got)
	}
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("Format", "AAC", "Format", "AAC")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	if got := matroskaStreamDisplay(stream, "Format"); got != "AAC" {
		t.Fatalf("canonical display = %q; want AAC", got)
	}
}

func TestMatroskaCanonicalFrameRatePrefersDirectRatio(t *testing.T) {
	snapshotOnly := Stream{Fields: []Field{{Name: "Frame rate", Value: "23.976 FPS"}}}
	if got, ok := matroskaCanonicalFrameRate(snapshotOnly); ok || got != 0 {
		t.Fatalf("snapshot-only frame rate = %v, %v; want 0, false", got, ok)
	}
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("FrameRate", "23.976")
	builder.DirectStructured("FrameRate_Num", "24000")
	builder.DirectStructured("FrameRate_Den", "1001")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	got, ok := matroskaCanonicalFrameRate(stream)
	if !ok || math.Abs(got-24000.0/1001.0) > 1e-12 {
		t.Fatalf("canonical frame rate = %.12f, %v; want %.12f, true", got, ok, 24000.0/1001.0)
	}
}

func TestStreamCanonicalBitRateUsesCanonicalPrecedence(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.DirectStructured("BitRate_Encoded", "384000")
	builder.DirectStructured("BitRate", "192000")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.JSON = map[string]string{"BitRate_Encoded": "1", "BitRate": "2"}

	got, ok := streamCanonicalBitRate(stream)
	if !ok || got != 384000 {
		t.Fatalf("streamCanonicalBitRate() = %v, %v; want 384000, true", got, ok)
	}
}

func TestStreamCanonicalBitRateRejectsSnapshotOnlyValues(t *testing.T) {
	stream := Stream{
		Kind:   StreamAudio,
		Fields: []Field{{Name: "Bit rate", Value: "192 kb/s"}},
		JSON:   map[string]string{"BitRate": "192000"},
	}

	if got, ok := streamCanonicalBitRate(stream); ok {
		t.Fatalf("streamCanonicalBitRate() = %v, true; want no canonical bitrate", got)
	}
}

func TestRemoveMatroskaCanonicalFieldPublishesThroughSnapshotRefresh(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("Format_Settings_GOP", "M=3, N=12", "Format settings, GOP", "M=3, N=12")
	stream := builder.Snapshot(canonicalStreamPolicy{})

	removeMatroskaCanonicalField(&stream, "Format_Settings_GOP", "Format settings, GOP")
	if _, found := canonicalSeedValue(stream, "Format_Settings_GOP"); found {
		t.Fatal("canonical Format_Settings_GOP remains after removal")
	}
	refreshCanonicalCompatibilitySnapshot(&stream)
	if stream.JSON["Format_Settings_GOP"] != "" {
		t.Fatalf("snapshot Format_Settings_GOP = %q; want removed", stream.JSON["Format_Settings_GOP"])
	}
	if got := findField(stream.Fields, "Format settings, GOP"); got != "" {
		t.Fatalf("snapshot Format settings, GOP = %q; want removed", got)
	}
}

func TestReplaceMatroskaCanonicalOverridesPublishesStableSnapshot(t *testing.T) {
	stream := Stream{Kind: StreamVideo}
	replaceMatroskaCanonicalOverrides(&stream, map[string]string{
		"Format":  "AVC",
		"BitRate": "192000",
	})

	if len(stream.canonicalSeed) != 2 || stream.canonicalSeed[0].StructuredKey != "BitRate" || stream.canonicalSeed[1].StructuredKey != "Format" {
		t.Fatalf("canonical override order = %#v; want BitRate, Format", stream.canonicalSeed)
	}
	refreshCanonicalCompatibilitySnapshot(&stream)
	for key, want := range map[string]string{"BitRate": "192000", "Format": "AVC"} {
		if got := stream.JSON[key]; got != want {
			t.Fatalf("snapshot %s = %q; want %q", key, got, want)
		}
	}
}

func TestReplaceMatroskaCanonicalJSONOnlyOverridesHidesXML(t *testing.T) {
	stream := Stream{Kind: StreamAudio}
	replaceMatroskaCanonicalJSONOnlyOverrides(&stream, map[string]string{
		"Video_Delay": "0.011",
		"Delay":       "0.011",
	})

	if len(stream.canonicalSeed) != 2 || stream.canonicalSeed[0].StructuredKey != "Delay" || stream.canonicalSeed[1].StructuredKey != "Video_Delay" {
		t.Fatalf("canonical JSON-only order = %#v; want Delay, Video_Delay", stream.canonicalSeed)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Options.ShowXML {
			t.Fatalf("JSON-only entry %q is visible in XML", entry.StructuredKey)
		}
	}
	refreshCanonicalCompatibilitySnapshot(&stream)
	for _, key := range []string{"Delay", "Video_Delay"} {
		if got := stream.JSON[key]; got != "0.011" {
			t.Fatalf("snapshot %s = %q; want 0.011", key, got)
		}
	}
}

func TestReplaceMatroskaCanonicalJSONOnlyProjectionKeepsRawUnits(t *testing.T) {
	stream := Stream{Kind: StreamGeneral}
	replaceMatroskaCanonicalJSONOnlyProjection(&stream, "Duration", "4021", "4.021")
	if got, found := canonicalSeedValue(stream, "Duration"); !found || got != "4021" {
		t.Fatalf("canonical Duration = %q, %v; want 4021, true", got, found)
	}
	if got, found := projectedCanonicalSeedValue(stream, "Duration"); !found || got != "4.021" {
		t.Fatalf("projected Duration = %q, %v; want 4.021, true", got, found)
	}
	refreshCanonicalCompatibilitySnapshot(&stream)
	if got := stream.JSON["Duration"]; got != "4.021" {
		t.Fatalf("snapshot Duration = %q; want 4.021", got)
	}
}

func TestSetMatroskaJSONExtrasPreservesCanonicalMemberOrder(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{
		{Key: "ComplexityIndex", Value: structuredNode{Kind: structuredString, Text: "12"}},
		{Key: "bsid", Value: structuredNode{Kind: structuredString, Text: "16"}},
		{Key: "compr_Average", Value: structuredNode{Kind: structuredString, Text: "1.00"}},
		{Key: "compr_Minimum", Value: structuredNode{Kind: structuredString, Text: "-1.00"}},
		{Key: "compr_Maximum", Value: structuredNode{Kind: structuredString, Text: "2.00"}},
		{Key: "compr_Count", Value: structuredNode{Kind: structuredString, Text: "10"}},
	}})
	stream := builder.Snapshot(canonicalStreamPolicy{})
	setMatroskaJSONExtras(&stream, map[string]string{
		"compr_Average": "1.19",
		"compr_Count":   "1179",
	})

	generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
	generalBuilder.DirectStructured("Format", "Matroska")
	report := Report{
		Ref:     "ordered-extra.mkv",
		General: generalBuilder.Snapshot(canonicalStreamPolicy{}),
		Streams: []Stream{stream},
	}
	attachCanonicalStore(&report)
	output := RenderJSON([]Report{report})
	want := `"extra":{"ComplexityIndex":"12","bsid":"16","compr_Average":"1.19","compr_Minimum":"-1.00","compr_Maximum":"2.00","compr_Count":"1179"}`
	if !strings.Contains(output, want) {
		t.Fatalf("canonical extra order changed: %s", output)
	}
}

func TestMatroskaWriterRulesIgnoreCorpusIdentities(t *testing.T) {
	generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
	generalBuilder.DirectStructured("UniqueID", "249145026190604183892181043117169235058")
	general := generalBuilder.Snapshot(canonicalStreamPolicy{})

	videoBuilder := newCanonicalStreamBuilder(StreamVideo)
	videoBuilder.DirectStructured("UniqueID", "1454056016244297151")
	videoBuilder.DirectStructured("BitRate", "1")
	trueHDBuilder := newCanonicalStreamBuilder(StreamAudio)
	trueHDBuilder.DirectStructured("UniqueID", "3")
	trueHDBuilder.DirectStructured("Format", "TrueHD")
	trueHDBuilder.DirectStructured("CodecID", "A_TRUEHD")
	streams := []Stream{
		videoBuilder.Snapshot(canonicalStreamPolicy{}),
		trueHDBuilder.Snapshot(canonicalStreamPolicy{}),
	}

	applyMatroskaWriterRules("", &general, streams)

	if got, found := canonicalSeedValue(streams[0], "BitRate"); !found || got != "1" {
		t.Fatalf("video BitRate = %q, %v; corpus identities must not replace parsed value", got, found)
	}
	if got, found := canonicalSeedValue(general, "FrameCount"); found {
		t.Fatalf("General FrameCount = %q; corpus identities must not inject values", got)
	}
	for _, key := range []fieldName{"BitDepth", "BitRate"} {
		if got, found := canonicalSeedValue(streams[1], key); found {
			t.Fatalf("TrueHD UID 3 %s = %q; identity must not inject metadata", key, got)
		}
	}
}

func TestMatroskaHandBrakeVFRRuleClearsDeferredTiming(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("Duration", "1537")
	builder.DirectStructured("FrameCount", "46053")
	builder.DirectStructured("FrameRate_Mode_Original", "VFR")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.matroskaDeferredFacts = &matroskaDeferredFacts{}
	for name, value := range map[fieldName]string{
		"Duration": "1537", "FrameCount": "46053", "FrameRate_Mode_Original": "VFR",
	} {
		stream.matroskaDeferredFacts.Set(name, value)
	}
	streams := []Stream{stream}
	general := Stream{Kind: StreamGeneral}

	applyMatroskaWriterRules("HandBrake 1.3.3 2020061300", &general, streams)
	streams[0].matroskaDeferredFacts.ApplyToStream(&streams[0])

	for _, name := range []fieldName{"Duration", "FrameCount", "FrameRate_Mode_Original"} {
		if got, found := canonicalSeedValue(streams[0], name); found {
			t.Fatalf("deferred %s survived VFR cleanup as %q", name, got)
		}
	}
	if got, found := canonicalSeedValue(streams[0], "FrameRate_Mode"); !found || got != "VFR" {
		t.Fatalf("FrameRate_Mode = %q, %v; want VFR", got, found)
	}
}

func TestMatroskaLavfIntegralDurationRuleKeepsBitRateNominal(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("BitRate", "6000000")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.matroskaDeferredFacts = &matroskaDeferredFacts{}
	stream.matroskaDeferredFacts.Set("BitRate", "6000000")
	streams := []Stream{stream}
	general := Stream{Kind: StreamGeneral}

	applyMatroskaWriterRules("Lavf59.27.100", &general, streams)
	streams[0].matroskaDeferredFacts.ApplyToStream(&streams[0])

	if got, found := canonicalSeedValue(streams[0], "BitRate"); found {
		t.Fatalf("deferred BitRate survived as %q", got)
	}
	if got, found := canonicalSeedValue(streams[0], "BitRate_Nominal"); !found || got != "6000000" {
		t.Fatalf("BitRate_Nominal = %q, %v", got, found)
	}
}

func TestMatroskaHandBrakeHEVCKeepsStatisticsDurationPrecision(t *testing.T) {
	videoBuilder := newCanonicalStreamBuilder(StreamVideo)
	videoBuilder.DirectStructured("Format", "HEVC")
	videoBuilder.DirectStructured("Duration", "1605560")
	videoBuilder.SetStructuredDecimals("Duration", 9)
	audioBuilder := newCanonicalStreamBuilder(StreamAudio)
	audioBuilder.DirectStructured("Format", "AC-3")
	audioBuilder.DirectStructured("Duration", "1605568")
	audioBuilder.SetStructuredDecimals("Duration", 9)
	streams := []Stream{
		videoBuilder.Snapshot(canonicalStreamPolicy{}),
		audioBuilder.Snapshot(canonicalStreamPolicy{}),
	}
	general := Stream{Kind: StreamGeneral}

	applyMatroskaWriterRules("HandBrake 1.3.3 2020061300", &general, streams)

	for index, want := range []string{"1605.560000000", "1605.568000000"} {
		if got, found := projectedCanonicalSeedValue(streams[index], "Duration"); !found || got != want {
			t.Fatalf("stream %d Duration = %q, %v; want %q", index, got, found, want)
		}
	}
}

func TestDeriveMatroskaAudioFrameCountsRequiresExactCodecEvidence(t *testing.T) {
	ac3Builder := newCanonicalStreamBuilder(StreamAudio)
	ac3Builder.DirectStructured("Format", "AC-3")
	ac3Builder.DirectStructured("Duration", "4812288")
	ac3Builder.SetStructuredDecimals("Duration", 3)
	ac3Builder.DirectStructured("FrameRate", "31.250")
	ac3 := ac3Builder.Snapshot(canonicalStreamPolicy{})

	aacBuilder := newCanonicalStreamBuilder(StreamAudio)
	aacBuilder.DirectStructured("Format", "AAC")
	aacBuilder.DirectStructured("Duration", "4812288")
	aacBuilder.DirectStructured("FrameRate", "46.875")
	aac := aacBuilder.Snapshot(canonicalStreamPolicy{})

	tracks := []Stream{ac3, aac}
	deriveMatroskaAudioFrameCounts(tracks)

	if got, found := canonicalSeedValue(tracks[0], "FrameCount"); found {
		t.Fatalf("AC-3 FrameCount was synthesized from rounded timing: %q", got)
	}
	if _, found := canonicalSeedValue(tracks[1], "FrameCount"); found {
		t.Fatal("AAC FrameCount was synthesized")
	}
}

func TestDeriveMatroskaVideoBitRateUsesCanonicalGeneralRate(t *testing.T) {
	generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
	generalBuilder.DirectStructured("OverallBitRate", "1000000")
	general := generalBuilder.Snapshot(canonicalStreamPolicy{})
	general.JSON = map[string]string{"OverallBitRate": "1"}

	videoBuilder := newCanonicalStreamBuilder(StreamVideo)
	videoBuilder.DirectStructured("FrameCount", "100")
	videoBuilder.DirectStructured("FrameRate", "25")
	audioBuilder := newCanonicalStreamBuilder(StreamAudio)
	audioBuilder.DirectStructured("BitRate", "100000")
	streams := []Stream{
		videoBuilder.Snapshot(canonicalStreamPolicy{}),
		audioBuilder.Snapshot(canonicalStreamPolicy{}),
	}
	deriveMatroskaVideoBitRateAndSize(general, streams, 1_000_000)

	if got, found := canonicalSeedValue(streams[0], "BitRate"); !found || got == "" {
		t.Fatalf("derived canonical video BitRate = %q, %v; want a positive value", got, found)
	}
	if got, found := canonicalSeedValue(streams[0], "StreamSize"); !found || got == "" {
		t.Fatalf("derived canonical video StreamSize = %q, %v; want a positive value", got, found)
	}
}

func TestMatroskaPayloadStreamSizesKnownRequiresCanonicalSize(t *testing.T) {
	snapshotOnly := Stream{Kind: StreamAudio, JSON: map[string]string{"StreamSize": "100"}}
	if matroskaPayloadStreamSizesKnown([]Stream{snapshotOnly}) {
		t.Fatal("snapshot-only StreamSize was accepted as canonical")
	}
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.DirectStructured("StreamSize", "100")
	if !matroskaPayloadStreamSizesKnown([]Stream{builder.Snapshot(canonicalStreamPolicy{})}) {
		t.Fatal("canonical StreamSize was rejected")
	}
}

func TestMatroskaAttachmentImageSeedsDirectCanonicalFields(t *testing.T) {
	attachment := matroskaAttachment{
		name:     "cover.png",
		mime:     "image/png",
		data:     minimalPNG(16, 9, 2),
		size:     int64(len(minimalPNG(16, 9, 2))),
		complete: true,
	}
	stream, ok := matroskaAttachmentImageStream(attachment)
	if !ok {
		t.Fatal("attachment did not produce an Image stream")
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}
	for key, want := range map[fieldName]string{
		"Format":           "PNG",
		"Width":            "16",
		"Height":           "9",
		"MuxingMode":       "Attachment",
		"StreamSize":       stream.JSON["StreamSize"],
		"ColorSpace":       "RGB",
		"Compression_Mode": "Lossless",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
}

func TestMatroskaChapterMenuSeedsOrderedCanonicalExtra(t *testing.T) {
	chapters := []matroskaChapter{
		{startMs: 0, name: "Opening", lang: "en"},
		{startMs: 62_345},
	}
	streams := appendMatroskaChapterMenus(nil, [][]matroskaChapter{chapters})
	if len(streams) != 1 {
		t.Fatalf("Menu streams = %d, want 1", len(streams))
	}
	stream := streams[0]
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || extra.Kind != structuredObject || len(extra.Object) != 2 {
		t.Fatalf("canonical extra = %#v", extra)
	}
	if extra.Object[0].Key != "_00_00_00_000" || extra.Object[0].Value.Text != "en:Opening" {
		t.Fatalf("first chapter = %#v", extra.Object[0])
	}
	if extra.Object[1].Key != "_00_01_02_345" || extra.Object[1].Value.Text != "Chapter 2" {
		t.Fatalf("second chapter = %#v", extra.Object[1])
	}
}

func TestMatroskaChapterMenuCanonicalExtraKeepsEditionsSeparate(t *testing.T) {
	streams := appendMatroskaChapterMenus(nil, [][]matroskaChapter{
		{{startMs: 0, name: "First edition"}},
		{{startMs: 1_000, name: "Second edition"}},
	})
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	restoreMatroskaRetainedFields(&general, streams, "", matroskaRetainedGeneralPresence{})

	extra := matroskaCanonicalSeedNode(streams[0], "extra")
	if extra == nil || len(extra.Object) != 1 {
		t.Fatalf("first-edition canonical extra = %#v", extra)
	}
	if extra.Object[0].Value.Text != "First edition" {
		t.Fatalf("first edition = %#v", extra.Object)
	}
	second := matroskaCanonicalSeedNode(streams[1], "extra")
	if second == nil || len(second.Object) != 1 || second.Object[0].Value.Text != "Second edition" {
		t.Fatalf("second edition = %#v", second)
	}
}

func TestMatroskaTextTrackSeedsDirectCanonicalFields(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(17))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("S_TEXT/ASS"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Signs"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguageIETF, []byte("ja-JP"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagHearingImpaired, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagForced, encodeMatroskaUint(1))...)

	stream, ok := parseMatroskaTrackEntry(payload, 0, 3)
	if !ok {
		t.Fatal("text TrackEntry did not parse")
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}
	for key, want := range map[fieldName]string{
		"Format":           "ASS",
		"ID":               "2",
		"UniqueID":         "123",
		"CodecID":          "S_TEXT/ASS",
		"Compression_Mode": "Lossless",
		"Title":            "Signs",
		"Language":         "ja-JP",
		"ServiceKind":      "HI",
		"Default":          "Yes",
		"Forced":           "Yes",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
}

func TestMatroskaTextCanonicalSeedTracksStatsAndTags(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(17))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("S_TEXT/UTF8"))...)
	stream, ok := parseMatroskaTrackEntry(payload, 0, 3)
	if !ok {
		t.Fatal("text TrackEntry did not parse")
	}
	set := &matroskaTagSet{}
	set.set(matroskaTagField{rawName: "COMMENT", name: "Comment", value: "translated"})
	info := MatroskaInfo{
		Tracks:     []Stream{stream},
		scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{123: set}},
	}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, dataBytes: 512, hasDataBytes: true,
		durationSeconds: 2.5, hasDuration: true, frameCount: 5, hasFrameCount: true,
		bitRate: 1638, hasBitRate: true, source: "Blu-ray", hasSource: true,
		extras: []jsonKV{{Key: "SOURCE", Val: "Blu-ray"}},
	}}, 4096)
	applyMatroskaTrackTags(&info)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Duration":     "2500",
		"StreamSize":   "512",
		"BitRate":      "1638",
		"FrameRate":    "2.000",
		"FrameCount":   "5",
		"ElementCount": "5",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 3 {
		t.Fatalf("canonical extra = %#v", extra)
	}
	if extra.Object[0].Key != "SOURCE" || extra.Object[1].Key != "Source" || extra.Object[2].Key != "Comment" {
		t.Fatalf("canonical extra order = %#v", extra.Object)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-text.mkv")
}

func TestMatroskaPCMTrackSeedsDirectCanonicalFields(t *testing.T) {
	codecPrivate := make([]byte, 16)
	binary.LittleEndian.PutUint16(codecPrivate[0:2], 1)
	binary.LittleEndian.PutUint16(codecPrivate[2:4], 2)
	binary.LittleEndian.PutUint32(codecPrivate[4:8], 48_000)
	binary.LittleEndian.PutUint32(codecPrivate[8:12], 192_000)
	binary.LittleEndian.PutUint16(codecPrivate[12:14], 4)
	binary.LittleEndian.PutUint16(codecPrivate[14:16], 16)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(456))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_MS/ACM"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(20_000_000))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Main audio"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguageIETF, []byte("en-US"))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
	if !ok {
		t.Fatal("PCM TrackEntry did not parse")
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}
	for key, want := range map[fieldName]string{
		"Format":                     "PCM",
		"ID":                         "2",
		"UniqueID":                   "456",
		"CodecID":                    "A_MS/ACM / 00000001-0000-0010-8000-00AA00389B71",
		"Duration":                   "2000",
		"BitRate_Mode":               "Constant",
		"BitRate":                    "1536000",
		"Channels":                   "2",
		"ChannelLayout":              "L R",
		"ChannelPositions":           "Front: L R",
		"SamplingRate":               "48000",
		"SamplingCount":              "96000",
		"BitDepth":                   "16",
		"Format_Settings_Endianness": "Little",
		"Format_Settings_Sign":       "Signed",
		"Delay":                      "0.000",
		"Delay_Source":               "Container",
		"Video_Delay":                "0.000",
		"Title":                      "Main audio",
		"Language":                   "en-US",
		"Default":                    "Yes",
		"Forced":                     "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	for _, key := range []fieldName{"FrameCount", "FrameRate", "SamplesPerFrame"} {
		if got, found := canonicalSeedValue(stream, key); found || got != "" {
			t.Fatalf("canonical %s = %q, %v; want absent without parsed block timing", key, got, found)
		}
	}
	if got := matroskaStreamDisplay(stream, "Bit rate mode"); got != "" {
		t.Fatalf("PCM text Bit rate mode = %q; want structured-only mode", got)
	}

	sortFields(stream.Kind, stream.Fields)
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-pcm.mkv")
}

func TestMatroskaWAVEFORMATEXTENSIBLEPCMUsesSubFormat(t *testing.T) {
	codecPrivate := make([]byte, 40)
	binary.LittleEndian.PutUint16(codecPrivate[0:2], 0xfffe)
	binary.LittleEndian.PutUint16(codecPrivate[2:4], 6)
	binary.LittleEndian.PutUint32(codecPrivate[4:8], 48_000)
	binary.LittleEndian.PutUint32(codecPrivate[8:12], 576_000)
	binary.LittleEndian.PutUint16(codecPrivate[12:14], 12)
	binary.LittleEndian.PutUint16(codecPrivate[14:16], 16)
	binary.LittleEndian.PutUint16(codecPrivate[16:18], 22)
	binary.LittleEndian.PutUint16(codecPrivate[18:20], 16)
	binary.LittleEndian.PutUint32(codecPrivate[20:24], 0x060f)
	copy(codecPrivate[24:], []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71})

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(456))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_MS/ACM"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)

	stream, ok := parseMatroskaTrackEntry(payload, 60, 3)
	if !ok {
		t.Fatal("WAVEFORMATEXTENSIBLE PCM TrackEntry did not parse")
	}
	for key, want := range map[fieldName]string{
		"Format":                     "PCM",
		"CodecID":                    "A_MS/ACM / 00000001-0000-0010-8000-00AA00389B71",
		"BitRate":                    "4608000",
		"Channels":                   "6",
		"ChannelLayout":              "L R C LFE Ls Rs",
		"SamplingRate":               "48000",
		"BitDepth":                   "16",
		"Format_Settings_Endianness": "Little",
		"Format_Settings_Sign":       "Signed",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
}

func TestMatroskaPCMCanonicalSeedTracksStatsAndTags(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDAudioBitDepth, encodeMatroskaUint(16))...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(456))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_PCM/INT/LIT"))...)
	payload = append(payload, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1_535_980))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
	if !ok {
		t.Fatal("PCM TrackEntry did not parse")
	}
	if got, found := canonicalSeedValue(stream, "BitRate"); !found || got != "1536000" {
		t.Fatalf("canonical PCM BitRate = %q, %v; want 1536000, true", got, found)
	}
	set := &matroskaTagSet{}
	set.set(matroskaTagField{rawName: "COMMENT", name: "Comment", value: "archival"})
	info := MatroskaInfo{
		Tracks:     []Stream{stream},
		scopedTags: matroskaScopedTags{tracks: map[uint64]*matroskaTagSet{456: set}},
	}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{456: {
		trusted: true, dataBytes: 384_000, hasDataBytes: true,
		durationSeconds: 2.5, durationPrec: 6, hasDuration: true,
		bitRate: 1_536_000, hasBitRate: true, source: "Blu-ray", hasSource: true,
		extras: []jsonKV{{Key: "SOURCE", Val: "Blu-ray"}},
	}}, 4_096_000)
	applyMatroskaTrackTags(&info)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Duration":   "2500",
		"StreamSize": "384000",
		"BitRate":    "1536000",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 3 {
		t.Fatalf("canonical extra = %#v", extra)
	}
	if extra.Object[0].Key != "SOURCE" || extra.Object[1].Key != "Source" || extra.Object[2].Key != "Comment" {
		t.Fatalf("canonical extra order = %#v", extra.Object)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-pcm-stats.mkv")
}

func TestMatroskaVorbisTrackSeedsDirectCanonicalFields(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(3))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(789))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_VORBIS"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, buildMatroskaVorbisPrivateForTest())...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Commentary"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguage, []byte("eng"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagDefault, encodeMatroskaUint(0))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
	if !ok {
		t.Fatal("Vorbis TrackEntry did not parse")
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}
	for key, want := range map[fieldName]string{
		"Format":                  "Vorbis",
		"ID":                      "3",
		"UniqueID":                "789",
		"CodecID":                 "A_VORBIS",
		"Duration":                "2000",
		"BitRate_Mode":            "Variable",
		"BitRate":                 "96000",
		"BitRate_Minimum":         "48000",
		"BitRate_Maximum":         "160000",
		"StreamSize":              "24000",
		"Channels":                "2",
		"SamplingRate":            "48000",
		"SamplingCount":           "96000",
		"Compression_Mode":        "Lossy",
		"Format_Settings_Floor":   "1",
		"Delay":                   "0.000",
		"Delay_Source":            "Container",
		"Video_Delay":             "0.000",
		"Title":                   "Commentary",
		"Encoded_Application":     "Made with BeSweet v1.5b31",
		"Encoded_Library":         "Xiph.Org libVorbis I 20020717",
		"Encoded_Library_Name":    "libVorbis",
		"Encoded_Library_Version": "1.0",
		"Encoded_Library_Date":    "2002-07-17",
		"Language":                "en",
		"Default":                 "No",
		"Forced":                  "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 1 || extra.Object[0].Key != "Encoded_Application_Url" || extra.Object[0].Value.Text != "http://DSPguru.doom9.org" {
		t.Fatalf("canonical extra = %#v", extra)
	}

	sortFields(stream.Kind, stream.Fields)
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-vorbis.mkv")
}

func TestMatroskaVorbisCanonicalSeedTracksParsedLibraryMetadata(t *testing.T) {
	stream := Stream{
		Kind: StreamAudio,
		JSON: map[string]string{"UniqueID": "3350138809"},
		canonicalSeed: matroskaVorbisCanonicalSeed(matroskaVorbisCanonicalFacts{
			codecID: "A_VORBIS", trackUID: 3350138809, defaultValue: true,
			codec: matroskaVorbisInfo{vendor: "Xiph.Org libVorbis I 20090709"},
		}),
	}
	got, found := projectedCanonicalSeedValue(stream, "Encoded_Library_Version")
	if !found || got != "20090709" {
		t.Fatalf("canonical Encoded_Library_Version = %q, %v; want 20090709", got, found)
	}
}

func TestMatroskaFLACTrackSeedsDirectCanonicalFields(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_FLAC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, buildMatroskaFLACPrivateForTest("reference libFLAC 1.2.1 20070917"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Main audio"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguageIETF, []byte("en-US"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagOriginal, encodeMatroskaUint(1))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
	if !ok {
		t.Fatal("FLAC TrackEntry did not parse")
	}
	for key, want := range map[fieldName]string{
		"Format":                  "FLAC",
		"ID":                      "2",
		"UniqueID":                "123",
		"CodecID":                 "A_FLAC",
		"Duration":                "2000",
		"BitRate_Mode":            "Variable",
		"Channels":                "2",
		"ChannelPositions":        "Front: L R",
		"ChannelLayout":           "L R",
		"SamplesPerFrame":         "4096",
		"SamplingRate":            "48000",
		"SamplingCount":           "96000",
		"FrameRate":               "11.719",
		"FrameCount":              "24",
		"BitDepth":                "16",
		"BitDepth_Detected":       "16",
		"Compression_Mode":        "Lossless",
		"Delay":                   "0.000",
		"Delay_Source":            "Container",
		"Video_Delay":             "0.000",
		"Title":                   "Main audio",
		"Encoded_Library":         "reference libFLAC 1.2.1 20070917",
		"Encoded_Library_Name":    "libFLAC",
		"Encoded_Library_Version": "1.2.1",
		"Encoded_Library_Date":    "2007-09-17",
		"Language":                "en-US",
		"ServiceKind":             "O",
		"Default":                 "Yes",
		"Forced":                  "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 1 || extra.Object[0].Key != "MD5_Unencoded" || extra.Object[0].Value.Text != "864D55F003143D8BAD47D3B997FAE64C" {
		t.Fatalf("canonical extra = %#v", extra)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	sortFields(stream.Kind, stream.Fields)
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-flac.mkv")
}

func TestMatroskaFLACCanonicalSeedTracksStatsAndEncoderTags(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_FLAC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, buildMatroskaFLACPrivateForTest(""))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 0, 3)
	if !ok {
		t.Fatal("FLAC TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, dataBytes: 50_000, hasDataBytes: true,
		durationSeconds: 2.5, durationPrec: 9, hasDuration: true,
		source: "Blu-ray", hasSource: true,
	}}, 4_096_000)
	applyMatroskaEncoders(info.Tracks, map[uint64]string{123: "Lavc58.91.100 flac"}, nil)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Duration":         "2500",
		"BitRate":          "160000",
		"Channels":         "2",
		"ChannelPositions": "Front: L R",
		"ChannelLayout":    "L R",
		"SamplingCount":    "120000",
		"FrameCount":       "30",
		"StreamSize":       "50000",
		"Encoded_Library":  "Lavc58.91.100 flac",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	if got, found := canonicalSeedValue(stream, "BitDepth_Detected"); found {
		t.Fatalf("canonical BitDepth_Detected retained removed value %q", got)
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 2 || extra.Object[0].Key != "Source" || extra.Object[1].Key != "MD5_Unencoded" {
		t.Fatalf("canonical extra order = %#v", extra)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-flac-stats.mkv")
}

func TestMatroskaMPEGAudioCanonicalSeedTracksBoundedProbe(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(24_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(456))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_MPEG/L3"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Commentary"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguage, []byte("eng"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagDefault, encodeMatroskaUint(0))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.4, 3)
	if !ok {
		t.Fatal("MPEG Audio TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{2: {
		format: "MPEG Audio", ok: true,
		mp3: mp3HeaderInfo{
			bitrateKbps: 64, sampleRate: 24_000, channels: 2,
			channelMode: 1, modeExt: 2, versionID: 0, layerID: 1,
		},
		mp3Library: "LAME3.99r", mp3AudioFrameSeen: true, mp3FrameCount: 100, mp3PayloadBytes: 19_200,
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                        "MPEG Audio",
		"ID":                            "2",
		"UniqueID":                      "456",
		"Format_Version":                "2",
		"Format_Profile":                "Layer 3",
		"Format_Settings_Mode":          "Joint stereo",
		"Format_Settings_ModeExtension": "MS Stereo",
		"CodecID":                       "A_MPEG/L3",
		"Duration":                      "2400",
		"BitRate_Mode":                  "Constant",
		"BitRate":                       "64000",
		"Channels":                      "2",
		"SamplesPerFrame":               "576",
		"SamplingRate":                  "24000",
		"SamplingCount":                 "57600",
		"FrameRate":                     "41.667",
		"FrameCount":                    "100",
		"Compression_Mode":              "Lossy",
		"Delay":                         "0.000",
		"Delay_Source":                  "Container",
		"Video_Delay":                   "0.000",
		"StreamSize":                    "19200",
		"Title":                         "Commentary",
		"Encoded_Library":               "LAME3.99r",
		"Encoded_Library_Settings":      "-m j -V 4 -q 2 -lowpass 11 -b 64",
		"Language":                      "en",
		"Default":                       "No",
		"Forced":                        "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-mpeg-audio.mkv")
}

func TestMatroskaMPEGAudioCanonicalSeedDerivesLayerTwoTiming(t *testing.T) {
	stream := Stream{
		Kind: StreamAudio,
		canonicalSeed: matroskaMPEGAudioCanonicalSeed(matroskaMPEGAudioCanonicalFacts{
			codecID: "A_MPEG/L2", trackUID: 42,
			audioChannels: 2, audioSampleRate: 48_000, structuredDuration: 811.632, defaultValue: true,
		}),
	}
	for key, want := range map[fieldName]string{
		"Format_Version": "1", "Format_Profile": "Layer 2", "SamplesPerFrame": "1152",
		"FrameRate": "41.667", "Compression_Mode": "Lossy",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
}

func TestNormalizeMatroskaDeclaredFrameRatesUsesSurvivingRatio(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("FrameRate_Mode", "CFR")
	builder.DirectStructured("FrameRate", "23.974")
	builder.DirectStructured("FrameRate_Num", "23976")
	builder.DirectStructured("FrameRate_Den", "1000")
	builder.DirectStructured("FrameRate_Original", "24.000")
	streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}

	normalizeMatroskaDeclaredFrameRates(streams)

	if got, found := canonicalSeedValue(streams[0], "FrameRate"); !found || got != "23.976" {
		t.Fatalf("FrameRate = %q, %v; want 23.976, true", got, found)
	}
}

func TestNormalizeMatroskaMPEG4VisualSettingsSeparatesRawAndDisplayValues(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.DirectStructured("Format", "MPEG-4 Visual")
	builder.Fill("Format_Settings_GMC", "No", "Format settings, GMC", "No")
	streams := []Stream{builder.Snapshot(canonicalStreamPolicy{})}

	normalizeMatroskaMPEG4VisualSettings(streams)

	if got, found := canonicalSeedValue(streams[0], "Format_Settings_GMC"); !found || got != "0" {
		t.Fatalf("Format_Settings_GMC = %q, %v; want 0, true", got, found)
	}
	if got, found := canonicalSeedTextValue(streams[0], "Format settings, GMC"); !found || got != "No" {
		t.Fatalf("Format settings, GMC = %q, %v; want No, true", got, found)
	}
}

func TestMatroskaTrueHDCanonicalSeedTracksAtmosProbe(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(8))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(789))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_TRUEHD"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Main audio"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguage, []byte("eng"))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("TrueHD TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{2: {
		format: "TrueHD", ok: true,
		truehd: trueHDInfo{
			atmos: true, dynamicObjects: 11, sampleRate: 48_000,
			samplesPerFrame: 40, maxBitRate: 18_000_000,
		},
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                    "MLP FBA",
		"ID":                        "2",
		"UniqueID":                  "789",
		"Format_Commercial_IfAny":   "Dolby TrueHD with Dolby Atmos",
		"Format_AdditionalFeatures": "16-ch",
		"CodecID":                   "A_TRUEHD",
		"Duration":                  "2500",
		"BitRate_Mode":              "Variable",
		"BitRate_Maximum":           "18000000",
		"Channels":                  "8",
		"ChannelLayout":             "L R C LFE Ls Rs Lb Rb",
		"ChannelPositions":          "Front: L C R, Side: L R, Back: L R, LFE",
		"SamplesPerFrame":           "40",
		"SamplingRate":              "48000",
		"SamplingCount":             "120000",
		"FrameRate":                 "1200.000",
		"FrameRate_Num":             "1200",
		"FrameRate_Den":             "1",
		"FrameCount":                "3000",
		"Compression_Mode":          "Lossless",
		"Delay":                     "0.000",
		"Delay_Source":              "Container",
		"Video_Delay":               "0.000",
		"Title":                     "Main audio",
		"Language":                  "en",
		"Default":                   "Yes",
		"Forced":                    "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 3 || extra.Object[0].Key != "NumberOfDynamicObjects" || extra.Object[1].Key != "BedChannelCount" || extra.Object[2].Key != "BedChannelConfiguration" {
		t.Fatalf("canonical extra = %#v", extra)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-truehd.mkv")
}

func TestMatroskaDTSCanonicalSeedTracksHDProbe(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(4))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(789))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_DTS"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("DTS TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{2: {
		format: "DTS", ok: true,
		dts: dtsInfo{
			sampleRate: 48_000, samplesPerFrame: 512, channels: 4, bitDepth: 24,
			hd: true, hdXLL: true, hdBitDepth: 24, hdChannels: 4,
			hdSpeakerMask: 0x13, hasSpeakerMask: true,
		},
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                     "DTS",
		"ID":                         "2",
		"UniqueID":                   "789",
		"Format_AdditionalFeatures":  "XLL",
		"Format_Commercial_IfAny":    "DTS-HD Master Audio",
		"Format_Settings_Mode":       "16",
		"Format_Settings_Endianness": "Big",
		"CodecID":                    "A_DTS",
		"Duration":                   "2500",
		"BitRate_Mode":               "Variable",
		"Channels":                   "4",
		"ChannelLayout":              "C L R Cb",
		"ChannelPositions":           "Front: L C R, Back: C",
		"SamplesPerFrame":            "512",
		"SamplingRate":               "48000",
		"SamplingCount":              "120000",
		"FrameRate":                  "93.750",
		"BitDepth":                   "24",
		"Compression_Mode":           "Lossless",
		"Delay":                      "0.000",
		"Delay_Source":               "Container",
		"Video_Delay":                "0.000",
		"Default":                    "Yes",
		"Forced":                     "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	if got := matroskaCanonicalSeedText(stream, "Channel layout"); got != "C L R Cs" {
		t.Fatalf("canonical text Channel layout = %q, want C L R Cs", got)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-dts-hd.mkv")
}

func TestMatroskaDTSCanonicalSeedTracksLBRProjection(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(321))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_DTS"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("DTS LBR TrackEntry did not parse")
	}
	stream.Fields = setFieldValue(stream.Fields, "Bit rate", "192 kb/s")
	stream.JSON["BitRate"] = "192000"
	stream.JSON["BitRate_Mode"] = "CBR"
	replaceCanonicalSeedFill(&stream, "BitRate", "192000", "Bit rate", "192 kb/s")
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{2: {
		format: "DTS", ok: true,
		dts: dtsInfo{
			sampleRate: 48_000, samplesPerFrame: 4096, channels: 2, bitDepth: 24,
			lbr: true, lbrLayout: "L R", lbrPositions: "Front: L R",
		},
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	if got, found := canonicalSeedValue(stream, "BitRate_Mode"); !found || got != "Constant" {
		t.Fatalf("canonical BitRate_Mode = %q, %v; want Constant", got, found)
	}
	if got := matroskaCanonicalSeedText(stream, "Bit rate mode"); got != "" {
		t.Fatalf("canonical text Bit rate mode = %q, want omitted", got)
	}
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-dts-lbr.mkv")
}

func TestMatroskaDTSCanonicalProbeMovesCoreESLayoutToOriginal(t *testing.T) {
	stream := Stream{Kind: StreamAudio, canonicalSeed: matroskaDTSCanonicalSeed(matroskaDTSCanonicalFacts{
		audioChannels: 6, audioSampleRate: 48_000,
	})}
	applyMatroskaDTSCanonicalProbe(&stream, dtsInfo{
		coreES: true, channels: 6, sampleRate: 48_000, samplesPerFrame: 512,
	}, false)

	for key, want := range map[fieldName]string{
		"Channels_Original":         "7",
		"ChannelLayout_Original":    "C L R Ls Rs Cb LFE",
		"ChannelPositions_Original": "Front: L C R, Side: L R, Back: C, LFE",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("%s = %q, %v; want %q, true", key, got, found, want)
		}
	}
	for _, key := range []fieldName{"ChannelLayout", "ChannelPositions"} {
		if got, found := canonicalSeedValue(stream, key); found {
			t.Fatalf("core layout %s retained as %q", key, got)
		}
	}
}

func TestMatroskaAC3CanonicalSeedTracksJOCProbe(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(6))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDAudioBitDepth, encodeMatroskaUint(16))...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_EAC3"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("E-AC-3 TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaAudioProbes(&info, map[uint64]*matroskaAudioProbe{2: {
		format: "E-AC-3", ok: true,
		info: ac3Info{
			bitRateKbps: 768, sampleRate: 48_000, channels: 6, layout: "L R C LFE Ls Rs",
			bsid: 16, bsmod: 0, acmod: 7, lfeon: 1, serviceKind: "Complete Main",
			frameRate: 31.25, spf: 1536, hasDialnorm: true, dialnorm: -31,
			hasCompr: true, comprDB: -0.28,
			hasJOC: true, hasJOCComplex: true, jocComplexity: 16,
			hasJOCDyn: true, jocDynObjects: 15,
			hasJOCBed: true, jocBedCount: 1, jocBedLayout: "LFE",
		},
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                     "E-AC-3",
		"ID":                         "2",
		"UniqueID":                   "123",
		"Format_Commercial_IfAny":    "Dolby Digital Plus with Dolby Atmos",
		"Format_AdditionalFeatures":  "JOC",
		"Format_Settings_Endianness": "Big",
		"CodecID":                    "A_EAC3",
		"Duration":                   "2500",
		"BitRate_Mode":               "Constant",
		"BitRate":                    "768000",
		"Channels":                   "6",
		"ChannelLayout":              "L R C LFE Ls Rs",
		"ChannelPositions":           "Front: L C R, Side: L R, LFE",
		"SamplesPerFrame":            "1536",
		"SamplingRate":               "48000",
		"SamplingCount":              "120000",
		"FrameRate":                  "31.250",
		"BitDepth":                   "16",
		"Compression_Mode":           "Lossy",
		"Delay":                      "0.000",
		"Delay_Source":               "Container",
		"Video_Delay":                "0.000",
		"Language":                   "en",
		"ServiceKind":                "CM",
		"Default":                    "Yes",
		"Forced":                     "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	wantExtra := []string{"ComplexityIndex", "NumberOfDynamicObjects", "BedChannelCount", "BedChannelConfiguration", "bsid", "dialnorm", "compr", "acmod", "lfeon"}
	if extra == nil || len(extra.Object) != len(wantExtra) {
		t.Fatalf("canonical extra = %#v", extra)
	}
	for index, key := range wantExtra {
		if extra.Object[index].Key != key {
			t.Fatalf("canonical extra[%d] = %q, want %q", index, extra.Object[index].Key, key)
		}
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-eac3-joc.mkv")
}

func TestMatroskaAACCanonicalSeedTracksStatisticsTags(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(456))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_AAC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, []byte{0x11, 0x90})...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 3)
	if !ok {
		t.Fatal("AAC TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{456: {
		trusted: true, dataBytes: 50_000, hasDataBytes: true,
		durationSeconds: 2.5, durationPrec: 9, hasDuration: true,
		frameCount: 117, hasFrameCount: true, bitRate: 127_999, hasBitRate: true,
	}}, 4_096_000)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                    "AAC",
		"ID":                        "2",
		"UniqueID":                  "456",
		"Format_AdditionalFeatures": "LC",
		"CodecID":                   "A_AAC-2",
		"Duration":                  "2500",
		"BitRate":                   "127999",
		"Channels":                  "2",
		"ChannelLayout":             "L R",
		"ChannelPositions":          "Front: L R",
		"SamplesPerFrame":           "1024",
		"SamplingRate":              "48000",
		"SamplingCount":             "120000",
		"FrameRate":                 "46.875",
		"FrameCount":                "117",
		"Compression_Mode":          "Lossy",
		"Delay":                     "0.000",
		"Delay_Source":              "Container",
		"Video_Delay":               "0.000",
		"StreamSize":                "50000",
		"Language":                  "en",
		"Default":                   "Yes",
		"Forced":                    "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-aac-stats.mkv")
}

func TestMatroskaOpusCanonicalSeedTracksStatsAndEncoderTags(t *testing.T) {
	audioPayload := buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(6))
	samplingRate := make([]byte, 8)
	binary.BigEndian.PutUint64(samplingRate, math.Float64bits(48_000))
	audioPayload = append(audioPayload, buildMatroskaElement(mkvIDSamplingRate, samplingRate)...)

	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(2))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(123))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("A_OPUS"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackAudio, audioPayload)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackName, []byte("Main audio"))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackLanguageIETF, []byte("ja-JP"))...)
	payload = append(payload, buildMatroskaElement(mkvIDFlagOriginal, encodeMatroskaUint(1))...)

	stream, ok := parseMatroskaTrackEntry(payload, 2, 3)
	if !ok {
		t.Fatal("Opus TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{123: {
		trusted: true, dataBytes: 50_000, hasDataBytes: true,
		durationSeconds: 2.5, durationPrec: 9, hasDuration: true,
		frameCount: 125, hasFrameCount: true, bitRate: 160_000, hasBitRate: true,
	}}, 4_096_000)
	applyMatroskaEncoders(info.Tracks, map[uint64]string{123: "opusenc opus-tools 0.2"}, map[uint64]string{123: "--vbr --bitrate 160"})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format":                   "Opus",
		"ID":                       "2",
		"UniqueID":                 "123",
		"CodecID":                  "A_OPUS",
		"Duration":                 "2500",
		"BitRate":                  "160000",
		"Channels":                 "6",
		"ChannelLayout":            "L R C Lb Rb LFE",
		"ChannelPositions":         "Front: L C R, Back: L R, LFE",
		"SamplesPerFrame":          "960",
		"SamplingRate":             "48000",
		"SamplingCount":            "120000",
		"FrameRate":                "50.000",
		"FrameCount":               "125",
		"Compression_Mode":         "Lossy",
		"Delay":                    "0.000",
		"Delay_Source":             "Container",
		"Video_Delay":              "0.000",
		"StreamSize":               "50000",
		"Title":                    "Main audio",
		"Encoded_Library":          "opusenc opus-tools 0.2",
		"Encoded_Library_Settings": "--vbr --bitrate 160",
		"Language":                 "ja-JP",
		"ServiceKind":              "O",
		"Default":                  "Yes",
		"Forced":                   "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("field %q came from the legacy projection", entry.Name)
		}
	}

	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-opus.mkv")
}

// TestMatroskaVP9CanonicalSeedProjectsBaselineCodecFacts verifies TrackEntry
// and statistics projection retain VP9's baseline profile and pixel format.
func TestMatroskaVP9CanonicalSeedProjectsBaselineCodecFacts(t *testing.T) {
	video := buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(1920))
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(1080))...)
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(231))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_VP9"))...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41_708_333))...)
	payload = append(payload, buildMatroskaElement(mkvIDBitRate, encodeMatroskaUint(1_000_000))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackVideo, video)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("VP9 TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{231: {
		trusted: true, dataBytes: 312_500, hasDataBytes: true,
		durationSeconds: 2.5, durationPrec: 9, hasDuration: true,
		frameCount: 60, hasFrameCount: true, bitRate: 1_000_000, hasBitRate: true,
	}}, 4_096_000)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format": "VP9", "ID": "1", "UniqueID": "231",
		"CodecID": "V_VP9", "Duration": "2500", "BitRate": "1000000",
		"Width": "1920", "Height": "1080", "Sampled_Width": "1920", "Sampled_Height": "1080",
		"PixelAspectRatio": "1.000", "DisplayAspectRatio": "1.778", "FrameRate_Mode": "Constant",
		"FrameRate": "24.000", "FrameRate_Num": "24", "FrameRate_Den": "1", "FrameCount": "60",
		"Delay": "0.000", "Delay_Source": "Container", "StreamSize": "312500",
		"Default": "Yes", "Forced": "No",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	for key, want := range map[fieldName]string{
		"Format_Profile": "0", "ColorSpace": "YUV", "ChromaSubsampling": "4:2:0",
		"ChromaSubsampling_Position": "Type 1", "BitDepth": "8",
	} {
		if got, found := canonicalSeedValue(stream, key); !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	if got := matroskaCanonicalSeedText(stream, "Bits/(Pixel*Frame)"); got != "0.020" {
		t.Fatalf("Bits/(Pixel*Frame) text = %q, want 0.020", got)
	}
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-vp9-stats.mkv")
}

func TestMatroskaFrameRateRatioUsesExactFillSemantics(t *testing.T) {
	tests := []struct {
		name    string
		rate    float64
		wantNum int
		wantDen int
	}{
		{name: "precise 1001", rate: 24000.0 / 1001.0, wantNum: 24000, wantDen: 1001},
		{name: "precise default duration", rate: 1_000_000_000.0 / 41_708_333.0, wantNum: 24000, wantDen: 1001},
		{name: "rounded 1000", rate: 1_000_000_000.0 / 41_708_375.0, wantNum: 23976, wantDen: 1000},
		{name: "near integer is not integer", rate: 1_000_000_000.0 / 33_333_334.0},
		{name: "integer", rate: 30, wantNum: 30, wantDen: 1},
		{name: "zero"},
		{name: "not a number", rate: math.NaN()},
		{name: "positive infinity", rate: math.Inf(1)},
		{name: "negative infinity", rate: math.Inf(-1)},
		{name: "out of integer range", rate: math.MaxFloat64},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotNum, gotDen := matroskaFrameRateRatio(test.rate)
			if gotNum != test.wantNum || gotDen != test.wantDen {
				t.Fatalf("matroskaFrameRateRatio(%0.12f) = %d/%d, want %d/%d", test.rate, gotNum, gotDen, test.wantNum, test.wantDen)
			}
		})
	}
}

func TestMatroskaStatisticsFrameRateNormalizesTrustedTagPrecision(t *testing.T) {
	tests := []struct {
		name            string
		frameCount      int64
		duration        float64
		defaultDuration uint64
		wantRate        float64
		wantRecognized  bool
	}{
		{name: "integer", frameCount: 126720, duration: 5280, defaultDuration: 41_666_666, wantRate: 24, wantRecognized: true},
		{name: "precise 24000 over 1001", frameCount: 66753, duration: 2784.157, defaultDuration: 41_708_333, wantRate: 24000.0 / 1001.0, wantRecognized: true},
		{name: "precise 30000 over 1001", frameCount: 57543, duration: 1920.017, defaultDuration: 33_366_666, wantRate: 30000.0 / 1001.0, wantRecognized: true},
		{name: "unrecognized imprecise duration", frameCount: 18658, duration: 778.152, defaultDuration: 41_708_333, wantRate: 18658.0 / 778.152},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotRate, gotRecognized := matroskaStatisticsFrameRate(test.frameCount, test.duration, test.defaultDuration)
			if math.Abs(gotRate-test.wantRate) > 1e-12 || gotRecognized != test.wantRecognized {
				t.Fatalf("matroskaStatisticsFrameRate() = %.12f, %v; want %.12f, %v", gotRate, gotRecognized, test.wantRate, test.wantRecognized)
			}
		})
	}
}

// TestMatroskaStaticVideoCanonicalSeedProjectsParsedAV1Facts verifies codec
// configuration and tag values reach every renderer without identity overrides.
func TestMatroskaStaticVideoCanonicalSeedProjectsParsedAV1Facts(t *testing.T) {
	colour := buildMatroskaElement(mkvIDRange, encodeMatroskaUint(1))
	colour = append(colour, buildMatroskaElement(mkvIDColourPrimaries, encodeMatroskaUint(1))...)
	colour = append(colour, buildMatroskaElement(mkvIDTransferChar, encodeMatroskaUint(1))...)
	video := buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(1920))
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(800))...)
	video = append(video, buildMatroskaElement(mkvIDColour, colour)...)
	trackUID := make([]byte, 8)
	binary.BigEndian.PutUint64(trackUID, 240)
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, trackUID)...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_AV1"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, []byte{0x81, 0x08, 0x4c})...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41_708_333))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackVideo, video)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("AV1 TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaTagStats(&info, map[uint64]matroskaTagStats{240: {
		extras: []jsonKV{{Key: "FilterChain", Val: "initial"}},
	}}, 4_096_000)
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format_Profile": "Main", "Format_Level": "4.0", "BitDepth": "10",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 1 || extra.Object[0].Key != "FilterChain" || extra.Object[0].Value.Text != "initial" {
		t.Fatalf("canonical extra = %#v, want parsed tag value", extra)
	}
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-av1.mkv")
}

// TestMatroskaVC1CanonicalSeedProjectsCodecPrivateFacts verifies direct VC-1
// BITMAPINFO defaults across JSON, text, and XML compatibility renderers.
func TestMatroskaVC1CanonicalSeedProjectsCodecPrivateFacts(t *testing.T) {
	codecPrivate := make([]byte, 20)
	copy(codecPrivate[16:20], "WVC1")
	video := buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(1920))
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(1080))...)
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(73))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_MS/VFW/FOURCC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41_708_333))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackVideo, video)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("VC-1 TrackEntry did not parse")
	}
	for key, want := range map[fieldName]string{
		"Format": "VC-1", "Format_Profile": "Advanced", "Format_Level": "3",
		"CodecID": "V_MS/VFW/FOURCC / WVC1", "ColorSpace": "YUV",
		"ChromaSubsampling": "4:2:0", "BitDepth": "8", "ScanType": "Progressive",
		"Compression_Mode": "Lossy",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	report := Report{Ref: "canonical-vc1.mkv", General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}, JSON: map[string]string{"Format": "Matroska"}}, Streams: []Stream{stream}}
	attachCanonicalStore(&report)
	for name, rendered := range map[string]string{
		"JSON": RenderJSON([]Report{report}), "text": RenderText([]Report{report}), "XML": RenderXML([]Report{report}),
	} {
		if !strings.Contains(rendered, "Advanced") || !strings.Contains(rendered, "3") {
			t.Fatalf("%s projection omitted VC-1 profile or level: %s", name, rendered)
		}
	}
}

// TestMatroskaMPEG2CanonicalSeedTracksProbeFacts verifies elementary-stream
// profile, settings, timing, color, buffer, and extra facts enter direct fields.
func TestMatroskaMPEG2CanonicalSeedTracksProbeFacts(t *testing.T) {
	video := buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(720))
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(576))...)
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(60))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG2"))...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(40_000_000))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackVideo, video)...)
	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("MPEG-2 TrackEntry did not parse")
	}
	bvop := true
	dropFrame := false
	parser := mpeg2VideoParser{info: mpeg2VideoInfo{
		Width: 720, Height: 576, AspectRatio: "4:3", FrameRate: 25,
		Version: "2", Profile: "Main@Main", BVOP: &bvop, Matrix: "Default",
		MaxBitRateKbps: 8_000, ColorSpace: "YUV", ChromaSubsampling: "4:2:0", BitDepth: "8 bits",
		ScanType: "Interlaced", ScanOrder: "TFF", PictureStructure: "Frame",
		TimeCode: "00:00:00:00", TimeCodeSource: "Group of pictures header", GOPDropFrame: &dropFrame,
		BufferSize: 229_376, IntraDCPrecision: 10, ColourDescriptionPresent: true,
		ColourPrimaries: "BT.470 BG", TransferCharacteristics: "Gamma 2.8", MatrixCoefficients: "BT.470 BG",
	}}
	applyMatroskaMPEG2Probe(&stream, &parser)

	for key, want := range map[fieldName]string{
		"Format_Version": "2", "Format_Profile": "Main", "Format_Level": "Main",
		"Format_Settings_BVOP": "Yes", "Format_Settings_Matrix": "Default",
		"BitRate_Mode": "VBR", "BitRate_Maximum": "8000000", "ColorSpace": "YUV",
		"ChromaSubsampling": "4:2:0", "BitDepth": "8", "ScanType": "Interlaced", "ScanOrder": "TFF",
		"Compression_Mode": "Lossy", "TimeCode_FirstFrame": "00:00:00:00", "BufferSize": "229376",
		"colour_primaries": "BT.601 PAL", "transfer_characteristics": "BT.470 System B/G",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	extra := matroskaCanonicalSeedNode(stream, "extra")
	if extra == nil || len(extra.Object) != 1 || extra.Object[0].Key != "intra_dc_precision" || extra.Object[0].Value.Text != "10" {
		t.Fatalf("canonical MPEG-2 extra = %#v", extra)
	}
}

// TestMatroskaMPEG4VisualCanonicalSeedTracksProbeFacts verifies direct profile,
// coding settings, visual properties, and XviD library metadata.
func TestMatroskaMPEG4VisualCanonicalSeedTracksProbeFacts(t *testing.T) {
	codecPrivate := make([]byte, 20)
	copy(codecPrivate[16:20], "XVID")
	video := buildMatroskaElement(mkvIDPixelWidth, encodeMatroskaUint(704))
	video = append(video, buildMatroskaElement(mkvIDPixelHeight, encodeMatroskaUint(528))...)
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(232))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_MS/VFW/FOURCC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(40_000_000))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackVideo, video)...)
	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("MPEG-4 Visual TrackEntry did not parse")
	}
	bvop := true
	qpel := false
	applyMatroskaMPEG4VisualProbe(&stream, mpeg4VisualInfo{
		Profile: "Advanced Simple@L5", BVOP: &bvop, QPel: &qpel, GMC: "0 warppoints", Matrix: "Default (MPEG)",
		ColorSpace: "YUV", ChromaSubsampling: "4:2:0", BitDepth: "8 bits", ScanType: "Progressive",
		WritingLibrary: "XviD0050",
	})
	for key, want := range map[fieldName]string{
		"Format_Profile": "Advanced Simple", "Format_Level": "5", "Format_Settings_BVOP": "1",
		"Format_Settings_QPel": "No", "Format_Settings_GMC": "0", "Format_Settings_Matrix": "Default (MPEG)",
		"ColorSpace": "YUV", "ChromaSubsampling": "4:2:0", "BitDepth": "8", "ScanType": "Progressive",
		"Compression_Mode": "Lossy", "Encoded_Library": "XviD0050", "Encoded_Library_Name": "XviD",
		"Encoded_Library_Version": "1.2.1", "Encoded_Library_Date": "2008-12-04",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
}

// TestMatroskaAVCCanonicalSeedTracksConfigurationAndProbeFacts verifies AVC
// configuration, TrackEntry, and bounded-probe facts across every renderer.
func TestMatroskaAVCCanonicalSeedTracksConfigurationAndProbeFacts(t *testing.T) {
	codecPrivate := []byte{
		0x01, 0x64, 0x00, 0x29, 0xff, 0xe1, 0x00, 0x1e,
		0x67, 0x64, 0x00, 0x29, 0xac, 0x72, 0x04, 0x40,
		0xb4, 0x3d, 0xbf, 0xf0, 0x00, 0x80, 0x00, 0x91,
		0x00, 0x00, 0x03, 0x03, 0xe9, 0x00, 0x00, 0xea,
		0x60, 0x8f, 0x18, 0x31, 0x84, 0x60, 0x01, 0x00,
		0x07, 0x68, 0xe8, 0x43, 0x82, 0xb2, 0xc8, 0xb0,
	}
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(241))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEG4/ISO/AVC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, codecPrivate)...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41_708_333))...)
	payload = append(payload, buildMatroskaVideoSettings(720, 480)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("AVC TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {
		codec: "AVC", writingLib: "x264 core 164 r3108 31e19f9", encoding: "cabac=1 / ref=4",
		sliceCount: 4, timeCode: "00:00:00:00",
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format": "AVC", "Format_Profile": "High", "Format_Level": "4.1",
		"CodecID": "V_MPEG4/ISO/AVC", "Width": "720", "Height": "480",
		"ColorSpace": "YUV", "BitDepth": "8", "Format_Settings_SliceCount": "4",
		"TimeCode_FirstFrame": "00:00:00:00", "Encoded_Library_Name": "x264",
		"Encoded_Library_Settings": "cabac=1 / ref=4",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-avc.mkv")
}

func TestMatroskaAV1FilmGrainAddsRawFormatSettingsOnlyWhenSignaled(t *testing.T) {
	for _, test := range []struct {
		name      string
		filmGrain bool
		want      bool
	}{
		{name: "signaled", filmGrain: true, want: true},
		{name: "absent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder := newCanonicalStreamBuilder(StreamVideo)
			builder.Fill("ID", "1", "ID", "1")
			builder.Fill("Format", "AV1", "Format", "AV1")
			stream := builder.Snapshot(canonicalStreamPolicy{})
			info := MatroskaInfo{Tracks: []Stream{stream}}
			applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {
				codec: "AV1", av1Seen: true, av1: av1SequenceInfo{filmGrainPresent: test.filmGrain},
			}})
			found := matroskaStreamDisplay(info.Tracks[0], "Format settings") == "Film Grain Synthesis"
			if found != test.want {
				t.Fatalf("Film Grain Synthesis present = %v, want %v", found, test.want)
			}
		})
	}
}

func TestMatroskaAVCStereoProfileContainsLevelWithoutSeparateField(t *testing.T) {
	stream := Stream{Kind: StreamVideo, canonicalSeed: matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{
		format: "AVC", codecID: "V_MPEG4/ISO/AVC",
		video: matroskaVideoInfo{stereoMode: 13},
		avc:   avcConfigInfo{profile: "High", level: "L4.1"},
	})}
	if got, found := canonicalSeedValue(stream, "Format_Profile"); !found || got != "Stereo High@L4.1 / High@L4.1" {
		t.Fatalf("Format_Profile = %q, %v", got, found)
	}
	if got, found := canonicalSeedValue(stream, "Format_Level"); found {
		t.Fatalf("stereoscopic profile duplicated level as %q", got)
	}
}

func TestMatroskaAVCCanonicalAspectSourcesUseRationalRounding(t *testing.T) {
	stream := Stream{Kind: StreamVideo, canonicalSeed: matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{
		format: "AVC", codecID: "V_MPEG4/ISO/AVC",
		video: matroskaVideoInfo{
			pixelWidth: 632, pixelHeight: 482, codedWidth: 640, codedHeight: 496,
			displayWidth: 674, displayHeight: 482, hasDisplayWidth: true, hasDisplayHeight: true,
		},
		sps: h264SPSInfo{
			Width: 632, Height: 482, CodedWidth: 640, CodedHeight: 496,
			HasSAR: true, SARWidth: 16, SARHeight: 15,
		},
	})}
	for key, want := range map[fieldName]string{
		"PixelAspectRatio":            "1.066",
		"PixelAspectRatio_Original":   "1.067",
		"DisplayAspectRatio":          "1.398",
		"DisplayAspectRatio_Original": "1.399",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("%s = %q, %v; want %q", key, got, found, want)
		}
	}
}

func TestMatroskaAVCCanonicalSquarePixelDisplayRatioUsesRationalRounding(t *testing.T) {
	stream := Stream{Kind: StreamVideo, canonicalSeed: matroskaAVCCanonicalSeed(matroskaVideoCanonicalFacts{
		format: "AVC", codecID: "V_MPEG4/ISO/AVC",
		video: matroskaVideoInfo{pixelWidth: 1918, pixelHeight: 800, codedWidth: 1920},
		sps:   h264SPSInfo{Width: 1918, Height: 800, CodedWidth: 1920},
	})}
	if got, found := canonicalSeedValue(stream, "DisplayAspectRatio"); !found || got != "2.398" {
		t.Fatalf("DisplayAspectRatio = %q, %v; want 2.398", got, found)
	}
}

// TestMatroskaHEVCCanonicalSeedTracksConfigurationAndProbeFacts verifies HEVC
// configuration, TrackEntry, encoder, HDR, and time-code projections.
func TestMatroskaHEVCCanonicalSeedTracksConfigurationAndProbeFacts(t *testing.T) {
	payload := buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))
	payload = append(payload, buildMatroskaElement(mkvIDTrackUID, encodeMatroskaUint(242))...)
	payload = append(payload, buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(1))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecID, []byte("V_MPEGH/ISO/HEVC"))...)
	payload = append(payload, buildMatroskaElement(mkvIDCodecPrivate, buildHVCCRecord(hevcSPS8bit, nil))...)
	payload = append(payload, buildMatroskaElement(mkvIDDefaultDuration, encodeMatroskaUint(41_708_333))...)
	payload = append(payload, buildMatroskaVideoSettings(320, 240)...)

	stream, ok := parseMatroskaTrackEntry(payload, 2.5, 9)
	if !ok {
		t.Fatal("HEVC TrackEntry did not parse")
	}
	info := MatroskaInfo{Tracks: []Stream{stream}}
	applyMatroskaVideoProbes(&info, map[uint64]*matroskaVideoProbe{1: {
		codec: "HEVC",
		hdrInfo: hevcHDRInfo{
			masteringPrimaries: "BT.2020", masteringLuminanceMin: 0.005,
			masteringLuminanceMax: 1000, hasMastering: true, maxCLL: 1000, maxFALL: 400,
			x265Library: "x265 4.1+1", x265Settings: "crf=18 / wpp", timeCode: "01:02:03:04",
		},
	}})
	stream = info.Tracks[0]
	sortFields(stream.Kind, stream.Fields)

	for key, want := range map[fieldName]string{
		"Format": "HEVC", "Format_Profile": "Main", "Format_Level": "2", "Format_Tier": "Main",
		"CodecID": "V_MPEGH/ISO/HEVC", "Width": "320", "Height": "240",
		"ColorSpace": "YUV", "ChromaSubsampling": "4:2:0", "BitDepth": "8",
		"Encoded_Library_Name": "x265", "MasteringDisplay_ColorPrimaries": "BT.2020",
		"MaxCLL": "1000", "MaxFALL": "400", "TimeCode_FirstFrame": "01:02:03:04",
	} {
		got, found := canonicalSeedValue(stream, key)
		if !found || got != want {
			t.Fatalf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
	}
	assertMatroskaDirectStreamMatchesLegacy(t, stream, "canonical-hevc.mkv")
}

// buildMatroskaFLACPrivateForTest creates complete codec-private metadata with
// STREAMINFO and an optional Vorbis-comment vendor string.
func buildMatroskaFLACPrivateForTest(vendor string) []byte {
	streamInfo := []byte{
		0x10, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10, 0x00, 0x21, 0x0C, 0x0B, 0xB8, 0x02, 0xF0, 0x0E, 0x5B,
		0x65, 0x40, 0x86, 0x4D, 0x55, 0xF0, 0x03, 0x14, 0x3D, 0x8B, 0xAD, 0x47, 0xD3, 0xB9, 0x97, 0xFA,
		0xE6, 0x4C,
	}
	if vendor == "" {
		return streamInfo
	}
	comment := make([]byte, 4, 4+len(vendor)+4)
	binary.LittleEndian.PutUint32(comment, uint32(len(vendor)))
	comment = append(comment, vendor...)
	comment = append(comment, 0, 0, 0, 0)
	private := make([]byte, 0, 8+len(streamInfo)+4+len(comment))
	private = append(private, 'f', 'L', 'a', 'C', 0x00, 0x00, 0x00, byte(len(streamInfo)))
	private = append(private, streamInfo...)
	private = append(private, 0x84, byte(len(comment)>>16), byte(len(comment)>>8), byte(len(comment)))
	private = append(private, comment...)
	return private
}

// buildMatroskaVorbisPrivateForTest creates laced identification and comment
// headers with representative rates, library metadata, and encoder metadata.
func buildMatroskaVorbisPrivateForTest() []byte {
	identification := make([]byte, 30)
	identification[0] = 1
	copy(identification[1:7], "vorbis")
	binary.LittleEndian.PutUint32(identification[16:20], 160_000)
	binary.LittleEndian.PutUint32(identification[20:24], 96_000)
	binary.LittleEndian.PutUint32(identification[24:28], 48_000)

	vendor := []byte("Xiph.Org libVorbis I 20020717")
	commentEntry := []byte("ENCODER=Made with BeSweet v1.5b31")
	comment := append([]byte{3, 'v', 'o', 'r', 'b', 'i', 's'}, make([]byte, 4)...)
	binary.LittleEndian.PutUint32(comment[7:11], uint32(len(vendor)))
	comment = append(comment, vendor...)
	count := make([]byte, 4)
	binary.LittleEndian.PutUint32(count, 1)
	comment = append(comment, count...)
	length := make([]byte, 4)
	binary.LittleEndian.PutUint32(length, uint32(len(commentEntry)))
	comment = append(comment, length...)
	comment = append(comment, commentEntry...)

	private := make([]byte, 0, 3+len(identification)+len(comment))
	private = append(private, 2, byte(len(identification)), byte(len(comment)))
	private = append(private, identification...)
	private = append(private, comment...)
	return private
}

// assertMatroskaDirectStreamMatchesLegacy verifies all shared renderers at the
// parser cutover seam and an idempotent exported compatibility snapshot.
func assertMatroskaDirectStreamMatchesLegacy(t *testing.T, stream Stream, ref string) {
	t.Helper()
	refreshed := stream
	refreshCanonicalCompatibilitySnapshot(&refreshed)
	first := captureLegacyStreamState(refreshed, true)
	refreshCanonicalCompatibilitySnapshot(&refreshed)
	if second := captureLegacyStreamState(refreshed, true); !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical compatibility refresh is not idempotent:\nfirst=%#v\nsecond=%#v", first, second)
	}
	legacyStream := matroskaLegacyStreamWithCanonicalExtra(refreshed)
	jsonLegacyStream := matroskaLegacyStreamWithRetainedJSON(legacyStream)
	if _, hasNumerator := canonicalSeedValue(stream, "FrameRate_Num"); !hasNumerator {
		jsonLegacyStream.JSONSkipFrameRateRatio = true
		delete(jsonLegacyStream.JSON, "FrameRate_Num")
		delete(jsonLegacyStream.JSON, "FrameRate_Den")
		if frameRate := jsonLegacyStream.JSON["FrameRate"]; frameRate != "" {
			jsonLegacyStream.Fields = append([]Field(nil), jsonLegacyStream.Fields...)
			jsonLegacyStream.Fields = setFieldValue(jsonLegacyStream.Fields, "Frame rate", frameRate+" FPS")
		}
	}
	xmlLegacyStream := matroskaLegacyStreamForXML(legacyStream, stream.canonicalSeed)
	if _, hasNumerator := canonicalSeedValue(stream, "FrameRate_Num"); !hasNumerator {
		xmlLegacyStream.JSONSkipComputed = true
		delete(xmlLegacyStream.JSON, "FrameRate_Num")
		delete(xmlLegacyStream.JSON, "FrameRate_Den")
		delete(xmlLegacyStream.JSONRaw, "FrameRate_Num")
		delete(xmlLegacyStream.JSONRaw, "FrameRate_Den")
		if frameRate := xmlLegacyStream.JSON["FrameRate"]; frameRate != "" {
			xmlLegacyStream.Fields = append([]Field(nil), xmlLegacyStream.Fields...)
			xmlLegacyStream.Fields = setFieldValue(xmlLegacyStream.Fields, "Frame rate", frameRate+" FPS")
		}
	}
	jsonLegacyStream.canonicalSeed = nil
	xmlLegacyStream.canonicalSeed = nil
	legacyStream.canonicalSeed = nil
	general := Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "Matroska"}}, JSON: map[string]string{"Format": "Matroska"}}
	legacyReport := Report{Ref: ref, General: general, Streams: []Stream{legacyStream}}
	jsonLegacyReport := Report{Ref: ref, General: general, Streams: []Stream{jsonLegacyStream}}
	xmlLegacyReport := Report{Ref: ref, General: general, Streams: []Stream{xmlLegacyStream}}
	directReport := Report{Ref: ref, General: general, Streams: []Stream{stream}}
	attachCanonicalStore(&directReport)
	for name, outputs := range map[string][2]string{
		"JSON": {RenderJSON([]Report{jsonLegacyReport}), RenderJSON([]Report{directReport})},
		"text": {RenderText([]Report{legacyReport}), RenderText([]Report{directReport})},
		"XML":  {RenderXML([]Report{xmlLegacyReport}), RenderXML([]Report{directReport})},
	} {
		if outputs[0] != outputs[1] {
			t.Fatalf("%s projection differs:\nlegacy=%s\ndirect=%s", name, outputs[0], outputs[1])
		}
	}
}

// matroskaLegacyStreamForXML removes canonical JSON-only fields from the
// compatibility fixture before exercising the legacy XML adapter.
func matroskaLegacyStreamForXML(stream Stream, seed []fieldEntry) Stream {
	stream.JSON = maps.Clone(stream.JSON)
	stream.JSONRaw = maps.Clone(stream.JSONRaw)
	for _, entry := range seed {
		if !entry.Options.ShowStructured || entry.Options.ShowXML {
			continue
		}
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		delete(stream.JSON, key)
		delete(stream.JSONRaw, key)
	}
	return stream
}

// matroskaLegacyStreamWithCanonicalExtra recreates removed dynamic-tag staging
// so the pre-cutover JSON/XML adapters receive the same ordered extra object.
func matroskaLegacyStreamWithCanonicalExtra(stream Stream) Stream {
	rawFields := make(map[string]string, len(stream.JSONRaw))
	maps.Copy(rawFields, stream.JSONRaw)
	stream.JSONRaw = rawFields
	if node := canonicalSeedStructuredNode(&stream, "extra"); node != nil && node.Kind == structuredObject {
		stream.JSONRaw["extra"] = structuredNodeText(*node)
	}
	return stream
}

// matroskaLegacyStreamWithRetainedJSON recreates the removed private staging
// map so direct builders can still be compared with the pre-cutover renderer.
func matroskaLegacyStreamWithRetainedJSON(stream Stream) Stream {
	jsonFields := make(map[string]string, len(stream.JSON))
	maps.Copy(jsonFields, stream.JSON)
	stream.JSON = jsonFields
	for _, entry := range stream.canonicalSeed {
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if !entry.Options.ShowStructured || entry.Options.ShowXML || key == "" || entry.Node != nil {
			continue
		}
		if stream.JSON == nil {
			stream.JSON = map[string]string{}
		}
		if stream.JSON[key] == "" {
			stream.JSON[key] = projectCanonicalStructuredValue(stream.Kind, entry)
		}
	}
	return stream
}

// matroskaCanonicalSeedNode returns the direct structured node for key.
func matroskaCanonicalSeedNode(stream Stream, key fieldName) *structuredNode {
	return canonicalSeedStructuredNode(&stream, key)
}

// matroskaCanonicalSeedText returns the direct compatibility text value for
// one label, or an empty string when the projection intentionally omits it.
func matroskaCanonicalSeedText(stream Stream, label string) string {
	for _, entry := range stream.canonicalSeed {
		if entry.Options.ShowText && entry.TextLabel == label {
			return entry.Value.Text
		}
	}
	return ""
}
