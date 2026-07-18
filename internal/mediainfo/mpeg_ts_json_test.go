package mediainfo

import (
	"reflect"
	"testing"
)

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

func TestUpdateDTSHDExtensionFlagsIncludesByte512Boundary(t *testing.T) {
	const xllSync = "\x41\xA2\x95\x47"
	const dtsxSync = "\x02\x00\x08\x50"

	boundary := append([]byte(xllSync), make([]byte, 504)...)
	boundary = append(boundary, []byte(dtsxSync)...)
	entry := &tsStream{}
	updateDTSHDExtensionFlags(entry, boundary)
	if !entry.dtsHDXLL || !entry.dtsHDX {
		t.Fatalf("byte-512 marker not recognized: XLL=%v X=%v", entry.dtsHDXLL, entry.dtsHDX)
	}

	outside := append([]byte(xllSync), make([]byte, 505)...)
	outside = append(outside, []byte(dtsxSync)...)
	entry = &tsStream{}
	updateDTSHDExtensionFlags(entry, outside)
	if entry.dtsHDX {
		t.Fatal("byte-513 marker escaped the bounded DTS-HD probe")
	}

	entry = &tsStream{}
	updateDTSHDExtensionFlags(entry, append([]byte(xllSync), []byte(dtsxSync)...))
	if !entry.dtsHDX {
		t.Fatal("existing adjacent DTS:X marker control regressed")
	}
}

func TestConsumeAC3AndEAC3RetainSplitTrueHDMajorSync(t *testing.T) {
	frame := []byte{
		0x61, 0xC8, 0xFF, 0xD5,
		0xF8, 0x72, 0x6F, 0xBA, 0x00, 0x17, 0x80, 0x4F,
		0xB7, 0x52, 0x10, 0x00, 0x00, 0x00, 0x8A, 0xAD,
		0x43, 0xFC, 0x00, 0x00, 0x7E, 0xEF, 0xE3, 0x07,
		0xE3, 0x01, 0x1F, 0xC6, 0xBC, 0x00, 0x5C, 0xBD,
	}
	for _, test := range []struct {
		name    string
		consume func(*tsStream, []byte)
	}{
		{name: "AC3", consume: func(entry *tsStream, payload []byte) { consumeAC3(entry, payload, false, false, false) }},
		{name: "EAC3", consume: func(entry *tsStream, payload []byte) { consumeEAC3(entry, payload, false, false, false) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := &tsStream{}
			test.consume(entry, frame[:6])
			if entry.hasTrueHDInfo {
				t.Fatal("incomplete major sync parsed early")
			}
			test.consume(entry, frame[6:])
			if !entry.hasTrueHD || !entry.hasTrueHDInfo || entry.trueHDInfo.channelMap != 0x4F {
				t.Fatalf("split major sync lost: TrueHD=%v info=%v map=%#x", entry.hasTrueHD, entry.hasTrueHDInfo, entry.trueHDInfo.channelMap)
			}

			control := &tsStream{}
			test.consume(control, frame)
			if !control.hasTrueHDInfo {
				t.Fatal("contiguous major-sync control regressed")
			}

			invalid := append([]byte(nil), frame...)
			invalid[7] ^= 1
			negative := &tsStream{}
			test.consume(negative, invalid[:6])
			test.consume(negative, invalid[6:])
			if negative.hasTrueHDInfo {
				t.Fatal("invalid neighboring sync parsed as TrueHD")
			}
		})
	}
}

func TestMPEG2CaptionQueueFlushesDeferredFinalPacket(t *testing.T) {
	payload := []byte{
		0x00, 0x00, 0x01, 0x00, 0x00, 0x08,
		0x00, 0x00, 0x01, 0xB2,
		'G', 'A', '9', '4', 0x03, 0x01, 0xFF, 0x04, 'H', 'i',
		0x00, 0x00, 0x01, 0xB7,
	}
	entry := &tsStream{}
	consumeMPEG2CaptionsTS(entry, payload, 90_000, true)
	if len(entry.mpeg2XDSReorder) != 1 || entry.ccFound {
		t.Fatalf("deferred queue state = len %d, parsed %v", len(entry.mpeg2XDSReorder), entry.ccFound)
	}

	// Both the bounded-jump reset and EOF finalization paths call this flush
	// before discarding or projecting stream state.
	flushMPEG2XDSReorder(entry)
	if len(entry.mpeg2XDSReorder) != 0 || !entry.ccFound || !entry.ccOdd.hasFirstContentPTS {
		t.Fatalf("final flush state = len %d, parsed %v, timed %v", len(entry.mpeg2XDSReorder), entry.ccFound, entry.ccOdd.hasFirstContentPTS)
	}
	flushMPEG2XDSReorder(entry)
	if len(entry.mpeg2XDSReorder) != 0 {
		t.Fatal("empty neighboring flush changed queue state")
	}
}

