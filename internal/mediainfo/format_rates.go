package mediainfo

import "fmt"

func formatFrameRate(rate float64) string {
	if rate <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS", rate)
}
