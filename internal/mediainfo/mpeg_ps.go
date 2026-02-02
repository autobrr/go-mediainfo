package mediainfo

import (
	"bufio"
	"io"
)

type psStream struct {
	kind   StreamKind
	format string
}

func ParseMPEGPS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	buf := make([]byte, 1<<20)
	carry := make([]byte, 0, 16)

	streams := map[StreamKind]psStream{}
	var firstPTSVideo uint64
	var lastPTSVideo uint64
	var hasPTSVideo bool
	var firstPTSAny uint64
	var lastPTSAny uint64
	var hasPTSAny bool

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
					streams[kind] = psStream{kind: kind, format: format}
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
				if !hasPTSAny {
					firstPTSAny = pts
					lastPTSAny = pts
					hasPTSAny = true
				} else {
					lastPTSAny = pts
				}
				if kind == StreamVideo {
					if !hasPTSVideo {
						firstPTSVideo = pts
						lastPTSVideo = pts
						hasPTSVideo = true
					} else {
						lastPTSVideo = pts
					}
				}
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
		streamsOut = append(streamsOut, Stream{Kind: st.kind, Fields: []Field{{Name: "Format", Value: st.format}}})
	}

	info := ContainerInfo{}
	if duration := durationFromPTS(firstPTSVideo, lastPTSVideo, hasPTSVideo); duration > 0 {
		info.DurationSeconds = duration
	} else if duration := durationFromPTS(firstPTSAny, lastPTSAny, hasPTSAny); duration > 0 {
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
