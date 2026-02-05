package mediainfo

import (
	"fmt"
	"math"
	"strconv"
	"strings"
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
	if bitsPerSecond >= 10_000_000 {
		mbps := bitsPerSecond / 1_000_000
		return fmt.Sprintf("%.1f Mb/s", mbps)
	}
	kbps := int64(math.Round(bitsPerSecond / 1000))
	return formatThousands(kbps) + " kb/s"
}

func formatBitrateKbps(kbps int64) string {
	if kbps <= 0 {
		return ""
	}
	return formatThousands(kbps) + " kb/s"
}

func formatBitratePrecise(bitsPerSecond float64) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	kbps := bitsPerSecond / 1000
	if kbps < 100 {
		return fmt.Sprintf("%.1f kb/s", kbps)
	}
	return formatThousands(int64(math.Round(kbps))) + " kb/s"
}

func formatBitrateSmall(bitsPerSecond float64) string {
	if bitsPerSecond <= 0 {
		return ""
	}
	if bitsPerSecond < 1000 {
		return fmt.Sprintf("%.0f b/s", bitsPerSecond)
	}
	return formatBitratePrecise(bitsPerSecond)
}

func formatThousands(value int64) string {
	if value < 1000 {
		return strconv.FormatInt(value, 10)
	}

	parts := []string{}
	for value > 0 {
		chunk := value % 1000
		value /= 1000
		if value > 0 {
			chunkStr := strconv.FormatInt(chunk, 10)
			for len(chunkStr) < 3 {
				chunkStr = "0" + chunkStr
			}
			parts = append(parts, chunkStr)
		} else {
			parts = append(parts, strconv.FormatInt(chunk, 10))
		}
	}

	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	var result strings.Builder
	result.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		result.WriteString(" " + parts[i])
	}
	return result.String()
}
