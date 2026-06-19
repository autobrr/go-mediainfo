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

// Official MediaInfo emits Video.FrameRate_Num/Den for Matroska CFR video even
// when the displayed rate is an integer (no "(num/den)" hint), which the
// text-token path can't recover. Parity audit (2026-06-19): 5 integer-rate MKVs.
func TestBuildJSONComputedFields_MatroskaFrameRateNumDen(t *testing.T) {
	t.Run("integer rate emits Num/Den", func(t *testing.T) {
		fields := []jsonKV{
			{Key: "Duration", Val: "1383.040"},
			{Key: "FrameRate", Val: "25.000"},
		}
		out := buildJSONComputedFields(StreamVideo, fields, "Matroska")
		if num, den := jsonFieldValue(out, "FrameRate_Num"), jsonFieldValue(out, "FrameRate_Den"); num != "25" || den != "1" {
			t.Fatalf("got Num=%q Den=%q, want 25/1", num, den)
		}
	})
	// 23.976 already carries Num/Den from the text-token path; must not re-emit.
	t.Run("ratio already present is not duplicated", func(t *testing.T) {
		fields := []jsonKV{
			{Key: "Duration", Val: "100"},
			{Key: "FrameRate", Val: "23.976"},
			{Key: "FrameRate_Num", Val: "24000"},
			{Key: "FrameRate_Den", Val: "1001"},
		}
		out := buildJSONComputedFields(StreamVideo, fields, "Matroska")
		if got := jsonFieldValue(out, "FrameRate_Num"); got != "" {
			t.Fatalf("re-emitted FrameRate_Num=%q, want none", got)
		}
	})
}
