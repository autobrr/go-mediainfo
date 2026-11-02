package mediainfo

import "fmt"

func formatBytes(size int64) string {
	return formatByteQuantity(size, true)
}

// formatByteQuantity selects units before rounding, with dialect-specific grouping.
func formatByteQuantity(size int64, group bool) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d Bytes", size)
	}
	div := float64(size)
	exp := 0
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	for div >= unit && exp < len(units) {
		div /= unit
		exp++
	}
	if exp == 0 {
		return fmt.Sprintf("%.2f %s", div, units[0])
	}
	value := formatByteValue(div)
	if group {
		value = groupDecimal(value)
	}
	return fmt.Sprintf("%s %s", value, units[exp-1])
}

func formatByteValue(value float64) string {
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f", value)
	case value >= 10:
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.2f", value)
	}
}
