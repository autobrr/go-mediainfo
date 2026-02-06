package mediainfo

import (
	"encoding/binary"
	"io"
	"math"
	"strconv"
)

func ParseFLAC(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	var header [4]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return ContainerInfo{}, nil, false
	}
	if header[0] != 'f' || header[1] != 'L' || header[2] != 'a' || header[3] != 'C' {
		return ContainerInfo{}, nil, false
	}

	var sampleRate uint32
	var channels uint8
	var bitsPerSample uint8
	var totalSamples uint64
	var audioStart int64

	for {
		var blockHeader [4]byte
		if _, err := io.ReadFull(file, blockHeader[:]); err != nil {
			break
		}
		isLast := (blockHeader[0] & 0x80) != 0
		blockType := blockHeader[0] & 0x7F
		blockLen := int(blockHeader[1])<<16 | int(blockHeader[2])<<8 | int(blockHeader[3])
		if blockLen <= 0 {
			if isLast {
				break
			}
			continue
		}
		if blockType == 0 {
			if blockLen < 34 {
				if _, err := file.Seek(int64(blockLen), io.SeekCurrent); err != nil {
					break
				}
			} else {
				var streamInfo [34]byte
				if _, err := io.ReadFull(file, streamInfo[:]); err != nil {
					break
				}
				sampleRate, channels, bitsPerSample, totalSamples = parseFLACStreamInfo(streamInfo[:])
				if blockLen > 34 {
					if _, err := file.Seek(int64(blockLen-34), io.SeekCurrent); err != nil {
						break
					}
				}
			}
		} else {
			if _, err := file.Seek(int64(blockLen), io.SeekCurrent); err != nil {
				break
			}
		}
		if isLast {
			// The file cursor is now positioned at the start of the audio frames.
			if pos, err := file.Seek(0, io.SeekCurrent); err == nil {
				audioStart = pos
			}
			break
		}
	}

	if sampleRate == 0 || channels == 0 {
		return ContainerInfo{}, nil, false
	}

	duration := 0.0
	if totalSamples > 0 {
		duration = float64(totalSamples) / float64(sampleRate)
	}

	bitrate := 0.0
	if duration > 0 {
		bitrate = (float64(size) * 8) / duration
	}

	info := ContainerInfo{
		DurationSeconds: duration,
		BitrateMode:     "Variable",
		StreamOverheadBytes: func() int64 {
			if audioStart <= 0 {
				return 0
			}
			return audioStart
		}(),
	}

	fields := []Field{
		{Name: "Format", Value: "FLAC"},
	}
	fields = appendChannelFields(fields, uint64(channels))
	fields = appendSampleRateField(fields, float64(sampleRate))
	if bitsPerSample > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(bitsPerSample)})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
	fields = addStreamCommon(fields, duration, bitrate)

	streamJSON := map[string]string{}
	if duration > 0 {
		streamJSON["Duration"] = formatJSONSeconds(duration)
	}
	if audioStart > 0 && audioStart < size {
		payload := size - audioStart
		streamJSON["StreamSize"] = strconv.FormatInt(payload, 10)
		if duration > 0 {
			br := int64(math.Round((float64(payload) * 8) / duration))
			if br > 0 {
				streamJSON["BitRate"] = strconv.FormatInt(br, 10)
			}
		}
	}

	return info, []Stream{{Kind: StreamAudio, Fields: fields, JSON: streamJSON}}, true
}

func parseFLACStreamInfo(data []byte) (uint32, uint8, uint8, uint64) {
	if len(data) < 34 {
		return 0, 0, 0, 0
	}
	sampleRate := uint32(data[10])<<12 | uint32(data[11])<<4 | uint32(data[12])>>4
	channels := ((data[12] & 0x0E) >> 1) + 1
	bitsPerSample := ((data[12] & 0x01) << 4) | (data[13] >> 4)
	bitsPerSample++

	totalSamples := uint64(data[13]&0x0F)<<32 | uint64(binary.BigEndian.Uint32(data[14:18]))
	return sampleRate, channels, bitsPerSample, totalSamples
}
