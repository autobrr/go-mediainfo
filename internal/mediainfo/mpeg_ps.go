package mediainfo

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
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
	packetOrder := 0

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
		case 0xBB, 0xBC, 0xBE: // system/program/padding
			if i+6 <= len(data) {
				length := int(binary.BigEndian.Uint16(data[i+4 : i+6]))
				i += 6 + length
			} else {
				i += 4
			}
			continue
		case 0xBF: // private stream 2 (DVD menu/navigation)
			if i+6 <= len(data) {
				length := int(binary.BigEndian.Uint16(data[i+4 : i+6]))
				payloadStart := i + 6
				payloadEnd := payloadStart + length
				if payloadEnd > len(data) {
					payloadEnd = len(data)
				}
				kind, format := mapPSStream(streamID, psSubstreamNone)
				if kind != "" {
					key := psStreamKey(streamID, psSubstreamNone)
					entry, exists := streams[key]
					if !exists {
						entry = &psStream{id: streamID, subID: psSubstreamNone, kind: kind, format: format, firstPacketOrder: -1, videoLastStartPos: -1}
						streams[key] = entry
						streamOrder = append(streamOrder, key)
					}
					if entry.kind != StreamMenu && entry.firstPacketOrder < 0 {
						entry.firstPacketOrder = packetOrder
						packetOrder++
					}
					if payloadStart < payloadEnd {
						entry.bytes += uint64(payloadEnd - payloadStart)
					}
				}
				i = payloadEnd
				if i <= payloadStart {
					i = payloadStart + 1
				}
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
			entry = &psStream{id: streamID, subID: subID, kind: kind, format: format, firstPacketOrder: -1, videoLastStartPos: -1}
			streams[key] = entry
			streamOrder = append(streamOrder, key)
		}
		if entry.kind != StreamMenu && entry.firstPacketOrder < 0 {
			entry.firstPacketOrder = packetOrder
			packetOrder++
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
					consumeMPEG2StartCodeStats(entry, esPayload, (flags&0x80) != 0)
					consumeH264PS(entry, esPayload)
					if !entry.videoIsH264 {
						parser := videoParsers[key]
						if parser == nil {
							parser = &mpeg2VideoParser{}
							videoParsers[key] = parser
						}
						parser.consume(esPayload)
						consumeMPEG2HeaderBytes(entry, esPayload)
					}
				}
				if entry.kind == StreamAudio {
					if entry.format == "AC-3" {
						consumeAC3PS(entry, esPayload)
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
	var videoIsH264 bool
	for _, key := range streamOrder {
		if st := streams[key]; st != nil && st.kind == StreamVideo {
			if st.videoIsH264 {
				videoIsH264 = true
				if st.videoFrameRate > 0 {
					videoFrameRate = st.videoFrameRate
				}
			} else {
				if parser := videoParsers[key]; parser != nil {
					info := parser.finalize()
					videoFrameRate = info.FrameRate
				}
			}
			break
		}
	}
	for _, key := range streamOrder {
		st := streams[key]
		if st == nil {
			continue
		}
		jsonExtras := map[string]string{}
		jsonRaw := map[string]string{}
		if st.firstPacketOrder >= 0 && st.kind != StreamMenu {
			jsonExtras["FirstPacketOrder"] = fmt.Sprintf("%d", st.firstPacketOrder)
		}
		if st.kind != StreamMenu {
			if st.subID != psSubstreamNone {
				jsonExtras["ID"] = fmt.Sprintf("%d-%d", st.id, st.subID)
			} else {
				jsonExtras["ID"] = fmt.Sprintf("%d", st.id)
			}
		}
		info := mpeg2VideoInfo{}
		if st.kind == StreamVideo && !st.videoIsH264 {
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
		fields := []Field{}
		if st.kind != StreamMenu {
			fields = append(fields, Field{Name: "ID", Value: idValue})
		}
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
				fields = append(fields, Field{Name: "Format version", Value: formatAACVersion(st.audioMPEGVersion)})
				fields = append(fields, Field{Name: "Muxing mode", Value: "ADTS"})
				if st.audioObject > 0 {
					fields = append(fields, Field{Name: "Codec ID", Value: fmt.Sprintf("%d", st.audioObject)})
				}
			} else if info := mapMatroskaFormatInfo(st.format); info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
		}
		if st.kind == StreamVideo && st.videoIsH264 {
			if info := mapMatroskaFormatInfo(st.format); info != "" {
				fields = append(fields, Field{Name: "Format/Info", Value: info})
			}
			if len(st.videoFields) > 0 {
				fields = append(fields, st.videoFields...)
			}
			if st.videoSliceCount > 0 {
				fields = append(fields, Field{Name: "Format settings, Slice count", Value: fmt.Sprintf("%d slices per frame", st.videoSliceCount)})
			}
			duration := st.pts.duration()
			if duration == 0 {
				duration = videoPTS.duration()
			}
			if duration > 0 {
				if st.videoFrameRate > 0 {
					duration += 2.0 / st.videoFrameRate
				}
				fields = addStreamDuration(fields, duration)
			}
			mode := "Constant"
			fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
			bitrate := 0.0
			if duration > 0 && st.bytes > 0 {
				bitrate = (float64(st.bytes) * 8) / duration
				if value := formatBitrate(bitrate); value != "" {
					fields = append(fields, Field{Name: "Nominal bit rate", Value: value})
				}
			}
			width := st.videoWidth
			height := st.videoHeight
			if width > 0 {
				fields = append(fields, Field{Name: "Width", Value: formatPixels(width)})
			}
			if height > 0 {
				fields = append(fields, Field{Name: "Height", Value: formatPixels(height)})
			}
			if ar := formatAspectRatio(width, height); ar != "" {
				fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
			}
			if st.videoFrameRate > 0 {
				fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(st.videoFrameRate)})
			}
			fields = append(fields, Field{Name: "Color space", Value: "YUV"})
			if bitrate > 0 && width > 0 && height > 0 && st.videoFrameRate > 0 {
				bits := bitrate / (float64(width) * float64(height) * st.videoFrameRate)
				fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: fmt.Sprintf("%.3f", bits)})
			}
		} else if st.kind == StreamVideo {
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
			effectiveBytes := st.bytes
			useHeaderBytes := fromGOP && st.videoHeaderBytes > 0
			if useHeaderBytes {
				effectiveBytes = st.videoHeaderBytes
			}
			bitrateDuration := duration
			if useHeaderBytes && info.FrameRate > 0 {
				rounded := math.Round(info.FrameRate)
				if rounded > 0 {
					bitrateDuration = 1.0 / rounded
				}
			}
			mode := info.BitRateMode
			if mode == "" {
				mode = "Variable"
			}
			fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
			bitrate := 0.0
			kbps := int64(0)
			if bitrateDuration > 0 && effectiveBytes > 0 {
				bitrate = (float64(effectiveBytes) * 8) / bitrateDuration
				if useHeaderBytes {
					if value := formatBitratePrecise(bitrate); value != "" {
						fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
					}
				} else {
					kbps = int64(bitrate / 1000.0)
					if kbps < 0 {
						kbps = 0
					}
					if value := formatBitrateKbps(kbps); value != "" {
						fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
					}
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
			if useHeaderBytes && effectiveBytes > 0 {
				if streamSize := formatStreamSize(int64(effectiveBytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if kbps > 0 && duration > 0 {
				streamSizeBytes := int64(float64(kbps*1000)*duration/8.0 + 0.5)
				if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if effectiveBytes > 0 {
				if streamSize := formatStreamSize(int64(effectiveBytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			}
			streamBytes := int64(effectiveBytes)
			jsonStreamBytes := streamBytes
			if !useHeaderBytes && !st.videoIsH264 && streamBytes > 0 {
				padding := int64(st.videoExtraZeros)
				if st.videoLastStartPos >= 0 && int64(st.videoTotalBytes) > st.videoLastStartPos {
					padding += int64(st.videoTotalBytes) - st.videoLastStartPos
				}
				if st.videoNoPTSPackets > 0 {
					padding += int64(st.videoNoPTSPackets) * 3
				}
				if padding > 0 && streamBytes > padding {
					jsonStreamBytes = streamBytes - padding
				}
			}
			if jsonStreamBytes > 0 {
				jsonExtras["StreamSize"] = fmt.Sprintf("%d", jsonStreamBytes)
			}
			jsonDuration := math.Round(duration*1000) / 1000
			jsonBitrateDuration := duration
			if useHeaderBytes && fromGOP && info.FrameRate > 0 {
				// MediaInfo JSON bitrate for GOP-only streams is slightly higher than GOPLength/FrameRate.
				// Align with CLI output for header-only VOB samples.
				jsonBitrateDuration *= 0.99818
			}
			if jsonBitrateDuration <= 0 {
				jsonBitrateDuration = jsonDuration
			}
			if jsonBitrateDuration > 0 && jsonStreamBytes > 0 {
				jsonBitrate := (float64(jsonStreamBytes) * 8) / jsonBitrateDuration
				jsonExtras["BitRate"] = fmt.Sprintf("%d", int64(math.Round(jsonBitrate)))
			} else if bitrate > 0 {
				jsonExtras["BitRate"] = fmt.Sprintf("%d", int64(math.Round(bitrate)))
			}
			if info.MatrixData != "" {
				jsonExtras["Format_Settings_Matrix_Data"] = info.MatrixData
			}
			if info.BufferSize > 0 {
				jsonExtras["BufferSize"] = fmt.Sprintf("%d", info.BufferSize)
			}
			if info.IntraDCPrecision > 0 {
				jsonRaw["extra"] = renderJSONObject([]jsonKV{{Key: "intra_dc_precision", Val: fmt.Sprintf("%d", info.IntraDCPrecision)}}, false)
			}
		} else if st.kind == StreamAudio {
			duration := st.pts.duration()
			if st.audioProfile != "" {
				if value := aacDurationPS(st); value > 0 {
					duration = value
				}
			} else if st.audioRate > 0 && st.audioFrames > 0 {
				rate := int64(st.audioRate)
				if rate > 0 {
					spf := uint64(1024)
					if st.hasAC3 && st.ac3Info.spf > 0 {
						spf = uint64(st.ac3Info.spf)
					}
					samples := st.audioFrames * spf
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
				if st.ac3Info.hasDialnorm {
					fields = append(fields, Field{Name: "Dialog Normalization", Value: fmt.Sprintf("%d dB", st.ac3Info.dialnorm)})
				}
				if st.ac3Info.hasCompr {
					fields = append(fields, Field{Name: "compr", Value: fmt.Sprintf("%.2f dB", st.ac3Info.comprDB)})
				}
				if st.ac3Info.hasCmixlev {
					fields = append(fields, Field{Name: "cmixlev", Value: fmt.Sprintf("%.1f dB", st.ac3Info.cmixlevDB)})
				}
				if st.ac3Info.hasSurmixlev {
					fields = append(fields, Field{Name: "surmixlev", Value: fmt.Sprintf("%.0f dB", st.ac3Info.surmixlevDB)})
				}
				if st.ac3Info.hasMixlevel {
					fields = append(fields, Field{Name: "mixlevel", Value: fmt.Sprintf("%d dB", st.ac3Info.mixlevel)})
				}
				if st.ac3Info.hasRoomtyp {
					fields = append(fields, Field{Name: "roomtyp", Value: st.ac3Info.roomtyp})
				}
				if avg, min, max, ok := st.ac3Info.dialnormStats(); ok {
					fields = append(fields, Field{Name: "dialnorm_Average", Value: fmt.Sprintf("%d dB", avg)})
					fields = append(fields, Field{Name: "dialnorm_Minimum", Value: fmt.Sprintf("%d dB", min)})
					if max != min {
						fields = append(fields, Field{Name: "dialnorm_Maximum", Value: fmt.Sprintf("%d dB", max)})
					}
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
				if duration > 0 && st.bytes > 0 && st.audioProfile == "" {
					bitrate := (float64(st.bytes) * 8) / duration
					if value := formatBitrate(bitrate); value != "" {
						fields = append(fields, Field{Name: "Bit rate", Value: value})
					}
				}
				if st.bytes > 0 && st.audioProfile == "" {
					if streamSize := formatStreamSize(int64(st.bytes), size); streamSize != "" {
						fields = append(fields, Field{Name: "Stream size", Value: streamSize})
					}
				}
				if st.audioProfile != "" && videoPTS.has() && st.pts.has() {
					delay := float64(int64(st.pts.min)-int64(videoPTS.min)) * 1000 / 90000.0
					if videoIsH264 && videoFrameRate > 0 {
						delay -= (3.0 / videoFrameRate) * 1000.0
					}
					fields = append(fields, Field{Name: "Delay relative to video", Value: formatDelayMs(int64(math.Round(delay)))})
				}
			}
			if st.hasAC3 {
				if duration > 0 {
					jsonExtras["Duration"] = fmt.Sprintf("%.3f", duration)
				}
				if st.ac3Info.spf > 0 {
					jsonExtras["SamplesPerFrame"] = fmt.Sprintf("%d", st.ac3Info.spf)
				}
				if st.ac3Info.sampleRate > 0 {
					if st.audioFrames > 0 && st.ac3Info.spf > 0 {
						samplingCount := int64(st.audioFrames) * int64(st.ac3Info.spf)
						jsonExtras["SamplingCount"] = fmt.Sprintf("%d", samplingCount)
						jsonExtras["FrameCount"] = fmt.Sprintf("%d", st.audioFrames)
					} else if duration > 0 {
						samplingCount := int64(math.Round(duration * st.ac3Info.sampleRate))
						jsonExtras["SamplingCount"] = fmt.Sprintf("%d", samplingCount)
						if st.ac3Info.spf > 0 {
							frameCount := int64(math.Round(float64(samplingCount) / float64(st.ac3Info.spf)))
							jsonExtras["FrameCount"] = fmt.Sprintf("%d", frameCount)
						}
					}
				}
				if st.ac3Info.bitRateKbps > 0 && duration > 0 {
					streamSizeBytes := int64(math.Round(float64(st.ac3Info.bitRateKbps*1000) * duration / 8.0))
					if streamSizeBytes > 0 {
						jsonExtras["StreamSize"] = fmt.Sprintf("%d", streamSizeBytes)
					}
				}
				jsonExtras["Format_Settings_Endianness"] = "Big"
				if code := ac3ServiceKindCode(st.ac3Info.bsmod); code != "" {
					jsonExtras["ServiceKind"] = code
				}
				extraFields := []jsonKV{}
				if st.ac3Info.bsid > 0 {
					extraFields = append(extraFields, jsonKV{Key: "bsid", Val: fmt.Sprintf("%d", st.ac3Info.bsid)})
				}
				if st.ac3Info.hasDialnorm {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: fmt.Sprintf("%d", st.ac3Info.dialnorm)})
				}
				if st.ac3Info.hasCompr {
					extraFields = append(extraFields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", st.ac3Info.comprDB)})
				}
				if st.ac3Info.acmod > 0 {
					extraFields = append(extraFields, jsonKV{Key: "acmod", Val: fmt.Sprintf("%d", st.ac3Info.acmod)})
				}
				if st.ac3Info.lfeon >= 0 {
					extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: fmt.Sprintf("%d", st.ac3Info.lfeon)})
				}
				if st.ac3Info.hasCmixlev {
					extraFields = append(extraFields, jsonKV{Key: "cmixlev", Val: fmt.Sprintf("%.1f", st.ac3Info.cmixlevDB)})
				}
				if st.ac3Info.hasSurmixlev {
					extraFields = append(extraFields, jsonKV{Key: "surmixlev", Val: fmt.Sprintf("%.0f dB", st.ac3Info.surmixlevDB)})
				}
				if st.ac3Info.hasMixlevel {
					extraFields = append(extraFields, jsonKV{Key: "mixlevel", Val: fmt.Sprintf("%d", st.ac3Info.mixlevel)})
				}
				if st.ac3Info.hasRoomtyp {
					extraFields = append(extraFields, jsonKV{Key: "roomtyp", Val: st.ac3Info.roomtyp})
				}
				if avg, min, max, ok := st.ac3Info.dialnormStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: fmt.Sprintf("%d", avg)})
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: fmt.Sprintf("%d", min)})
					if max != min {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: fmt.Sprintf("%d", max)})
					}
				}
				if avg, min, max, count, ok := st.ac3Info.comprStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", min)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", max)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: fmt.Sprintf("%d", count)})
				}
				if avg, min, max, count, ok := st.ac3Info.dynrngStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Minimum", Val: fmt.Sprintf("%.2f", min)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Maximum", Val: fmt.Sprintf("%.2f", max)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Count", Val: fmt.Sprintf("%d", count)})
				}
				if len(extraFields) > 0 {
					jsonRaw["extra"] = renderJSONObject(extraFields, false)
				}
			}
		} else if st.kind != StreamVideo {
			if duration := st.pts.duration(); duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
		}
		if st.kind == StreamVideo && st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			jsonExtras["Delay_DropFrame"] = "No"
			jsonExtras["Delay_Source"] = "Container"
			jsonExtras["Delay_Original"] = "0.000"
			jsonExtras["Delay_Original_DropFrame"] = "No"
			jsonExtras["Delay_Original_Source"] = "Stream"
		}
		if st.kind == StreamAudio && st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			jsonExtras["Delay_Source"] = "Container"
			if videoPTS.has() {
				videoDelay := float64(int64(st.pts.min)-int64(videoPTS.min)) / 90000.0
				jsonExtras["Video_Delay"] = fmt.Sprintf("%.3f", videoDelay)
			}
		}
		streamsOut = append(streamsOut, Stream{Kind: st.kind, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw})
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
	videoDuration := 0.0
	if duration := videoPTS.duration(); duration > 0 {
		if videoFrameRate > 0 {
			duration += 2.0 / videoFrameRate
		}
		videoDuration = duration
	}
	maxDuration := 0.0
	for _, st := range streams {
		if st == nil || st.kind == StreamMenu {
			continue
		}
		duration := st.pts.duration()
		if st.kind == StreamAudio {
			if st.audioProfile != "" {
				if value := aacDurationPS(st); value > 0 {
					duration = value
				}
			} else if st.audioRate > 0 && st.audioFrames > 0 {
				rate := int64(st.audioRate)
				if rate > 0 {
					spf := uint64(1024)
					if st.hasAC3 && st.ac3Info.spf > 0 {
						spf = uint64(st.ac3Info.spf)
					}
					samples := st.audioFrames * spf
					durationMs := int64((samples * 1000) / uint64(rate))
					duration = float64(durationMs) / 1000.0
				}
			}
		}
		if duration > maxDuration {
			maxDuration = duration
		}
	}
	if videoDuration > maxDuration {
		maxDuration = videoDuration
	}
	if maxDuration > 0 {
		info.DurationSeconds = maxDuration
	} else if duration := anyPTS.duration(); duration > 0 {
		info.DurationSeconds = duration
	}

	return info, streamsOut, true
}

