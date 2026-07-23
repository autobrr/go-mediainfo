package mediainfo

import "testing"

func TestParseStszFixed(t *testing.T) {
	payload := make([]byte, 12)
	payload[4] = 0
	payload[5] = 0
	payload[6] = 0
	payload[7] = 4
	payload[8] = 0
	payload[9] = 0
	payload[10] = 0
	payload[11] = 10
	if total, ok := parseStsz(payload); !ok || total != 40 {
		t.Fatalf("total=%d ok=%v", total, ok)
	}
}

func TestParseStszTable(t *testing.T) {
	payload := make([]byte, 12+3*4)
	payload[8] = 0
	payload[9] = 0
	payload[10] = 0
	payload[11] = 3
	payload[12+3] = 1
	payload[12+7] = 2
	payload[12+11] = 3
	if total, ok := parseStsz(payload); !ok || total != 6 {
		t.Fatalf("total=%d ok=%v", total, ok)
	}
}

func TestParseStszWithHeadBoundsRetainedSampleSizes(t *testing.T) {
	const sampleCount = 256
	payload := make([]byte, 12)
	payload[7] = 4
	payload[10] = byte(sampleCount >> 8)
	payload[11] = byte(sampleCount & 0xFF)

	total, head, tail, ok := parseStszWithHead(payload, sampleCount)
	if !ok || total != sampleCount*4 {
		t.Fatalf("total=%d ok=%v; want %d/true", total, ok, sampleCount*4)
	}
	if len(head) != mp4SampleSizeHeadMax {
		t.Fatalf("head sizes=%d; want bounded %d", len(head), mp4SampleSizeHeadMax)
	}
	if len(tail) != mp4SampleSizeTailMax {
		t.Fatalf("tail sizes=%d; want bounded %d", len(tail), mp4SampleSizeTailMax)
	}
	for index, size := range append(append([]uint32(nil), head...), tail...) {
		if size != 4 {
			t.Fatalf("retained size %d = %d; want 4", index, size)
		}
	}
}
