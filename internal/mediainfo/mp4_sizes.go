package mediainfo

import "io"

func mp4TopLevelSizes(r io.ReaderAt, size int64) (int64, int64, int64, int, bool, bool) {
	var headerSize int64
	var dataSize int64
	var footerSize int64
	var mdatCount int
	var moovBeforeMdat bool
	seenMdat := false
	offset := int64(0)
	for offset+8 <= size {
		boxSize, boxType, _, ok := readMP4BoxHeader(r, offset, size)
		if !ok || boxSize <= 0 {
			break
		}
		if boxType == "moov" && !seenMdat {
			moovBeforeMdat = true
		}
		switch {
		case boxType == "mdat":
			dataSize += boxSize
			mdatCount++
			seenMdat = true
		case !seenMdat:
			headerSize += boxSize
		default:
			footerSize += boxSize
		}
		offset += boxSize
	}
	if mdatCount == 0 {
		return 0, 0, 0, 0, false, false
	}
	return headerSize, dataSize, footerSize, mdatCount, moovBeforeMdat, true
}
