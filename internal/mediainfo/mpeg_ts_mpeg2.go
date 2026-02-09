package mediainfo

import (
	"math"
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
			packetLen := int(b & 0x3F) // 6-bit packet_size
			if packetLen == 0 {
				continue
			}
			s.remaining = packetLen
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
		entry.mpeg2Parser = &mpeg2VideoParser{}
	}
	entry.mpeg2Parser.consume(payload)
	consumeMPEG2CaptionsTS(entry, payload, pts, hasPTS)
}

func consumeMPEG2CaptionsTS(entry *tsStream, payload []byte, pts uint64, hasPTS bool) {
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
			parseMPEG2UserDataTS(entry, buf[i+4:end], pts, hasPTS, entry.videoFrameCount)
		}
		return true
	})
	if len(buf) >= 3 {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf[len(buf)-3:]...)
	} else {
		entry.videoCCCarry = append(entry.videoCCCarry[:0], buf...)
	}
}

func parseMPEG2UserDataTS(entry *tsStream, data []byte, pts uint64, hasPTS bool, framesBefore int) {
	if entry == nil || len(data) < 6 {
		return
	}
	// GA94 / A/53 user data: CEA-708 + CEA-608-in-708.
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
		track.lastPTS = pts
		if track.firstCommandPTS == 0 && isCCCommand(ccData1, ccData2) {
			track.firstCommandPTS = pts
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
		if track.firstType == "" && ccData2 == 0x2F {
			track.firstType = "PopOn"
			track.firstDisplayPTS = pts
			track.firstFrame = framesBefore
		}
	}

	// EIA-608 XDS is carried on the 608 channel bytes; parse it for General metadata parity.
	// MediaInfoLib tracks XDS state across both fields (single XDS_Level), so do the same here.
	if title, rating, ok := entry.xds.feed(ccData1, ccData2); ok {
		if rating != "" {
			entry.xdsLawRating = rating
		}
		_ = title // keep decode side-effects local; Title is not reliable in TS without full EPG parity.
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
