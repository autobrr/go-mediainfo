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
}
