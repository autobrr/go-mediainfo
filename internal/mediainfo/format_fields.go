package mediainfo

import (
	"fmt"
	"math"
)

func formatPixels(value uint64) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d pixels", value)
}

func formatChannels(value uint64) string {
	if value == 0 {
		return ""
	}
	if value == 1 {
		return "1 channel"
	}
	return fmt.Sprintf("%d channels", value)
}

func formatSampleRate(rate float64) string {
	if rate <= 0 {
		return ""
	}
	if rate >= 1000 {
		return fmt.Sprintf("%.1f kHz", rate/1000)
	}
	return fmt.Sprintf("%.0f Hz", rate)
}

func formatBitDepth(bits uint8) string {
	if bits == 0 {
		return ""
	}
	return fmt.Sprintf("%d bits", bits)
}

func formatAspectRatio(width, height uint64) string {
	if width == 0 || height == 0 {
		return ""
	}
	g := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/g, height/g)
}

func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func formatBitsPerPixelFrame(bitrate float64, width, height uint64, fps float64) string {
	if bitrate <= 0 || width == 0 || height == 0 || fps <= 0 {
		return ""
	}
	value := bitrate / (float64(width) * float64(height) * fps)
	return fmt.Sprintf("%.3f", value)
}

func formatStreamSize(bytes int64, total int64) string {
	if bytes <= 0 || total <= 0 {
		return ""
	}
	percent := int(math.Round(float64(bytes) * 100 / float64(total)))
	return fmt.Sprintf("%s (%d%%)", formatBytes(bytes), percent)
}
