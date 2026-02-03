package mediainfo

import (
	"bytes"
	"io"
)

type mp3HeaderInfo struct {
	bitrateKbps int
	sampleRate  int
	channels    int
	versionID   byte
	layerID     byte
}

func ParseMP3(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	offset, err := skipID3v2(file)
	if err != nil {
		return ContainerInfo{}, nil, false
	}

	dataSize := size - offset
	if dataSize <= 0 {
		return ContainerInfo{}, nil, false
	}

	if hasID3v1(file, size) {
		dataSize -= 128
	}
	if dataSize <= 0 {
		return ContainerInfo{}, nil, false
	}

	header, vbr, ok := findMP3Header(file, offset)
	if !ok {
		return ContainerInfo{}, nil, false
	}

	duration := 0.0
	if header.bitrateKbps > 0 {
		duration = (float64(dataSize) * 8) / (float64(header.bitrateKbps) * 1000)
	}

	mode := "Constant"
	if vbr {
		mode = "Variable"
	}

	info := ContainerInfo{
		DurationSeconds: duration,
		BitrateMode:     mode,
	}

	fields := []Field{
		{Name: "Format", Value: "MPEG Audio"},
	}
	if header.channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(header.channels))})
		if layout := channelLayout(uint64(header.channels)); layout != "" {
			fields = append(fields, Field{Name: "Channel layout", Value: layout})
		}
	}
	if header.sampleRate > 0 {
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(float64(header.sampleRate))})
	}
	fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
	fields = addStreamCommon(fields, duration, float64(header.bitrateKbps)*1000)

	return info, []Stream{{Kind: StreamAudio, Fields: fields}}, true
}

func skipID3v2(file io.ReadSeeker) (int64, error) {
	header := make([]byte, 10)
	if _, err := io.ReadFull(file, header); err != nil {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		return 0, nil
	}
	if !bytes.HasPrefix(header, []byte("ID3")) {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		return 0, nil
	}
	size := int64(header[6]&0x7F)<<21 | int64(header[7]&0x7F)<<14 | int64(header[8]&0x7F)<<7 | int64(header[9]&0x7F)
	offset := int64(10) + size
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	return offset, nil
}

func hasID3v1(file io.ReadSeeker, size int64) bool {
	if size < 128 {
		return false
	}
	if _, err := file.Seek(size-128, io.SeekStart); err != nil {
		return false
	}
	buf := make([]byte, 3)
	if _, err := io.ReadFull(file, buf); err != nil {
		return false
	}
	return bytes.Equal(buf, []byte("TAG"))
}

func findMP3Header(file io.ReadSeeker, offset int64) (mp3HeaderInfo, bool, bool) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return mp3HeaderInfo{}, false, false
	}
	buf := make([]byte, 1<<16)
	n, _ := io.ReadFull(file, buf)
	buf = buf[:n]
	for i := 0; i+4 <= len(buf); i++ {
		info, ok := parseMP3Header(buf[i : i+4])
		if !ok {
			continue
		}
		vbr := hasVBRHeader(buf[i:], info)
		return info, vbr, true
	}
	return mp3HeaderInfo{}, false, false
}

func parseMP3Header(header []byte) (mp3HeaderInfo, bool) {
	if len(header) < 4 {
		return mp3HeaderInfo{}, false
	}
	if header[0] != 0xFF || (header[1]&0xE0) != 0xE0 {
		return mp3HeaderInfo{}, false
	}
	versionID := (header[1] >> 3) & 0x03
	layerID := (header[1] >> 1) & 0x03
	if versionID == 0x01 || layerID == 0x00 {
		return mp3HeaderInfo{}, false
	}
	bitrateIndex := (header[2] >> 4) & 0x0F
	sampleRateIndex := (header[2] >> 2) & 0x03
	if bitrateIndex == 0x00 || bitrateIndex == 0x0F || sampleRateIndex == 0x03 {
		return mp3HeaderInfo{}, false
	}
	bitrate := mp3Bitrate(versionID, layerID, bitrateIndex)
	sampleRate := mp3SampleRate(versionID, sampleRateIndex)
	if bitrate == 0 || sampleRate == 0 {
		return mp3HeaderInfo{}, false
	}
	channelMode := (header[3] >> 6) & 0x03
	channels := 2
	if channelMode == 0x03 {
		channels = 1
	}
	return mp3HeaderInfo{
		bitrateKbps: bitrate,
		sampleRate:  sampleRate,
		channels:    channels,
		versionID:   versionID,
		layerID:     layerID,
	}, true
}

func mp3Bitrate(versionID, layerID, index byte) int {
	table := [][]int{
		{}, // unused
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}, // MPEG1 Layer III
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160},     // MPEG2/2.5 Layer III
	}
	if layerID != 0x01 {
		return 0
	}
	if versionID == 0x03 {
		return table[1][index]
	}
	return table[2][index]
}

func mp3SampleRate(versionID, index byte) int {
	switch versionID {
	case 0x03: // MPEG1
		return []int{44100, 48000, 32000}[index]
	case 0x02: // MPEG2
		return []int{22050, 24000, 16000}[index]
	case 0x00: // MPEG2.5
		return []int{11025, 12000, 8000}[index]
	default:
		return 0
	}
}

func hasVBRHeader(buf []byte, info mp3HeaderInfo) bool {
	if info.layerID != 0x01 {
		return false
	}
	sideInfo := 32
	if info.versionID != 0x03 {
		sideInfo = 17
	}
	if info.channels == 1 {
		if info.versionID == 0x03 {
			sideInfo = 17
		} else {
			sideInfo = 9
		}
	}
	offset := 4 + sideInfo
	if len(buf) < offset+4 {
		return false
	}
	tag := string(buf[offset : offset+4])
	return tag == "Xing"
}
