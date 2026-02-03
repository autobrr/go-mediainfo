package mediainfo

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
)

const psSubstreamNone = 0xFF

func psStreamKey(id, subID byte) uint16 {
	return uint16(id)<<8 | uint16(subID)
}

func ParseMPEGPS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	data, err := io.ReadAll(reader)
	if err != nil || len(data) < 4 {
		return ContainerInfo{}, nil, false
	}

	streams := map[uint16]*psStream{}
	streamOrder := []uint16{}
	videoParsers := map[uint16]*mpeg2VideoParser{}
	var videoPTS ptsTracker
	var anyPTS ptsTracker

	for i := 0; i+4 <= len(data); {
		if data[i] != 0x00 || data[i+1] != 0x00 || data[i+2] != 0x01 {
			i++
			continue
		}
		streamID := data[i+3]
		switch streamID {
		case 0xBA: // pack header (MPEG-2)
			if i+13 < len(data) {
				stuffing := int(data[i+13] & 0x07)
				i += 4 + 10 + stuffing
			} else {
				i += 4
			}
			continue
		case 0xBB, 0xBC, 0xBE, 0xBF: // system/program/padding/private stream 2
			if i+6 <= len(data) {
				length := int(binary.BigEndian.Uint16(data[i+4 : i+6]))
				i += 6 + length
			} else {
				i += 4
			}
			continue
		}

		if i+9 >= len(data) {
			i += 4
			continue
		}
		pesLen := int(binary.BigEndian.Uint16(data[i+4 : i+6]))
		if (data[i+6] & 0xC0) != 0x80 {
			i++
			continue
		}
		flags := data[i+7]
		headerLen := int(data[i+8])
		payloadStart := i + 9 + headerLen
		if payloadStart > len(data) {
			i += 4
			continue
		}

		payloadLen := 0
		if pesLen > 0 {
			payloadLen = pesLen - 3 - headerLen
			if payloadLen < 0 {
				payloadLen = 0
			}
			if payloadStart+payloadLen > len(data) {
				payloadLen = len(data) - payloadStart
			}
		} else {
			next := nextPESStart(data, payloadStart)
			payloadLen = next - payloadStart
		}
		payloadEnd := payloadStart + payloadLen
		if payloadEnd > len(data) {
			payloadEnd = len(data)
		}
		if payloadEnd < payloadStart {
			payloadEnd = payloadStart
		}

		payload := data[payloadStart:payloadEnd]
		subID := byte(psSubstreamNone)
		payloadOffset := 0
		if streamID == 0xBD && len(payload) > 0 {
			subID = payload[0]
			payloadOffset = 1
			if subID >= 0x80 && subID <= 0x87 && len(payload) > 4 {
				payloadOffset = 4
			}
		}

		kind, format := mapPSStream(streamID, subID)
		if kind == "" {
			i = payloadEnd
			if i <= payloadStart {
				i = payloadStart + 1
			}
			continue
		}

		key := psStreamKey(streamID, subID)
		entry, exists := streams[key]
		if !exists {
			entry = &psStream{id: streamID, subID: subID, kind: kind, format: format}
			streams[key] = entry
			streamOrder = append(streamOrder, key)
		}

		if (flags&0x80) != 0 && i+9+headerLen <= len(data) {
			if pts, ok := parsePTS(data[i+9:]); ok {
				anyPTS.add(pts)
				entry.pts.add(pts)
				if entry.kind == StreamVideo {
					videoPTS.add(pts)
				}
			}
		}

		if len(payload) > 0 {
			esPayload := payload
			if payloadOffset > 0 {
				if payloadOffset >= len(payload) {
					esPayload = nil
				} else {
					esPayload = payload[payloadOffset:]
				}
			}
			if len(esPayload) > 0 {
				entry.bytes += uint64(len(esPayload))
				if entry.kind == StreamVideo {
					parser := videoParsers[key]
					if parser == nil {
						parser = &mpeg2VideoParser{}
						videoParsers[key] = parser
					}
					parser.consume(esPayload)
				}
				if entry.kind == StreamAudio {
					if entry.format == "AC-3" {
						if !entry.hasAC3 {
							if info, ok := parseAC3Header(esPayload); ok {
								entry.ac3Info = info
								entry.hasAC3 = true
							}
						}
					} else {
						consumeADTSPS(entry, esPayload)
						if entry.hasAudioInfo && entry.format == "MPEG Audio" {
							entry.format = "AAC"
						}
					}
				}
			}
		}

		i = payloadEnd
		if i <= payloadStart {
			i = payloadStart + 1
		}
	}

	var streamsOut []Stream
	sort.Slice(streamOrder, func(i, j int) bool { return streamOrder[i] < streamOrder[j] })
	var videoFrameRate float64
	for _, key := range streamOrder {
		if st := streams[key]; st != nil && st.kind == StreamVideo {
			if parser := videoParsers[key]; parser != nil {
				info := parser.finalize()
				videoFrameRate = info.FrameRate
			}
			break
		}
	}
	for _, key := range streamOrder {
		st := streams[key]
		if st == nil {
			continue
		}
		info := mpeg2VideoInfo{}
		if st.kind == StreamVideo {
			if parser := videoParsers[key]; parser != nil {
				info = parser.finalize()
			}
			if !st.pts.has() && info.Width == 0 && info.Height == 0 && info.FrameRate == 0 && info.FrameRateNumer == 0 {
				continue
			}
		}
		if st.kind == StreamAudio && !st.pts.has() && !st.hasAC3 && !st.hasAudioInfo {
			continue
		}
		idValue := formatID(uint64(st.id))
		if st.subID != psSubstreamNone {
			idValue = formatIDPair(uint64(st.id), uint64(st.subID))
		}
		fields := []Field{{Name: "ID", Value: idValue}}
		format := st.format
		if st.kind == StreamAudio && st.audioProfile != "" {
			format = "AAC " + st.audioProfile
		}
		if format != "" {
			fields = append(fields, Field{Name: "Format", Value: format})
		}
		if st.kind == StreamAudio {
			if format == "AC-3" {
				if info := mapMatroskaFormatInfo(st.format); info != "" {
					fields = append(fields, Field{Name: "Format/Info", Value: info})
				}
				fields = append(fields, Field{Name: "Commercial name", Value: "Dolby Digital"})
				fields = append(fields, Field{Name: "Muxing mode", Value: "DVD-Video"})
			} else if st.audioProfile == "LC" {
				fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
				fields = append(fields, Field{Name: "Format version", Value: "Version 4"})
				fields = append(fields, Field{Name: "Muxing mode", Value: "ADTS"})
			} else if info := mapMatroskaFormatInfo(st.format); info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
		}
		if st.kind == StreamVideo {
			if info.Version != "" {
				fields = append(fields, Field{Name: "Format version", Value: info.Version})
			}
			if info.Profile != "" {
				fields = append(fields, Field{Name: "Format profile", Value: info.Profile})
			}
			formatSettings := ""
			if info.Matrix == "Custom" {
				formatSettings = "CustomMatrix"
			}
			if info.BVOP != nil && *info.BVOP {
				if formatSettings != "" {
					formatSettings += " / BVOP"
				} else {
					formatSettings = "BVOP"
				}
			}
			if formatSettings != "" {
				fields = append(fields, Field{Name: "Format settings", Value: formatSettings})
			}
			if info.BVOP != nil {
				fields = append(fields, Field{Name: "Format settings, BVOP", Value: formatYesNo(*info.BVOP)})
			}
			if info.Matrix != "" {
				fields = append(fields, Field{Name: "Format settings, Matrix", Value: info.Matrix})
			}
			if info.GOPLength > 1 {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: fmt.Sprintf("N=%d", info.GOPLength)})
			}
			duration := st.pts.duration()
			if duration == 0 {
				duration = videoPTS.duration()
			}
			fromGOP := false
			if duration == 0 {
				if info.FrameRate > 0 && info.GOPLength > 0 {
					duration = float64(info.GOPLength) / info.FrameRate
					fromGOP = true
				} else {
					duration = anyPTS.duration()
				}
			}
			if duration > 0 && info.FrameRate > 0 && !fromGOP {
				duration += 2.0 / info.FrameRate
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
			mode := info.BitRateMode
			if mode == "" {
				mode = "Variable"
			}
			fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
			bitrate := 0.0
			kbps := int64(0)
			if duration > 0 && st.bytes > 0 {
				bitrate = (float64(st.bytes) * 8) / duration
				kbps = int64(bitrate / 1000.0)
				if kbps < 0 {
					kbps = 0
				}
				if value := formatBitrateKbps(kbps); value != "" {
					fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
				}
			}
			if info.MaxBitRateKbps > 0 {
				if value := formatBitrateKbps(info.MaxBitRateKbps); value != "" {
					fields = append(fields, Field{Name: "Maximum bit rate", Value: value})
				}
			}
			if info.Width > 0 {
				fields = append(fields, Field{Name: "Width", Value: formatPixels(info.Width)})
			}
			if info.Height > 0 {
				fields = append(fields, Field{Name: "Height", Value: formatPixels(info.Height)})
			}
			if info.AspectRatio != "" {
				fields = append(fields, Field{Name: "Display aspect ratio", Value: info.AspectRatio})
			}
			if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateRatio(info.FrameRateNumer, info.FrameRateDenom)})
			} else if info.FrameRate > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(info.FrameRate)})
			}
			if standard := mapMPEG2Standard(info.FrameRate); standard != "" {
				if (standard == "NTSC" && info.Width == 720 && info.Height == 480) ||
					(standard == "PAL" && info.Width == 720 && info.Height == 576) {
					fields = append(fields, Field{Name: "Standard", Value: standard})
				}
			}
			if info.ColorSpace != "" {
				fields = append(fields, Field{Name: "Color space", Value: info.ColorSpace})
			}
			if info.ChromaSubsampling != "" {
				fields = append(fields, Field{Name: "Chroma subsampling", Value: info.ChromaSubsampling})
			}
			if info.BitDepth != "" {
				fields = append(fields, Field{Name: "Bit depth", Value: info.BitDepth})
			}
			if info.ScanType != "" {
				fields = append(fields, Field{Name: "Scan type", Value: info.ScanType})
			}
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
			if bitrate > 0 && info.Width > 0 && info.Height > 0 {
				if bits := formatBitsPerPixelFrame(bitrate, info.Width, info.Height, info.FrameRate); bits != "" {
					fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
				}
			}
			if info.TimeCode != "" {
				fields = append(fields, Field{Name: "Time code of first frame", Value: info.TimeCode})
			}
			if info.TimeCodeSource != "" {
				fields = append(fields, Field{Name: "Time code source", Value: info.TimeCodeSource})
			}
			if info.GOPLength > 1 {
				if info.GOPOpenClosed != "" {
					fields = append(fields, Field{Name: "GOP, Open/Closed", Value: info.GOPOpenClosed})
				}
				if info.GOPFirstClosed != "" {
					fields = append(fields, Field{Name: "GOP, Open/Closed of first frame", Value: info.GOPFirstClosed})
				}
			}
			if kbps > 0 && duration > 0 {
				streamSizeBytes := int64(float64(kbps*1000)*duration/8.0 + 0.5)
				if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if st.bytes > 0 {
				if streamSize := formatStreamSize(int64(st.bytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			}
		} else if st.kind == StreamAudio {
			duration := st.pts.duration()
			if st.audioRate > 0 && st.audioFrames > 0 {
				rate := int64(st.audioRate)
				if rate > 0 {
					samples := st.audioFrames * 1024
					durationMs := int64((samples * 1000) / uint64(rate))
					duration = float64(durationMs) / 1000.0
				}
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
			if st.hasAC3 {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
				if st.ac3Info.bitRateKbps > 0 {
					fields = append(fields, Field{Name: "Bit rate", Value: formatBitrateKbps(st.ac3Info.bitRateKbps)})
				}
				if st.ac3Info.channels > 0 {
					fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.ac3Info.channels)})
				}
				if st.ac3Info.layout != "" {
					fields = append(fields, Field{Name: "Channel layout", Value: st.ac3Info.layout})
				}
				if st.ac3Info.sampleRate > 0 {
					fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.ac3Info.sampleRate)})
				}
				if value := formatAudioFrameRate(st.ac3Info.frameRate, st.ac3Info.spf); value != "" {
					fields = append(fields, Field{Name: "Frame rate", Value: value})
				}
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				if duration > 0 && st.ac3Info.bitRateKbps > 0 {
					streamSizeBytes := int64(float64(st.ac3Info.bitRateKbps*1000)*duration/8.0 + 0.5)
					if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
						fields = append(fields, Field{Name: "Stream size", Value: streamSize})
					}
				}
				if st.ac3Info.serviceKind != "" {
					fields = append(fields, Field{Name: "Service kind", Value: st.ac3Info.serviceKind})
				}
			} else if st.audioRate > 0 {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
				fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.audioChannels)})
				if layout := channelLayout(st.audioChannels); layout != "" {
					fields = append(fields, Field{Name: "Channel layout", Value: layout})
				}
				fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.audioRate)})
				frameRate := st.audioRate / 1024.0
				fields = append(fields, Field{Name: "Frame rate", Value: formatAudioFrameRate(frameRate, 1024)})
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				if duration > 0 && st.bytes > 0 {
					bitrate := (float64(st.bytes) * 8) / duration
					if value := formatBitrate(bitrate); value != "" {
						fields = append(fields, Field{Name: "Bit rate", Value: value})
					}
				}
				if st.bytes > 0 {
					if streamSize := formatStreamSize(int64(st.bytes), size); streamSize != "" {
						fields = append(fields, Field{Name: "Stream size", Value: streamSize})
					}
				}
			}
		} else if st.kind != StreamVideo {
			if duration := st.pts.duration(); duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
		}
		streamsOut = append(streamsOut, Stream{Kind: st.kind, Fields: fields})
	}

	info := ContainerInfo{}
	var hasMode bool
	allConstant := true
	for _, key := range streamOrder {
		st := streams[key]
		if st == nil || (st.kind != StreamVideo && st.kind != StreamAudio) {
			continue
		}
		mode := ""
		if st.kind == StreamVideo {
			if parser := videoParsers[key]; parser != nil {
				videoInfo := parser.finalize()
				mode = videoInfo.BitRateMode
			}
		} else if st.kind == StreamAudio {
			if st.hasAC3 {
				mode = "Constant"
			} else if st.audioRate > 0 {
				mode = "Variable"
			}
		}
		if mode == "" {
			continue
		}
		hasMode = true
		if mode != "Constant" {
			allConstant = false
		}
	}
	if hasMode {
		if allConstant {
			info.BitrateMode = "Constant"
		} else {
			info.BitrateMode = "Variable"
		}
	}
	if duration := videoPTS.duration(); duration > 0 {
		if videoFrameRate > 0 {
			duration += 2.0 / videoFrameRate
		}
		info.DurationSeconds = duration
	} else if duration := anyPTS.duration(); duration > 0 {
		info.DurationSeconds = duration
	}

	return info, streamsOut, true
}