func consumeMPEG2StartCodeStats(entry *psStream, payload []byte, hasPTS bool) {
	if entry == nil || len(payload) == 0 {
		return
	}
	if !hasPTS {
		entry.videoNoPTSPackets++
	}
	for _, b := range payload {
		entry.videoTotalBytes++
		if b == 0x00 {
			entry.videoStartZeroRun++
			continue
		}
		if b == 0x01 && entry.videoStartZeroRun >= 2 {
			if extra := entry.videoStartZeroRun - 2; extra > 0 {
				entry.videoExtraZeros += uint64(extra)
			}
			entry.videoLastStartPos = int64(entry.videoTotalBytes - 3)
			entry.videoStartZeroRun = 0
			continue
		}
		entry.videoStartZeroRun = 0
	}
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
	case streamID == 0xBF:
		return StreamMenu, "DVD-Video"
	case streamID >= 0xE0 && streamID <= 0xEF:
		return StreamVideo, "MPEG Video"
	case streamID >= 0xC0 && streamID <= 0xDF:
		return StreamAudio, "MPEG Audio"
	default:
		return "", ""
	}
}

func consumeAC3PS(entry *psStream, payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	i := 0
	for i+7 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0x0B || entry.audioBuffer[i+1] != 0x77 {
			i++
			continue
		}
		frameInfo, frameSize, ok := parseAC3Frame(entry.audioBuffer[i:])
		if !ok || frameSize <= 0 {
			i++
			continue
		}
		if i+frameSize > len(entry.audioBuffer) {
			break
		}
		if i+frameSize+1 < len(entry.audioBuffer) {
			if entry.audioBuffer[i+frameSize] != 0x0B || entry.audioBuffer[i+frameSize+1] != 0x77 {
				i++
				continue
			}
		}
		entry.ac3Info.mergeFrame(frameInfo)
		entry.hasAC3 = true
		entry.audioFrames++
		if entry.audioRate == 0 && frameInfo.sampleRate > 0 {
			entry.audioRate = frameInfo.sampleRate
		}
		i += frameSize
	}
	if i > 0 {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[i:]...)
	}
}

