package mediainfo

import (
	"math"
	"sort"
)

type dtvccState struct {
	remaining int
	buf       []byte
}

func (s *dtvccState) feed(data []byte, services map[int]struct{}) {
	if services == nil {
		return
	}
	for _, b := range data {
		if s.remaining == 0 {
			packetSizeCode := int(b & 0x3F) // 6-bit packet_size_code
			// CEA-708 DTVCC packet payload size after the header byte:
			// packet_size_code == 0 => 127 bytes, else (packet_size_code*2)-1 bytes.
			if packetSizeCode == 0 {
				s.remaining = 127
			} else {
				s.remaining = packetSizeCode*2 - 1
			}
			s.buf = s.buf[:0]
			continue
		}
		s.buf = append(s.buf, b)
		s.remaining--
		if s.remaining == 0 {
			parseDTVCCServices(s.buf, services)
			s.buf = s.buf[:0]
		}
	}
}

func parseDTVCCServices(packet []byte, services map[int]struct{}) {
	for i := 0; i < len(packet); {
		b := packet[i]
		if b == 0 {
			i++
			continue
		}
		service := int(b >> 5)
		blockSize := int(b & 0x1F)
		header := 1
		if service == 7 {
			if i+1 >= len(packet) {
				return
			}
			service = int(packet[i+1] & 0x3F)
			header = 2
		}
		if service > 0 {
			services[service] = struct{}{}
		}
		i += header + blockSize
	}
}

func consumeMPEG2TSVideo(entry *tsStream, payload []byte, pts uint64, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	if entry.mpeg2Parser == nil {
		entry.mpeg2Parser = &mpeg2VideoParser{intraFreezeAfterI: 8}
	}
	entry.mpeg2Parser.consume(payload)
	consumeMPEG2CaptionsTS(entry, payload, pts, hasPTS)
}

func consumeMPEG2CaptionsTS(entry *tsStream, payload []byte, pts uint64, hasPTS bool) {
	entry.videoCCCarry = append(entry.videoCCCarry, payload...)
	buf := entry.videoCCCarry
	carryStart := 0
	if len(buf) >= 3 {
		carryStart = len(buf) - 3
	}
	scanMPEG2StartCodes(buf, 0, func(i int, code byte) bool {
		switch code {
		case 0x00:
			// picture_start_code: parse temporal_reference (10 bits) for XDS reorder.
			if i+6 <= len(buf) {
				b0 := buf[i+4]
				b1 := buf[i+5]
				entry.mpeg2CurTemporalReference = uint16(b0)<<2 | uint16(b1>>6)
				pictureCodingType := (b1 >> 3) & 0x07
				if hasPTS {
					switch pictureCodingType {
					case 1:
						entry.mpeg2LastISeen = true
						entry.mpeg2PresentationEndPTS = 0
					case 2:
						if entry.mpeg2LastISeen && entry.mpeg2PresentationEndPTS == 0 {
							entry.mpeg2PresentationEndPTS = pts
						}
					}
				}
			}
			entry.videoFrameCount++
		case 0xB8:
			// group_start_code: flush any buffered XDS packets (temporal_reference resets per GOP).
			flushMPEG2XDSReorder(entry)
		case 0xB2:
			end := nextStartCode(buf, i+4)
			if end < 0 {
				// User data may span PES packets; keep it for the next call.
				carryStart = i
				return false
			}
			userData := buf[i+4 : end]
			// MediaInfo feeds GA94 caption and XDS payloads to the sub-parser in
			// presentation order, not MPEG-2 decode order.
			entry.mpeg2XDSReorder = append(entry.mpeg2XDSReorder, mpeg2UserDataPacket{
				temporalReference: entry.mpeg2CurTemporalReference,
				data:              append([]byte(nil), userData...),
				pts:               pts,
				hasPTS:            hasPTS,
				frame:             entry.videoFrameCount,
			})
		}
		return true
	})
	entry.videoCCCarry = append(entry.videoCCCarry[:0], buf[carryStart:]...)

	// Safety bound: flush opportunistically to keep memory bounded even if GOP headers are missing.
	if len(entry.mpeg2XDSReorder) > 64 {
		flushMPEG2XDSReorder(entry)
	}
}

func flushMPEG2XDSReorder(entry *tsStream) {
	if entry == nil || len(entry.mpeg2XDSReorder) == 0 {
		return
	}
	pkts := entry.mpeg2XDSReorder
	entry.mpeg2XDSReorder = entry.mpeg2XDSReorder[:0]
	sort.Slice(pkts, func(i, j int) bool {
		return pkts[i].temporalReference < pkts[j].temporalReference
	})
	for i := range pkts {
		parseMPEG2UserDataTS(entry, pkts[i].data, pkts[i].pts, pkts[i].hasPTS, pkts[i].frame)
		parseMPEG2UserDataTSXDS(entry, pkts[i].data)
	}
}

