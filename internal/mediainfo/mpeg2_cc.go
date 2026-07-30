package mediainfo

import "strings"

// consumeMPEG2Captions incrementally scans MPEG-2 user data for DVD or GA94
// caption headers and updates the stream's CC timing observations.
func consumeMPEG2Captions(entry *psStream, payload []byte, pts uint64, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	entry.videoCCCarry = append(entry.videoCCCarry, payload...)
	buf := entry.videoCCCarry
	scanMPEG2StartCodes(buf, 0, func(i int, code byte) bool {
		switch code {
		case 0x00:
			entry.videoFrameCount++
		case 0xB2:
			end := nextStartCode(buf, i+4)
			if end < 0 {
				end = len(buf)
			}
			userData := buf[i+4 : end]
			if hasMPEG2CaptionHeader(userData) {
				entry.ccHeaderFound = true
			}
			if hasCC, ccType, hasCommand, hasDisplay := parseMPEG2UserData(userData); hasCC {
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
		return true
	})
	if len(buf) >= 3 {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf[len(buf)-3:]...)
	} else {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf...)
	}
}

// hasMPEG2CaptionHeader reports whether user data contains a DVD CC or GA94
// caption-data header, even when no renderable caption pair is present.
func hasMPEG2CaptionHeader(data []byte) bool {
	for i := 0; i+2 < len(data); i++ {
		if data[i] == 'C' && data[i+1] == 'C' && data[i+2] == 0x01 {
			return true
		}
		if i+4 < len(data) && data[i] == 'G' && data[i+1] == 'A' && data[i+2] == '9' && data[i+3] == '4' && data[i+4] == 0x03 {
			return true
		}
	}
	return false
}

func parseGA94UserData(data []byte) (bool, int, bool, bool) {
	if len(data) < 6 {
		return false, 0, false, false
	}
	state := ccParseState{}
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
				state.apply(ccData1, ccData2, ccTypeVal == 1)
			}
			idx += 3
		}
	}
	return resolveCCResult(state.hasCC, state.seenType0, state.seenType1, state.hasCommand, state.hasDisplay)
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

	state := ccParseState{}
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
			state.apply(ccData1, ccData2, odd)
		}
		idx += 3
	}
	return resolveCCResult(state.hasCC, state.seenType0, state.seenType1, state.hasCommand, state.hasDisplay)
}

func parseMPEG2UserData(data []byte) (bool, int, bool, bool) {
	if hasCC, ccType, hasCommand, hasDisplay := parseGA94UserData(data); hasCC {
		return hasCC, ccType, hasCommand, hasDisplay
	}
	return parseDVDUserData(data)
}

type ccParseState struct {
	hasCC      bool
	hasCommand bool
	hasDisplay bool
	seenType0  bool
	seenType1  bool
}

func (s *ccParseState) apply(ccData1 byte, ccData2 byte, type1 bool) {
	if ccData1 == 0 && ccData2 == 0 {
		return
	}
	s.hasCC = true
	if type1 {
		s.seenType1 = true
	} else {
		s.seenType0 = true
	}
	if isCCCommand(ccData1, ccData2) {
		s.hasCommand = true
		if ccData2 == 0x2F {
			s.hasDisplay = true
		}
	}
}

func isCCCommand(ccData1 byte, ccData2 byte) bool {
	return (ccData1 == 0x14 || ccData1 == 0x1C || ccData1 == 0x15 || ccData1 == 0x1D) &&
		ccData2 >= 0x20 && ccData2 <= 0x2F
}

func isCCStartCommand(ccData1 byte, ccData2 byte) bool {
	// Match MediaInfoLib File_Eia608.cpp: Duration_Start_Command is set on specific commands,
	// not on all 0x20..0x2F control codes (notably excluding EOC 0x2F).
	if !isCCCommand(ccData1, ccData2) {
		return false
	}
	switch ccData2 {
	case 0x20, 0x25, 0x26, 0x27, 0x29, 0x2A, 0x2B, 0x2C:
		return true
	default:
		return false
	}
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
