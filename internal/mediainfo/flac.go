package mediainfo

import (
	"bytes"
	"encoding/binary"
	"io"
)

func ParseFLAC(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return ContainerInfo{}, nil, false
	}
	if !bytes.Equal(header, []byte("fLaC")) {
		return ContainerInfo{}, nil, false
	}

	var sampleRate uint32
	var channels uint8
	var bitsPerSample uint8
	var totalSamples uint64

	for {
		blockHeader := make([]byte, 4)
		if _, err := io.ReadFull(file, blockHeader); err != nil {
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
		blockData := make([]byte, blockLen)
		if _, err := io.ReadFull(file, blockData); err != nil {
			break
		}
		if blockType == 0 && blockLen >= 34 {
			sampleRate, channels, bitsPerSample, totalSamples = parseFLACStreamInfo(blockData)
		}
		if isLast {
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
	}

	fields := []Field{
		{Name: "Format", Value: "FLAC"},
		{Name: "Channel(s)", Value: formatChannels(uint64(channels))},
	}
	if layout := channelLayout(uint64(channels)); layout != "" {
		fields = append(fields, Field{Name: "Channel layout", Value: layout})
	}
	fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(float64(sampleRate))})
	if bitsPerSample > 0 {
		fields = append(fields, Field{Name: "Bit depth", Value: formatBitDepth(bitsPerSample)})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
	fields = addStreamCommon(fields, duration, bitrate)

	return info, []Stream{{Kind: StreamAudio, Fields: fields}}, true
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
