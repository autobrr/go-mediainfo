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

// TestFormatBitrate locks the unit and precision rule from MediaInfoLib
// Kilo_Kilo123 (File__Analyze_Streams.cpp). Each tier keeps one decimal until
// the scaled value passes 100, and every boundary is a strict "greater than".
func TestFormatBitrate(t *testing.T) {
	cases := []struct {
		bits float64
		want string
	}{
		{bits: 0, want: ""},
		{bits: 8000, want: "8 000 b/s"},
		{bits: 10_000, want: "10 000 b/s"},
		{bits: 10_001, want: "10.0 kb/s"},
		{bits: 63_488, want: "63.5 kb/s"},
		{bits: 100_000, want: "100.0 kb/s"},
		{bits: 100_001, want: "100 kb/s"},
		{bits: 1_536_000, want: "1 536 kb/s"},
		{bits: 9_515_000, want: "9 515 kb/s"},
		{bits: 10_000_000, want: "10 000 kb/s"},
		{bits: 10_414_978, want: "10.4 Mb/s"},
		{bits: 100_000_000, want: "100.0 Mb/s"},
		{bits: 100_000_001, want: "100 Mb/s"},
		{bits: 10_000_000_000, want: "10 000 Mb/s"},
		{bits: 10_000_000_001, want: "10.0 Gb/s"},
	}
	for _, tc := range cases {
		if got := formatBitrate(tc.bits); got != tc.want {
			t.Errorf("formatBitrate(%v)=%q want %q", tc.bits, got, tc.want)
		}
	}
}

func TestFormatBitrateKbps(t *testing.T) {
	if got := formatBitrateKbps(64); got != "64.0 kb/s" {
		t.Errorf("formatBitrateKbps(64)=%q want %q", got, "64.0 kb/s")
	}
	if got := formatBitrateKbps(448); got != "448 kb/s" {
		t.Errorf("formatBitrateKbps(448)=%q want %q", got, "448 kb/s")
	}
}
