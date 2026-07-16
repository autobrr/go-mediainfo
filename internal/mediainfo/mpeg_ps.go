package mediainfo

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
)

const psSubstreamNone = 0xFF
const dvdHeaderStreamScale = 1.13503156

// psStreamKey combines a PES stream ID and private substream ID into a stable
// parser-state key.
func psStreamKey(id, subID byte) uint16 {
	return uint16(id)<<8 | uint16(subID)
}

// ParseMPEGPS parses an MPEG program stream with default analysis options.
func ParseMPEGPS(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, bool) {
	return ParseMPEGPSWithOptions(file, size, mpegPSOptions{})
}

// ParseMPEGPSWithOptions parses an MPEG program stream using bounded sampling
// and DVD-specific options.
func ParseMPEGPSWithOptions(file io.ReadSeeker, size int64, opts mpegPSOptions) (ContainerInfo, []Stream, bool) {
	parseSpeed := opts.parseSpeed
	if parseSpeed == 0 {
		parseSpeed = 1
	}
	if parseSpeed < 1 && size > 0 {
		if readerAt, ok := file.(io.ReaderAt); ok {
			parser := newPSStreamParser(opts)
			reader := func(r io.Reader) bool {
				buf := bufio.NewReaderSize(r, 1<<20)
				return parser.parseReader(buf)
			}
			sampleSize := int64(8 << 20)
			if parseSpeed > 0 && parseSpeed < 1 {
				sampleSize = max(int64(float64(sampleSize)*parseSpeed), 4<<20)
			}
			if opts.dvdParsing && sampleSize < 8<<20 {
				sampleSize = 8 << 20
			}
			opts2 := opts
			opts2.sampled = size > sampleSize
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
					tailSample := sampleSize
					if opts.dvdParsing && parseSpeed < 1 {
						tailSample = min(tailSample, int64(8<<20))
					}
					mid := (size - sampleSize) / 2
					midSample := sampleSize
					if opts.dvdParsing && parseSpeed < 1 {
						midSample = min(midSample, int64(4<<20))
						mid = (size - midSample) / 2
					}
					if !opts.dvdParsing {
						middle := io.NewSectionReader(readerAt, mid, midSample)
						if reader(middle) {
							parsedAny = true
						}
					}
					start := size - tailSample
					last := io.NewSectionReader(readerAt, start, tailSample)
					if reader(last) {
						parsedAny = true
					}
				}
			}
			if parsedAny {
				return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts2)
			}
		}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, false
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	parser := newPSStreamParser(opts)
	if !parser.parseReader(reader) {
		return ContainerInfo{}, nil, false
	}
	return finalizeMPEGPS(parser.streams, parser.streamOrder, parser.videoParsers, parser.videoPTS, parser.anyPTS, size, opts)
}

// mpegPSOptions controls bounded program-stream parsing and DVD-specific
// canonical metadata.
type mpegPSOptions struct {
	dvdExtras  bool
	dvdParsing bool
	parseSpeed float64
	sampled    bool
}

// ptsDurationPS selects a positive PTS duration, using sampled-span accounting
// when bounded program-stream parsing skipped file regions.
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
		// For plain MPEG-PS/VOB parsing, MediaInfo effectively uses a min/max span even when
		// there are discontinuities (DVD cell boundaries can cause large PTS jumps).
		return tracker.duration()
	}
	return tracker.duration()
}

