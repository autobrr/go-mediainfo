package mediainfo

import "testing"

func TestMapMatroskaCodecID(t *testing.T) {
	tests := []struct {
		codecID   string
		trackType uint64
		kind      StreamKind
		format    string
	}{
		{codecID: "A_OPUS", trackType: 2, kind: StreamAudio, format: "Opus"},
		{codecID: "S_DVBSUB", trackType: 17, kind: StreamText, format: "DVB Subtitle"},
	}
	for _, test := range tests {
		kind, format := mapMatroskaCodecID(test.codecID, test.trackType)
		if kind != test.kind || format != test.format {
			t.Fatalf("mapMatroskaCodecID(%q) = %v, %q; want %v, %q", test.codecID, kind, format, test.kind, test.format)
		}
	}
}
