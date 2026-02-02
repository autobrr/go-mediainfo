package mediainfo

import (
	"bufio"
	"encoding/binary"
	"io"
)

type tsStream struct {
	pid    uint16
	kind   StreamKind
	format string
	frames uint64
	bytes  uint64
	pts    ptsTracker
}

func ParseMPEGTS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	reader := bufio.NewReaderSize(file, 188*200)

	var pmtPID uint16
	streams := map[uint16]tsStream{}
	var videoPTS ptsTracker
	var anyPTS ptsTracker
	videoPIDs := map[uint16]struct{}{}

	packet := make([]byte, 188)
	for {
		_, err := io.ReadFull(reader, packet)
		if err != nil {
			break
		}
		if packet[0] != 0x47 {
			continue
		}
		pid := uint16(packet[1]&0x1F)<<8 | uint16(packet[2])
		payloadStart := packet[1]&0x40 != 0
		adaptation := (packet[3] & 0x30) >> 4
		payloadIndex := 4
		switch adaptation {
		case 2:
			continue
		case 3:
			adaptationLen := int(packet[4])
			payloadIndex += 1 + adaptationLen
		}
		if payloadIndex >= len(packet) {
			continue
		}
		payload := packet[payloadIndex:]

		if pid == 0 && payloadStart {
			if p := parsePAT(payload); p != 0 {
				pmtPID = p
			}
			continue
		}
		if pmtPID != 0 && pid == pmtPID && payloadStart {
			parsed := parsePMT(payload)
			for _, st := range parsed {
				streams[st.pid] = st
				if st.kind == StreamVideo {
					videoPIDs[st.pid] = struct{}{}
				}
			}
			continue
		}

		if !payloadStart || len(payload) < 9 {
			continue
		}
		if payload[0] != 0x00 || payload[1] != 0x00 || payload[2] != 0x01 {
			continue
		}
		flags := payload[7]
		headerLen := int(payload[8])
		if (flags&0x80) == 0 || len(payload) < 9+headerLen {
			continue
		}
		pts, ok := parsePTS(payload[9:])
		if !ok {
			continue
		}
		anyPTS.add(pts)

		if _, ok := videoPIDs[pid]; ok {
			entry := streams[pid]
			entry.frames++
			entry.bytes += uint64(len(payload))
			entry.pts.add(pts)
			streams[pid] = entry
			videoPTS.add(pts)
		}
	}

	var streamsOut []Stream
	videoDuration := videoPTS.duration()
	for _, st := range streams {
		fields := []Field{{Name: "ID", Value: formatStreamID(st.pid)}}
		if st.format != "" {
			fields = append(fields, Field{Name: "Format", Value: st.format})
		}
		if st.kind == StreamVideo && videoDuration > 0 {
			bits := 0.0
			if st.bytes > 0 {
				bits = (float64(st.bytes) * 8) / videoDuration
			}
			fields = addStreamCommon(fields, videoDuration, bits)
			if rate := estimateTSFrameRate(st.frames, videoDuration); rate != "" {
				fields = append(fields, Field{Name: "Frame rate", Value: rate})
			}
		}
		if st.kind != StreamVideo {
			if duration := st.pts.duration(); duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
		}
		streamsOut = append(streamsOut, Stream{Kind: st.kind, Fields: fields})
	}

	info := ContainerInfo{}
	if duration := videoPTS.duration(); duration > 0 {
		info.DurationSeconds = duration
	} else if duration := anyPTS.duration(); duration > 0 {
		info.DurationSeconds = duration
	}

	return info, streamsOut, true
}

func parsePAT(payload []byte) uint16 {
	if len(payload) < 8 {
		return 0
	}
	pointer := int(payload[0])
	if pointer+8 > len(payload) {
		return 0
	}
	section := payload[1+pointer:]
	if len(section) < 8 {
		return 0
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen+3 > len(section) {
		return 0
	}
	entries := section[8 : 3+sectionLen-4]
	for i := 0; i+4 <= len(entries); i += 4 {
		programNumber := binary.BigEndian.Uint16(entries[i : i+2])
		pid := binary.BigEndian.Uint16(entries[i+2:i+4]) & 0x1FFF
		if programNumber != 0 {
			return pid
		}
	}
	return 0
}

func parsePMT(payload []byte) []tsStream {
	if len(payload) < 12 {
		return nil
	}
	pointer := int(payload[0])
	if pointer+12 > len(payload) {
		return nil
	}
	section := payload[1+pointer:]
	if len(section) < 12 {
		return nil
	}
	sectionLen := int(binary.BigEndian.Uint16(section[1:3]) & 0x0FFF)
	if sectionLen+3 > len(section) {
		return nil
	}
	programInfoLen := int(binary.BigEndian.Uint16(section[10:12]) & 0x0FFF)
	pos := 12 + programInfoLen
	end := 3 + sectionLen - 4
	if pos > end {
		return nil
	}
	streams := make([]tsStream, 0)
	for pos+5 <= end {
		streamType := section[pos]
		pid := binary.BigEndian.Uint16(section[pos+1:pos+3]) & 0x1FFF
		esInfoLen := int(binary.BigEndian.Uint16(section[pos+3:pos+5]) & 0x0FFF)
		kind, format := mapTSStream(streamType)
		if kind != "" {
			streams = append(streams, tsStream{pid: pid, kind: kind, format: format})
		}
		pos += 5 + esInfoLen
	}
	return streams
}

func mapTSStream(streamType byte) (StreamKind, string) {
	switch streamType {
	case 0x01:
		return StreamVideo, "MPEG Video"
	case 0x02:
		return StreamVideo, "MPEG Video"
	case 0x10:
		return StreamVideo, "MPEG-4 Visual"
	case 0x1B:
		return StreamVideo, "AVC"
	case 0x24:
		return StreamVideo, "HEVC"
	case 0xEA:
		return StreamVideo, "VC-1"
	case 0x03:
		return StreamAudio, "MPEG Audio"
	case 0x04:
		return StreamAudio, "MPEG Audio"
	case 0x0F:
		return StreamAudio, "AAC"
	case 0x11:
		return StreamAudio, "AAC"
	case 0x81:
		return StreamAudio, "AC-3"
	case 0x06:
		return StreamText, "Private"
	case 0x90:
		return StreamText, "PGS"
	default:
		return "", ""
	}
}

func durationFromPTS(first, last uint64, ok bool) float64 {
	if !ok || last == 0 {
		return 0
	}
	if last < first {
		last += 1 << 33
	}
	delta := last - first
	return float64(delta) / 90000.0
}

func formatStreamID(pid uint16) string {
	return formatID(uint64(pid))
}
