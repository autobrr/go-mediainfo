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
