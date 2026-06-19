package mediainfo

import "testing"

// audioStreamWithID builds a minimal Matroska audio Stream carrying the given
// container track ID (so streamTrackNumber can match it to a probe).
func audioStreamWithID(id string) Stream {
	return Stream{Kind: StreamAudio, Fields: []Field{{Name: "ID", Value: id}}}
}

// Official MediaInfo emits Format_Settings_Endianness="Big" for every AC-3 and
// E-AC-3 track, not just E-AC-3. Regression guard for the parity audit (2026-06-19).
func TestApplyMatroskaAudioProbes_Endianness(t *testing.T) {
	for _, format := range []string{"AC-3", "E-AC-3"} {
		t.Run(format, func(t *testing.T) {
			info := &MatroskaInfo{Tracks: []Stream{audioStreamWithID("1")}}
			probes := map[uint64]*matroskaAudioProbe{
				1: {format: format, ok: true},
			}
			applyMatroskaAudioProbes(info, probes)
			if got := info.Tracks[0].JSON["Format_Settings_Endianness"]; got != "Big" {
				t.Fatalf("%s Format_Settings_Endianness = %q, want %q", format, got, "Big")
			}
		})
	}
}
