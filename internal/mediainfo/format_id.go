package mediainfo

import "fmt"

func formatID(id uint64) string {
	return fmt.Sprintf("%d (0x%X)", id, id)
}
