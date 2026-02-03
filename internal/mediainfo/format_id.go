package mediainfo

import "fmt"

func formatID(id uint64) string {
	return fmt.Sprintf("%d (0x%X)", id, id)
}

func formatIDPair(id uint64, subID uint64) string {
	return fmt.Sprintf("%d (0x%X)-%d (0x%X)", id, id, subID, subID)
}
