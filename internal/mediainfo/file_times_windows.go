//go:build windows

package mediainfo

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// fileTimes returns MediaInfo-formatted creation and modification timestamps in
// UTC and local time. It reports false when Windows file attributes are unavailable.
func fileTimes(path string) (string, string, string, string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", "", false
	}
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return "", "", "", "", false
	}
	created := time.Unix(0, stat.CreationTime.Nanoseconds())
	modified := time.Unix(0, stat.LastWriteTime.Nanoseconds())
	return formatWindowsFileTime(created.UTC(), true),
		formatWindowsFileTime(created.Local(), false),
		formatWindowsFileTime(modified.UTC(), true),
		formatWindowsFileTime(modified.Local(), false), true
}

// formatWindowsFileTime reproduces MediaInfo's Windows timestamp precision.
func formatWindowsFileTime(value time.Time, utc bool) string {
	// MediaInfo's Windows output keeps three significant fractional digits,
	// padding on the right when the millisecond field starts with zero.
	fraction := strconv.Itoa(value.Nanosecond() / int(time.Millisecond))
	if fraction == "0" {
		fraction = "000"
	} else {
		fraction = (fraction + strings.Repeat("0", 3))[:3]
	}
	formatted := value.Format("2006-01-02 15:04:05") + "." + fraction
	if utc {
		formatted += " UTC"
	}
	return formatted
}
