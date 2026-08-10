package mediainfo

import "testing"

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		seconds float64
		want    string
	}{
		{seconds: 0, want: ""},
		{seconds: 0.25, want: "250 ms"},
		{seconds: 1, want: "1 s 0 ms"},
		{seconds: 1.5, want: "1 s 500 ms"},
		{seconds: 61, want: "1 min 1 s"},
		{seconds: 3661, want: "1 h 1 min"},
		{seconds: 3600, want: "1 h 0 min"},
		{seconds: 6006.080, want: "1 h 40 min"}, // official v23.04 on mp4-01
		{seconds: 3300.923, want: "55 min 0 s"}, // truncates, never rounds up
	}
	for _, tc := range cases {
		got := formatDuration(tc.seconds)
		if got != tc.want {
			t.Fatalf("formatDuration(%v)=%q want %q", tc.seconds, got, tc.want)
		}
	}
}

func TestFormatBitrate(t *testing.T) {
	if got := formatBitrate(9515000); got != "9 515 kb/s" {
		t.Fatalf("formatBitrate=%q", got)
	}
}
