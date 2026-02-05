package mediainfo

func consumeMPEG2HeaderBytes(entry *psStream, payload []byte, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	buf := append(entry.videoHeaderCarry, payload...)
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] != 0x00 || buf[i+1] != 0x00 || buf[i+2] != 0x01 {
			continue
		}
		switch buf[i+3] {
		case 0xB3:
			entry.videoHeaderBytes += 12
			if hasPTS {
				entry.videoSeqExtBytes += 12
			}
		case 0xB5:
			entry.videoHeaderBytes += 4
			if hasPTS {
				entry.videoSeqExtBytes += 4
			}
		case 0xB8:
			entry.videoHeaderBytes += 8
			entry.videoGOPBytes += 8
		case 0x00:
			entry.videoHeaderBytes += 6
		default:
			if buf[i+3] >= 0x01 && buf[i+3] <= 0xAF {
				entry.videoHeaderBytes += 6
			}
		}
	}
	if len(buf) >= 3 {
		entry.videoHeaderCarry = append(entry.videoHeaderCarry[:0], buf[len(buf)-3:]...)
	} else {
		entry.videoHeaderCarry = append(entry.videoHeaderCarry[:0], buf...)
	}
}

func consumeMPEG2FrameBytes(entry *psStream, payload []byte) {
	if entry == nil || len(payload) == 0 || entry.videoFrameBytesCount >= 2 {
		if entry != nil && entry.videoFrameBytesCount < 2 {
			entry.videoFramePos += int64(len(payload))
		}
		return
	}
	buf := append(entry.videoFrameCarry, payload...)
	basePos := entry.videoFramePos - int64(len(entry.videoFrameCarry))
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] != 0x00 || buf[i+1] != 0x00 || buf[i+2] != 0x01 || buf[i+3] != 0x00 {
			continue
		}
		pos := basePos + int64(i)
		if !entry.videoFrameStartSet {
			entry.videoFrameStartSet = true
			entry.videoFrameStart = pos
			continue
		}
		frameBytes := pos - entry.videoFrameStart
		if frameBytes > 0 {
			entry.videoFrameBytes += uint64(frameBytes)
			entry.videoFrameBytesCount++
		}
		entry.videoFrameStart = pos
		if entry.videoFrameBytesCount >= 2 {
			break
		}
	}
	if len(buf) >= 3 {
		entry.videoFrameCarry = append(entry.videoFrameCarry[:0], buf[len(buf)-3:]...)
	} else {
		entry.videoFrameCarry = append(entry.videoFrameCarry[:0], buf...)
	}
	entry.videoFramePos += int64(len(payload))
}

func mpeg2HeaderSize(code byte) uint64 {
	switch {
	case code == 0xB3:
		return 12
	case code == 0xB5:
		return 4
	case code == 0xB8:
		return 8
	case code == 0x00:
		return 6
	case code >= 0x01 && code <= 0xAF:
		return 6
	default:
		return 0
	}
}
