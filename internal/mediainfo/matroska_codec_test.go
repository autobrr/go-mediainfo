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
		{codecID: "V_AV1", trackType: 1, kind: StreamVideo, format: "AV1"},
	}
	for _, test := range tests {
		kind, format := mapMatroskaCodecID(test.codecID, test.trackType)
		if kind != test.kind || format != test.format {
			t.Fatalf("mapMatroskaCodecID(%q) = %v, %q; want %v, %q", test.codecID, kind, format, test.kind, test.format)
		}
	}
}

func TestMapMatroskaFormatInfoAV1(t *testing.T) {
	got := mapMatroskaFormatInfo("AV1")
	if got != "AOMedia Video 1" {
		t.Fatalf("mapMatroskaFormatInfo(AV1) = %q; want %q", got, "AOMedia Video 1")
	}
}