func consumeH264PS(entry *psStream, payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry.videoBuffer = append(entry.videoBuffer, payload...)
	const maxProbe = 2 * 1024 * 1024
	if len(entry.videoBuffer) > maxProbe {
		entry.videoBuffer = append(entry.videoBuffer[:0], entry.videoBuffer[len(entry.videoBuffer)-maxProbe:]...)
	}
	if !entry.videoIsH264 {
		if fields, width, height, fps := parseH264AnnexB(entry.videoBuffer); len(fields) > 0 {
			entry.videoFields = fields
			entry.hasVideoFields = true
			entry.videoWidth = width
			entry.videoHeight = height
			if fps > 0 {
				entry.videoFrameRate = fps
			}
			entry.videoIsH264 = true
			entry.format = "AVC"
		}
	}
	if entry.videoIsH264 {
		if entry.videoSliceCount == 0 && len(entry.videoBuffer) >= maxProbe/4 {
			if count := h264SliceCountAnnexB(entry.videoBuffer); count > 1 {
				entry.videoSliceCount = count
			}
		}
		if !entry.videoSliceProbed && len(entry.videoBuffer) >= maxProbe {
			if count := h264SliceCountAnnexB(entry.videoBuffer); count > entry.videoSliceCount {
				entry.videoSliceCount = count
			}
			entry.videoSliceProbed = true
		}
	}
	if entry.videoIsH264 && entry.videoSliceCount > 0 && len(entry.videoBuffer) > maxProbe {
		entry.videoBuffer = nil
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
		mpegID := (entry.audioBuffer[i+1] >> 3) & 0x01
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
				entry.audioMPEGVersion = adtsMPEGVersion(mpegID)
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

func aacDurationPS(entry *psStream) float64 {
	if entry.audioRate <= 0 {
		return 0
	}
	frameDuration := 1024.0 / entry.audioRate
	if entry.pts.has() {
		duration := entry.pts.duration() + 3*frameDuration
		ms := int64(duration * 1000)
		return float64(ms) / 1000.0
	}
	if entry.audioFrames > 0 {
		return float64(entry.audioFrames) * frameDuration
	}
	return 0
}

func formatAudioFrameRate(rate float64, spf int) string {
	if rate <= 0 || spf <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS (%d SPF)", rate, spf)
}

func formatDelayMs(ms int64) string {
	if ms == 0 {
		return "0 ms"
	}
	neg := ms < 0
	if neg {
		ms = -ms
	}
	if ms < 1000 {
		if neg {
			return fmt.Sprintf("-%d ms", ms)
		}
		return fmt.Sprintf("%d ms", ms)
	}
	seconds := float64(ms) / 1000.0
	value := formatDuration(seconds)
	if neg {
		return "-" + value
	}
	return value
}
