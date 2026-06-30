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
