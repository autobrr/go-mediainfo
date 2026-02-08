package mediainfo

import "testing"

func TestApplyMatroskaStats_AudioDurationAlsoSetsJSON(t *testing.T) {
	info := MatroskaInfo{
		Tracks: []Stream{
			{
				Kind: StreamAudio,
				Fields: []Field{
					{Name: "ID", Value: "1"},
					{Name: "Format", Value: "AAC"},
				},
			},
		},
	}

	stats := map[uint64]*matroskaTrackStats{
		1: {
			hasTime:   true,
			minTimeNs: 0,
			maxTimeNs: int64(4.321 * 1e9),
		},
	}

	applyMatroskaStats(&info, stats, 0)

	if got := findField(info.Tracks[0].Fields, "Duration"); got == "" {
		t.Fatalf("expected Duration field set")
	}
	if info.Tracks[0].JSON == nil || info.Tracks[0].JSON["Duration"] == "" {
		t.Fatalf("expected JSON Duration set")
	}
}