func TestCaptionDisplayBeginningAtPTSZeroIsPresent(t *testing.T) {
	track := &ccTrack{found: true, firstDisplayFrame: 0}
	recordCCDisplayChange(track, 0)
	track.lastContentPTS = 90_000
	if !track.hasFirstContentPTS || track.firstContentPTS != 0 {
		t.Fatalf("zero PTS presence lost: present=%v value=%d", track.hasFirstContentPTS, track.firstContentPTS)
	}
	stream := buildTSCaptionStream(256, 1, 0, 1, 30, "EIA-608", "CC1", track, tsCaptionService{}, false, true)
	if got, ok := canonicalSeedValue(stream, "Duration_Start"); !ok || got != "0" {
		t.Fatalf("zero caption start = %q, present=%v", got, ok)
	}
	video := &tsStream{ccOdd: *track}
	video.ccOdd.hasFirstCommandPTS = true
	video.ccOdd.firstCommandPTS = 0
	if !video.hasValidCEA608() {
		t.Fatal("hasValidCEA608() rejected command present at PTS zero")
	}
	video.dtvccServices = map[int]struct{}{1: {}}
	if !video.readyForDTVCCHeadLock() {
		t.Fatal("readyForDTVCCHeadLock() rejected content present at PTS zero")
	}
}

func TestTSCaptionReadinessRequiresIndependentEvidence(t *testing.T) {
	if (&tsStream{ccOdd: ccTrack{found: true}}).hasValidCEA608() {
		t.Fatal("hasValidCEA608() accepted an untimed caption track")
	}
	if (&tsStream{ccOdd: ccTrack{hasFirstCommandPTS: true}}).hasValidCEA608() {
		t.Fatal("hasValidCEA608() accepted timing without a discovered caption track")
	}
	if (&tsStream{}).readyForDTVCCHeadLock() {
		t.Fatal("readyForDTVCCHeadLock() accepted no DTVCC service")
	}
	video := &tsStream{
		dtvccServices: map[int]struct{}{1: {}},
		ccOdd:         ccTrack{found: true},
	}
	if video.readyForDTVCCHeadLock() {
		t.Fatal("readyForDTVCCHeadLock() accepted co-carried captions without first content")
	}
	video.ccOdd.hasFirstContentPTS = true
	video.ccOdd.firstContentPTS = 0
	if !video.readyForDTVCCHeadLock() {
		t.Fatal("readyForDTVCCHeadLock() rejected first content present at PTS zero")
	}
}

func TestMPEGTSMPEG2ExtraFieldsOmitEmptyObject(t *testing.T) {
	if extras := mpegTSMPEG2ExtraFields(mpeg2VideoInfo{}, false, 60); len(extras) != 0 {
		t.Fatalf("empty non-BDAV extras = %#v", extras)
	}
	extras := mpegTSMPEG2ExtraFields(mpeg2VideoInfo{IntraDCPrecision: 9}, false, 60)
	if !reflect.DeepEqual(extras, []jsonKV{{Key: "intra_dc_precision", Val: "9"}}) {
		t.Fatalf("populated extras = %#v", extras)
	}
	extras = mpegTSMPEG2ExtraFields(mpeg2VideoInfo{IntraDCPrecision: 9, IntraDCPrecisionFirst: 8}, true, 20)
	want := []jsonKV{{Key: "intra_dc_precision", Val: "8"}, {Key: "format_identifier", Val: "HDMV"}}
	if !reflect.DeepEqual(extras, want) {
		t.Fatalf("BDAV extras = %#v, want %#v", extras, want)
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
