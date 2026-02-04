package mediainfo

import "strings"

func consumeMPEG2Captions(entry *psStream, payload []byte, pts uint64, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	buf := append(entry.videoCCCarry, payload...)
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
			if hasCC, ccType, hasCommand, hasDisplay := parseGA94UserData(buf[i+4 : end]); hasCC {
				framesBefore := entry.videoFrameCount - 1
				if framesBefore < 0 {
					framesBefore = 0
				}
				if !entry.ccFound {
					entry.ccFound = true
					entry.ccFirstFrame = framesBefore
					if hasPTS {
						entry.ccFirstPTS = pts
					}
				}
				entry.ccLastFrame = framesBefore
				if hasPTS {
					entry.ccLastPTS = pts
				}
				if ccType == 1 {
					entry.ccService = "CC3"
				} else if entry.ccService == "" {
					entry.ccService = "CC1"
				}
				if hasPTS && hasCommand {
					if entry.ccFirstCommandPTS == 0 {
						entry.ccFirstCommandPTS = pts
					}
				}
				if hasPTS && hasDisplay {
					if entry.ccFirstDisplayPTS == 0 {
						entry.ccFirstDisplayPTS = pts
						entry.ccFirstType = "PopOn"
					}
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
	ccType := 0
	hasCommand := false
	hasDisplay := false
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
				ccType = ccTypeVal
				if (ccData1 == 0x14 || ccData1 == 0x1C) && ccData2 >= 0x20 && ccData2 <= 0x2F {
					hasCommand = true
					if ccData2 == 0x2F {
						hasDisplay = true
					}
				}
			}
			idx += 3
		}
	}
	if hasCC && ccType == 1 {
		return true, 1, hasCommand, hasDisplay
	}
	if hasCC && ccType == 0 {
		return true, 0, hasCommand, hasDisplay
	}
	return false, 0, false, false
}

func ccServiceName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "CC1"
	}
	return name
}
