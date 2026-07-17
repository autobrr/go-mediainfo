package mediainfo

import "testing"

func TestSetEAC3CommercialJSON_JOCGatesBDAVMetadata(t *testing.T) {
	info := ac3Info{hasJOC: true}

	tsExtras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(tsExtras, info, false)
	if got := tsExtras.Projection("Format_Commercial_IfAny"); got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("non-BDAV commercial label = %q", got)
	}
	if got := tsExtras.Projection("Format_Profile"); got != "" {
		t.Fatal("non-BDAV JOC must not set Format_Profile")
	}
	if got := tsExtras.Projection("MuxingMode"); got != "" {
		t.Fatal("non-BDAV JOC must not set MuxingMode")
	}

	bdavExtras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(bdavExtras, info, true)
	if got := bdavExtras.Projection("Format_Commercial_IfAny"); got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("BDAV commercial label = %q", got)
	}
	if got := bdavExtras.Projection("Format_Profile"); got != "Blu-ray Disc" {
		t.Fatalf("BDAV Format_Profile = %q", got)
	}
	if got := bdavExtras.Projection("MuxingMode"); got != "Stream extension" {
		t.Fatalf("BDAV MuxingMode = %q", got)
	}
}

func TestSetEAC3CommercialJSON_NonJOCUsesDolbyDigitalPlus(t *testing.T) {
	extras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(extras, ac3Info{}, false)

	if got := extras.Projection("Format_Commercial_IfAny"); got != "Dolby Digital Plus" {
		t.Fatalf("commercial label = %q", got)
	}
}

func TestBuildTSCaptionStreamSeedsDirectCanonicalFields(t *testing.T) {
	track := &ccTrack{firstCommandPTS: 135_000}
	stream := buildTSCaptionStream(256, 1, 1.25, 3.5, 30, "EIA-608", "CC1", track, tsCaptionService{}, false, true)
	if len(stream.canonicalSeed) == 0 {
		t.Fatal("caption stream did not retain a canonical seed")
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("caption field %q came from a legacy projection", entry.Name)
		}
	}
	if duration, ok := canonicalSeedValue(stream, "Duration"); !ok || duration != "3500" {
		t.Fatalf("canonical duration = %q, %v", duration, ok)
	}
	if extra := canonicalSeedNode(stream, "extra"); extra == nil || extra.Kind != structuredObject {
		t.Fatal("caption extra was not retained as a source-built object")
	}
}

func TestUpdateCCTrackTSCapturesFirstRollUpEvent(t *testing.T) {
	entry := &tsStream{}
	updateCCTrackTS(entry, 0, 0x14, 0x25, 900_000, true, 100)
	updateCCTrackTS(entry, 0, 'H', 'i', 903_000, true, 101)
	updateCCTrackTS(entry, 0, 0x14, 0x2D, 906_000, true, 102)

	if entry.ccOdd.firstType != "RollUp" {
		t.Fatalf("first type=%q, want RollUp", entry.ccOdd.firstType)
	}
	if entry.ccOdd.firstContentPTS != 903_000 || entry.ccOdd.firstDisplayFrame != 94 {
		t.Fatalf("first content=%d first display frame=%d", entry.ccOdd.firstContentPTS, entry.ccOdd.firstDisplayFrame)
	}
	if entry.ccOdd.lastContentPTS != 906_000 {
		t.Fatalf("last content PTS=%d", entry.ccOdd.lastContentPTS)
	}
}

func TestUpdateCCTrackTSSuppressesRepeatedControlPair(t *testing.T) {
	entry := &tsStream{}
	updateCCTrackTS(entry, 0, 0x14, 0x26, 900_000, true, 100)
	updateCCTrackTS(entry, 0, 0x14, 0x26, 903_000, true, 102)

	if entry.ccOdd.lastCommandPTS != 903_000 || !entry.ccOdd.commandDuplicated {
		t.Fatalf("last command=%d duplicated=%v", entry.ccOdd.lastCommandPTS, entry.ccOdd.commandDuplicated)
	}
	if entry.ccOdd.rollUpLines != 3 {
		t.Fatalf("roll-up lines=%d", entry.ccOdd.rollUpLines)
	}
}

