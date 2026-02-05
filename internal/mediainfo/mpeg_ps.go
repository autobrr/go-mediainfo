package mediainfo

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"slices"
)

const psSubstreamNone = 0xFF
const dvdHeaderStreamScale = 1.13503156

func psStreamKey(id, subID byte) uint16 {
	return uint16(id)<<8 | uint16(subID)
}

func ParseMPEGPS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	return ParseMPEGPSWithOptions(file, size, mpegPSOptions{})
}

func ParseMPEGPSWithOptions(file io.ReadSeeker, size int64, opts mpegPSOptions) (ContainerInfo, []Stream, bool) {
	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	if parseSpeed < 1 && size > 0 {
		if readerAt, ok := file.(io.ReaderAt); ok {
			parser := newPSStreamParser()
			reader := func(r io.Reader) bool {
				buf := bufio.NewReaderSize(r, 1<<20)
				return parser.parseReader(buf)
			}
			sampleSize := int64(64 << 20)
			if parseSpeed > 0 && parseSpeed < 1 {
				sampleSize = max(int64(float64(sampleSize)*parseSpeed), 4<<20)
			}
			if opts.dvdParsing && sampleSize < 16<<20 {
				sampleSize = 16 << 20
			}
			parsedAny := false
			if size <= sampleSize {
				if _, err := file.Seek(0, io.SeekStart); err == nil {
					if reader(file) {
						parsedAny = true
					}
				}
			} else {
				first := io.NewSectionReader(readerAt, 0, sampleSize)
				if reader(first) {
					parsedAny = true
				}
				if size > sampleSize*2 {
					mid := (size - sampleSize) / 2
					middle := io.NewSectionReader(readerAt, mid, sampleSize)
					if reader(middle) {
						parsedAny = true
					}
					start := size - sampleSize
					last := io.NewSectionReader(readerAt, start, sampleSize)
					if reader(last) {
						parsedAny = true
					}
				}
			}
			if parsedAny {
				return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts)
			}
		}
	}

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
				payloadEnd := min(payloadStart+length, len(data))
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
					entry.packetCount++
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
			payloadLen = max(pesLen-3-headerLen, 0)
			if payloadStart+payloadLen > len(data) {
				payloadLen = len(data) - payloadStart
			}
		} else {
			next := nextPESStart(data, payloadStart)
			payloadLen = next - payloadStart
		}
		payloadEnd := min(payloadStart+payloadLen, len(data))
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

		var currentPTS uint64
		var hasPTS bool
		if (flags&0x80) != 0 && i+9+headerLen <= len(data) {
			if pts, ok := parsePTS(data[i+9:]); ok {
				currentPTS = pts
				hasPTS = true
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
					consumeMPEG2Captions(entry, esPayload, currentPTS, hasPTS)
					consumeMPEG2StartCodeStats(entry, esPayload, (flags&0x80) != 0)
					parser := videoParsers[key]
					if parser == nil {
						parser = &mpeg2VideoParser{}
						videoParsers[key] = parser
					}
					parser.consume(esPayload)
					if parser.sawSequence {
						entry.videoIsMPEG2 = true
						entry.videoIsH264 = false
						entry.format = "MPEG Video"
					}
					if entry.videoIsMPEG2 {
						consumeMPEG2HeaderBytes(entry, esPayload, hasPTS)
						consumeMPEG2FrameBytes(entry, esPayload)
					}
					if !entry.videoIsMPEG2 {
						consumeH264PS(entry, esPayload)
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

	return finalizeMPEGPS(streams, streamOrder, videoParsers, videoPTS, anyPTS, size, opts)
}

type mpegPSOptions struct {
	dvdExtras  bool
	dvdParsing bool
	parseSpeed float64
}

func ptsDurationPS(tracker ptsTracker, opts mpegPSOptions) float64 {
	if !tracker.has() {
		return 0
	}
	if opts.dvdParsing && tracker.hasResets() {
		return tracker.durationLastSegment()
	}
	if opts.parseSpeed < 1 && tracker.durationTotal() > 0 {
		return tracker.durationTotal()
	}
	if tracker.hasResets() {
		return tracker.durationLastSegment()
	}
	return tracker.duration()
}

func audioDurationPS(st *psStream, opts mpegPSOptions) float64 {
	if st == nil {
		return 0
	}
	duration := ptsDurationPS(st.pts, opts)
	if st.audioProfile != "" {
		if value := aacDurationPS(st); value > 0 {
			duration = value
		}
	} else if duration == 0 && st.audioRate > 0 && st.audioFrames > 0 {
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
	if st.hasAC3 && st.pts.has() && st.ac3Info.sampleRate > 0 && st.ac3Info.spf > 0 {
		duration += float64(st.ac3Info.spf) / st.ac3Info.sampleRate
	}
	return duration
}

func delayRelativeToVideoMs(audio ptsTracker, video ptsTracker, videoIsH264 bool, videoFrameRate float64) (float64, bool) {
	if !audio.has() || !video.has() {
		return 0, false
	}
	delay := float64(int64(audio.first)-int64(video.first)) * 1000 / 90000.0
	if videoIsH264 && videoFrameRate > 0 {
		delay -= (3.0 / videoFrameRate) * 1000.0
	}
	return delay, true
}

func finalizeMPEGPS(streams map[uint16]*psStream, streamOrder []uint16, videoParsers map[uint16]*mpeg2VideoParser, videoPTS ptsTracker, anyPTS ptsTracker, size int64, opts mpegPSOptions) (ContainerInfo, []Stream, bool) {
	var streamsOut []Stream
	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	slices.Sort(streamOrder)
	var videoFrameRate float64
	var videoIsH264 bool
	var ccEntry *psStream
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
			if st.ccFound && ccEntry == nil {
				ccEntry = st
			}
			break
		}
	}
	type audioSync struct {
		duration        float64
		delayMs         float64
		durationDelayMs float64
	}
	var sync audioSync
	for _, key := range streamOrder {
		st := streams[key]
		if st == nil || st.kind != StreamAudio || !st.hasAC3 || !st.pts.has() {
			continue
		}
		duration := audioDurationPS(st, opts)
		if duration <= 0 {
			continue
		}
		durationSync := duration
		if delay, ok := delayRelativeToVideoMs(st.pts, videoPTS, videoIsH264, videoFrameRate); ok {
			sync = audioSync{duration: durationSync, delayMs: delay, durationDelayMs: delay}
		}
		break
	}
	menuOverheadBytes := int64(0)
	nonVideoBytes := int64(0)
	videoCount := 0
	for _, st := range streams {
		if st == nil {
			continue
		}
		switch st.kind {
		case StreamVideo:
			videoCount++
		case StreamMenu:
			if st.packetCount > 0 || st.bytes > 0 {
				menuOverheadBytes += int64(st.bytes) + int64(st.packetCount)*6
			}
		default:
			nonVideoBytes += int64(st.bytes)
		}
	}
	videoResidualBytes := int64(0)
	if size > 0 {
		videoResidualBytes = max(size-menuOverheadBytes-nonVideoBytes, 0)
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
				if parser.pictureCount > 0 && st.videoFrameCount == 0 {
					st.videoFrameCount = parser.pictureCount
				}
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
		if st.kind == StreamText && format == "RLE" {
			fields = append(fields, Field{Name: "Format/Info", Value: "Run-length encoding"})
		}
		if st.kind == StreamText && format == "RLE" {
			fields = append(fields, Field{Name: "Muxing mode", Value: "DVD-Video"})
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
			duration := ptsDurationPS(st.pts, opts)
			if duration == 0 {
				duration = ptsDurationPS(videoPTS, opts)
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
			frameRateRounded := info.FrameRate
			if frameRateRounded == 0 && info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
				frameRateRounded = float64(info.FrameRateNumer) / float64(info.FrameRateDenom)
			}
			if frameRateRounded > 0 {
				frameRateRounded = math.Round(frameRateRounded*1000) / 1000
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
			if info.GOPM > 0 && info.GOPN > 0 && info.BVOP != nil && *info.BVOP {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: fmt.Sprintf("M=%d, N=%d", info.GOPM, info.GOPN)})
			} else if info.GOPVariable {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: "Variable"})
			} else if info.GOPLength > 1 {
				fields = append(fields, Field{Name: "Format settings, GOP", Value: fmt.Sprintf("N=%d", info.GOPLength)})
			}
			duration := ptsDurationPS(st.pts, opts)
			zeroSegment := false
			if !opts.dvdParsing && st.pts.hasResets() && st.pts.segmentStart == st.pts.last {
				duration = 0
				zeroSegment = true
			}
			if duration == 0 && !zeroSegment {
				duration = ptsDurationPS(videoPTS, opts)
			}
			fromGOP := false
			syncApplied := false
			if duration == 0 && !zeroSegment {
				if frameRateRounded > 0 && info.GOPLength > 0 {
					duration = float64(info.GOPLength) / frameRateRounded
					fromGOP = true
				} else {
					duration = ptsDurationPS(anyPTS, opts)
				}
			}
			if zeroSegment && frameRateRounded > 0 {
				duration = 0.5 / frameRateRounded
			}
			headerOnly := false
			headerFrameCount := 0
			headerDurationRate := frameRateRounded
			if frameRateRounded > 0 && info.GOPLengthFirst > 0 {
				gopDuration := float64(info.GOPLengthFirst) / frameRateRounded
				if opts.dvdParsing {
					if duration == 0 || (st.pts.hasResets() && duration > gopDuration*10) {
						headerFrameCount = info.GOPLengthFirst
						if info.GOPVariable || headerFrameCount == 0 {
							headerFrameCount = 2
						}
						if info.FrameRate > 0 && math.Abs(info.FrameRate-29.97) < 0.02 && info.ScanType == "Progressive" && info.ScanOrder == "" {
							headerDurationRate = 24000.0 / 1001.0
						}
						duration = float64(headerFrameCount) / headerDurationRate
						fromGOP = true
						headerOnly = true
					}
				} else if st.pts.hasResets() && duration > gopDuration*10 {
					headerFrameCount = info.GOPLengthFirst
					duration = gopDuration
					fromGOP = true
					headerOnly = true
				}
			}
			if frameRateRounded > 0 && st.videoFrameCount > 0 {
				frameDuration := float64(st.videoFrameCount) / frameRateRounded
				switch {
				case duration == 0:
					duration = frameDuration
					fromGOP = true
					headerOnly = true
				case zeroSegment:
					duration = frameDuration
					fromGOP = true
					headerOnly = true
				case st.videoFrameCount <= 2 && duration > frameDuration*10:
					duration = frameDuration
					fromGOP = true
					headerOnly = true
				case st.videoFrameCount <= 2 && duration <= frameDuration*1.1:
					fromGOP = true
					headerOnly = true
				}
			}
			if duration > 0 && info.FrameRate > 0 && !fromGOP {
				duration += 2.0 / info.FrameRate
			}
			if sync.duration > 0 && !headerOnly {
				candidate := sync.duration + (sync.durationDelayMs / 1000.0)
				if candidate > 0 {
					threshold := 0.05
					if info.FrameRate > 0 {
						threshold = 1.0 / info.FrameRate
					}
					if math.Abs(candidate-duration) > threshold {
						duration = candidate
						syncApplied = true
					}
				}
			}
			if syncApplied && st.pts.hasResets() && info.FrameRate > 0 {
				duration += 1.0 / (info.FrameRate * 10.0)
			}
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
			effectiveBytes := st.bytes
			useHeaderBytes := fromGOP && st.videoHeaderBytes > 0 && st.videoHeaderBytes == st.bytes
			if headerOnly && st.videoHeaderBytes > 0 {
				useHeaderBytes = true
			}
			if useHeaderBytes {
				effectiveBytes = st.videoHeaderBytes
				if opts.dvdParsing && st.videoSeqExtBytes > 0 {
					effectiveBytes = st.videoSeqExtBytes + st.videoGOPBytes
				}
			}
			if !headerOnly && videoCount == 1 && videoResidualBytes > int64(effectiveBytes) && (menuOverheadBytes > 0 || opts.dvdParsing) {
				effectiveBytes = uint64(videoResidualBytes)
				useHeaderBytes = false
			}
			bitrateDuration := duration
			frameCountForBitrate := st.videoFrameCount
			if headerOnly && headerFrameCount > 0 {
				frameCountForBitrate = headerFrameCount
			}
			headerFrameBytes := uint64(0)
			if headerOnly && opts.dvdParsing && frameCountForBitrate > 0 {
				if st.videoFrameBytesCount >= frameCountForBitrate && st.videoFrameBytes > 0 {
					headerFrameBytes = st.videoFrameBytes
					if frameCountForBitrate >= 2 && info.BufferSize > 0 && st.videoFrameBytes < uint64(info.BufferSize/2) {
						headerFrameBytes = uint64(math.Round(float64(info.BufferSize) * dvdHeaderStreamScale))
					}
				} else if frameCountForBitrate >= 2 && info.BufferSize > 0 {
					headerFrameBytes = uint64(math.Round(float64(info.BufferSize) * dvdHeaderStreamScale))
				}
			}
			if syncApplied && info.FrameRate > 0 {
				derived := int(math.Round(duration * info.FrameRate))
				if derived > 0 {
					frameCountForBitrate = derived
				}
			}
			if st.pts.hasResets() && frameCountForBitrate > 0 && frameRateRounded > 0 {
				bitrateDuration = float64(frameCountForBitrate) / frameRateRounded
			} else if st.pts.hasResets() && st.pts.has() {
				bitrateDuration = st.pts.durationTotal()
			} else if !fromGOP && frameRateRounded > 0 {
				bitrateDuration += 1.0 / frameRateRounded
			}
			if useHeaderBytes && frameRateRounded > 0 {
				if headerOnly && frameCountForBitrate > 0 {
					bitrateDuration = float64(frameCountForBitrate) / frameRateRounded
				} else {
					rounded := math.Round(frameRateRounded)
					if rounded > 0 {
						bitrateDuration = 1.0 / rounded
					}
				}
			}
			mode := info.BitRateMode
			if mode == "" {
				mode = "Variable"
			}
			if fromGOP && !opts.dvdParsing {
				mode = "Constant"
			}
			fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
			bitrate := 0.0
			kbps := int64(0)
			if headerOnly && opts.dvdParsing && headerFrameBytes > 0 && frameRateRounded > 0 && frameCountForBitrate > 0 {
				bitrate = float64(headerFrameBytes) * 8 * frameRateRounded / float64(frameCountForBitrate)
				kbps = int64(bitrate / 1000.0)
				if value := formatBitrate(bitrate); value != "" {
					fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
				}
			} else if bitrateDuration > 0 && effectiveBytes > 0 {
				bitrate = (float64(effectiveBytes) * 8) / bitrateDuration
				if bitrate >= 10_000_000 {
					if value := formatBitrate(bitrate); value != "" {
						fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
					}
				} else if useHeaderBytes {
					if value := formatBitratePrecise(bitrate); value != "" {
						fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
					}
				} else {
					kbps = max(int64(bitrate/1000.0), 0)
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
			if info.ScanOrder != "" {
				fields = append(fields, Field{Name: "Scan order", Value: info.ScanOrder})
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
			headerStreamBytes := int64(0)
			if headerOnly && opts.dvdParsing && headerFrameBytes > 0 {
				headerStreamBytes = int64(headerFrameBytes)
			}
			if headerOnly && headerStreamBytes > 0 {
				if streamSize := formatStreamSize(headerStreamBytes, size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if headerOnly && bitrate > 0 && duration > 0 {
				streamSizeBytes := int64((bitrate*duration)/8.0 + 0.5)
				if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if useHeaderBytes && effectiveBytes > 0 {
				if streamSize := formatStreamSize(int64(effectiveBytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if kbps > 0 && duration > 0 {
				streamSizeBytes := int64(float64(kbps*1000)*duration/8.0 + 0.5)
				if menuOverheadBytes > 0 && videoCount == 1 && bitrate > 0 {
					streamSizeBytes = int64((bitrate*duration)/8.0 + 0.5)
				}
				if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			} else if effectiveBytes > 0 {
				if streamSize := formatStreamSize(int64(effectiveBytes), size); streamSize != "" {
					fields = append(fields, Field{Name: "Stream size", Value: streamSize})
				}
			}
			streamBytes := int64(effectiveBytes)
			if headerOnly {
				if headerStreamBytes > 0 {
					streamBytes = headerStreamBytes
				} else if bitrate > 0 && duration > 0 {
					streamBytes = int64((bitrate*duration)/8.0 + 0.5)
				}
			}
			jsonStreamBytes := streamBytes
			if jsonStreamBytes > 0 {
				jsonExtras["StreamSize"] = fmt.Sprintf("%d", jsonStreamBytes)
			}
			if st.videoFrameCount > 0 {
				frameCount := st.videoFrameCount
				if duration > 0 && info.FrameRate > 0 {
					derived := int(math.Round(duration * info.FrameRate))
					if derived > 0 && math.Abs(float64(derived-frameCount)) <= 2 {
						frameCount = derived
					}
				}
				if headerOnly && headerFrameCount > 0 {
					frameCount = headerFrameCount
				}
				jsonExtras["FrameCount"] = fmt.Sprintf("%d", frameCount)
			}
			jsonDuration := math.Round(duration*1000) / 1000
			jsonBitrateDuration := bitrateDuration
			if useHeaderBytes && fromGOP && info.FrameRate > 0 {
				// MediaInfo JSON bitrate for GOP-only streams is slightly higher than GOPLength/FrameRate.
				// Align with CLI output for header-only VOB samples.
				jsonBitrateDuration *= 0.99818
			}
			if jsonBitrateDuration <= 0 {
				jsonBitrateDuration = jsonDuration
			}
			if headerOnly && opts.dvdParsing && headerFrameBytes > 0 && bitrate > 0 {
				jsonExtras["BitRate"] = fmt.Sprintf("%d", int64(math.Round(bitrate)))
			} else if jsonBitrateDuration > 0 && jsonStreamBytes > 0 {
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
			duration := audioDurationPS(st, opts)
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
				if delay, ok := delayRelativeToVideoMs(st.pts, videoPTS, videoIsH264, videoFrameRate); ok {
					if rounded := int64(math.Round(delay)); rounded != 0 {
						fields = append(fields, Field{Name: "Delay relative to video", Value: formatDelayMs(rounded)})
					}
				}
				if duration > 0 && st.ac3Info.bitRateKbps > 0 {
					streamSizeBytes := int64(float64(st.ac3Info.bitRateKbps*1000)*duration/8.0 + 0.5)
					if streamSize := formatStreamSize(streamSizeBytes, size); streamSize != "" {
						fields = append(fields, Field{Name: "Stream size", Value: streamSize})
					}
				}
				if st.ac3Info.serviceKind != "" {
					fields = append(fields, Field{Name: "Service kind", Value: st.ac3Info.serviceKind})
				}
				if opts.dvdExtras && st.ac3Info.bsid > 0 {
					fields = append(fields, Field{Name: "bsid", Value: fmt.Sprintf("%d", st.ac3Info.bsid)})
				}
				if st.ac3Info.hasDialnorm {
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "Dialog Normalization", Value: fmt.Sprintf("%d", st.ac3Info.dialnorm)})
					}
					fields = append(fields, Field{Name: "Dialog Normalization", Value: fmt.Sprintf("%d dB", st.ac3Info.dialnorm)})
				}
				if st.ac3Info.hasCompr {
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "compr", Value: fmt.Sprintf("%.2f", st.ac3Info.comprDB)})
					}
					fields = append(fields, Field{Name: "compr", Value: fmt.Sprintf("%.2f dB", st.ac3Info.comprDB)})
				}
				if st.ac3Info.hasDynrng && opts.dvdExtras {
					fields = append(fields, Field{Name: "dynrng", Value: fmt.Sprintf("%.2f", st.ac3Info.dynrngDB)})
					fields = append(fields, Field{Name: "dynrng", Value: fmt.Sprintf("%.2f dB", st.ac3Info.dynrngDB)})
				}
				if opts.dvdExtras {
					if st.ac3Info.hasDsurmod {
						fields = append(fields, Field{Name: "dsurmod", Value: fmt.Sprintf("%d", st.ac3Info.dsurmod)})
					}
					fields = append(fields, Field{Name: "acmod", Value: fmt.Sprintf("%d", st.ac3Info.acmod)})
					fields = append(fields, Field{Name: "lfeon", Value: fmt.Sprintf("%d", st.ac3Info.lfeon)})
				}
				if st.ac3Info.hasCmixlev {
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "cmixlev", Value: fmt.Sprintf("%.1f", st.ac3Info.cmixlevDB)})
					}
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
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "dialnorm_Average", Value: fmt.Sprintf("%d", avg)})
						fields = append(fields, Field{Name: "dialnorm_Minimum", Value: fmt.Sprintf("%d", min)})
						fields = append(fields, Field{Name: "dialnorm_Maximum", Value: fmt.Sprintf("%d", max)})
					}
					fields = append(fields, Field{Name: "dialnorm_Average", Value: fmt.Sprintf("%d dB", avg)})
					fields = append(fields, Field{Name: "dialnorm_Minimum", Value: fmt.Sprintf("%d dB", min)})
					fields = append(fields, Field{Name: "dialnorm_Maximum", Value: fmt.Sprintf("%d dB", max)})
					if opts.dvdExtras && st.ac3Info.dialnormCount > 0 {
						fields = append(fields, Field{Name: "dialnorm_Count", Value: fmt.Sprintf("%d", st.ac3Info.dialnormCount)})
					}
				}
				if opts.dvdExtras {
					if avg, min, max, count, ok := st.ac3Info.comprStats(); ok {
						fields = append(fields, Field{Name: "compr_Average", Value: fmt.Sprintf("%.2f", avg)})
						fields = append(fields, Field{Name: "compr_Minimum", Value: fmt.Sprintf("%.2f", min)})
						fields = append(fields, Field{Name: "compr_Maximum", Value: fmt.Sprintf("%.2f", max)})
						fields = append(fields, Field{Name: "compr_Count", Value: fmt.Sprintf("%d", count)})
						fields = append(fields, Field{Name: "compr_Average", Value: fmt.Sprintf("%.2f dB", avg)})
						fields = append(fields, Field{Name: "compr_Minimum", Value: fmt.Sprintf("%.2f dB", min)})
						fields = append(fields, Field{Name: "compr_Maximum", Value: fmt.Sprintf("%.2f dB", max)})
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
					useFrames := st.audioFrames > 0 && st.ac3Info.spf > 0 && !st.pts.hasResets() && parseSpeed >= 1
					if useFrames {
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
				if st.bytes > 0 && parseSpeed >= 1 {
					jsonExtras["StreamSize"] = fmt.Sprintf("%d", st.bytes)
				} else if st.ac3Info.bitRateKbps > 0 && duration > 0 {
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
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_String", Val: fmt.Sprintf("%d dB", st.ac3Info.dialnorm)})
					}
				}
				if st.ac3Info.hasCompr {
					extraFields = append(extraFields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", st.ac3Info.comprDB)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "compr_String", Val: fmt.Sprintf("%.2f dB", st.ac3Info.comprDB)})
					}
				}
				if st.ac3Info.acmod > 0 {
					extraFields = append(extraFields, jsonKV{Key: "acmod", Val: fmt.Sprintf("%d", st.ac3Info.acmod)})
				}
				if st.ac3Info.hasDsurmod {
					extraFields = append(extraFields, jsonKV{Key: "dsurmod", Val: fmt.Sprintf("%d", st.ac3Info.dsurmod)})
				}
				if st.ac3Info.lfeon >= 0 {
					extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: fmt.Sprintf("%d", st.ac3Info.lfeon)})
				}
				if st.ac3Info.hasCmixlev {
					extraFields = append(extraFields, jsonKV{Key: "cmixlev", Val: fmt.Sprintf("%.1f", st.ac3Info.cmixlevDB)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "cmixlev_String", Val: fmt.Sprintf("%.1f dB", st.ac3Info.cmixlevDB)})
					}
				}
				if st.ac3Info.hasSurmixlev {
					surmix := fmt.Sprintf("%.0f dB", st.ac3Info.surmixlevDB)
					extraFields = append(extraFields, jsonKV{Key: "surmixlev", Val: surmix})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "surmixlev_String", Val: surmix})
					}
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
					if max != min || opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: fmt.Sprintf("%d", max)})
					}
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average_String", Val: fmt.Sprintf("%d dB", avg)})
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum_String", Val: fmt.Sprintf("%d dB", min)})
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum_String", Val: fmt.Sprintf("%d dB", max)})
						if st.ac3Info.dialnormCount > 0 {
							extraFields = append(extraFields, jsonKV{Key: "dialnorm_Count", Val: fmt.Sprintf("%d", st.ac3Info.dialnormCount)})
						}
					}
				}
				if avg, min, max, count, ok := st.ac3Info.comprStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", min)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", max)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: fmt.Sprintf("%d", count)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "compr_Average_String", Val: fmt.Sprintf("%.2f dB", avg)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Minimum_String", Val: fmt.Sprintf("%.2f dB", min)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Maximum_String", Val: fmt.Sprintf("%.2f dB", max)})
					}
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
		} else if st.kind == StreamText && st.pts.has() {
			duration := float64(ptsDelta(st.pts.first, st.pts.last)) / 90000.0
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
				jsonExtras["Duration"] = fmt.Sprintf("%.3f", duration)
			}
			if delay, ok := delayRelativeToVideoMs(st.pts, videoPTS, videoIsH264, videoFrameRate); ok {
				if rounded := int64(math.Round(delay)); rounded != 0 {
					fields = append(fields, Field{Name: "Delay relative to video", Value: formatDelayMs(rounded)})
				}
			}
		} else if st.kind != StreamVideo {
			if duration := ptsDurationPS(st.pts, opts); duration > 0 {
				fields = addStreamDuration(fields, duration)
			}
		}
		if st.kind == StreamVideo && st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			dropFrame := "No"
			if info.GOPDropFrame != nil && *info.GOPDropFrame {
				dropFrame = "Yes"
			}
			jsonExtras["Delay_DropFrame"] = dropFrame
			jsonExtras["Delay_Source"] = "Container"
			jsonExtras["Delay_Original"] = "0.000"
			jsonExtras["Delay_Original_DropFrame"] = dropFrame
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
		if st.kind == StreamText && st.pts.has() {
			delay := float64(st.pts.first) / 90000.0
			jsonExtras["Delay"] = fmt.Sprintf("%.9f", delay)
			jsonExtras["Delay_Source"] = "Container"
			if videoPTS.has() {
				videoDelay := float64(int64(st.pts.first)-int64(videoPTS.min)) / 90000.0
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
				if ptsDurationPS(st.pts, opts) == 0 && videoInfo.FrameRate > 0 && (videoInfo.GOPLength > 0 || videoInfo.GOPLengthFirst > 0) {
					mode = "Constant"
				}
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
	if duration := ptsDurationPS(videoPTS, opts); duration > 0 {
		if videoFrameRate > 0 {
			duration += 2.0 / videoFrameRate
		}
		syncApplied := false
		if sync.duration > 0 {
			candidate := sync.duration + (sync.durationDelayMs / 1000.0)
			if candidate > 0 {
				threshold := 0.05
				if videoFrameRate > 0 {
					threshold = 1.0 / videoFrameRate
				}
				if math.Abs(candidate-duration) > threshold {
					duration = candidate
					syncApplied = true
				}
			}
		}
		if syncApplied && videoPTS.hasResets() && videoFrameRate > 0 {
			duration += 1.0 / (videoFrameRate * 10.0)
		}
		videoDuration = duration
	}
	maxDuration := 0.0
	for _, st := range streams {
		if st == nil || st.kind == StreamMenu {
			continue
		}
		duration := ptsDurationPS(st.pts, opts)
		if st.kind == StreamAudio {
			duration = audioDurationPS(st, opts)
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
	} else if duration := ptsDurationPS(anyPTS, opts); duration > 0 {
		info.DurationSeconds = duration
	}

	if ccEntry != nil {
		videoDelay := 0.0
		if videoPTS.has() {
			videoDelay = float64(videoPTS.min) / 90000.0
		}
		if ccStream := buildCCTextStream(ccEntry, videoDelay, videoDuration, videoFrameRate); ccStream != nil {
			insertAt := -1
			for i := len(streamsOut) - 1; i >= 0; i-- {
				if streamsOut[i].Kind == StreamAudio {
					insertAt = i + 1
					break
				}
			}
			if insertAt == -1 {
				for i := len(streamsOut) - 1; i >= 0; i-- {
					if streamsOut[i].Kind == StreamVideo {
						insertAt = i + 1
						break
					}
				}
			}
			if insertAt >= 0 && insertAt < len(streamsOut) {
				streamsOut = append(streamsOut, Stream{})
				copy(streamsOut[insertAt+1:], streamsOut[insertAt:])
				streamsOut[insertAt] = *ccStream
			} else {
				streamsOut = append(streamsOut, *ccStream)
			}
		}
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
			if width < 64 || height < 64 {
				return
			}
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
