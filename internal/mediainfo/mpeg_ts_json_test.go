package mediainfo

import "testing"

func TestSetEAC3CommercialJSON_JOCGatesBDAVMetadata(t *testing.T) {
	info := ac3Info{hasJOC: true}

	tsExtras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(tsExtras, info, false)
	if got := tsExtras.Legacy("Format_Commercial_IfAny"); got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("non-BDAV commercial label = %q", got)
	}
	if got := tsExtras.Legacy("Format_Profile"); got != "" {
		t.Fatal("non-BDAV JOC must not set Format_Profile")
	}
	if got := tsExtras.Legacy("MuxingMode"); got != "" {
		t.Fatal("non-BDAV JOC must not set MuxingMode")
	}

	bdavExtras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(bdavExtras, info, true)
	if got := bdavExtras.Legacy("Format_Commercial_IfAny"); got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("BDAV commercial label = %q", got)
	}
	if got := bdavExtras.Legacy("Format_Profile"); got != "Blu-ray Disc" {
		t.Fatalf("BDAV Format_Profile = %q", got)
	}
	if got := bdavExtras.Legacy("MuxingMode"); got != "Stream extension" {
		t.Fatalf("BDAV MuxingMode = %q", got)
	}
}

func TestSetEAC3CommercialJSON_NonJOCUsesDolbyDigitalPlus(t *testing.T) {
	extras := &mpegTSStructuredFacts{}
	setEAC3CommercialFacts(extras, ac3Info{}, false)

	if got := extras.Legacy("Format_Commercial_IfAny"); got != "Dolby Digital Plus" {
		t.Fatalf("commercial label = %q", got)
	}
}

func TestBuildTSCaptionStreamSeedsDirectCanonicalFields(t *testing.T) {
	stream := buildTSCaptionStream(256, 1, 1.25, 3.5, "EIA-608", "CC1", 0.5, true)
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