// audioDurationPS selects the best program-stream audio duration from PTS,
// sample counts, or complete AC-3 frames according to sampling mode.
func audioDurationPS(st *psStream, opts mpegPSOptions) float64 {
	if st == nil {
		return 0
	}
	duration := ptsDurationPS(st.pts, opts)
	if st.audioProfile != "" {
		if value := aacDurationPS(st); value > 0 {
			duration = value
		}
	} else if st.mpegAudioLayer != 0 && st.audioRate > 0 && st.audioFrames > 0 {
		rate := st.audioRate
		spf := mpegAudioSamplesPerFrame(st.mpegAudioVersion, st.mpegAudioLayer)
		if rate > 0 && spf > 0 {
			derived := (float64(st.audioFrames) * float64(spf)) / rate
			threshold := (float64(spf) / rate) / 2.0
			if duration == 0 || math.Abs(derived-duration) > threshold {
				duration = derived
			}
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
	if st.hasAC3 && st.ac3Info.sampleRate > 0 && st.ac3Info.spf > 0 && st.audioFrames > 0 {
		derived := float64(st.audioFrames*uint64(st.ac3Info.spf)) / st.ac3Info.sampleRate
		frameDur := float64(st.ac3Info.spf) / st.ac3Info.sampleRate
		if !opts.sampled {
			duration = derived
		} else if duration == 0 || math.Abs(derived-duration) <= frameDur {
			duration = derived
		}
	}
	return duration
}

// delayRelativeToVideoMs returns the first audio PTS relative to video in
// milliseconds, including the AVC presentation adjustment when applicable.
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

// finalizeMPEGPS converts accumulated program-stream parser state into
// canonical container and stream results.
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
		case StreamGeneral, StreamAudio, StreamText, StreamImage:
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
		facts := &mpegPSStructuredFacts{}
		builder := newCanonicalStreamBuilder(st.kind)
		var extraNode *structuredNode
		if st.firstPacketOrder >= 0 && st.kind != StreamMenu {
			facts.Set("FirstPacketOrder", strconv.Itoa(st.firstPacketOrder))
		}
		if st.kind != StreamMenu {
			if st.subID != psSubstreamNone {
				facts.Set("ID", fmt.Sprintf("%d-%d", st.id, st.subID))
			} else {
				facts.Set("ID", strconv.FormatUint(uint64(st.id), 10))
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
			builder.Fill("ID", facts.Legacy("ID"), "ID", idValue)
		}
		format := st.format
		if st.kind == StreamAudio && st.audioProfile != "" {
			format = "AAC " + st.audioProfile
		}
		if format != "" {
			fields = append(fields, Field{Name: "Format", Value: format})
			rawFormat, additional := splitAACFormat(format)
			builder.Fill("Format", rawFormat, "Format", format)
			builder.DirectStructured("Format_AdditionalFeatures", additional)
		}
		if st.kind == StreamText && format == "RLE" {
			fields = append(fields, Field{Name: "Format/Info", Value: "Run-length encoding"})
		}
		if st.kind == StreamText && format == "RLE" {
			fields = append(fields, Field{Name: "Muxing mode", Value: "DVD-Video"})
			builder.Fill("MuxingMode", "DVD-Video", "Muxing mode", "DVD-Video")
		}
		if st.kind == StreamAudio {
			switch {
			case format == "AC-3":
				if info := mapMatroskaFormatInfo(st.format); info != "" {
					fields = append(fields, Field{Name: "Format/Info", Value: info})
				}
				fields = append(fields, Field{Name: "Commercial name", Value: "Dolby Digital"})
				fields = append(fields, Field{Name: "Muxing mode", Value: "DVD-Video"})
				builder.Fill("Format_Commercial_IfAny", "Dolby Digital", "Commercial name", "Dolby Digital")
				builder.Fill("MuxingMode", "DVD-Video", "Muxing mode", "DVD-Video")
			case st.audioProfile == "LC":
				fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
				version := formatAACVersion(st.audioMPEGVersion)
				fields = append(fields, Field{Name: "Format version", Value: version})
				builder.Fill("Format_Version", extractVersionNumber(version), "Format version", version)
				fields = append(fields, Field{Name: "Muxing mode", Value: "ADTS"})
				builder.Fill("MuxingMode", "ADTS", "Muxing mode", "ADTS")
				if st.audioObject > 0 {
					codecID := strconv.Itoa(st.audioObject)
					fields = append(fields, Field{Name: "Codec ID", Value: codecID})
					builder.Fill("CodecID", codecID, "Codec ID", codecID)
				}
			default:
				if info := mapMatroskaFormatInfo(st.format); info != "" {
					fields = append(fields, Field{Name: "Format/Info", Value: info})
				}
			}
		}
		switch st.kind {
		case StreamVideo:
			if st.videoIsH264 {
				if info := mapMatroskaFormatInfo(st.format); info != "" {
					fields = append(fields, Field{Name: "Format/Info", Value: info})
				}
				if len(st.videoFields) > 0 {
					fields = append(fields, st.videoFields...)
				}
				fillCanonicalMPEGPSH264(builder, st)
				if st.videoSliceCount > 0 {
					fields = append(fields, Field{Name: "Format settings, Slice count", Value: fmt.Sprintf("%d slices per frame", st.videoSliceCount)})
				}
				duration := ptsDurationPS(st.pts, opts)
				if duration == 0 {
					duration = ptsDurationPS(videoPTS, opts)
				}
				if duration > 0 {
					if st.videoFrameRate > 0 {
						duration += 1.0 / st.videoFrameRate
					}
					fields = addStreamDuration(fields, duration)
					builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
				}
				mode := "Constant"
				fields = append(fields, Field{Name: "Bit rate mode", Value: mode})
				builder.Fill("BitRate_Mode", mode, "Bit rate mode", mode)
				bitrate := 0.0
				if duration > 0 && st.bytes > 0 {
					bitrate = (float64(st.bytes) * 8) / duration
					if value := formatBitrate(bitrate); value != "" {
						fields = append(fields, Field{Name: "Nominal bit rate", Value: value})
						builder.Fill("BitRate_Nominal", roundedMPEGPSBitRate(bitrate, false), "Nominal bit rate", value)
					}
				}
				width := st.videoWidth
				height := st.videoHeight
				if width > 0 {
					fields = append(fields, Field{Name: "Width", Value: formatPixels(width)})
					builder.Fill("Width", strconv.FormatUint(width, 10), "Width", formatPixels(width))
				}
				if height > 0 {
					fields = append(fields, Field{Name: "Height", Value: formatPixels(height)})
					builder.Fill("Height", strconv.FormatUint(height, 10), "Height", formatPixels(height))
				}
				if ar := formatAspectRatio(width, height); ar != "" {
					fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
					if ratio, ok := parseRatioFloat(ar); ok {
						builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", ar)
					}
				}
				if st.videoFrameRate > 0 {
					fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(st.videoFrameRate)})
					builder.Fill("FrameRate", formatJSONFloat(st.videoFrameRate), "Frame rate", formatFrameRate(st.videoFrameRate))
				}
				fields = append(fields, Field{Name: "Color space", Value: "YUV"})
				builder.Fill("ColorSpace", "YUV", "Color space", "YUV")
				if bitrate > 0 && width > 0 && height > 0 && st.videoFrameRate > 0 {
					bits := bitrate / (float64(width) * float64(height) * st.videoFrameRate)
					fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: fmt.Sprintf("%.3f", bits)})
				}
			} else {
				if info.Version != "" {
					fields = append(fields, Field{Name: "Format version", Value: info.Version})
					builder.Fill("Format_Version", extractVersionNumber(info.Version), "Format version", info.Version)
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
					profile, level := splitProfileLevel(info.Profile)
					builder.Fill("Format_Profile", profile, "Format profile", info.Profile)
					builder.DirectStructured("Format_Level", level)
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
					value := formatYesNo(*info.BVOP)
					fields = append(fields, Field{Name: "Format settings, BVOP", Value: value})
					builder.Fill("Format_Settings_BVOP", value, "Format settings, BVOP", value)
				}
				if info.Matrix != "" {
					fields = append(fields, Field{Name: "Format settings, Matrix", Value: info.Matrix})
					builder.Fill("Format_Settings_Matrix", info.Matrix, "Format settings, Matrix", info.Matrix)
				}
				switch {
				case info.GOPM > 0 && info.GOPN > 0 && info.BVOP != nil && *info.BVOP:
					value := fmt.Sprintf("M=%d, N=%d", info.GOPM, info.GOPN)
					fields = append(fields, Field{Name: "Format settings, GOP", Value: value})
					builder.Fill("Format_Settings_GOP", value, "Format settings, GOP", value)
				case info.GOPLength > 1:
					value := fmt.Sprintf("N=%d", info.GOPLength)
					fields = append(fields, Field{Name: "Format settings, GOP", Value: value})
					builder.Fill("Format_Settings_GOP", value, "Format settings, GOP", value)
				case info.GOPVariable:
					fields = append(fields, Field{Name: "Format settings, GOP", Value: "Variable"})
					builder.Fill("Format_Settings_GOP", "Variable", "Format settings, GOP", "Variable")
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
				durationFromFrames := false
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
						durationFromFrames = true
						headerOnly = true
					case zeroSegment:
						duration = frameDuration
						durationFromFrames = true
						headerOnly = true
					case st.videoFrameCount <= 2 && duration > frameDuration*10:
						duration = frameDuration
						durationFromFrames = true
						headerOnly = true
					case st.videoFrameCount <= 2 && duration <= frameDuration*1.1:
						durationFromFrames = true
						headerOnly = true
					}
					if st.videoFrameCount > 1000 {
						// For long MPEG-PS video, MediaInfo aligns duration to frame count when available.
						// This also stabilizes overall bitrate computations for DVD VOBs.
						duration = frameDuration
						durationFromFrames = true
					}
				}
				if duration > 0 && frameRateRounded > 0 && !fromGOP && !durationFromFrames {
					duration += 1.0 / frameRateRounded
				}
				if sync.duration > 0 && !headerOnly && !durationFromFrames {
					candidate := sync.duration + (sync.durationDelayMs / 1000.0)
					if candidate > 0 {
						threshold := 0.05
						if frameRateRounded > 0 {
							threshold = 1.0 / frameRateRounded
						}
						if math.Abs(candidate-duration) > threshold {
							duration = candidate
							syncApplied = true
						}
					}
				}
				if syncApplied && st.pts.hasResets() && frameRateRounded > 0 {
					duration += 1.0 / (frameRateRounded * 10.0)
				}
				if duration > 0 {
					st.derivedDuration = duration
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
				switch {
				case st.pts.hasResets() && frameCountForBitrate > 0 && frameRateRounded > 0:
					bitrateDuration = float64(frameCountForBitrate) / frameRateRounded
				case st.pts.hasResets() && st.pts.has():
					bitrateDuration = st.pts.durationTotal()
				case !fromGOP && frameRateRounded > 0:
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
				builder.Fill("BitRate_Mode", mode, "Bit rate mode", mode)
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
					switch {
					case bitrate >= 10_000_000:
						if value := formatBitrate(bitrate); value != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
						}
					case useHeaderBytes:
						if value := formatBitratePrecise(bitrate); value != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
						}
					default:
						kbps = max(int64(bitrate/1000.0), 0)
						if value := formatBitrateKbps(kbps); value != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bit rate", Value: value})
						}
					}
				}
				if info.MaxBitRateKbps > 0 {
					if value := formatBitrateKbps(info.MaxBitRateKbps); value != "" {
						fields = append(fields, Field{Name: "Maximum bit rate", Value: value})
						builder.Fill("BitRate_Maximum", strconv.FormatInt(info.MaxBitRateKbps*1000, 10), "Maximum bit rate", value)
					}
				}
				if info.Width > 0 {
					fields = append(fields, Field{Name: "Width", Value: formatPixels(info.Width)})
					builder.Fill("Width", strconv.FormatUint(info.Width, 10), "Width", formatPixels(info.Width))
				}
				if info.Height > 0 {
					fields = append(fields, Field{Name: "Height", Value: formatPixels(info.Height)})
					builder.Fill("Height", strconv.FormatUint(info.Height, 10), "Height", formatPixels(info.Height))
				}
				if info.AspectRatio != "" {
					fields = append(fields, Field{Name: "Display aspect ratio", Value: info.AspectRatio})
					if ratio, ok := parseRatioFloat(info.AspectRatio); ok {
						builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", info.AspectRatio)
					}
				} else if info.Width > 0 && info.Height > 0 {
					// Square pixels: MediaInfo reports display aspect ratio derived from stored dimensions.
					if ar := formatAspectRatio(info.Width, info.Height); ar != "" {
						fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
						if ratio, ok := parseRatioFloat(ar); ok {
							builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", ar)
						}
					}
				}
				if info.FrameRateNumer > 0 && info.FrameRateDenom > 0 {
					display := formatFrameRateRatio(info.FrameRateNumer, info.FrameRateDenom)
					fields = append(fields, Field{Name: "Frame rate", Value: display})
					builder.Fill("FrameRate", formatJSONFloat(info.FrameRate), "Frame rate", display)
					builder.DirectStructured("FrameRate_Num", strconv.FormatUint(uint64(info.FrameRateNumer), 10))
					builder.DirectStructured("FrameRate_Den", strconv.FormatUint(uint64(info.FrameRateDenom), 10))
				} else if info.FrameRate > 0 {
					fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(info.FrameRate)})
					builder.Fill("FrameRate", formatJSONFloat(info.FrameRate), "Frame rate", formatFrameRate(info.FrameRate))
				}
				if standard := mapMPEG2Standard(info.FrameRate); standard != "" {
					fields = append(fields, Field{Name: "Standard", Value: standard})
					builder.Fill("Standard", standard, "Standard", standard)
				}
				if info.ColorSpace != "" {
					fields = append(fields, Field{Name: "Color space", Value: info.ColorSpace})
					builder.Fill("ColorSpace", info.ColorSpace, "Color space", info.ColorSpace)
				}
				if info.ChromaSubsampling != "" {
					fields = append(fields, Field{Name: "Chroma subsampling", Value: info.ChromaSubsampling})
					builder.Fill("ChromaSubsampling", info.ChromaSubsampling, "Chroma subsampling", info.ChromaSubsampling)
				}
				if info.BitDepth != "" {
					fields = append(fields, Field{Name: "Bit depth", Value: info.BitDepth})
					builder.Fill("BitDepth", extractLeadingNumber(info.BitDepth), "Bit depth", info.BitDepth)
				}
				if info.ScanType != "" {
					fields = append(fields, Field{Name: "Scan type", Value: info.ScanType})
					builder.Fill("ScanType", info.ScanType, "Scan type", info.ScanType)
				}
				if info.ScanOrder != "" {
					fields = append(fields, Field{Name: "Scan order", Value: info.ScanOrder})
					builder.Fill("ScanOrder", info.ScanOrder, "Scan order", info.ScanOrder)
				}
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
				if bitrate > 0 && info.Width > 0 && info.Height > 0 {
					if bits := formatBitsPerPixelFrame(bitrate, info.Width, info.Height, info.FrameRate); bits != "" {
						fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
					}
				}
				if info.TimeCode != "" {
					fields = append(fields, Field{Name: "Time code of first frame", Value: info.TimeCode})
					builder.Fill("TimeCode_FirstFrame", info.TimeCode, "Time code of first frame", info.TimeCode)
				}
				if info.TimeCodeSource != "" {
					fields = append(fields, Field{Name: "Time code source", Value: info.TimeCodeSource})
					builder.Fill("TimeCode_Source", info.TimeCodeSource, "Time code source", info.TimeCodeSource)
				}
				if info.GOPLength > 1 {
					if info.GOPOpenClosed != "" {
						fields = append(fields, Field{Name: "GOP, Open/Closed", Value: info.GOPOpenClosed})
						builder.Fill("Gop_OpenClosed", info.GOPOpenClosed, "GOP, Open/Closed", info.GOPOpenClosed)
					}
					if info.GOPFirstClosed != "" && info.GOPFirstClosed != info.GOPOpenClosed {
						fields = append(fields, Field{Name: "GOP, Open/Closed of first frame", Value: info.GOPFirstClosed})
						builder.Fill("Gop_OpenClosed_FirstFrame", info.GOPFirstClosed, "GOP, Open/Closed of first frame", info.GOPFirstClosed)
					}
				}
				headerStreamBytes := int64(0)
				if headerOnly && opts.dvdParsing && headerFrameBytes > 0 {
					headerStreamBytes = int64(headerFrameBytes)
				}
				// Prefer bitrate-derived StreamSize for MPEG-PS/VOB parity (official JSON uses it),
				// and fall back to observed bytes when bitrate isn't available.
				chosenStreamBytes := int64(0)
				switch {
				case headerOnly && headerStreamBytes > 0:
					chosenStreamBytes = headerStreamBytes
				case headerOnly && bitrate > 0 && duration > 0:
					chosenStreamBytes = int64((bitrate*duration)/8.0 + 0.5)
				case bitrate > 0 && duration > 0:
					chosenStreamBytes = int64((bitrate*duration)/8.0 + 0.5)
				case effectiveBytes > 0:
					chosenStreamBytes = int64(effectiveBytes)
				}
				if chosenStreamBytes > 0 {
					if streamSize := formatStreamSize(chosenStreamBytes, size); streamSize != "" {
						fields = append(fields, Field{Name: "Stream size", Value: streamSize})
					}
					facts.Set("StreamSize", strconv.FormatInt(chosenStreamBytes, 10))
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
					facts.Set("FrameCount", strconv.Itoa(frameCount))
				}
				jsonDuration := math.Round(duration*1000) / 1000
				if jsonDuration > 0 {
					facts.Set("Duration", formatJSONSeconds(jsonDuration))
				}
				jsonBitrateDuration := bitrateDuration
				if useHeaderBytes && fromGOP && info.FrameRate > 0 {
					// MediaInfo JSON bitrate for GOP-only streams is slightly higher than GOPLength/FrameRate.
					// Align with CLI output for header-only VOB samples.
					jsonBitrateDuration *= 0.99818
				}
				if jsonBitrateDuration <= 0 {
					jsonBitrateDuration = jsonDuration
				}
				switch {
				case headerOnly && opts.dvdParsing && headerFrameBytes > 0 && bitrate > 0:
					facts.Set("BitRate", strconv.FormatInt(int64(math.Round(bitrate)), 10))
				case jsonBitrateDuration > 0 && chosenStreamBytes > 0:
					jsonBitrate := (float64(chosenStreamBytes) * 8) / jsonBitrateDuration
					facts.Set("BitRate", strconv.FormatInt(int64(math.Round(jsonBitrate)), 10))
				case bitrate > 0:
					facts.Set("BitRate", strconv.FormatInt(int64(math.Round(bitrate)), 10))
				}
				if info.MatrixData != "" {
					facts.Set("Format_Settings_Matrix_Data", info.MatrixData)
				}
				if info.BufferSize > 0 {
					facts.Set("BufferSize", strconv.FormatInt(info.BufferSize, 10))
				}
				if info.IntraDCPrecision > 0 {
					extraFields := []jsonKV{{Key: "intra_dc_precision", Val: strconv.Itoa(info.IntraDCPrecision)}}
					node := structuredObjectFromKVs(extraFields)
					extraNode = &node
				}
				if info.ColourDescriptionPresent {
					facts.Set("colour_description_present", "Yes")
					facts.Set("colour_description_present_Source", "Stream")
					if info.ColourPrimaries != "" {
						facts.Set("colour_primaries", info.ColourPrimaries)
						facts.Set("colour_primaries_Source", "Stream")
					}
					if info.TransferCharacteristics != "" {
						facts.Set("transfer_characteristics", info.TransferCharacteristics)
						facts.Set("transfer_characteristics_Source", "Stream")
					}
					if info.MatrixCoefficients != "" {
						facts.Set("matrix_coefficients", info.MatrixCoefficients)
						facts.Set("matrix_coefficients_Source", "Stream")
					}
				}
			}
		case StreamAudio:
			duration := audioDurationPS(st, opts)
			if duration > 0 {
				fields = addStreamDuration(fields, duration)
				builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
			}
			if st.hasAC3 {
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
				builder.Fill("BitRate_Mode", "Constant", "Bit rate mode", "Constant")
				if st.ac3Info.bitRateKbps > 0 {
					display := formatBitrateKbps(st.ac3Info.bitRateKbps)
					fields = append(fields, Field{Name: "Bit rate", Value: display})
					builder.Fill("BitRate", strconv.FormatInt(st.ac3Info.bitRateKbps*1000, 10), "Bit rate", display)
				}
				if st.ac3Info.channels > 0 {
					fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.ac3Info.channels)})
					builder.Fill("Channels", strconv.FormatUint(st.ac3Info.channels, 10), "Channel(s)", formatChannels(st.ac3Info.channels))
					builder.DirectStructured("ChannelPositions", channelPositionsFromCount(strconv.FormatUint(st.ac3Info.channels, 10)))
				}
				if st.ac3Info.layout != "" {
					fields = append(fields, Field{Name: "Channel layout", Value: st.ac3Info.layout})
					builder.Fill("ChannelLayout", st.ac3Info.layout, "Channel layout", st.ac3Info.layout)
				}
				if st.ac3Info.sampleRate > 0 {
					fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.ac3Info.sampleRate)})
					builder.Fill("SamplingRate", strconv.FormatInt(int64(math.Round(st.ac3Info.sampleRate)), 10), "Sampling rate", formatSampleRate(st.ac3Info.sampleRate))
				}
				if value := formatAudioFrameRate(st.ac3Info.frameRate, st.ac3Info.spf); value != "" {
					fields = append(fields, Field{Name: "Frame rate", Value: value})
					builder.Fill("FrameRate", formatJSONFloat(st.ac3Info.frameRate), "Frame rate", value)
				}
				fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
				builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
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
					fields = append(fields, Field{Name: "bsid", Value: strconv.Itoa(st.ac3Info.bsid)})
				}
				if st.ac3Info.hasDialnorm {
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "Dialog Normalization", Value: strconv.Itoa(st.ac3Info.dialnorm)})
					}
					fields = append(fields, Field{Name: "Dialog Normalization", Value: strconv.Itoa(st.ac3Info.dialnorm) + " dB"})
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
						fields = append(fields, Field{Name: "dsurmod", Value: strconv.Itoa(st.ac3Info.dsurmod)})
					}
					fields = append(fields, Field{Name: "acmod", Value: strconv.Itoa(st.ac3Info.acmod)})
					fields = append(fields, Field{Name: "lfeon", Value: strconv.Itoa(st.ac3Info.lfeon)})
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
					fields = append(fields, Field{Name: "mixlevel", Value: strconv.Itoa(st.ac3Info.mixlevel) + " dB"})
				}
				if st.ac3Info.hasRoomtyp {
					fields = append(fields, Field{Name: "roomtyp", Value: st.ac3Info.roomtyp})
				}
				if avg, minVal, maxVal, ok := st.ac3Info.dialnormStats(); ok {
					if opts.dvdExtras {
						fields = append(fields, Field{Name: "dialnorm_Average", Value: strconv.Itoa(avg)})
						fields = append(fields, Field{Name: "dialnorm_Minimum", Value: strconv.Itoa(minVal)})
						fields = append(fields, Field{Name: "dialnorm_Maximum", Value: strconv.Itoa(maxVal)})
					}
					fields = append(fields, Field{Name: "dialnorm_Average", Value: strconv.Itoa(avg) + " dB"})
					fields = append(fields, Field{Name: "dialnorm_Minimum", Value: strconv.Itoa(minVal) + " dB"})
					fields = append(fields, Field{Name: "dialnorm_Maximum", Value: strconv.Itoa(maxVal) + " dB"})
					if opts.dvdExtras && st.ac3Info.dialnormCount > 0 {
						fields = append(fields, Field{Name: "dialnorm_Count", Value: strconv.Itoa(st.ac3Info.dialnormCount)})
					}
				}
				if opts.dvdExtras {
					if avg, minVal, maxVal, count, ok := st.ac3Info.comprStats(); ok {
						fields = append(fields, Field{Name: "compr_Average", Value: fmt.Sprintf("%.2f", avg)})
						fields = append(fields, Field{Name: "compr_Minimum", Value: fmt.Sprintf("%.2f", minVal)})
						fields = append(fields, Field{Name: "compr_Maximum", Value: fmt.Sprintf("%.2f", maxVal)})
						fields = append(fields, Field{Name: "compr_Count", Value: strconv.Itoa(count)})
						fields = append(fields, Field{Name: "compr_Average", Value: fmt.Sprintf("%.2f dB", avg)})
						fields = append(fields, Field{Name: "compr_Minimum", Value: fmt.Sprintf("%.2f dB", minVal)})
						fields = append(fields, Field{Name: "compr_Maximum", Value: fmt.Sprintf("%.2f dB", maxVal)})
					}
				}
			} else if st.audioRate > 0 {
				if st.format == "MPEG Audio" && st.mpegAudioLayer != 0 {
					spf := mpegAudioSamplesPerFrame(st.mpegAudioVersion, st.mpegAudioLayer)
					frameRate := 0.0
					if st.audioRate > 0 && spf > 0 {
						frameRate = st.audioRate / float64(spf)
					}

					brMode := "Variable"
					brModeJSON := "VBR"
					if st.mpegAudioBitrateMin > 0 && st.mpegAudioBitrateMin == st.mpegAudioBitrateMax {
						brMode = "Constant"
						brModeJSON = "CBR"
					}
					fields = append(fields, Field{Name: "Bit rate mode", Value: brMode})
					builder.Fill("BitRate_Mode", brMode, "Bit rate mode", brMode)
					if st.mpegAudioBitrateKbps > 0 {
						fields = append(fields, Field{Name: "Bit rate", Value: formatBitrateKbps(int64(st.mpegAudioBitrateKbps))})
					} else if duration > 0 && st.bytes > 0 {
						bitrate := (float64(st.bytes) * 8) / duration
						if value := formatBitrate(bitrate); value != "" {
							fields = append(fields, Field{Name: "Bit rate", Value: value})
						}
					}
					fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.audioChannels)})
					builder.Fill("Channels", strconv.FormatUint(st.audioChannels, 10), "Channel(s)", formatChannels(st.audioChannels))
					fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.audioRate)})
					builder.Fill("SamplingRate", strconv.FormatInt(int64(math.Round(st.audioRate)), 10), "Sampling rate", formatSampleRate(st.audioRate))
					if frameRate > 0 && spf > 0 {
						display := formatAudioFrameRate(frameRate, spf)
						fields = append(fields, Field{Name: "Frame rate", Value: display})
						builder.Fill("FrameRate", formatJSONFloat(frameRate), "Frame rate", display)
					}
					fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
					builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
					if st.bytes > 0 {
						if streamSize := formatStreamSize(int64(st.bytes), size); streamSize != "" {
							fields = append(fields, Field{Name: "Stream size", Value: streamSize})
						}
					}

					facts.Set("BitRate_Mode", brModeJSON)
					facts.Set("Compression_Mode", "Lossy")
					if st.mpegAudioLayer == 0x02 {
						facts.Set("Format_Profile", "Layer 2")
					} else if st.mpegAudioLayer == 0x01 {
						facts.Set("Format_Profile", "Layer 3")
					} else if st.mpegAudioLayer == 0x03 {
						facts.Set("Format_Profile", "Layer 1")
					}
					if st.mpegAudioVersion == 0x03 {
						facts.Set("Format_Version", "1")
					}
					if st.mpegAudioBitrateKbps > 0 {
						facts.Set("BitRate", strconv.Itoa(st.mpegAudioBitrateKbps*1000))
					} else if duration > 0 && st.bytes > 0 {
						facts.Set("BitRate", strconv.FormatInt(int64(math.Round((float64(st.bytes)*8)/duration)), 10))
					}
					if st.audioRate > 0 {
						facts.Set("SamplingRate", strconv.Itoa(int(math.Round(st.audioRate))))
					}
					if st.audioChannels > 0 {
						facts.Set("Channels", strconv.FormatUint(st.audioChannels, 10))
					}
					if spf > 0 {
						facts.Set("SamplesPerFrame", strconv.Itoa(spf))
					}
					if st.audioFrames > 0 && spf > 0 {
						samplingCount := int64(st.audioFrames) * int64(spf)
						facts.Set("SamplingCount", strconv.FormatInt(samplingCount, 10))
						facts.Set("FrameCount", strconv.FormatUint(st.audioFrames, 10))
						if frameRate > 0 {
							facts.Set("FrameRate", formatJSONFloat(frameRate))
						}
					}
					if st.bytes > 0 {
						facts.Set("StreamSize", strconv.FormatUint(st.bytes, 10))
					}
				} else {
					fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
					builder.Fill("BitRate_Mode", "Variable", "Bit rate mode", "Variable")
					fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(st.audioChannels)})
					builder.Fill("Channels", strconv.FormatUint(st.audioChannels, 10), "Channel(s)", formatChannels(st.audioChannels))
					builder.DirectStructured("ChannelPositions", channelPositionsFromCount(strconv.FormatUint(st.audioChannels, 10)))
					if layout := channelLayout(st.audioChannels); layout != "" {
						fields = append(fields, Field{Name: "Channel layout", Value: layout})
						builder.Fill("ChannelLayout", layout, "Channel layout", layout)
					}
					fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(st.audioRate)})
					builder.Fill("SamplingRate", strconv.FormatInt(int64(math.Round(st.audioRate)), 10), "Sampling rate", formatSampleRate(st.audioRate))
					frameRate := st.audioRate / 1024.0
					frameDisplay := formatAudioFrameRate(frameRate, 1024)
					fields = append(fields, Field{Name: "Frame rate", Value: frameDisplay})
					builder.Fill("FrameRate", formatJSONFloat(frameRate), "Frame rate", frameDisplay)
					fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
					builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
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
			}
			if st.hasAC3 {
				if duration > 0 {
					facts.Set("Duration", fmt.Sprintf("%.3f", duration))
				}
				if st.ac3Info.spf > 0 {
					facts.Set("SamplesPerFrame", strconv.Itoa(st.ac3Info.spf))
				}
				if st.ac3Info.sampleRate > 0 {
					useFrames := st.audioFrames > 0 && st.ac3Info.spf > 0 && !st.pts.hasResets() && !opts.sampled
					if useFrames {
						samplingCount := int64(st.audioFrames) * int64(st.ac3Info.spf)
						facts.Set("SamplingCount", strconv.FormatInt(samplingCount, 10))
						facts.Set("FrameCount", strconv.FormatUint(st.audioFrames, 10))
					} else if duration > 0 {
						samplingCount := int64(math.Round(duration * st.ac3Info.sampleRate))
						facts.Set("SamplingCount", strconv.FormatInt(samplingCount, 10))
						if st.ac3Info.spf > 0 {
							frameCount := int64(math.Round(float64(samplingCount) / float64(st.ac3Info.spf)))
							facts.Set("FrameCount", strconv.FormatInt(frameCount, 10))
						}
					}
				}
				if st.ac3Info.bitRateKbps > 0 && duration > 0 {
					streamSizeBytes := int64(math.Round(float64(st.ac3Info.bitRateKbps*1000) * duration / 8.0))
					if streamSizeBytes > 0 {
						facts.Set("StreamSize", strconv.FormatInt(streamSizeBytes, 10))
					}
				} else if st.bytes > 0 && !opts.sampled {
					facts.Set("StreamSize", strconv.FormatUint(st.bytes, 10))
				}
				facts.Set("Format_Settings_Endianness", "Big")
				if code := ac3ServiceKindCode(st.ac3Info.bsmod); code != "" {
					facts.Set("ServiceKind", code)
				}
				extraFields := []jsonKV{}
				if st.ac3Info.bsid > 0 {
					extraFields = append(extraFields, jsonKV{Key: "bsid", Val: strconv.Itoa(st.ac3Info.bsid)})
				}
				if st.ac3Info.hasDialnorm {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(st.ac3Info.dialnorm)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_String", Val: strconv.Itoa(st.ac3Info.dialnorm) + " dB"})
					}
				}
				if st.ac3Info.hasCompr {
					extraFields = append(extraFields, jsonKV{Key: "compr", Val: fmt.Sprintf("%.2f", st.ac3Info.comprDB)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "compr_String", Val: fmt.Sprintf("%.2f dB", st.ac3Info.comprDB)})
					}
				}
				if st.ac3Info.acmod > 0 {
					extraFields = append(extraFields, jsonKV{Key: "acmod", Val: strconv.Itoa(st.ac3Info.acmod)})
				}
				if st.ac3Info.hasDsurmod {
					extraFields = append(extraFields, jsonKV{Key: "dsurmod", Val: strconv.Itoa(st.ac3Info.dsurmod)})
				}
				if st.ac3Info.lfeon >= 0 {
					extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: strconv.Itoa(st.ac3Info.lfeon)})
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
					extraFields = append(extraFields, jsonKV{Key: "mixlevel", Val: strconv.Itoa(st.ac3Info.mixlevel)})
				}
				if st.ac3Info.hasRoomtyp {
					extraFields = append(extraFields, jsonKV{Key: "roomtyp", Val: st.ac3Info.roomtyp})
				}
				if avg, minVal, maxVal, ok := st.ac3Info.dialnormStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
					extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
					if maxVal != minVal || opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: strconv.Itoa(maxVal)})
					}
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average_String", Val: strconv.Itoa(avg) + " dB"})
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum_String", Val: strconv.Itoa(minVal) + " dB"})
						extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum_String", Val: strconv.Itoa(maxVal) + " dB"})
						if st.ac3Info.dialnormCount > 0 {
							extraFields = append(extraFields, jsonKV{Key: "dialnorm_Count", Val: strconv.Itoa(st.ac3Info.dialnormCount)})
						}
					}
				}
				if avg, minVal, maxVal, count, ok := st.ac3Info.comprStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "compr_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
					extraFields = append(extraFields, jsonKV{Key: "compr_Count", Val: strconv.Itoa(count)})
					if opts.dvdExtras {
						extraFields = append(extraFields, jsonKV{Key: "compr_Average_String", Val: fmt.Sprintf("%.2f dB", avg)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Minimum_String", Val: fmt.Sprintf("%.2f dB", minVal)})
						extraFields = append(extraFields, jsonKV{Key: "compr_Maximum_String", Val: fmt.Sprintf("%.2f dB", maxVal)})
					}
				}
				if avg, minVal, maxVal, count, ok := st.ac3Info.dynrngStats(); ok {
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Average", Val: fmt.Sprintf("%.2f", avg)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Minimum", Val: fmt.Sprintf("%.2f", minVal)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Maximum", Val: fmt.Sprintf("%.2f", maxVal)})
					extraFields = append(extraFields, jsonKV{Key: "dynrng_Count", Val: strconv.Itoa(count)})
				}
				if len(extraFields) > 0 {
					node := structuredObjectFromKVs(extraFields)
					extraNode = &node
				}
			}
		case StreamText:
			if st.pts.has() {
				duration := float64(ptsDelta(st.pts.first, st.pts.last)) / 90000.0
				if duration > 0 {
					fields = addStreamDuration(fields, duration)
					builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
					facts.Set("Duration", fmt.Sprintf("%.3f", duration))
				}
				if delay, ok := delayRelativeToVideoMs(st.pts, videoPTS, videoIsH264, videoFrameRate); ok {
					if rounded := int64(math.Round(delay)); rounded != 0 {
						fields = append(fields, Field{Name: "Delay relative to video", Value: formatDelayMs(rounded)})
					}
				}
			} else if duration := ptsDurationPS(st.pts, opts); duration > 0 {
				fields = addStreamDuration(fields, duration)
				builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
			}
		case StreamGeneral, StreamMenu, StreamImage:
			if duration := ptsDurationPS(st.pts, opts); duration > 0 {
				fields = addStreamDuration(fields, duration)
				builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
			}
		}
		if st.kind == StreamVideo && st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			facts.Set("Delay", fmt.Sprintf("%.9f", delay))
			dropFrame := "No"
			if info.GOPDropFrame != nil && *info.GOPDropFrame {
				dropFrame = "Yes"
			}
			facts.Set("Delay_DropFrame", dropFrame)
			facts.Set("Delay_Source", "Container")
			facts.Set("Delay_Original", "0.000")
			facts.Set("Delay_Original_DropFrame", dropFrame)
			facts.Set("Delay_Original_Source", "Stream")
		}
		if st.kind == StreamAudio && st.pts.has() {
			delay := float64(st.pts.min) / 90000.0
			facts.Set("Delay", fmt.Sprintf("%.9f", delay))
			facts.Set("Delay_Source", "Container")
			if videoPTS.has() {
				videoDelay := float64(int64(st.pts.min)-int64(videoPTS.min)) / 90000.0
				facts.Set("Video_Delay", fmt.Sprintf("%.3f", videoDelay))
			}
		}
		if st.kind == StreamText && st.pts.has() {
			delay := float64(st.pts.first) / 90000.0
			facts.Set("Delay", fmt.Sprintf("%.9f", delay))
			facts.Set("Delay_Source", "Container")
			if videoPTS.has() {
				videoDelay := float64(int64(st.pts.first)-int64(videoPTS.min)) / 90000.0
				facts.Set("Video_Delay", fmt.Sprintf("%.3f", videoDelay))
			}
		}
		streamsOut = append(streamsOut, buildMPEGPSCanonicalSnapshot(builder, fields, facts, extraNode, canonicalStreamPolicy{}))
	}

	info := ContainerInfo{}
	var hasMode bool
	allConstant := true
	for _, key := range streamOrder {
		st := streams[key]
		if st == nil {
			continue
		}
		mode := ""
		switch st.kind {
		case StreamVideo:
			if parser := videoParsers[key]; parser != nil {
				videoInfo := parser.finalize()
				mode = videoInfo.BitRateMode
				if ptsDurationPS(st.pts, opts) == 0 && videoInfo.FrameRate > 0 && (videoInfo.GOPLength > 0 || videoInfo.GOPLengthFirst > 0) {
					mode = "Constant"
				}
			}
		case StreamAudio:
			if st.hasAC3 {
				mode = "Constant"
			} else if st.audioRate > 0 {
				mode = "Variable"
			}
		case StreamGeneral, StreamText, StreamImage, StreamMenu:
			continue
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
			duration += 1.0 / videoFrameRate
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
	usedDerivedDuration := false
	for _, st := range streams {
		if st == nil || st.kind == StreamMenu {
			continue
		}
		duration := ptsDurationPS(st.pts, opts)
		if st.kind == StreamAudio {
			duration = audioDurationPS(st, opts)
		}
		if st.kind == StreamVideo && st.derivedDuration > 0 {
			if duration == 0 || st.derivedDuration > duration {
				duration = st.derivedDuration
				usedDerivedDuration = true
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
		// MediaInfo often quantizes very short durations (e.g. 1-frame menu VOBs)
		// to milliseconds, which affects derived overall bitrate rounding.
		if usedDerivedDuration && ptsDurationPS(anyPTS, opts) == 0 && ptsDurationPS(videoPTS, opts) == 0 {
			maxDuration = math.Round(maxDuration*1000) / 1000
		}
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

// consumeMPEG2StartCodeStats incrementally counts MPEG-2 picture start codes,
// attributing only PTS-bearing payloads to the stream frame count.
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

// nextPESStart returns the next MPEG start-code prefix at or after start.
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

// isPESStreamID reports whether a start-code stream ID carries a PES or DVD
// private-stream payload parsed by this package.
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

// mapPSStream maps MPEG program-stream and private substream IDs to a canonical
// stream kind and format.
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

// consumeAC3PS incrementally parses complete AC-3 frames from one program
// stream while retaining an incomplete trailing frame for the next payload.
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

// consumeH264PS accumulates bounded Annex B payload data and updates one
// program stream's AVC metadata when a valid configuration is found.
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
		if fields, sps, avc, ok := parseH264AnnexBDetails(entry.videoBuffer); ok && len(fields) > 0 {
			width, height, fps := sps.Width, sps.Height, sps.FrameRate
			if width < 64 || height < 64 {
				return
			}
			entry.videoFields = fields
			entry.videoAVC = avc
			entry.videoSPS = sps
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

func consumeMPEGAudioPS(entry *psStream, payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry.audioBuffer = append(entry.audioBuffer, payload...)
	const maxProbe = 512 << 10
	if len(entry.audioBuffer) > maxProbe {
		entry.audioBuffer = append(entry.audioBuffer[:0], entry.audioBuffer[len(entry.audioBuffer)-maxProbe:]...)
	}

	i := 0
	for i+4 <= len(entry.audioBuffer) {
		if entry.audioBuffer[i] != 0xFF || (entry.audioBuffer[i+1]&0xE0) != 0xE0 {
			i++
			continue
		}
		hdr, ok := parseMPEGAudioHeader(entry.audioBuffer[i : i+4])
		if !ok {
			i++
			continue
		}
		frameLen := mpegAudioFrameLengthBytes(hdr)
		if frameLen <= 0 {
			i++
			continue
		}
		if i+frameLen > len(entry.audioBuffer) {
			break
		}
		// Validate next frame header when possible.
		if i+frameLen+4 <= len(entry.audioBuffer) {
			next, ok := parseMPEGAudioHeader(entry.audioBuffer[i+frameLen : i+frameLen+4])
			if !ok || next.sampleRate != hdr.sampleRate || next.layerID != hdr.layerID || next.versionID != hdr.versionID || next.channels != hdr.channels {
				i++
				continue
			}
		}

		entry.audioFrames++
		if !entry.hasAudioInfo {
			entry.audioRate = float64(hdr.sampleRate)
			entry.audioChannels = uint64(hdr.channels)
			entry.hasAudioInfo = true
			entry.mpegAudioVersion = hdr.versionID
			entry.mpegAudioLayer = hdr.layerID
			entry.mpegAudioBitrateMin = hdr.bitrateKbps
			entry.mpegAudioBitrateMax = hdr.bitrateKbps
		} else {
			if hdr.bitrateKbps > 0 {
				if entry.mpegAudioBitrateMin == 0 || hdr.bitrateKbps < entry.mpegAudioBitrateMin {
					entry.mpegAudioBitrateMin = hdr.bitrateKbps
				}
				if hdr.bitrateKbps > entry.mpegAudioBitrateMax {
					entry.mpegAudioBitrateMax = hdr.bitrateKbps
				}
			}
		}
		i += frameLen
	}
	if entry.mpegAudioBitrateMin > 0 && entry.mpegAudioBitrateMin == entry.mpegAudioBitrateMax {
		entry.mpegAudioBitrateKbps = entry.mpegAudioBitrateMin
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
