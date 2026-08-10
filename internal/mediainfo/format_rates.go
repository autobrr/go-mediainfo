package mediainfo

import "fmt"

func formatFrameRate(rate float64) string {
	if rate <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS", rate)
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
	return fmt.Sprintf("%.3f (%d/%d) FPS", rate, numer, denom)
}