func nextPESStart(data []byte, start int) int {
	for i := start; i+3 < len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 && data[i+2] == 0x01 {
			if isPESStreamID(data[i+3]) {
				return i
			}
		}
	}
	return len(data)
}

func isPESStreamID(streamID byte) bool {
	switch {
	case streamID == 0xBA || streamID == 0xBB || streamID == 0xBC || streamID == 0xBD:
		return true
	case streamID == 0xBE || streamID == 0xBF:
		return true
	case streamID >= 0xC0 && streamID <= 0xEF:
		return true
	default:
		return false
	}
}

func mapPSStream(streamID byte, subID byte) (StreamKind, string) {
	if streamID == 0xBD {
		switch {
		case subID >= 0x80 && subID <= 0x87:
			return StreamAudio, "AC-3"
		case subID >= 0x88 && subID <= 0x8F:
			return StreamAudio, "DTS"
		case subID >= 0xA0 && subID <= 0xAF:
			return StreamAudio, "PCM"
		case subID >= 0x20 && subID <= 0x3F:
			return StreamText, "RLE"
		default:
			return "", ""
		}
	}
	switch {
	case streamID >= 0xE0 && streamID <= 0xEF:
		return StreamVideo, "MPEG Video"
	case streamID >= 0xC0 && streamID <= 0xDF:
		return StreamAudio, "MPEG Audio"
	default:
		return "", ""
	}
}