func parseMPEG2UserDataTSXDS(entry *tsStream, data []byte) {
	if entry == nil || len(data) < 6 {
		return
	}
	for i := 0; i+5 < len(data); i++ {
		if data[i] != 'G' || data[i+1] != 'A' || data[i+2] != '9' || data[i+3] != '4' {
			continue
		}
		if data[i+4] != 0x03 {
			continue
		}
		flags := data[i+5]
		count := int(flags & 0x1F)
		idx := i + 6
		if idx >= len(data) {
			continue
		}
		// reserved / em_data
		idx++
		for j := 0; j < count && idx+2 < len(data); j++ {
			ccValid := (data[idx] & 0x04) != 0
			ccTypeVal := int(data[idx] & 0x03)
			ccData1 := data[idx+1]
			ccData2 := data[idx+2]
			if ccValid && (ccTypeVal == 0 || ccTypeVal == 1) && ccTypeVal < len(entry.xds) {
				// MediaInfoLib strips the parity bit before EIA-608 parsing.
				ccData1 &= 0x7F
				ccData2 &= 0x7F
				if title, rating, ok := entry.xds[ccTypeVal].feed(ccData1, ccData2); ok {
					if rating != "" {
						entry.xdsLawRating = rating
					}
					if title != "" {
						title = normalizeXDSTitle(title)
						if title != "" {
							entry.xdsLastTitle = title
							if entry.xdsTitleCounts == nil {
								entry.xdsTitleCounts = map[string]int{}
							}
							entry.xdsTitleCounts[title]++
						}
					}
				}
			}
			idx += 3
		}
	}
}

func parseMPEG2UserDataTS(entry *tsStream, data []byte, pts uint64, hasPTS bool, framesBefore int) {
	if entry == nil || len(data) < 6 {
		return
	}
	// GA94 / A/53 user data: CEA-708 + CEA-608-in-708.
	foundGA94 := false
	for i := 0; i+5 < len(data); i++ {
		if data[i] != 'G' || data[i+1] != 'A' || data[i+2] != '9' || data[i+3] != '4' {
			continue
		}
		if data[i+4] != 0x03 {
			continue
		}
		foundGA94 = true
		flags := data[i+5]
		count := int(flags & 0x1F)
		idx := i + 6
		if idx >= len(data) {
			continue
		}
		// reserved
		idx++
		for j := 0; j < count && idx+2 < len(data); j++ {
			ccValid := (data[idx] & 0x04) != 0
			ccTypeVal := int(data[idx] & 0x03)
			ccData1 := data[idx+1] & 0x7F
			ccData2 := data[idx+2] & 0x7F
			if ccValid {
				switch ccTypeVal {
				case 0, 1:
					updateCCTrackTS(entry, ccTypeVal, ccData1, ccData2, pts, hasPTS, framesBefore)
				case 2, 3:
					if entry.dtvccServices == nil {
						entry.dtvccServices = map[int]struct{}{}
					}
					entry.dtvcc.feed([]byte{ccData1, ccData2}, entry.dtvccServices)
				}
			}
			idx += 3
		}
		// Some broadcasts include multiple GA94 blocks; keep scanning to match MediaInfo.
	}
	if foundGA94 {
		return
	}
	// DVD-style user data fallback (no DTVCC).
	if hasCC, ccType, _, _ := parseDVDUserData(data); hasCC {
		updateCCTrackTS(entry, ccType, 0, 0, pts, hasPTS, framesBefore)
	}
}

