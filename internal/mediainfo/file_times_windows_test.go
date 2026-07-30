//go:build windows

package mediainfo

import (
	"testing"
	"time"
)

func TestFormatWindowsFileTimeMatchesMediaInfoFraction(t *testing.T) {
	value := time.Date(2026, time.June, 30, 7, 1, 59, 98_588_200, time.UTC)
	if got := formatWindowsFileTime(value, true); got != "2026-06-30 07:01:59.980 UTC" {
		t.Fatalf("formatWindowsFileTime() = %q", got)
	}

	tests := []struct {
		name string
		ns   int
		utc  bool
		want string
	}{
		{name: "zero", ns: 0, utc: true, want: "2026-06-30 07:01:59.000 UTC"},
		{name: "five milliseconds", ns: 5_000_000, utc: true, want: "2026-06-30 07:01:59.500 UTC"},
		{name: "fifty milliseconds", ns: 50_000_000, utc: true, want: "2026-06-30 07:01:59.500 UTC"},
		{name: "999 milliseconds", ns: 999_000_000, utc: true, want: "2026-06-30 07:01:59.999 UTC"},
		{name: "local", ns: 98_588_200, utc: false, want: "2026-06-30 07:01:59.980"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := time.Date(2026, time.June, 30, 7, 1, 59, test.ns, time.UTC)
			if got := formatWindowsFileTime(value, test.utc); got != test.want {
				t.Fatalf("formatWindowsFileTime() = %q, want %q", got, test.want)
			}
		})
	}
}
