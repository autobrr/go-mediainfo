package mediainfo

import (
	"math"
	"testing"
)

func TestParseMPEGTimecodeSeconds(t *testing.T) {
	tests := []struct {
		name     string
		tc       string
		num      uint32
		den      uint32
		fallback float64
		want     float64
		ok       bool
	}{
		{name: "pal-hour", tc: "01:00:00:00", num: 25, den: 1, want: 3600, ok: true},
		{name: "ntsc-frame", tc: "00:00:00:01", num: 30000, den: 1001, want: 1001.0 / 30000.0, ok: true},
		{name: "semicolon", tc: "00:00:10;12", num: 25, den: 1, want: 10.48, ok: true},
		{name: "invalid", tc: "nope", num: 25, den: 1, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMPEGTimecodeSeconds(tt.tc, tt.num, tt.den, tt.fallback)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if math.Abs(got-tt.want) > 1e-6 {
				t.Fatalf("seconds=%f want %f", got, tt.want)
			}
		})
	}
}
