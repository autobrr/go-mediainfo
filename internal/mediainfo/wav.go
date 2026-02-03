package mediainfo

import (
	"encoding/binary"
	"io"
)

func ParseWAV(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return ContainerInfo{}, nil, false
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return ContainerInfo{}, nil, false
	}

	var (
		audioFormat   uint16
		channels      uint16
		sampleRate    uint32
		byteRate      uint32
		bitsPerSample uint16
		dataSize      uint32
		fmtFound      bool
	)

	for {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(file, chunkHeader); err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return ContainerInfo{}, nil, false
			}
			data := make([]byte, chunkSize)
			if _, err := io.ReadFull(file, data); err != nil {
				return ContainerInfo{}, nil, false
			}
			audioFormat = binary.LittleEndian.Uint16(data[0:2])
			channels = binary.LittleEndian.Uint16(data[2:4])
			sampleRate = binary.LittleEndian.Uint32(data[4:8])
			byteRate = binary.LittleEndian.Uint32(data[8:12])
			bitsPerSample = binary.LittleEndian.Uint16(data[14:16])
			fmtFound = true
		case "data":
			dataSize = chunkSize
			if _, err := file.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, false
			}
		default:
			if _, err := file.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, false
			}
		}

		if chunkSize%2 == 1 {
			if _, err := file.Seek(1, io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, false
			}
		}

		if fmtFound && dataSize > 0 {
			break
		}
	}

	if !fmtFound {
		return ContainerInfo{}, nil, false
	}

	duration := 0.0
	bitrate := 0.0
	mode := "Variable"
	if byteRate > 0 && dataSize > 0 {
		duration = float64(dataSize) / float64(byteRate)
		bitrate = float64(byteRate) * 8
		mode = "Constant"
	}

	info := ContainerInfo{
		DurationSeconds: duration,
		BitrateMode:     mode,
	}

	format := "PCM"
	if audioFormat != 1 {
		format = "Unknown"
	}

	fields := []Field{
		{Name: "Format", Value: format},
	}
	if channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(channels))})
		if layout := channelLayout(uint64(channels)); layout != "" {
			fields = append(fields, Field{Name: "Channel layout", Value: layout})
		}
	}
	if sampleRate > 0 {
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(float64(sampleRate))})
	}
	if bitsPerSample > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(uint8(bitsPerSample))})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
	fields = addStreamCommon(fields, duration, bitrate)

	_ = size
	return info, []Stream{{Kind: StreamAudio, Fields: fields}}, true
}
