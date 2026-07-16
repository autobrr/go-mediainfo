package mediainfo

import "testing"

func TestFinalizeMPEGPSFallsBackToDerivedVideoDuration(t *testing.T) {
	streams := map[uint16]*psStream{
		psStreamKey(0xE0, psSubstreamNone): {
			id:              0xE0,
			subID:           psSubstreamNone,
			kind:            StreamVideo,
			format:          "MPEG Video",
			derivedDuration: 0.033,
		},
	}
	streamOrder := []uint16{psStreamKey(0xE0, psSubstreamNone)}

	info, _, ok := finalizeMPEGPS(streams, streamOrder, nil, ptsTracker{}, ptsTracker{}, 8<<10, mpegPSOptions{dvdParsing: true, parseSpeed: 0.5})
	if !ok {
		t.Fatalf("expected ok")
	}
	if info.DurationSeconds == 0 {
		t.Fatalf("expected DurationSeconds > 0")
	}
	if info.DurationSeconds < 0.032 || info.DurationSeconds > 0.034 {
		t.Fatalf("DurationSeconds = %f, want ~0.033", info.DurationSeconds)
	}
}

func TestDecimalSecondsToMilliseconds(t *testing.T) {
	for _, test := range []struct {
		seconds string
		want    string
		ok      bool
	}{
		{seconds: "4.008", want: "4008", ok: true},
		{seconds: "0.000001", want: "0.001", ok: true},
		{seconds: "-0.005", want: "-5", ok: true},
		{seconds: "12", want: "12000", ok: true},
		{seconds: ".", ok: false},
		{seconds: "bad", ok: false},
	} {
		got, ok := decimalSecondsToMilliseconds(test.seconds)
		if ok != test.ok || got != test.want {
			t.Fatalf("decimalSecondsToMilliseconds(%q) = %q, %v; want %q, %v", test.seconds, got, ok, test.want, test.ok)
		}
	}
}