func TestUpdateCCTrackTSCarriageReturnChangesDisplayedState(t *testing.T) {
	entry := &tsStream{}
	updateCCTrackTS(entry, 0, 0x14, 0x26, 900_000, true, 100)
	updateCCTrackTS(entry, 0, 0x14, 0x2D, 903_000, true, 102)

	if entry.ccOdd.firstContentPTS != 903_000 || entry.ccOdd.lastContentPTS != 903_000 {
		t.Fatalf("display change=%d..%d", entry.ccOdd.firstContentPTS, entry.ccOdd.lastContentPTS)
	}
}

func TestMediaInfoCaptionTimestampUsesFloat32Milliseconds(t *testing.T) {
	const fps = 60000.0 / 1001.0
	tests := []struct {
		pts  uint64
		want float64
	}{
		{pts: 7_133_746_994, want: 79263.84},
		{pts: 7_133_762_009, want: 79264.008},
		{pts: 7_201_984_163, want: 80022.032},
	}
	for _, test := range tests {
		if got := mediaInfoCaptionTimestamp(test.pts, fps, -1); got != test.want {
			t.Fatalf("timestamp for %d=%f, want %f", test.pts, got, test.want)
		}
	}
}

func TestMPEGTSMenuRetainsNanosecondStructuredDuration(t *testing.T) {
	fields := []Field{
		{Name: "ID", Value: "4096 (0x1000)"},
		{Name: "Format", Value: "AVC / AAC"},
		{Name: "Duration", Value: "3 s 937 ms"},
	}
	facts := &mpegTSStructuredFacts{}
	facts.Set("ID", "4096")
	facts.Set("Duration", "3.937266667")
	stream := buildCanonicalMPEGTSStream(StreamMenu, nil, fields, facts, nil, false)
	if duration, ok := projectedCanonicalSeedValue(stream, "Duration"); !ok || duration != "3.937266667" {
		t.Fatalf("structured duration = %q, %v", duration, ok)
	}
	for _, entry := range stream.canonicalSeed {
		if entry.Projected {
			t.Fatalf("menu field %q came from a legacy projection", entry.Name)
		}
	}
}

func TestHasTSCaptionStreamForPIDUsesCanonicalID(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamText)
	builder.DirectStructured("ID", "100-CC1")
	stream := builder.Snapshot(canonicalStreamPolicy{})
	stream.JSON = map[string]string{"ID": "200-CC1"}
	stream.Fields = []Field{{Name: "ID", Value: "200-CC1"}}

	if !hasTSCaptionStreamForPID([]Stream{stream}, 100) {
		t.Fatal("canonical caption PID was not detected")
	}
	if hasTSCaptionStreamForPID([]Stream{stream}, 200) {
		t.Fatal("legacy snapshot overrode canonical caption PID")
	}
}

func TestReplaceMPEGTSCanonicalSnapshotFieldPublishesLegacyState(t *testing.T) {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("BitRate", "1000", "Bit rate", "1.00 kb/s")
	stream := builder.Snapshot(canonicalStreamPolicy{})

	replaceMPEGTSCanonicalSnapshotField(&stream, "BitRate", "2000", "Bit rate", "2.00 kb/s")

	if got, _ := canonicalSeedValue(stream, "BitRate"); got != "2000" {
		t.Fatalf("canonical bitrate = %q, want 2000", got)
	}
	if got := stream.JSON["BitRate"]; got != "2000" {
		t.Fatalf("legacy JSON bitrate = %q, want 2000", got)
	}
	if got := findField(stream.Fields, "Bit rate"); got != "2.00 kb/s" {
		t.Fatalf("legacy text bitrate = %q, want 2.00 kb/s", got)
	}
}

// canonicalSeedNode returns a direct structured node for a test key.
func canonicalSeedNode(stream Stream, key fieldName) *structuredNode {
	for _, entry := range stream.canonicalSeed {
		structuredKey := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && structuredKey == string(key) {
			return entry.Node
		}
	}
	return nil
}
