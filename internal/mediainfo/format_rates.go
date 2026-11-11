package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func formatFrameRate(rate float64) string {
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return ""
	}
	return groupDecimal(fmt.Sprintf("%.3f", rate)) + " FPS"
}

func formatFrameRateWithRatio(rate float64) string {
	if rate <= 0 {
		return ""
	}
	num, den := rationalizeFrameRate(rate)
	if num > 0 && den > 1 {
		return formatFrameRateRatio(uint32(num), uint32(den))
	}
	return formatFrameRate(rate)
}

func formatFrameRateRatio(numer, denom uint32) string {
	if numer == 0 || denom == 0 {
		return ""
	}
	rate := float64(numer) / float64(denom)
	// MediaInfo only decorates fractional cadences; x/1 displays plain.
	if denom == 1 {
		return formatFrameRate(rate)
	}
	return fmt.Sprintf("%s (%d/%d) FPS", groupDecimal(fmt.Sprintf("%.3f", rate)), numer, denom)
}

// groupDecimal groups the rounded mantissa without changing its fractional digits.
func groupDecimal(value string) string {
	whole, fraction, decimal := strings.Cut(value, ".")
	integer, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || integer < 0 {
		return value
	}
	result := formatThousands(integer)
	if decimal {
		result += "." + fraction
	}
	return result
}

func formatReferenceFrames(count int) string {
	if count <= 0 {
		return ""
	}
	if count == 1 {
		return "1 frame"
	}
	return strconv.Itoa(count) + " frames"
}

func formatRawTextAspectRatio(value string) string {
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	switch {
	case math.Abs(ratio-4.0/3.0) < 0.01:
		return "4:3"
	case math.Abs(ratio-16.0/9.0) < 0.01:
		return "16:9"
	default:
		return value
	}
}

func formatRawTextBitRate(value string) string {
	parsed, err := strconv.ParseFloat(value, 32)
	bitRate := float32(parsed)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if bitRate <= 10_000 {
		return fmt.Sprintf("%.0f bps", bitRate)
	}
	if bitRate <= 100_000 {
		return fmt.Sprintf("%.1f Kbps", bitRate/1_000)
	}
	return fmt.Sprintf("%.0f Kbps", bitRate/1_000)
}

// formatRawTextPCMBitRate preserves the fractional decimal-kilobit precision
// MediaInfo uses for common AVI PCM rates.
func formatRawTextPCMBitRate(value string) string {
	bitRate, err := strconv.ParseFloat(value, 64)
	if err != nil || bitRate <= 0 {
		return ""
	}
	if math.Mod(bitRate, 1000) != 0 {
		return fmt.Sprintf("%.1f Kbps", bitRate/1000)
	}
	return formatRawTextBitRate(value)
}

func formatRawTextDuration(milliseconds float64) string {
	total := int64(math.Round(milliseconds))
	total = max(total, 0)
	hours := total / 3_600_000
	minutes := total / 60_000 % 60
	seconds := total / 1_000 % 60
	millis := total % 1_000
	switch {
	case hours > 0:
		return fmt.Sprintf("%dh %dmn", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dmn %ds", minutes, seconds)
	default:
		return fmt.Sprintf("%ds %dms", seconds, millis)
	}
}
