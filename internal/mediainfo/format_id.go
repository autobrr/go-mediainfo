package mediainfo

import (
	"fmt"
	"strconv"
	"strings"
)

func formatID(id uint64) string {
	return fmt.Sprintf("%d (0x%X)", id, id)
}

func formatIDPair(id uint64, subID uint64) string {
	return fmt.Sprintf("%d (0x%X)-%d (0x%X)", id, id, subID, subID)
}

// formatOriginalSourceID preserves the reference's unseparated packed-ID form.
func formatOriginalSourceID(value, fallback string) string {
	var result strings.Builder
	for _, part := range strings.Split(value, "-") {
		id, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return fallback
		}
		result.WriteString(formatID(id))
	}
	return result.String()
}