func consumeADTSPS(entry *psStream, payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	i := 0
	for i+7 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0xFF || (entry.audioBuffer[i+1]&0xF0) != 0xF0 {
			i++
			continue
		}
		if (entry.audioBuffer[i+1] & 0x06) != 0 {
			i++
			continue
		}
		protectionAbsent := entry.audioBuffer[i+1] & 0x01
		profile := (entry.audioBuffer[i+2] >> 6) & 0x03
		samplingIndex := (entry.audioBuffer[i+2] >> 2) & 0x0F
		channelConfig := ((entry.audioBuffer[i+2] & 0x01) << 2) | ((entry.audioBuffer[i+3] >> 6) & 0x03)
		frameLen := ((int(entry.audioBuffer[i+3]) & 0x03) << 11) | (int(entry.audioBuffer[i+4]) << 3) | ((int(entry.audioBuffer[i+5]) >> 5) & 0x07)
		headerLen := 7
		if protectionAbsent == 0 {
			headerLen = 9
		}
		if samplingIndex == 0x0F || frameLen < headerLen {
			i++
			continue
		}
		if i+frameLen > len(entry.audioBuffer) {
			break
		}
		if i+frameLen+1 < len(entry.audioBuffer) {
			if entry.audioBuffer[i+frameLen] != 0xFF || (entry.audioBuffer[i+frameLen+1]&0xF0) != 0xF0 {
				i++
				continue
			}
		}
		entry.audioFrames++
		if !entry.hasAudioInfo {
			objType := int(profile) + 1
			sampleRate := adtsSampleRate(int(samplingIndex))
			if sampleRate > 0 {
				entry.audioProfile = mapAACProfile(objType)
				entry.audioObject = objType
				entry.audioRate = sampleRate
				entry.audioChannels = uint64(channelConfig)
				entry.hasAudioInfo = true
			}
		}
		i += frameLen
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

func formatAudioFrameRate(rate float64, spf int) string {
	if rate <= 0 || spf <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS (%d SPF)", rate, spf)
}
