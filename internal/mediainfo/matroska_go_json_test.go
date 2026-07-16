package mediainfo

import (
	"strings"
	"testing"
)

func TestParseMatroskaTrackRetainsGoOnlyPCMJSON(t *testing.T) {
	audio := buildMatroskaElement(mkvIDTrackAudio,
		buildMatroskaElement(mkvIDChannels, encodeMatroskaUint(2)),
	)
	entry := append(
		buildMatroskaElement(mkvIDTrackType, encodeMatroskaUint(2)),
		buildMatroskaElement(mkvIDTrackNumber, encodeMatroskaUint(1))...,
	)
	entry = append(entry, buildMatroskaElement(mkvIDCodecID, []byte("A_PCM/INT/LIT"))...)
	entry = append(entry, audio...)

	stream, ok := parseMatroskaTrackEntry(entry, 1, 3)
	if !ok {
		t.Fatal("expected parsed PCM stream")
	}
	for key, want := range map[string]string{
		"ChannelLayout":    "L R",
		"ChannelPositions": "Front: L R",
		"Compression_Mode": "Lossless",
	} {
		if got, found := projectedCanonicalSeedValue(stream, fieldName(key)); !found || got != want {
			t.Errorf("canonical %s = %q, %v; want %q", key, got, found, want)
		}
		if got := stream.JSON[key]; got != "" {
			t.Errorf("%s leaked into parity JSON before final restoration: %q", key, got)
		}
	}
	if got := findField(stream.Fields, "Channel layout"); got != "" {
		t.Fatalf("Go-only channel layout leaked into text fields: %q", got)
	}
	if got := findField(stream.Fields, "Compression mode"); got != "" {
		t.Fatalf("Go-only compression mode leaked into text fields: %q", got)
	}
}

func TestRestoreMatroskaRetainedFieldsUsesDirectState(t *testing.T) {
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	videoBuilder := newCanonicalStreamBuilder(StreamVideo)
	videoBuilder.Structured("BitRate_Maximum", "official")
	videoBuilder.Structured("BitRate_Nominal", "")
	video := videoBuilder.Snapshot(canonicalStreamPolicy{})
	video.JSON = map[string]string{"BitRate_Maximum": "official"}
	fillMatroskaRetainedJSON(&video, "BitRate_Maximum", "legacy")
	fillMatroskaRetainedJSON(&video, "BitRate_Nominal", "20000000")
	fillMatroskaRetainedJSON(&video, "BitRate_Mode", "VBR")
	fillMatroskaRetainedJSON(&video, "FrameRate_Num", "24000")

	audioBuilder := newCanonicalStreamBuilder(StreamAudio)
	audio := audioBuilder.Snapshot(canonicalStreamPolicy{})
	fillMatroskaRetainedJSON(&audio, "ChannelLayout", "L R")
	fillMatroskaRetainedJSON(&audio, "ChannelPositions", "Front: L R")

	menuBuilder := newCanonicalStreamBuilder(StreamMenu)
	menuBuilder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{Key: "chapter", Value: structuredNode{Kind: structuredString, Text: "one"}}}})
	menu := menuBuilder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
	menu.JSONRaw = map[string]string{"extra": `{"chapter":"one"}`}
	editionBuilder := newCanonicalStreamBuilder(StreamMenu)
	editionBuilder.StructuredNode("extra", structuredNode{Kind: structuredObject, Object: []structuredMember{{Key: "edition", Value: structuredNode{Kind: structuredString, Text: "two"}}}})
	edition := editionBuilder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
	edition.JSONRaw = map[string]string{"extra": `{"edition":"two"}`}
	streams := []Stream{video, audio, menu, edition}

	restoreMatroskaRetainedFields(&general, streams, "1234", matroskaRetainedGeneralPresence{})

	if got, found := projectedCanonicalSeedValue(streams[0], "BitRate_Maximum"); !found || got != "official" {
		t.Fatalf("CLI-backed field = %q, %v; want official", got, found)
	}
	if got, found := projectedCanonicalSeedValue(streams[0], "BitRate_Nominal"); !found || got != "20000000" {
		t.Fatalf("empty placeholder fill = %q, %v; want 20000000", got, found)
	}
	if got, found := projectedCanonicalSeedValue(streams[0], "BitRate_Mode"); !found || got != "VBR" {
		t.Fatalf("BitRate_Mode = %q, %v; want VBR", got, found)
	}
	if got, found := projectedCanonicalSeedValue(streams[0], "FrameRate_Num"); !found || got != "24000" {
		t.Fatalf("FrameRate_Num = %q, %v; want 24000", got, found)
	}
	if got, found := projectedCanonicalSeedValue(streams[1], "ChannelLayout"); !found || got != "L R" {
		t.Fatalf("ChannelLayout = %q, %v; want L R", got, found)
	}
	if got, found := projectedCanonicalSeedValue(general, "StreamSize"); !found || got != "1234" {
		t.Fatalf("General StreamSize = %q, %v; want 1234", got, found)
	}
	if got, found := projectedCanonicalSeedValue(general, "OverallBitRate_Mode"); !found || got != "VBR" {
		t.Fatalf("General OverallBitRate_Mode = %q, %v; want VBR", got, found)
	}
	report := Report{Ref: "retained.mkv", General: general, Streams: streams}
	attachCanonicalStore(&report)
	if output := RenderJSON([]Report{report}); !strings.Contains(output, `"chapter":"one","edition":"two"`) {
		t.Fatalf("merged Menu.extra missing from direct output: %s", output)
	}
	if streams[0].JSON["BitRate_Mode"] != "" || streams[1].JSON["ChannelLayout"] != "" || general.JSON["StreamSize"] != "" {
		t.Fatal("Go-only fields leaked into the shared report")
	}
	if got := streams[2].JSONRaw["extra"]; got != `{"chapter":"one"}` {
		t.Fatalf("Go-only Menu.extra mutated the shared report: %s", got)
	}
	if len(streams[0].Fields) != 0 || len(streams[1].Fields) != 0 {
		t.Fatal("restoration changed text fields")
	}
}

func TestRestoreMatroskaRetainedFieldsUsesCapturedGeneralPresence(t *testing.T) {
	general := Stream{Kind: StreamGeneral, JSON: map[string]string{}}
	replaceMatroskaCanonicalJSONOnlyOverrides(&general, map[string]string{"StreamSize": "326"})

	restoreMatroskaRetainedFields(&general, nil, "1234", matroskaRetainedGeneralPresence{})
	if got, _ := canonicalSeedValue(general, "StreamSize"); got != "1234" {
		t.Fatalf("late compatibility size = %q, want retained size 1234", got)
	}

	replaceMatroskaCanonicalJSONOnlyOverrides(&general, map[string]string{"StreamSize": "326"})
	restoreMatroskaRetainedFields(&general, nil, "1234", matroskaRetainedGeneralPresence{streamSize: true})
	if got, _ := canonicalSeedValue(general, "StreamSize"); got != "326" {
		t.Fatalf("captured General size = %q, want 326", got)
	}
}

func TestMatroskaGoFormatLevelUsesFirstStereoProfile(t *testing.T) {
	if got := matroskaGoFormatLevel("Stereo High@L4.1 / High@L4.1"); got != "4.1" {
		t.Fatalf("level = %q, want 4.1", got)
	}
	if got := matroskaGoFormatLevel("High@L4.1"); got != "" {
		t.Fatalf("single profile produced redundant retained level %q", got)
	}
}
