package mediainfo

import (
	"fmt"
	"math"
	"strconv"
)

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}

	totalMs := int64(math.Round(seconds * 1000))
	if totalMs < 1000 {
		return fmt.Sprintf("%d ms", totalMs)
	}

	totalSec := totalMs / 1000
	remMs := totalMs % 1000
	if totalSec == 59 && remMs >= 500 {
		totalSec = 60
		remMs = 0
	}
	if totalSec < 60 {
		return fmt.Sprintf("%d s %d ms", totalSec, remMs)
	}

	// Only the two most significant units are shown, so seconds drop out once
	// the duration reaches an hour.
	if totalSec >= 3600 {
		return fmt.Sprintf("%d h %d min", totalSec/3600, totalSec%3600/60)
	}
	return fmt.Sprintf("%d min %d s", totalSec/60, totalSec%60)
}

// formatBitrate renders a bit rate the way MediaInfoLib Kilo_Kilo123
// (File__Analyze_Streams.cpp) does. Each unit tier keeps one decimal until the
// source value passes ten times the tier, and every boundary is a strict
// "greater than": 10 000 000 b/s still prints as "10 000 kb/s".
func formatBitrate(bitsPerSecond float64) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	bits := int64(bitsPerSecond)
	switch {
	case bits > 10_000_000_000:
		return scaleBitrate(bits, 1_000_000_000, "Gb/s")
	case bits > 10_000_000:
		return scaleBitrate(bits, 1_000_000, "Mb/s")
	case bits > 10_000:
		return scaleBitrate(bits, 1_000, "kb/s")
	}
	return formatThousands(bits) + " b/s"
}

// scaleBitrate divides into one unit tier. MediaInfo does the division in
// float32, so the rounding of the last digit follows single precision.
func scaleBitrate(bits, unit int64, label string) string {
	scaled := float64(float32(bits) / float32(unit))
	if bits <= unit*100 {
		// The scaled value is at most 100.0 here, so it needs no grouping.
		return strconv.FormatFloat(scaled, 'f', 1, 64) + " " + label
	}
	return formatThousands(int64(math.RoundToEven(scaled))) + " " + label
}

func formatBitrateKbps(kbps int64) string {
	return formatBitrate(float64(kbps) * 1000)
}

// formatThousands groups a non-negative value into digit triples. Both callers
// guard against negative input.
func formatThousands(value int64) string {
	text := strconv.FormatInt(value, 10)
	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + " " + text[index:]
	}
	return text
}
