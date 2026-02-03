package mediainfo

import (
	"fmt"
	"math"
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
		totalMs = totalSec * 1000
	}
	if totalSec < 60 {
		return fmt.Sprintf("%d s %d ms", totalSec, remMs)
	}

	hours := totalSec / 3600
	minutes := (totalSec % 3600) / 60
	secondsOnly := totalSec % 60
	if hours > 0 {
		return fmt.Sprintf("%d h %d min %d s", hours, minutes, secondsOnly)
	}
	return fmt.Sprintf("%d min %d s", minutes, secondsOnly)
}

func formatBitrate(bitsPerSecond float64) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	if bitsPerSecond >= 1_000_000 {
		mbps := bitsPerSecond / 1_000_000
		return fmt.Sprintf("%.1f Mb/s", mbps)
	}
	kbps := int64(math.Round(bitsPerSecond / 1000))
	return fmt.Sprintf("%s kb/s", formatThousands(kbps))
}

func formatBitrateKbps(kbps int64) string {
	if kbps <= 0 {
		return ""
	}
	return fmt.Sprintf("%s kb/s", formatThousands(kbps))
}

func formatBitratePrecise(bitsPerSecond float64) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	kbps := bitsPerSecond / 1000
	if kbps < 100 {
		return fmt.Sprintf("%.1f kb/s", kbps)
	}
	return fmt.Sprintf("%s kb/s", formatThousands(int64(math.Round(kbps))))
}

func formatThousands(value int64) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}

	parts := []string{}
	for value > 0 {
		chunk := value % 1000
		value /= 1000
		if value > 0 {
			parts = append(parts, fmt.Sprintf("%03d", chunk))
		} else {
			parts = append(parts, fmt.Sprintf("%d", chunk))
		}
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += " " + parts[i]
	}
	return result
}
