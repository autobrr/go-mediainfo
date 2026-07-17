package mediainfo

import "testing"

func TestNormalizeTSStreamOrderBDAV(t *testing.T) {
	order := []uint16{0x1202, 0x1101, 0x1011, 0x1201, 0x1100, 0x1200, 0x1202}
	streams := map[uint16]*tsStream{
		0x1011: {pid: 0x1011, kind: StreamVideo, programNumber: 1},
		0x1100: {pid: 0x1100, kind: StreamAudio, programNumber: 1},
		0x1101: {pid: 0x1101, kind: StreamAudio, programNumber: 1},
		0x1200: {pid: 0x1200, kind: StreamText, programNumber: 1},
		0x1201: {pid: 0x1201, kind: StreamText, programNumber: 1},
		0x1202: {pid: 0x1202, kind: StreamText, programNumber: 1},
	}
	got := normalizeTSStreamOrder(order, streams, true)
	want := []uint16{0x1011, 0x1100, 0x1101, 0x1200, 0x1201, 0x1202}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeTSStreamOrder(bdav) mismatch at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestNormalizeTSStreamOrderTSKeepsDiscoveryOrder(t *testing.T) {
	order := []uint16{0x1201, 0x1100, 0x1200, 0x1100}
	streams := map[uint16]*tsStream{
		0x1100: {pid: 0x1100, kind: StreamAudio, programNumber: 1},
		0x1200: {pid: 0x1200, kind: StreamText, programNumber: 1},
		0x1201: {pid: 0x1201, kind: StreamText, programNumber: 1},
	}
	got := normalizeTSStreamOrder(order, streams, false)
	want := []uint16{0x1201, 0x1100, 0x1200}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeTSStreamOrder(ts) mismatch at %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestStructuredBDAVFieldOrder(t *testing.T) {
	tests := []struct {
		kind   StreamKind
		before string
		after  string
	}{
		{kind: StreamGeneral, before: "OverallBitRate_Maximum", after: "FrameRate"},
		{kind: StreamVideo, before: "Format_Settings_SliceCount", after: "CodecID"},
		{kind: StreamAudio, before: "Format_Settings_Mode", after: "Format_Settings_Endianness"},
		{kind: StreamAudio, before: "BitRate_Encoded", after: "BitRate_Maximum"},
		{kind: StreamAudio, before: "StreamSize", after: "StreamSize_Encoded"},
		{kind: StreamText, before: "MenuID", after: "Format"},
	}
	for _, tt := range tests {
		order := structuredFieldOrderForContainer(tt.kind, "BDAV")
		if order[tt.before] >= order[tt.after] {
			t.Errorf("%s order=%d must precede %s order=%d", tt.before, order[tt.before], tt.after, order[tt.after])
		}
	}
}

func TestRecordH264SliceCountPreservesDominantValue(t *testing.T) {
	entry := &tsStream{h264SliceCounts: map[int]int{1: 1, 4: 2}, h264SliceCount: 4}
	// A payload without complete slice NAL units must preserve accumulated state.
	recordH264SliceCount(entry, []byte{0, 0, 1, 9, 0xf0})
	if entry.h264SliceCount != 4 {
		t.Fatalf("h264SliceCount=%d want=4", entry.h264SliceCount)
	}
}

func TestMergeTSStreamFromPMTPreservesLanguageOnEmptyUpdate(t *testing.T) {
	existing := &tsStream{
		pid:      4611,
		kind:     StreamText,
		format:   "PGS",
		language: "zho",
	}
	mergeTSStreamFromPMT(existing, tsStream{
		pid:           4611,
		kind:          StreamText,
		format:        "PGS",
		streamType:    0x90,
		programNumber: 1,
		language:      "",
	})
	if existing.language != "zho" {
		t.Fatalf("mergeTSStreamFromPMT() language=%q, want %q", existing.language, "zho")
	}
}

func TestMergeTSStreamFromPMTUpdatesLanguageWhenPresent(t *testing.T) {
	existing := &tsStream{
		pid:      4611,
		kind:     StreamText,
		format:   "PGS",
		language: "",
	}
	mergeTSStreamFromPMT(existing, tsStream{
		pid:           4611,
		kind:          StreamText,
		format:        "PGS",
		streamType:    0x90,
		programNumber: 1,
		language:      "zho",
	})
	if existing.language != "zho" {
		t.Fatalf("mergeTSStreamFromPMT() language=%q, want %q", existing.language, "zho")
	}
}

func TestMergeTSStreamFromPMTReplacesProvisionalParserState(t *testing.T) {
	existing := &tsStream{
		pid: 49, provisional: true, kind: StreamVideo, format: "MPEG Video",
		videoFrameCount: 15, ccFound: true,
	}
	existing.pts.add(90_000)
	parsed := tsStream{pid: 49, programNumber: 3, streamType: 0x02, kind: StreamVideo, format: "MPEG Video"}

	mergeTSStreamFromPMT(existing, parsed)

	if existing.provisional || existing.pts.has() || existing.videoFrameCount != 0 || existing.ccFound {
		t.Fatalf("provisional state survived PMT merge: %+v", existing)
	}
	if existing.programNumber != 3 || existing.streamType != 0x02 {
		t.Fatalf("PMT identity not applied: %+v", existing)
	}
}
