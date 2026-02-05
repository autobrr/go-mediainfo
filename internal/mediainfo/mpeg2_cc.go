package mediainfo

import "strings"

func consumeMPEG2Captions(entry *psStream, payload []byte, pts uint64, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	entry.videoCCCarry = append(entry.videoCCCarry, payload...)
	buf := entry.videoCCCarry
	for i := 0; i+4 <= len(buf); i++ {
		if buf[i] != 0x00 || buf[i+1] != 0x00 || buf[i+2] != 0x01 {
			continue
		}
		code := buf[i+3]
		switch code {
		case 0x00:
			entry.videoFrameCount++
		case 0xB2:
			end := nextStartCode(buf, i+4)
			if end < 0 {
				end = len(buf)
			}
			if hasCC, ccType, hasCommand, hasDisplay := parseMPEG2UserData(buf[i+4 : end]); hasCC {
				framesBefore := entry.videoFrameCount
				entry.ccFound = true
				track := &entry.ccOdd
				if ccType == 1 {
					track = &entry.ccEven
				}
				if !track.found {
					track.found = true
					if hasPTS {
						track.firstPTS = pts
					}
				}
				if track.firstFrame < 0 {
					track.firstFrame = framesBefore
				}
				track.lastFrame = framesBefore
				if hasPTS {
					track.lastPTS = pts
				}
				if hasPTS && hasCommand && track.firstCommandPTS == 0 {
					track.firstCommandPTS = pts
				}
				if hasDisplay && track.firstType == "" {
					if hasPTS {
						track.firstDisplayPTS = pts
					}
					track.firstFrame = framesBefore
					track.firstType = "PopOn"
				}
			}
		}
	}
	if len(buf) >= 3 {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf[len(buf)-3:]...)
	} else {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf...)
	}
}

func nextStartCode(data []byte, start int) int {
	for i := start; i+3 < len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x01 {
			return i
		}
	}
	return -1
}

func parseGA94UserData(data []byte) (bool, int, bool, bool) {
	if len(data) < 6 {
		return false, 0, false, false
	}
	hasCC := false
	hasCommand := false
	hasDisplay := false
	seenType0 := false
	seenType1 := false
	for i := 0; i+5 < len(data); i++ {
		if data[i] != 'G' || data[i+1] != 'A' || data[i+2] != '9' || data[i+3] != '4' {
			continue
		}
		if data[i+4] != 0x03 {
			continue
		}
		if i+6 > len(data) {
			continue
		}
		flags := data[i+5]
		count := int(flags & 0x1F)
		idx := i + 6
		if idx >= len(data) {
			continue
		}
		idx++
		for j := 0; j < count && idx+2 < len(data); j++ {
			ccValid := (data[idx] & 0x04) != 0
			ccTypeVal := int(data[idx] & 0x03)
			ccData1 := data[idx+1] & 0x7F
			ccData2 := data[idx+2] & 0x7F
			if ccValid && (ccTypeVal == 0 || ccTypeVal == 1) {
				hasCC = true
				if ccTypeVal == 1 {
					seenType1 = true
				} else {
					seenType0 = true
				}
				if (ccData1 == 0x14 || ccData1 == 0x1C || ccData1 == 0x15 || ccData1 == 0x1D) && ccData2 >= 0x20 && ccData2 <= 0x2F {
					hasCommand = true
					if ccData2 == 0x2F {
						hasDisplay = true
					}
				}
			}
			idx += 3
		}
	}
	return resolveCCResult(hasCC, seenType0, seenType1, hasCommand, hasDisplay)
}

func parseDVDUserData(data []byte) (bool, int, bool, bool) {
	if len(data) < 6 {
		return false, 0, false, false
	}
	if data[0] != 'C' || data[1] != 'C' {
		return false, 0, false, false
	}
	if data[2] != 0x01 {
		return false, 0, false, false
	}
	if data[3] != 0xF8 {
		return false, 0, false, false
	}

	flags := data[4]
	blockCount := int((flags >> 1) & 0x1F)
	extra := int(flags & 0x01)
	totalBlocks := blockCount*2 + extra
	if totalBlocks <= 0 {
		return false, 0, false, false
	}

	hasCC := false
	hasCommand := false
	hasDisplay := false
	seenType0 := false
	seenType1 := false
	idx := 5
	for j := 0; j < totalBlocks && idx+2 < len(data); j++ {
		if idx+3 > len(data) {
			break
		}
		field := data[idx]
		if (field & 0xFE) != 0xFE {
			idx += 3
			continue
		}
		odd := (field & 0x01) != 0
		raw1 := data[idx+1]
		raw2 := data[idx+2]
		if raw1 == 0x80 && raw2 == 0x80 {
			idx += 3
			continue
		}
		ccData1 := raw1 & 0x7F
		ccData2 := raw2 & 0x7F
		if ccData1 != 0 || ccData2 != 0 {
			hasCC = true
			if odd {
				seenType1 = true
			} else {
				seenType0 = true
			}
			if (ccData1 == 0x14 || ccData1 == 0x1C || ccData1 == 0x15 || ccData1 == 0x1D) && ccData2 >= 0x20 && ccData2 <= 0x2F {
				hasCommand = true
				if ccData2 == 0x2F {
					hasDisplay = true
				}
			}
		}
		idx += 3
	}
	return resolveCCResult(hasCC, seenType0, seenType1, hasCommand, hasDisplay)
}

func parseMPEG2UserData(data []byte) (bool, int, bool, bool) {
	if hasCC, ccType, hasCommand, hasDisplay := parseGA94UserData(data); hasCC {
		return hasCC, ccType, hasCommand, hasDisplay
	}
	return parseDVDUserData(data)
}

func resolveCCResult(hasCC bool, seenType0 bool, seenType1 bool, hasCommand bool, hasDisplay bool) (bool, int, bool, bool) {
	if !hasCC {
		return false, 0, false, false
	}
	if seenType1 {
		return true, 1, hasCommand, hasDisplay
	}
	if seenType0 {
		return true, 0, hasCommand, hasDisplay
	}
	return true, 0, hasCommand, hasDisplay
}

func ccServiceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "CC1"
	}
	return name
}
