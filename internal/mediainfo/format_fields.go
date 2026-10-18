package mediainfo

import (
	"fmt"
	"math"
)

func formatPixels(value uint64) string {
	if value == 0 {
		return ""
	}
	return formatThousands(int64(value)) + " pixels"
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
	dar := float64(width) / float64(height)
	// Match MediaInfoLib's DisplayAspectRatio_Fill named-ratio table
	// (File__Analyze_Streams.cpp).
	switch {
	case dar >= 0.54 && dar < 0.58:
		return "9:16"
	case dar >= 1.23 && dar < 1.27:
		return "5:4"
	case dar >= 1.30 && dar < 1.37:
		return "4:3"
	case dar >= 1.45 && dar < 1.55:
		return "3:2"
	case dar >= 1.55 && dar < 1.65:
		return "16:10"
	case dar >= 1.65 && dar < 1.70:
		return "5:3"
	case dar >= 1.74 && dar < 1.82:
		return "16:9"
	case dar >= 1.82 && dar < 1.88:
		return "1.85:1"
	case dar >= 2.15 && dar < 2.22:
		return "2.2:1"
	case dar >= 2.23 && dar < 2.30:
		return "2.25:1"
	case dar >= 2.30 && dar < 2.37:
		return "2.35:1"
	case dar >= 2.37 && dar < 2.395:
		return "2.39:1"
	case dar >= 2.395 && dar < 2.45:
		return "2.40:1"
	default:
		return fmt.Sprintf("%.3f", dar)
	}
}

// formatBitsPerPixelFrame retains the CLI's float32 arithmetic before decimal rounding.
func formatBitsPerPixelFrame(bitrate float64, width, height uint64, fps float64) string {
	if bitrate <= 0 || width == 0 || height == 0 || fps <= 0 {
		return ""
	}
	value := float32(bitrate) / (float32(width) * float32(height) * float32(fps))
	return fmt.Sprintf("%.3f", value)
}

func formatStreamSize(bytes int64, total int64) string {
	if bytes <= 0 || total <= 0 {
		return ""
	}
	percent := int(math.Round(float64(bytes) * 100 / float64(total)))
	return fmt.Sprintf("%s (%d%%)", formatBytes(bytes), percent)
}
