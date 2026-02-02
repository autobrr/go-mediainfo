package mediainfo

import (
	"bufio"
	"io"
)

func ParseMPEGPS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	buf := make([]byte, 1<<20)
	carry := make([]byte, 0, 16)

	streams := map[byte]psStream{}
	var videoPTS ptsTracker
	var anyPTS ptsTracker

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := append(carry, buf[:n]...)
			maxIndex := len(chunk) - 14
			for i := 0; i <= maxIndex; i++ {
				if chunk[i] != 0x00 || chunk[i+1] != 0x00 || chunk[i+2] != 0x01 {
					continue
				}
				streamID := chunk[i+3]
				kind, format := mapPSStream(streamID)
				if kind != "" {
					entry := streams[streamID]
					entry.id = streamID
					entry.kind = kind
					entry.format = format
					entry.bytes += uint64(len(chunk))
					streams[streamID] = entry
				}
				if i+9 >= len(chunk) {
					continue
				}
				flags := chunk[i+7]
				headerLen := int(chunk[i+8])
				if (flags&0x80) == 0 || i+9+headerLen > len(chunk) {
					continue
				}
				pts, ok := parsePTS(chunk[i+9:])
				if !ok {
					continue
				}
				anyPTS.add(pts)
				entry := streams[streamID]
				entry.pts.add(pts)
				if kind == StreamVideo {
					videoPTS.add(pts)
					entry.frames++
				}
				streams[streamID] = entry
			}
			if len(chunk) > 16 {
				carry = append(carry[:0], chunk[len(chunk)-16:]...)
			} else {
				carry = append(carry[:0], chunk...)
			}
		}
		if err != nil {
			break
		}
	}

	var streamsOut []Stream
	for _, st := range streams {
		fields := []Field{}
		fields = append(fields, Field{Name: "ID", Value: formatID(uint64(st.id))})
		if st.format != "" {
			fields = append(fields, Field{Name: "Format", Value: st.format})
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
		for i := range streamsOut {
			if streamsOut[i].Kind == StreamVideo {
				streamsOut[i].Fields = addStreamDuration(streamsOut[i].Fields, duration)
				if st, ok := findPSStreamByKind(streams, StreamVideo); ok && st.bytes > 0 {
					bits := (float64(st.bytes) * 8) / duration
					if mode := bitrateMode(bits); mode != "" {
						streamsOut[i].Fields = appendFieldUnique(streamsOut[i].Fields, Field{Name: "Bit rate mode", Value: mode})
					}
					streamsOut[i].Fields = addStreamBitrate(streamsOut[i].Fields, bits)
					if st.frames > 0 {
						if rate := estimateTSFrameRate(st.frames, duration); rate != "" {
							streamsOut[i].Fields = appendFieldUnique(streamsOut[i].Fields, Field{Name: "Frame rate", Value: rate})
						}
					}
				}
			}
		}
	} else if duration := anyPTS.duration(); duration > 0 {
		info.DurationSeconds = duration
	}

	return info, streamsOut, true
}

func mapPSStream(streamID byte) (StreamKind, string) {
	switch {
	case streamID >= 0xE0 && streamID <= 0xEF:
		return StreamVideo, "MPEG Video"
	case streamID >= 0xC0 && streamID <= 0xDF:
		return StreamAudio, "MPEG Audio"
	case streamID == 0xBD:
		return StreamText, "Private"
	default:
		return "", ""
	}
}
