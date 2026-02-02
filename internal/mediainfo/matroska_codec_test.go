package mediainfo

import "testing"

func TestMapMatroskaCodecID(t *testing.T) {
	kind, format := mapMatroskaCodecID("A_OPUS", 2)
	if kind != StreamAudio || format != "Opus" {
		t.Fatalf("unexpected mapping: %v %s", kind, format)
	}
}
