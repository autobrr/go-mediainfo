package mediainfo

func bitrateMode(bits float64) string {
	if bits <= 0 {
		return ""
	}
	return "Variable"
}
