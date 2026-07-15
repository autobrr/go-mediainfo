package mediainfo

import (
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"strings"
)

// ParseWAV reads RIFF/WAVE metadata and returns legacy-compatible audio and General fields.
func ParseWAV(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, map[string]string, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}

	var header [12]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return ContainerInfo{}, nil, nil, nil, false
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return ContainerInfo{}, nil, nil, nil, false
	}

	var (
		audioFormat   uint16
		channels      uint16
		sampleRate    uint32
		byteRate      uint32
		blockAlign    uint16
		bitsPerSample uint16
		dataSize      uint32
		fmtFound      bool
		encodedApp    string
	)

	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(file, chunkHeader[:]); err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return ContainerInfo{}, nil, nil, nil, false
			}
			var fmtData [16]byte
			if _, err := io.ReadFull(file, fmtData[:]); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
			audioFormat = binary.LittleEndian.Uint16(fmtData[0:2])
			channels = binary.LittleEndian.Uint16(fmtData[2:4])
			sampleRate = binary.LittleEndian.Uint32(fmtData[4:8])
			byteRate = binary.LittleEndian.Uint32(fmtData[8:12])
			blockAlign = binary.LittleEndian.Uint16(fmtData[12:14])
			bitsPerSample = binary.LittleEndian.Uint16(fmtData[14:16])
			if chunkSize > 16 {
				if _, err := file.Seek(int64(chunkSize-16), io.SeekCurrent); err != nil {
					return ContainerInfo{}, nil, nil, nil, false
				}
			}
			fmtFound = true
		case "data":
			dataSize = chunkSize
			if _, err := file.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
		case "LIST":
			// Match MediaInfo: surface ffmpeg's ISFT as General Encoded_Application.
			// LIST chunk: 4-byte type then subchunks (id, size, data...).
			if chunkSize < 4 {
				if _, err := file.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
					return ContainerInfo{}, nil, nil, nil, false
				}
				break
			}
			payload := make([]byte, chunkSize)
			if _, err := io.ReadFull(file, payload); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
			if string(payload[0:4]) == "INFO" {
				rest := payload[4:]
				for len(rest) >= 8 {
					id := string(rest[0:4])
					sz := int(binary.LittleEndian.Uint32(rest[4:8]))
					rest = rest[8:]
					if sz < 0 || sz > len(rest) {
						break
					}
					data := rest[:sz]
					rest = rest[sz:]
					if sz%2 == 1 && len(rest) > 0 {
						rest = rest[1:]
					}
					if id == "ISFT" && encodedApp == "" {
						encodedApp = strings.TrimRight(string(data), "\x00")
						encodedApp = strings.TrimSpace(encodedApp)
					}
				}
			}
		default:
			if _, err := file.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
		}

		if chunkSize%2 == 1 {
			if _, err := file.Seek(1, io.SeekCurrent); err != nil {
				return ContainerInfo{}, nil, nil, nil, false
			}
		}

		if fmtFound && dataSize > 0 {
			break
		}
	}

	if !fmtFound {
		return ContainerInfo{}, nil, nil, nil, false
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
	if size > 0 && dataSize > 0 && int64(dataSize) <= size {
		info.StreamOverheadBytes = size - int64(dataSize)
	}

	format := "PCM"
	if audioFormat != 1 {
		format = "Unknown"
	}

	generalFields := []Field{}
	if audioFormat == 1 {
		generalFields = append(generalFields, Field{Name: "Format settings", Value: "PcmWaveformat"})
	}
	if encodedApp != "" {
		generalFields = append(generalFields, Field{Name: "Writing application", Value: encodedApp})
	}
	generalJSON := map[string]string{}
	if info.StreamOverheadBytes > 0 {
		generalJSON["StreamSize"] = strconv.FormatInt(info.StreamOverheadBytes, 10)
	}

	audioStream := canonicalWAVAudioStream(
		format,
		audioFormat,
		channels,
		sampleRate,
		bitsPerSample,
		duration,
		bitrate,
		mode,
		dataSize,
		blockAlign,
	)
	streams := []Stream{audioStream}
	return info, streams, generalFields, generalJSON, true
}

// canonicalWAVAudioStream records PCM stream facts in canonical units before creating a legacy snapshot.
func canonicalWAVAudioStream(format string, audioFormat, channels uint16, sampleRate uint32, bitsPerSample uint16, duration, bitrate float64, mode string, dataSize uint32, blockAlign uint16) Stream {
	store := &fieldStore{}
	ref := store.Prepare(StreamAudio)
	store.streams[ref].SkipStreamOrder = true
	store.Fill(ref, "Format", format, fillReplace)
	if audioFormat > 0 {
		store.Fill(ref, "CodecID", strconv.Itoa(int(audioFormat)), fillReplace)
	}
	if duration > 0 {
		store.Fill(ref, "Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), fillReplace)
	}
	if mode != "" {
		store.Fill(ref, "BitRate_Mode", mode, fillReplace)
	}
	if bitrate > 0 {
		store.Fill(ref, "BitRate", strconv.FormatInt(int64(math.Round(bitrate)), 10), fillReplace)
	}
	if channels > 0 {
		store.Fill(ref, "Channels", strconv.Itoa(int(channels)), fillReplace)
	}
	if sampleRate > 0 {
		store.Fill(ref, "SamplingRate", strconv.FormatUint(uint64(sampleRate), 10), fillReplace)
	}
	if dataSize > 0 && blockAlign > 0 {
		if samplingCount := int64(dataSize) / int64(blockAlign); samplingCount > 0 {
			value := strconv.FormatInt(samplingCount, 10)
			fillGeneratedStructured(store, ref, "SamplingCount", value)
			store.MarkLegacyJSON(ref, "SamplingCount", value, false)
		}
	}
	if dataSize > 0 {
		value := strconv.FormatUint(uint64(dataSize), 10)
		fillGeneratedStructured(store, ref, "StreamSize", value)
		store.MarkLegacyJSON(ref, "StreamSize", value, false)
	}
	if bitsPerSample > 0 {
		store.Fill(ref, "BitDepth", strconv.Itoa(int(bitsPerSample)), fillReplace)
	}
	if audioFormat == 1 {
		store.Fill(ref, "Format_Settings_Endianness", "Little", fillReplace)
		store.Fill(ref, "Format_Settings_Sign", "Signed", fillReplace)
	}
	return canonicalStreamSnapshot(store, ref, canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
}
