package mediainfo

import "fmt"

func formatFrameRate(rate float64) string {
	if rate <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS", rate)
}

func formatFrameRateRatio(numer, denom uint32) string {
	if numer == 0 || denom == 0 {
		return ""
	}
	rate := float64(numer) / float64(denom)
	return fmt.Sprintf("%.3f (%d/%d) FPS", rate, numer, denom)
}
