package mediainfo

import (
	"math"
	"strconv"
)

// sumCanonicalStreamSizes totals direct canonical stream sizes only.
func sumCanonicalStreamSizes(streams []Stream) int64 {
	var sum int64
	for _, stream := range streams {
		value, _ := canonicalSeedValue(stream, "StreamSize")
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			sum += parsed
		}
	}
	return sum
}

// remainingStreamSizeValue returns the non-negative container remainder after
// at least one payload stream contributes a known byte size.
func remainingStreamSizeValue(total int64, streamSizeSum int64) (string, bool) {
	if streamSizeSum <= 0 {
		return "", false
	}
	remaining := total - streamSizeSum
	if remaining < 0 {
		return "", false
	}
	return strconv.FormatInt(remaining, 10), true
}

// overallBitRateValue returns MediaInfo's integer-millisecond whole-file rate.
func overallBitRateValue(size int64, duration float64) (string, bool) {
	if duration <= 0 {
		return "", false
	}
	// Match MediaInfo: bitrate computations are effectively based on integer milliseconds.
	durationMs := int64(math.Round(duration * 1000))
	if durationMs <= 0 {
		return "", false
	}
	overall := (size*8000 + durationMs/2) / durationMs
	if overall > 0 {
		return strconv.FormatInt(overall, 10), true
	}
	return "", false
}