func updateCCTrackTS(entry *tsStream, ccType int, ccData1 byte, ccData2 byte, pts uint64, hasPTS bool, framesBefore int) {
	entry.ccFound = true
	track := &entry.ccOdd
	if ccType == 1 {
		track = &entry.ccEven
	}
	if !track.found {
		track.found = true
		track.firstFrame = framesBefore
		if hasPTS {
			track.firstPTS = pts
		}
	}
	track.lastFrame = framesBefore
	if hasPTS {
		isSpecial := ccData1 >= 0x10 && ccData1 <= 0x1F
		if track.hasOldSpecial {
			if track.oldSpecialData1 == ccData1 && track.oldSpecialData2 == ccData2 {
				track.hasOldSpecial = false
				if track.commandAwaitingDuplicate {
					track.lastCommandPTS = pts
					track.commandDuplicated = true
				}
				track.commandAwaitingDuplicate = false
				return
			}
			track.hasOldSpecial = false
		}
		track.commandAwaitingDuplicate = false
		track.lastPTS = pts
		isCommand := isCCCommand(ccData1, ccData2)
		if isCommand {
			track.lastCommandPTS = pts
			track.commandDuplicated = false
			track.commandAwaitingDuplicate = true
			if mode := ccDisplayMode(ccData1, ccData2); mode != "" {
				track.currentType = mode
			}
			switch ccData2 {
			case 0x20:
				track.inBack = true
				track.rollUpLines = 0
			case 0x25, 0x26, 0x27:
				track.inBack = false
				track.rollUpLines = int(ccData2-0x25) + 2
			case 0x29:
				track.inBack = false
				track.rollUpLines = 0
				track.currentHasContent = false
			case 0x2C:
				if track.currentHasContent {
					recordCCDisplayChange(track, pts)
					track.currentHasContent = false
					if track.firstType == "" {
						track.firstType = "PaintOn"
						track.firstDisplayFrame = mediaInfoCCFrameCount(framesBefore)
					}
				}
			case 0x2D:
				if !track.inBack {
					recordCCDisplayChange(track, pts)
				}
				if track.rollUpLines > 0 && track.currentHasContent && track.firstType == "" {
					track.firstType = "RollUp"
					track.firstDisplayFrame = mediaInfoCCFrameCount(framesBefore)
				}
			case 0x2F:
				recordCCDisplayChange(track, pts)
				if track.firstType == "" {
					track.firstType = "PopOn"
					track.firstDisplayFrame = mediaInfoCCFrameCount(framesBefore)
				}
			}
		}
		if !hasCCFirstCommandPTS(track) && isCCStartCommand(ccData1, ccData2) {
			track.firstCommandPTS = pts
			track.hasFirstCommandPTS = true
			if framesBefore > 0 {
				// Official mediainfo reports Duration_Start_Command aligned to a 0-based frame index:
				// Delay + (frame_index / fps).
				//
				// Empirically, it applies an additional 2-frame offset for CEA-608-in-708 commands.
				// (E.g. for 59.94fps, first command at frameCount=459 -> frame_index=456.)
				if framesBefore > 3 {
					track.firstCommandFrame = framesBefore - 3
				} else {
					track.firstCommandFrame = 0
				}
			}
		}
		if !isCommand && isCCDisplayContent(ccData1, ccData2) {
			if !track.inBack {
				track.currentHasContent = true
				recordCCDisplayChange(track, pts)
			}
		}
		if isSpecial {
			track.oldSpecialData1 = ccData1
			track.oldSpecialData2 = ccData2
			track.hasOldSpecial = true
		}
	}

	// EIA-608 XDS is carried in GA94 user data; MediaInfoLib updates General fields with the
	// most recently completed Program Name packet. XDS parsing is done via temporal_reference
	// reorder in flushMPEG2XDSReorder to match MediaInfoLib.
}

// recordCCDisplayChange applies File_Eia608::HasChanged timing semantics.
func recordCCDisplayChange(track *ccTrack, pts uint64) {
	if track == nil {
		return
	}
	if !track.hasFirstContentPTS {
		track.firstContentPTS = pts
		track.hasFirstContentPTS = true
	}
	track.lastContentPTS = pts
}

// mediaInfoCCFrameCount removes the MPEG-2 caption reorder lookahead from the
// parser's one-based decoded-picture count.
func mediaInfoCCFrameCount(decodedPictures int) int {
	const captionReorderLookahead = 8
	if decodedPictures <= captionReorderLookahead {
		return 0
	}
	return decodedPictures - captionReorderLookahead
}

// ccDisplayMode maps CEA-608 presentation-mode commands to MediaInfo labels.
func ccDisplayMode(ccData1, ccData2 byte) string {
	if !isCCCommand(ccData1, ccData2) {
		return ""
	}
	switch ccData2 {
	case 0x25, 0x26, 0x27:
		return "RollUp"
	case 0x29:
		return "PaintOn"
	case 0x20:
		return "PopOn"
	default:
		return ""
	}
}

// isCCDisplayContent reports whether a pair carries printable caption data
// rather than a control code, PAC, XDS packet, or padding.
func isCCDisplayContent(ccData1, ccData2 byte) bool {
	if isCCCommand(ccData1, ccData2) {
		return false
	}
	if ccData1 >= 0x20 {
		return true
	}
	switch ccData1 {
	case 0x11:
		return ccData2 >= 0x30 && ccData2 <= 0x3F
	case 0x12, 0x13:
		return ccData2 >= 0x20 && ccData2 <= 0x3F
	default:
		return false
	}
}

func mpeg2CommercialNameTS(info mpeg2VideoInfo) string {
	if info.Width == 1280 && info.Height == 720 {
		if (info.FrameRateNumer == 60000 && info.FrameRateDenom == 1001) || (info.FrameRate > 0 && math.Abs(info.FrameRate-59.94) < 0.02) {
			return "HDV 720p"
		}
	}
	return ""
}
