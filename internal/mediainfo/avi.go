package mediainfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	// aviMaxVisualScan bounds header-oriented MPEG-4 Visual probing.
	aviMaxVisualScan = 1 << 20
	// aviMaxVOPScan bounds the larger payload window used for VOP statistics.
	aviMaxVOPScan = 2 << 20
	// aviMaxAudioScan bounds codec sync and encoder-tag probing per audio stream.
	aviMaxAudioScan = 2 << 20
	// aviAC3StatsFrames matches MediaInfo's bounded AVI AC-3 statistics window.
	aviAC3StatsFrames  = 404
	maxAVINestingDepth = 32
)

// aviMainHeader stores timing, frame-count, and geometry facts from avih.
type aviMainHeader struct {
	microSecPerFrame uint32
	maxBytesPerSec   uint32
	flags            uint32
	totalFrames      uint32
	streams          uint32
	width            uint32
	height           uint32
	openDML          bool
	recordLists      bool
}

// aviStream stores decoded AVI stream-header, format, timing, and codec facts
// before canonical projection.
type aviStream struct {
	index           int
	kind            StreamKind
	handler         string
	compression     string
	scale           uint32
	rate            uint32
	start           uint32
	length          uint32
	indxDuration    uint64
	suggestedBuf    uint32
	sampleSize      uint32
	width           uint32
	height          uint32
	bitCount        uint16
	audioTag        uint16
	audioChans      uint16
	audioRate       uint32
	audioAvgBps     uint32
	audioAlign      uint16
	audioBits       uint16
	audioExtra      []byte
	bytes           uint64
	paddingBytes    uint64
	packetCount     uint32
	delayBytes      uint32
	title           string
	writingLib      string
	profile         string
	bvop            *bool
	bvopCount       int
	qpel            *bool
	gmc             string
	matrix          string
	matrixData      string
	colorSpace      string
	chroma          string
	bitDepth        string
	scanType        string
	scanOrder       string
	bitRateNominal  int64
	bufferSize      int64
	packedBitstream bool
	hasVideoInfo    bool
	vopScan         vopScanner
	vopScanned      int
	audioData       []byte
}

// vopScanner incrementally counts MPEG-4 Visual picture coding types across
// payload chunk boundaries.
type vopScanner struct {
	carry       []byte
	bvop        *bool
	consecutive int
	maxCount    int
}

// feed scans one payload chunk for MPEG-4 VOP start codes while retaining the
// boundary bytes needed by the next chunk.
func (s *vopScanner) feed(data []byte) {
	buf := append(append([]byte{}, s.carry...), data...)
	stop := false
	scanMPEG2StartCodes(buf, 0, func(i int, code byte) bool {
		if code != 0xB6 {
			return true
		}
		if i+4 >= len(buf) {
			s.carry = append([]byte{}, buf[i:]...)
			stop = true
			return false
		}
		vopType := (buf[i+4] >> 6) & 0x03
		if vopType == 2 {
			s.consecutive++
			if s.consecutive > s.maxCount {
				s.maxCount = s.consecutive
			}
			val := true
			s.bvop = &val
		} else {
			s.consecutive = 0
			if s.bvop == nil {
				val := false
				s.bvop = &val
			}
		}
		return true
	})
	if stop {
		return
	}
	if len(buf) >= 3 {
		s.carry = append([]byte{}, buf[len(buf)-3:]...)
	} else {
		s.carry = append([]byte{}, buf...)
	}
}

// ParseAVI parses RIFF/AVI metadata and bounded payload data with default
// analysis options.
func ParseAVI(file io.ReadSeeker, size int64) (ContainerInfo, []Stream, []Field, bool) {
	info, streams, fields, _, ok := ParseAVIWithOptions(file, size, defaultAnalyzeOptions())
	return info, streams, fields, ok
}

// ParseAVIWithOptions parses RIFF/AVI metadata and bounded stream payloads. It
// returns the writing application separately and reports false for invalid AVI.
func ParseAVIWithOptions(file io.ReadSeeker, size int64, opts AnalyzeOptions) (ContainerInfo, []Stream, []Field, string, bool) {
	opts = normalizeAnalyzeOptions(opts)
	if size < 12 {
		return ContainerInfo{}, nil, nil, "", false
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ContainerInfo{}, nil, nil, "", false
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return ContainerInfo{}, nil, nil, "", false
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "AVI " {
		return ContainerInfo{}, nil, nil, "", false
	}

	var main aviMainHeader
	streams := []*aviStream{}
	var writingApp string
	var writingLib string
	var videoData []byte
	setFormatSettings := false
	type byteRange struct {
		start int64
		end   int64
	}
	var moviRanges []byteRange
	var indexRanges []byteRange
	haveIndex := false
	var interleaved string
	var audioFirstBytes uint64
	var generalExtra *structuredNode

	var parseTopLevel func(int64, int64, int)
	parseTopLevel = func(start, end int64, depth int) {
		if depth > maxAVINestingDepth {
			return
		}
		for offset := start; offset+8 <= end; {
			var chunkHeader [8]byte
			if _, err := readAt(file, offset, chunkHeader[:]); err != nil {
				return
			}
			chunkID := string(chunkHeader[0:4])
			chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
			dataStart := offset + 8
			dataEnd := dataStart + chunkSize
			if dataEnd < dataStart || dataEnd > end {
				if generalExtra == nil && dataEnd > size {
					generalExtra = aviTruncatedChunkExtra(chunkID, size, dataStart, dataEnd)
				}
				return
			}
			switch chunkID {
			case "LIST":
				if chunkSize >= 4 {
					var listTypeBytes [4]byte
					if _, err := readAt(file, dataStart, listTypeBytes[:]); err != nil {
						return
					}
					listType := string(listTypeBytes[:])
					listDataStart := dataStart + 4
					listDataSize := chunkSize - 4
					switch listType {
					case "hdrl":
						listData := make([]byte, listDataSize)
						if _, err := readAt(file, listDataStart, listData); err != nil {
							return
						}
						parseAVIHDRL(listData, &main, &streams)
						setFormatSettings = len(streams) > 0
					case "INFO":
						listData := make([]byte, listDataSize)
						if _, err := readAt(file, listDataStart, listData); err != nil {
							return
						}
						if app := parseAVIINFO(listData); app != "" {
							writingLib = aviGeneralEncodedLibrary(app)
							if !strings.HasPrefix(app, "VirtualDub build ") {
								writingApp = app
							}
						}
					case "movi":
						moviRanges = append(moviRanges, byteRange{start: listDataStart, end: dataEnd})
					}
				}
			case "idx1":
				indexRanges = append(indexRanges, byteRange{start: dataStart, end: dataEnd})
			case "JUNK":
				if writingLib == "" && chunkSize > 0 {
					readSize := min(chunkSize, int64(4096))
					data := make([]byte, readSize)
					if _, err := readAt(file, dataStart, data); err == nil {
						writingLib = aviJUNKEncodedLibrary(data)
					}
				}
			case "RIFF":
				if chunkSize >= 4 {
					var form [4]byte
					if _, err := readAt(file, dataStart, form[:]); err != nil {
						return
					}
					if string(form[:]) == "AVIX" {
						main.openDML = true
						parseTopLevel(dataStart+4, dataEnd, depth+1)
					}
				}
			}
			offset = dataEnd + chunkSize%2
		}
	}
	parseTopLevel(12, size, 0)

	if !main.openDML {
		for _, indexRange := range indexRanges {
			indexData := make([]byte, indexRange.end-indexRange.start)
			if _, err := readAt(file, indexRange.start, indexData); err != nil {
				continue
			}
			if ok, stats := parseAVIIndex(indexData, streams); ok {
				haveIndex = true
				interleaved = firstNonEmpty(stats.interleaved, interleaved)
				if stats.audioFirstBytes > 0 {
					audioFirstBytes = stats.audioFirstBytes
				}
			}
		}
	}

	collectMoviStats := main.openDML || !haveIndex
	if collectMoviStats {
		resetAVIPayloadStats(streams)
	}
	moviStats := aviIndexStats{}
	for index, moviRange := range moviRanges {
		maxScanBytes := int64(32 << 20)
		if collectMoviStats || opts.ParseSpeed >= 1 {
			maxScanBytes = 0
		} else if index > 0 {
			break
		}
		parseAVIMovi(file, moviRange.start, moviRange.end, streams, &videoData, maxScanBytes, collectMoviStats, &moviStats)
	}
	if collectMoviStats {
		finalizeAVIIndexStats(&moviStats)
		finalizeAVIPayloadStats(streams)
		interleaved = firstNonEmpty(moviStats.interleaved, interleaved)
		if moviStats.audioFirstBytes > 0 {
			audioFirstBytes = moviStats.audioFirstBytes
		}
	}
	main.recordLists = moviStats.recordLists
	if main.openDML {
		interleaved = "Yes"
	} else if interleaved == "No" {
		for _, stream := range streams {
			if stream.kind == StreamAudio && stream.audioTag == 0x0001 {
				interleaved = "Yes"
				break
			}
		}
	}

	if len(streams) == 0 {
		return ContainerInfo{}, nil, nil, "", false
	}

	var containerDuration float64
	var streamsOut []Stream
	var videoFrameRate float64
	hasVideo := false
	hasAudio := false
	for _, st := range streams {
		switch st.kind {
		case StreamVideo:
			hasVideo = true
			if fr := aviFrameRate(st); fr > 0 {
				videoFrameRate = fr
			}
		case StreamAudio:
			hasAudio = true
		}
	}

	if len(videoData) > 0 {
		info := parseMPEG4Visual(videoData)
		for _, st := range streams {
			if st.kind != StreamVideo {
				continue
			}
			if st.handler == "FMP4" || st.compression == "FMP4" || st.compression == "MP4V" || st.compression == "DIVX" || st.compression == "XVID" || st.compression == "DX50" || st.handler == "DX50" {
				if info.Profile != "" {
					st.profile = info.Profile
				}
				if info.WritingLibrary != "" {
					st.writingLib = info.WritingLibrary
				}
				st.qpel = info.QPel
				st.bvop = info.BVOP
				st.gmc = info.GMC
				st.matrix = info.Matrix
				st.matrixData = info.MatrixData
				st.bitRateNominal = info.BitRateNominal
				st.bufferSize = info.BufferSize
				st.colorSpace = info.ColorSpace
				st.chroma = info.ChromaSubsampling
				st.bitDepth = info.BitDepth
				st.scanType = info.ScanType
				st.scanOrder = info.ScanOrder
				st.packedBitstream = info.PackedBitstream
				if info.BVOPCount > 0 {
					st.bvopCount = info.BVOPCount
				}
				st.hasVideoInfo = true
			}
		}
	}
	for _, st := range streams {
		if st.kind == StreamVideo && st.vopScan.bvop != nil {
			st.bvop = st.vopScan.bvop
			if st.vopScan.maxCount > st.bvopCount {
				st.bvopCount = st.vopScan.maxCount
			}
		}
	}

	// MPEG-4 user data can identify encoder-specific frame-rate handling, so
	// derive the container duration only after the bounded video probe.
	for _, st := range streams {
		if d := aviStreamDuration(st, main); d > containerDuration {
			containerDuration = d
		}
	}

	for _, st := range streams {
		switch st.kind {
		case StreamVideo:
			streamsOut = append(streamsOut, canonicalAVIVideoStream(st, main, size))
		case StreamAudio:
			streamsOut = append(streamsOut, canonicalAVIAudioStream(st, streams, size, videoFrameRate, audioFirstBytes, st.audioData))
		case StreamGeneral, StreamText, StreamImage, StreamMenu:
			continue
		}
	}

	info := ContainerInfo{}
	info.containerFrameCount = uint64(main.totalFrames)
	info.generalExtra = generalExtra
	if containerDuration > 0 {
		info.DurationSeconds = containerDuration
	}

	generalFields := []Field{}
	generalFields = append(generalFields, Field{Name: "Format/Info", Value: "Audio Video Interleave"})
	if main.openDML {
		generalFields = append(generalFields, Field{Name: "Format profile", Value: "OpenDML"})
	}
	if setFormatSettings {
		parts := []string{}
		if main.recordLists {
			parts = append(parts, "rec")
		}
		if hasVideo {
			parts = append(parts, "BitmapInfoHeader")
		}
		if hasAudio {
			parts = append(parts, "WaveFormatEx")
		}
		if len(parts) == 0 {
			parts = append(parts, "BitmapInfoHeader")
		}
		generalFields = append(generalFields, Field{Name: "Format settings", Value: strings.Join(parts, " / ")})
	}
	if rate := firstAVIReportedFrameRate(streams, main); rate > 0 {
		generalFields = append(generalFields, Field{Name: "Frame rate", Value: formatFrameRate(rate)})
	}
	if writingApp != "" {
		generalFields = append(generalFields, Field{Name: "Writing application", Value: writingApp})
	}
	if writingLib != "" {
		generalFields = append(generalFields, Field{Name: "Writing library", Value: writingLib})
	}

	return info, streamsOut, generalFields, interleaved, true
}

// canonicalAVIVideoStream converts parsed RIFF video facts directly into the field store.
func canonicalAVIVideoStream(st *aviStream, main aviMainHeader, size int64) Stream {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("ID", strconv.Itoa(st.index), "ID", strconv.Itoa(st.index))
	format := mapAVICompression(st)
	if format != "" {
		builder.Fill("Format", format, "Format", format)
	}
	if st.profile != "" {
		profile, level := splitProfileLevel(st.profile)
		builder.Fill("Format_Profile", profile, "Format profile", st.profile)
		builder.Structured("Format_Level", level)
	}
	if st.bvop != nil {
		value := formatYesNo(*st.bvop)
		if format == "MPEG-4 Visual" && *st.bvop {
			value = strconv.Itoa(max(1, st.bvopCount))
		}
		builder.Fill("Format_Settings_BVOP", value, "Format settings, BVOP", value)
	}
	if st.qpel != nil {
		value := formatYesNo(*st.qpel)
		builder.Fill("Format_Settings_QPel", value, "Format settings, QPel", value)
	}
	if st.gmc != "" {
		raw := extractLeadingNumber(st.gmc)
		if strings.HasPrefix(st.gmc, "No") {
			raw = "0"
		}
		builder.Fill("Format_Settings_GMC", raw, "Format settings, GMC", st.gmc)
	}
	if st.matrix != "" {
		builder.Fill("Format_Settings_Matrix", st.matrix, "Format settings, Matrix", st.matrix)
	}
	if st.packedBitstream {
		builder.Fill("MuxingMode", "MuxingMode_PackedBitstream", "Muxing mode", "MuxingMode_PackedBitstream")
	}
	codec := st.compression
	if codec == "" {
		codec = st.handler
	}
	if codec != "" {
		codecID, _ := splitCodecID(codec)
		builder.Fill("CodecID", codecID, "Codec ID", codec)
	}

	structuredFacts := &canonicalStructuredFacts{}
	duration := aviStreamDuration(st, main)
	if duration > 0 {
		builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
		structuredFacts.Set("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), formatJSONSeconds(duration))
	}
	if st.bytes > 0 && duration > 0 {
		_, reportDenominator, reportFrameRate := aviReportedFrameRateRatio(st, duration)
		durationForBitrate := duration
		useRoundedFrameRate := st.compression == "XVID" || (st.packedBitstream && reportDenominator == 1001)
		if useRoundedFrameRate && st.length > 0 && reportFrameRate > 0 {
			if rounded := math.Round(reportFrameRate*1000) / 1000; rounded > 0 {
				durationForBitrate = float64(st.length) / rounded
			}
		}
		bitrate := float64(st.bytes) * 8 / durationForBitrate
		raw := strconv.FormatInt(int64(math.Round(bitrate)), 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(bitrate))
		structuredFacts.SetSame("BitRate", raw)
		if st.width > 0 && st.height > 0 && reportFrameRate > 0 {
			builder.Text("Bits/(Pixel*Frame)", formatBitsPerPixelFrame(bitrate, uint64(st.width), uint64(st.height), reportFrameRate))
		}
	}
	if st.bitRateNominal > 0 {
		raw := strconv.FormatInt(st.bitRateNominal, 10)
		builder.Fill("BitRate_Nominal", raw, "Nominal bit rate", formatBitrate(float64(st.bitRateNominal)))
		structuredFacts.SetSame("BitRate_Nominal", raw)
	}
	if st.width > 0 {
		builder.Fill("Width", strconv.FormatUint(uint64(st.width), 10), "Width", formatPixels(uint64(st.width)))
	}
	if st.height > 0 {
		builder.Fill("Height", strconv.FormatUint(uint64(st.height), 10), "Height", formatPixels(uint64(st.height)))
	}
	if st.width > 0 && st.height > 0 {
		ratio := float64(st.width) / float64(st.height)
		builder.Fill("PixelAspectRatio", "1.000", "Pixel aspect ratio", "1.000")
		builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", formatAVIAspectRatio(ratio))
	}
	if numerator, denominator, frameRate := aviReportedFrameRateRatio(st, duration); frameRate > 0 {
		frameRateDisplay := formatFrameRateRatio(numerator, denominator)
		if denominator == 1 {
			frameRateDisplay = fmt.Sprintf("%.3f FPS", frameRate)
		}
		builder.Fill("FrameRate", formatJSONFloat(frameRate), "Frame rate", frameRateDisplay)
		builder.Structured("FrameRate_Num", strconv.FormatUint(uint64(numerator), 10))
		builder.Structured("FrameRate_Den", strconv.FormatUint(uint64(denominator), 10))
		if strings.HasPrefix(st.writingLib, "XviD0050") {
			builder.Structured("FrameRate_Original", formatJSONFloat(aviFrameRate(st)))
		}
		if st.length > 0 {
			frameCount := strconv.FormatUint(uint64(st.length), 10)
			structuredFacts.SetSame("FrameCount", frameCount)
			builder.Structured("FrameCount", frameCount)
		}
	}
	for _, value := range []struct {
		name  fieldName
		label string
		value string
	}{
		{"ColorSpace", "Color space", st.colorSpace},
		{"ChromaSubsampling", "Chroma subsampling", st.chroma},
		{"BitDepth", "Bit depth", st.bitDepth},
		{"ScanType", "Scan type", st.scanType},
		{"ScanOrder", "Scan order", st.scanOrder},
	} {
		raw := value.value
		if value.name == "BitDepth" {
			raw = extractLeadingNumber(raw)
		}
		builder.Fill(value.name, raw, value.label, value.value)
	}
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	builder.Fill("Delay", "0.000", "Delay", "0.000")
	structuredFacts.SetSame("Delay", "0.000")
	if st.bytes > 0 {
		raw := strconv.FormatUint(st.bytes, 10)
		builder.Fill("StreamSize", raw, "Stream size", formatStreamSize(int64(st.bytes), size))
		structuredFacts.SetSame("StreamSize", raw)
	}
	if st.matrixData != "" {
		structuredFacts.SetSame("Format_Settings_Matrix_Data", st.matrixData)
		builder.Structured("Format_Settings_Matrix_Data", st.matrixData)
	}
	if st.writingLib != "" {
		encoded := st.writingLib
		if strings.HasPrefix(encoded, "x264 ") && !strings.HasPrefix(encoded, "x264 - ") {
			encoded = "x264 - " + strings.TrimPrefix(encoded, "x264 ")
		}
		if strings.HasPrefix(encoded, "x265 ") && !strings.HasPrefix(encoded, "x265 - ") {
			encoded = "x265 - " + strings.TrimPrefix(encoded, "x265 ")
		}
		builder.Fill("Encoded_Library", encoded, "Writing library", st.writingLib)
		if name, version := splitEncodedLibrary(encoded); name != "" {
			builder.Structured("Encoded_Library_Name", name)
			builder.Structured("Encoded_Library_Version", version)
		}
	}
	if strings.HasPrefix(st.writingLib, "XviD") && !strings.Contains(st.writingLib, "build=") {
		if version, date, ok := xvidLibraryVersionDate(st.writingLib); ok {
			structuredFacts.SetSame("Encoded_Library_Name", "XviD")
			structuredFacts.SetSame("Encoded_Library_Version", version)
			structuredFacts.SetSame("Encoded_Library_Date", date)
		}
	}
	if strings.HasPrefix(st.writingLib, "DivX") {
		structuredFacts.SetSame("Encoded_Library_Name", "DivX")
		if version, date, ok := divxLibraryVersionDate(st.writingLib); ok {
			structuredFacts.SetSame("Encoded_Library_Version", version)
			structuredFacts.SetSame("Encoded_Library_Date", date)
		}
	}
	if st.bufferSize > 0 {
		structuredFacts.SetSame("BufferSize", strconv.FormatInt(st.bufferSize, 10))
	}
	structuredFacts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{})
}

// canonicalAVIAudioStream converts parsed RIFF audio facts and bounded codec
// probes directly into the field store.
func canonicalAVIAudioStream(st *aviStream, streams []*aviStream, size int64, videoFrameRate float64, audioFirstBytes uint64, audioData []byte) Stream {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("ID", strconv.Itoa(st.index), "ID", strconv.Itoa(st.index))
	structuredFacts := &canonicalStructuredFacts{}
	var mp3Header mp3HeaderInfo
	mp3Offset := 0
	hasMP3 := false
	var lame aviLAMEInfo
	if st.audioTag == 0x0055 {
		mp3Header, mp3Offset, hasMP3 = findFirstMP3HeaderAt(audioData)
		lame = parseAVILAMEInfo(audioData)
	}
	var ac3 ac3Info
	ac3Offset := 0
	hasAC3 := false
	if st.audioTag == 0x2000 {
		ac3, ac3Offset, hasAC3 = probeAVIAC3(audioData)
	}

	codec := fmt.Sprintf("%X", st.audioTag)
	switch st.audioTag {
	case 0x0001:
		builder.Fill("Format", "PCM", "Format", "PCM")
		structuredFacts.SetSame("Format_Settings_Endianness", "Little")
		structuredFacts.SetSame("Format_Settings_Sign", "Signed")
		structuredFacts.SetSame("BitRate_Mode", "CBR")
	case 0x0055:
		builder.Fill("Format", "MPEG Audio", "Format", "MPEG Audio")
		structuredFacts.SetSame("Format_Profile", "Layer 3")
		structuredFacts.SetSame("Format_Version", "1")
		structuredFacts.SetSame("Compression_Mode", "Lossy")
	case 0x00FF:
		builder.Fill("Format", "AAC", "Format", "AAC")
		builder.Text("Format/Info", "Advanced Audio Codec Low Complexity")
		structuredFacts.SetSame("Compression_Mode", "Lossy")
		if objectType := aviAACObjectType(st.audioExtra); objectType > 0 {
			codec += "-" + strconv.Itoa(objectType)
			if profile := mapAACProfile(objectType); profile != "" {
				structuredFacts.SetSame("Format_AdditionalFeatures", profile)
			}
		}
	case 0x2000:
		builder.Fill("Format", "AC-3", "Format", "AC-3")
		builder.Text("Format/Info", "Audio Coding 3")
		builder.Fill("Format_Commercial_IfAny", "Dolby Digital", "Commercial name", "Dolby Digital")
		structuredFacts.SetSame("Format_Settings_Endianness", "Big")
		structuredFacts.SetSame("BitRate_Mode", "CBR")
		structuredFacts.SetSame("Compression_Mode", "Lossy")
	}
	if st.audioTag != 0 {
		builder.Fill("CodecID", codec, "Codec ID", codec)
		structuredFacts.SetSame("CodecID", codec)
	}

	channels := uint64(st.audioChans)
	sampleRate := float64(st.audioRate)
	if hasAC3 {
		if ac3.channels > 0 {
			channels = ac3.channels
		}
		if ac3.sampleRate > 0 {
			sampleRate = ac3.sampleRate
		}
	}
	if channels > 0 {
		raw := strconv.FormatUint(channels, 10)
		builder.Fill("Channels", raw, "Channel(s)", formatChannels(channels))
		structuredFacts.SetSame("Channels", raw)
		if hasAC3 && ac3.layout != "" {
			structuredFacts.SetSame("ChannelLayout", ac3.layout)
			positions := ac3ChannelPositions(ac3.layout)
			if positions == "" {
				positions = channelPositionsFromCount(raw)
			}
			if positions != "" {
				structuredFacts.SetSame("ChannelPositions", positions)
			}
		} else if st.audioTag == 0x00FF {
			if layout := channelLayout(channels); layout != "" {
				structuredFacts.SetSame("ChannelLayout", layout)
			}
			if positions := channelPositionsFromCount(raw); positions != "" {
				structuredFacts.SetSame("ChannelPositions", positions)
			}
		}
	}
	if sampleRate > 0 {
		raw := strconv.FormatFloat(sampleRate, 'f', -1, 64)
		builder.Fill("SamplingRate", raw, "Sampling rate", formatSampleRate(sampleRate))
		structuredFacts.SetSame("SamplingRate", raw)
	}

	duration := aviStreamDuration(st, aviMainHeader{})
	if duration == 0 {
		duration = aviAudioDurationSeconds(st)
	}
	bitRate := int64(st.audioAvgBps) * 8
	if hasAC3 && ac3.bitRateKbps > 0 {
		bitRate = ac3.bitRateKbps * 1000
	}
	if lame.variable && lame.frameCount > 0 && st.bytes > 0 && sampleRate > 0 {
		frameLength := float64(st.bytes+st.paddingBytes) / float64(lame.frameCount)
		average := frameLength * float64(sampleRate) / float64(1152/8)
		bitRate = int64(math.Round(average))
		// MediaInfoLib 26.05's LAME 3.97 path retains the next integer for
		// this fractional average; later LAME revisions use ordinary rounding.
		if strings.HasPrefix(lame.library, "LAME3.97") {
			bitRate++
		}
	}
	if lame.targetBitRate > 0 && bitRate > 0 && math.Abs(float64(bitRate-lame.targetBitRate))/float64(lame.targetBitRate) < 0.02 {
		bitRate = lame.targetBitRate
	}
	if bitRate > 0 {
		raw := strconv.FormatInt(bitRate, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(bitRate)))
		structuredFacts.SetSame("BitRate", raw)
	}
	if st.audioTag == 0x0055 {
		mode := "CBR"
		if lame.variable || !isMP3CBRBitrate(bitRate) {
			mode = "VBR"
		}
		structuredFacts.SetSame("BitRate_Mode", mode)
		if mode == "VBR" && lame.targetBitRate > 0 && lame.targetBitRate != bitRate {
			structuredFacts.SetSame("BitRate_Nominal", strconv.FormatInt(lame.targetBitRate, 10))
		}
	}

	if duration > 0 {
		milliseconds := strconv.FormatInt(int64(math.Round(duration*1000)), 10)
		builder.Fill("Duration", milliseconds, "Duration", formatDuration(duration))
		structuredFacts.Set("Duration", milliseconds, fmt.Sprintf("%.3f", math.Round(duration*1000)/1000))
		if st.audioTag == 0x2000 && aviAudioAlignment(st) == "Aligned" {
			structuredFacts.SetSame("Source_Duration", milliseconds)
		}
		if sampleRate > 0 {
			samples := int64(math.Round((math.Round(duration*1000) / 1000) * sampleRate))
			if st.audioTag == 0x0055 && lame.frameCount > 0 {
				samples = int64(lame.frameCount) * 1152
			}
			if samples > 0 {
				structuredFacts.SetSame("SamplingCount", strconv.FormatInt(samples, 10))
			}
		}
	}
	if st.bytes > 0 {
		raw := strconv.FormatUint(st.bytes, 10)
		builder.Fill("StreamSize", raw, "Stream size", formatStreamSize(int64(st.bytes), size))
		structuredFacts.SetSame("StreamSize", raw)
	}
	if st.audioBits > 0 {
		structuredFacts.SetSame("BitDepth", strconv.FormatUint(uint64(st.audioBits), 10))
	}

	if st.audioTag == 0x0055 && hasMP3 {
		spf := mpegAudioSamplesPerFrame(mp3Header.versionID, mp3Header.layerID)
		if spf <= 0 {
			spf = 1152
		}
		if lame.tagged {
			structuredFacts.SetSame("SamplesPerFrame", strconv.Itoa(spf))
			structuredFacts.SetSame("FrameRate", fmt.Sprintf("%.3f", float64(mp3Header.sampleRate)/float64(spf)))
			if lame.frameCount > 0 {
				structuredFacts.SetSame("FrameCount", strconv.FormatUint(uint64(lame.frameCount), 10))
			}
		}
		if mp3Header.channels == 2 && mp3Header.channelMode == 0x01 {
			structuredFacts.SetSame("Format_Settings_Mode", "Joint stereo")
			if mp3Header.modeExt&0x02 != 0 && !strings.HasPrefix(lame.library, "LAME3.98") {
				structuredFacts.SetSame("Format_Settings_ModeExtension", "MS Stereo")
			} else if mp3Header.modeExt&0x01 != 0 {
				structuredFacts.SetSame("Format_Settings_ModeExtension", "Intensity Stereo")
			}
		}
	}
	if st.audioTag == 0x00FF {
		structuredFacts.SetSame("SamplesPerFrame", "1024")
		if sampleRate > 0 {
			structuredFacts.SetSame("FrameRate", fmt.Sprintf("%.3f", sampleRate/1024))
		}
	}
	if hasAC3 {
		if ac3.spf > 0 {
			structuredFacts.SetSame("SamplesPerFrame", strconv.Itoa(ac3.spf))
		}
		if ac3.frameRate > 0 {
			structuredFacts.SetSame("FrameRate", fmt.Sprintf("%.3f", ac3.frameRate))
		}
		if code := ac3ServiceKindCode(ac3.bsmod); code != "" {
			structuredFacts.SetSame("ServiceKind", code)
		}
		applyAVIAC3Text(builder, ac3)
		extraFields := matroskaAC3CanonicalExtraFields(&matroskaAudioProbe{format: "AC-3"}, ac3, false, "")
		if len(extraFields) > 0 {
			node := structuredObjectFromKVs(extraFields)
			builder.StructuredNode("extra", node)
		}
	}

	videoPackets := float64(0)
	for _, video := range streams {
		if video.kind == StreamVideo && video.packetCount > 0 {
			videoPackets = float64(video.packetCount)
			break
		}
	}
	if st.packetCount > 0 && videoPackets > 0 && videoFrameRate > 0 {
		ratio := videoPackets / float64(st.packetCount)
		structuredFacts.SetSame("Interleave_VideoFrames", fmt.Sprintf("%.2f", ratio))
		structuredFacts.SetSame("Interleave_Duration", fmt.Sprintf("%.3f", ratio/videoFrameRate))
		if audioFirstBytes > 0 && st.audioAvgBps > 0 {
			preload := math.Floor(float64(audioFirstBytes)/float64(st.audioAvgBps)*1000) / 1000
			structuredFacts.SetSame("Interleave_Preload", fmt.Sprintf("%.3f", preload))
		}
	}
	structuredFacts.SetSame("Alignment", aviAudioAlignment(st))
	delay := 0.0
	if st.audioAvgBps > 0 {
		switch {
		case hasMP3:
			delay = aviMP3DelaySeconds(st, mp3Offset)
		case hasAC3:
			delay = float64(firstAC3SyncOffset(audioData, ac3Offset)) / float64(st.audioAvgBps)
		}
	}
	delayRaw := fmt.Sprintf("%.3f", delay)
	structuredFacts.SetSame("Delay", delayRaw)
	structuredFacts.SetSame("Delay_Source", "Stream")
	structuredFacts.SetSame("Video_Delay", delayRaw)
	if st.title != "" {
		builder.Fill("Title", st.title, "Title", st.title)
	}
	if lame.library != "" {
		structuredFacts.SetSame("Encoded_Library", lame.library)
	}
	if lame.settings != "" {
		structuredFacts.SetSame("Encoded_Library_Settings", lame.settings)
	}
	structuredFacts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipComputed: true})
}

func parseAVIHDRL(data []byte, main *aviMainHeader, streams *[]*aviStream) {
	parseRIFFChunks(data, func(id string, payload []byte) {
		switch id {
		case "avih":
			if len(payload) < 40 {
				return
			}
			main.microSecPerFrame = binary.LittleEndian.Uint32(payload[0:4])
			main.maxBytesPerSec = binary.LittleEndian.Uint32(payload[4:8])
			main.flags = binary.LittleEndian.Uint32(payload[12:16])
			main.totalFrames = binary.LittleEndian.Uint32(payload[16:20])
			main.streams = binary.LittleEndian.Uint32(payload[24:28])
			main.width = binary.LittleEndian.Uint32(payload[32:36])
			main.height = binary.LittleEndian.Uint32(payload[36:40])
		case "LIST":
			if len(payload) < 4 {
				return
			}
			listType := string(payload[0:4])
			switch listType {
			case "strl":
				stream := parseAVIStrl(payload[4:], len(*streams))
				if stream != nil {
					*streams = append(*streams, stream)
				}
			case "odml":
				parseRIFFChunks(payload[4:], func(id string, data []byte) {
					if id == "dmlh" && len(data) >= 4 {
						main.totalFrames = binary.LittleEndian.Uint32(data[:4])
					}
				})
			}
		}
	})
}

func parseAVIStrl(data []byte, index int) *aviStream {
	stream := &aviStream{index: index}
	parseRIFFChunks(data, func(id string, payload []byte) {
		switch id {
		case "strh":
			parseAVIStrh(payload, stream)
		case "strf":
			parseAVIStrf(payload, stream)
		case "indx":
			parseAVISuperIndex(payload, stream)
		case "strn":
			stream.title = decodeAVIText(payload)
		}
	})
	if stream.kind == "" {
		return nil
	}
	return stream
}

// parseAVISuperIndex accumulates OpenDML time spans from AVI_INDEX_OF_INDEXES
// entries. RIFF uses this duration with higher priority than the strh float32
// clock whenever an indx duration is available.
func parseAVISuperIndex(payload []byte, stream *aviStream) {
	if stream == nil || len(payload) < 24 {
		return
	}
	indexSubType := payload[2]
	indexType := payload[3]
	if indexType != 0 || (indexSubType != 0 && indexSubType != 1) {
		return
	}
	entryCount := int(binary.LittleEndian.Uint32(payload[4:8]))
	if entryCount > (len(payload)-24)/16 {
		entryCount = (len(payload) - 24) / 16
	}
	for i := 0; i < entryCount; i++ {
		entry := 24 + i*16
		stream.indxDuration += uint64(binary.LittleEndian.Uint32(payload[entry+12 : entry+16]))
	}
}

func parseAVIStrh(payload []byte, stream *aviStream) {
	if len(payload) < 56 {
		return
	}
	fccType := string(payload[0:4])
	fccHandler := string(payload[4:8])
	stream.handler = fccHandler
	stream.scale = binary.LittleEndian.Uint32(payload[20:24])
	stream.rate = binary.LittleEndian.Uint32(payload[24:28])
	stream.start = binary.LittleEndian.Uint32(payload[28:32])
	stream.length = binary.LittleEndian.Uint32(payload[32:36])
	stream.suggestedBuf = binary.LittleEndian.Uint32(payload[36:40])
	stream.sampleSize = binary.LittleEndian.Uint32(payload[44:48])
	switch fccType {
	case "vids":
		stream.kind = StreamVideo
		stream.handler = strings.ToUpper(strings.TrimSpace(stream.handler))
	case "auds":
		stream.kind = StreamAudio
	case "txts":
		stream.kind = StreamText
	}
}

func parseAVIStrf(payload []byte, stream *aviStream) {
	if len(payload) < 16 {
		return
	}
	if stream.kind == StreamVideo {
		if len(payload) < 40 {
			return
		}
		stream.width = binary.LittleEndian.Uint32(payload[4:8])
		stream.height = binary.LittleEndian.Uint32(payload[8:12])
		stream.bitCount = binary.LittleEndian.Uint16(payload[14:16])
		compression := binary.LittleEndian.Uint32(payload[16:20])
		stream.compression = strings.ToUpper(fourCC(compression))
		return
	}
	if stream.kind == StreamAudio {
		// WAVEFORMATEX.
		stream.audioTag = binary.LittleEndian.Uint16(payload[0:2])
		stream.audioChans = binary.LittleEndian.Uint16(payload[2:4])
		stream.audioRate = binary.LittleEndian.Uint32(payload[4:8])
		stream.audioAvgBps = binary.LittleEndian.Uint32(payload[8:12])
		stream.audioAlign = binary.LittleEndian.Uint16(payload[12:14])
		stream.audioBits = binary.LittleEndian.Uint16(payload[14:16])
		if len(payload) >= 18 {
			extraSize := int(binary.LittleEndian.Uint16(payload[16:18]))
			extraSize = min(extraSize, len(payload)-18)
			if extraSize > 0 {
				stream.audioExtra = append([]byte(nil), payload[18:18+extraSize]...)
			}
		}
	}
}

func parseAVIINFO(data []byte) string {
	var writingApp string
	parseRIFFChunks(data, func(id string, payload []byte) {
		if id != "ISFT" {
			return
		}
		writingApp = decodeAVIText(payload)
	})
	return writingApp
}

type aviIndexStats struct {
	interleaved     string
	audioFirstBytes uint64
	recordLists     bool
	first00         uint64
	second00        uint64
	first01         uint64
	second01        uint64
	hasFirst00      bool
	hasSecond00     bool
	hasFirst01      bool
	hasSecond01     bool
	seenFirstVideo  bool
}

// parseAVIMovi walks one movi list, including nested rec lists, and optionally
// accumulates payload sizes and packet counts when no complete legacy index is available.
// parseAVIMovi scans AVI media chunks, updating per-stream frame/index stats
// and optionally collecting bounded video payload bytes for codec probing.
func parseAVIMovi(file io.ReadSeeker, start, end int64, streams []*aviStream, videoData *[]byte, maxScanBytes int64, collectBytes bool, stats *aviIndexStats) {
	const aviScanChunk = 256 << 10
	scanBuf := make([]byte, aviScanChunk)
	var walk func(int64, int64, int)
	walk = func(listStart, listEnd int64, depth int) {
		if depth > maxAVINestingDepth {
			return
		}
		for offset := listStart; offset+8 <= listEnd; {
			if maxScanBytes > 0 && offset-start >= maxScanBytes {
				return
			}
			var header [8]byte
			if _, err := readAt(file, offset, header[:]); err != nil {
				return
			}
			chunkID := string(header[0:4])
			chunkSize := int64(binary.LittleEndian.Uint32(header[4:8]))
			dataStart := offset + 8
			dataEnd := dataStart + chunkSize
			if dataEnd < dataStart || dataEnd > listEnd {
				return
			}
			if chunkID == "LIST" && chunkSize >= 4 {
				var listType [4]byte
				if _, err := readAt(file, dataStart, listType[:]); err != nil {
					return
				}
				if string(listType[:]) == "rec " {
					if stats != nil {
						stats.recordLists = true
					}
					walk(dataStart+4, dataEnd, depth+1)
				}
			} else if index, ok := parseAVIStreamIndex(chunkID); ok && index >= 0 && index < len(streams) {
				stream := streams[index]
				if !aviChunkMatchesStream(chunkID, stream.kind) {
					offset = dataEnd + chunkSize%2
					continue
				}
				if collectBytes {
					if stream.kind == StreamAudio && stream.packetCount == 0 && stats != nil && !stats.seenFirstVideo {
						stream.delayBytes = uint32(min(chunkSize, int64(^uint32(0))))
					}
					stream.bytes += uint64(chunkSize)
					stream.paddingBytes += uint64(chunkSize % 2)
					stream.packetCount++
					if stats != nil {
						noteAVIIndexEntry(stats, chunkID, uint64(offset-start), uint64(chunkSize), stream.kind)
					}
				}
				if stream.kind == StreamVideo && chunkSize > 0 {
					needVOP := stream.vopScanned < aviMaxVOPScan
					needVisual := videoData != nil && len(*videoData) < aviMaxVisualScan
					if needVOP || needVisual {
						remainingVisual := 0
						if needVisual {
							remainingVisual = aviMaxVisualScan - len(*videoData)
						}
						remainingChunk := chunkSize
						readPos := dataStart
						for remainingChunk > 0 && (needVOP || remainingVisual > 0) {
							readLen := min(int64(len(scanBuf)), remainingChunk)
							buf := scanBuf[:readLen]
							if _, err := readAt(file, readPos, buf); err != nil {
								break
							}
							if needVOP {
								feedLen := len(buf)
								remaining := aviMaxVOPScan - stream.vopScanned
								if feedLen > remaining {
									feedLen = remaining
								}
								if feedLen > 0 {
									stream.vopScan.feed(buf[:feedLen])
									stream.vopScanned += feedLen
								}
								if stream.vopScanned >= aviMaxVOPScan {
									needVOP = false
								}
							}
							if remainingVisual > 0 {
								take := min(len(buf), remainingVisual)
								*videoData = append(*videoData, buf[:take]...)
								remainingVisual -= take
							}
							remainingChunk -= readLen
							readPos += readLen
						}
					}
				} else if stream.kind == StreamAudio && chunkSize > 0 && len(stream.audioData) < aviMaxAudioScan {
					remaining := aviMaxAudioScan - len(stream.audioData)
					readLen := min(int64(remaining), chunkSize)
					if readLen > 0 {
						buf := make([]byte, readLen)
						if _, err := readAt(file, dataStart, buf); err == nil {
							stream.audioData = append(stream.audioData, buf...)
						}
					}
				}
			}
			offset = dataEnd + chunkSize%2
		}
	}
	walk(start, end, 0)
}

// aviChunkMatchesStream reports whether an AVI chunk identifier belongs to the
// requested audio or video stream kind.
func aviChunkMatchesStream(id string, kind StreamKind) bool {
	if len(id) != 4 {
		return false
	}
	suffix := id[2:]
	switch kind {
	case StreamVideo:
		return suffix == "dc" || suffix == "db"
	case StreamAudio:
		return suffix == "wb"
	case StreamText:
		return suffix == "tx"
	case StreamGeneral, StreamImage, StreamMenu:
		return false
	default:
		return false
	}
}

func parseAVIIndex(data []byte, streams []*aviStream) (bool, aviIndexStats) {
	found := false
	pos := 0
	stats := aviIndexStats{}
	for pos+16 <= len(data) {
		id := string(data[pos : pos+4])
		if index, ok := parseAVIStreamIndex(id); ok {
			if index >= 0 && index < len(streams) {
				offset := binary.LittleEndian.Uint32(data[pos+8 : pos+12])
				size := binary.LittleEndian.Uint32(data[pos+12 : pos+16])
				if streams[index].kind == StreamAudio && streams[index].packetCount == 0 && !stats.seenFirstVideo {
					streams[index].delayBytes = size
				}
				streams[index].bytes += uint64(size)
				streams[index].paddingBytes += uint64(size % 2)
				suffix := ""
				if len(id) == 4 {
					suffix = id[2:4]
				}
				switch streams[index].kind {
				case StreamVideo:
					if suffix == "dc" || suffix == "db" {
						streams[index].packetCount++
					}
				case StreamAudio:
					if suffix == "wb" {
						streams[index].packetCount++
					}
				}
				noteAVIIndexEntry(&stats, id, uint64(offset), uint64(size), streams[index].kind)
				found = true
			}
		}
		pos += 16
	}
	finalizeAVIIndexStats(&stats)
	finalizeAVIPayloadStats(streams)
	return found, stats
}

// noteAVIIndexEntry records the packet positions needed for interleave and
// preload decisions without retaining the full AVI index.
func noteAVIIndexEntry(stats *aviIndexStats, id string, offset, size uint64, kind StreamKind) {
	if stats == nil || len(id) != 4 {
		return
	}
	switch id[:2] {
	case "00":
		if !stats.hasFirst00 {
			stats.first00, stats.hasFirst00 = offset, true
		} else if !stats.hasSecond00 {
			stats.second00, stats.hasSecond00 = offset, true
		}
	case "01":
		if !stats.hasFirst01 {
			stats.first01, stats.hasFirst01 = offset, true
		} else if !stats.hasSecond01 {
			stats.second01, stats.hasSecond01 = offset, true
		}
	}
	if stats.seenFirstVideo {
		return
	}
	if kind == StreamVideo && (id[2:] == "dc" || id[2:] == "db") {
		stats.seenFirstVideo = true
	} else if kind == StreamAudio {
		stats.audioFirstBytes += size
	}
}

// finalizeAVIIndexStats derives MediaInfo's coarse interleaved decision from
// the first two packet positions of streams zero and one.
func finalizeAVIIndexStats(stats *aviIndexStats) {
	if stats == nil || !stats.hasFirst00 || !stats.hasSecond00 || !stats.hasFirst01 || !stats.hasSecond01 {
		return
	}
	if (stats.first00 < stats.first01 && stats.second00 > stats.first01) || (stats.first01 < stats.first00 && stats.second01 > stats.first00) {
		stats.interleaved = "Yes"
	} else {
		stats.interleaved = "No"
	}
}

// resetAVIPayloadStats clears index-derived counters before a complete OpenDML
// movi traversal replaces a partial legacy index.
func resetAVIPayloadStats(streams []*aviStream) {
	for _, stream := range streams {
		stream.bytes = 0
		stream.paddingBytes = 0
		stream.packetCount = 0
		stream.delayBytes = 0
	}
}

// finalizeAVIPayloadStats uses counted video chunks as the authoritative frame
// count while preserving audio stream-header length semantics.
func finalizeAVIPayloadStats(streams []*aviStream) {
	for _, stream := range streams {
		if stream.kind == StreamVideo && stream.packetCount > 0 {
			stream.length = stream.packetCount
		}
	}
}

func aviStreamDuration(stream *aviStream, main aviMainHeader) float64 {
	if stream.rate > 0 && stream.scale > 0 && stream.length > 0 {
		if stream.indxDuration > 0 {
			return float64(stream.indxDuration) * float64(stream.scale) / float64(stream.rate)
		}
		return float64(aviStreamDurationMilliseconds(stream)) / 1000
	}
	if main.microSecPerFrame > 0 && main.totalFrames > 0 {
		return float64(main.microSecPerFrame) * float64(main.totalFrames) / 1e6
	}
	return 0
}

// aviStreamDurationMilliseconds mirrors MediaInfoLib's AVI strh clock path.
// RIFF normalizes common frame rates and performs the duration calculation in
// float32 before rounding to integer milliseconds.
func aviStreamDurationMilliseconds(stream *aviStream) int64 {
	if stream == nil || stream.rate == 0 || stream.scale == 0 || stream.length == 0 {
		return 0
	}
	frameRate := float32(stream.rate) / float32(stream.scale)
	if frameRate > 1 {
		rest := frameRate - float32(uint32(frameRate))
		switch {
		case rest < 0.01:
			frameRate -= rest
		case rest > 0.99:
			frameRate += 1 - rest
		default:
			rate1001 := frameRate * 1001 / 1000
			rest1001 := rate1001 - float32(uint32(rate1001))
			if rest1001 < 0.001 {
				frameRate = float32(uint32(rate1001)) * 1000 / 1001
			}
			if rest1001 > 0.999 {
				frameRate = float32(uint32(rate1001)+1) * 1000 / 1001
			}
		}
	}
	if frameRate == 0 {
		return 0
	}
	duration := (float32(1000) * float32(stream.length)) / frameRate
	return int64(math.Round(float64(duration)))
}

// aviTruncatedChunkExtra reports a RIFF element whose declared payload extends
// beyond the physical file. All values come from the chunk header and file
// boundary; no file identity participates.
func aviTruncatedChunkExtra(chunkID string, fileSize, dataStart, dataEnd int64) *structuredNode {
	if dataStart < 0 || dataEnd <= fileSize || dataEnd <= dataStart {
		return nil
	}
	availableElementSize := max(int64(0), fileSize-dataStart)
	declaredElementSize := dataEnd - dataStart
	percent := float64(fileSize) * 100 / float64(dataEnd)
	message := fmt.Sprintf(
		"File size is less than expected size (actual %d %.4f%%, expected %d, offset 0x%X) / Element size is more than maximal permitted size (actual %d, expected %d, offset 0x%X)",
		fileSize, percent, dataEnd, dataStart, declaredElementSize, availableElementSize, dataStart,
	)
	nameBytes := make([]byte, 0, len(chunkID))
	for i := 0; i < len(chunkID); i++ {
		if chunkID[i] >= 0x20 && chunkID[i] <= 0x7E {
			nameBytes = append(nameBytes, chunkID[i])
		}
	}
	name := strings.TrimSpace(string(nameBytes))
	if name == "" {
		name = "RIFF"
	}
	general := structuredNode{Kind: structuredObject, Object: []structuredMember{{
		Key: "GeneralCompliance", Value: structuredNode{Kind: structuredString, Text: message},
	}}}
	details := structuredNode{Kind: structuredArray, Array: []structuredNode{general}}
	group := structuredNode{Kind: structuredObject, Object: []structuredMember{{Key: name, Value: details}}}
	errors := structuredNode{Kind: structuredArray, Array: []structuredNode{group}}
	extra := structuredNode{Kind: structuredObject, Object: []structuredMember{
		{Key: "IsTruncated", Value: structuredNode{Kind: structuredString, Text: "Yes"}},
		{Key: "ConformanceErrors", Value: errors},
	}}
	return &extra
}

func aviAudioDurationSeconds(stream *aviStream) float64 {
	if stream.bytes == 0 {
		return 0
	}
	if stream.audioAvgBps > 0 {
		bps := float64(stream.audioAvgBps) * 8
		if bps > 0 {
			return (float64(stream.bytes) * 8) / bps
		}
	}
	return 0
}

func aviFrameRate(stream *aviStream) float64 {
	if stream.rate > 0 && stream.scale > 0 {
		return float64(stream.rate) / float64(stream.scale)
	}
	return 0
}

func aviReportedFrameRateRatio(stream *aviStream, durationSeconds float64) (uint32, uint32, float64) {
	return aviConventionalFrameRateRatio(stream)
}

// aviConventionalFrameRateRatio maps AVI's often approximate integer
// timebases to the conventional rational reported by MediaInfo.
func aviConventionalFrameRateRatio(stream *aviStream) (uint32, uint32, float64) {
	rate := aviFrameRate(stream)
	for _, candidate := range []struct {
		numerator   uint32
		denominator uint32
	}{
		{24000, 1001},
		{30000, 1001},
		{60000, 1001},
		{24, 1},
		{25, 1},
		{30, 1},
		{50, 1},
		{60, 1},
	} {
		candidateRate := float64(candidate.numerator) / float64(candidate.denominator)
		if math.Abs(rate-candidateRate) < 0.002 {
			return candidate.numerator, candidate.denominator, candidateRate
		}
	}
	return stream.rate, stream.scale, rate
}

func firstAVIFrameRate(streams []*aviStream) float64 {
	for _, st := range streams {
		if st.kind == StreamVideo {
			if rate := aviFrameRate(st); rate > 0 {
				return rate
			}
		}
	}
	return 0
}

func firstAVIReportedFrameRate(streams []*aviStream, main aviMainHeader) float64 {
	for _, st := range streams {
		if st.kind != StreamVideo {
			continue
		}
		dur := aviStreamDuration(st, main)
		_, _, rate := aviReportedFrameRateRatio(st, dur)
		if rate > 0 {
			return rate
		}
	}
	return 0
}

func mapAVICompression(stream *aviStream) string {
	code := stream.handler
	if code == "" {
		code = stream.compression
	}
	switch code {
	case "FMP4", "MP4V", "DIVX", "XVID", "DX50":
		return "MPEG-4 Visual"
	case "H264", "AVC1":
		return "AVC"
	case "MJPG":
		return "Motion JPEG"
	default:
		return code
	}
}

// formatAVIAspectRatio reports square-pixel AVI display geometry while using
// conventional ratio names only for exact 4:3 and 16:9 pictures.
func formatAVIAspectRatio(ratio float64) string {
	switch {
	case math.Abs(ratio-4.0/3.0) < 0.0005:
		return "4:3"
	case math.Abs(ratio-16.0/9.0) < 0.0005:
		return "16:9"
	default:
		return fmt.Sprintf("%.3f", ratio)
	}
}

// divxLibraryRecord stores the display version and release date associated
// with one DivX build identifier.
type divxLibraryRecord struct {
	version string
	date    string
}

// divxLibraryRecords contains MediaInfo-compatible mappings from DivX build
// identifiers embedded in MPEG-4 user data to released versions and dates.
var divxLibraryRecords = map[string]divxLibraryRecord{
	"830":  {version: "5.0.5", date: "2003-04-24"},
	"1025": {version: "5.1.1 Beta2", date: "2003-11"},
	"1338": {version: "5.2.1 (DrDivX 106)", date: "2004-09-08"},
	"1571": {version: "6.0.0", date: "2005-06-15"},
	"1988": {version: "6.2.5", date: "2006-07"},
	"2207": {version: "6.5.1", date: "2007-03"},
	"2432": {version: "6.7.0", date: "2007-09-20"},
	"2816": {version: "6.8.5", date: "2009-08-20"},
}

// divxLibraryVersionDate maps DivX build identifiers embedded in MPEG-4 user
// data through the MediaInfo DivX library database subset used by this parser.
func divxLibraryVersionDate(library string) (string, string, bool) {
	const prefix = "DivX503b"
	if !strings.HasPrefix(library, prefix) {
		return "", "", false
	}
	build := strings.TrimSuffix(strings.TrimPrefix(library, prefix), "p")
	record, ok := divxLibraryRecords[build]
	return record.version, record.date, ok
}

// aviLAMEInfo contains Xing counts and LAME encoder facts recovered from a
// bounded AVI MPEG-audio payload probe.
type aviLAMEInfo struct {
	library       string
	settings      string
	targetBitRate int64
	frameCount    uint32
	vbrBytes      uint32
	variable      bool
	tagged        bool
}

// parseAVILAMEInfo extracts the fixed-width LAME tag identity and its bounded
// method, low-pass, and target-bitrate settings from an AVI audio probe.
func parseAVILAMEInfo(data []byte) aviLAMEInfo {
	index := bytes.Index(data, []byte("LAME"))
	if index < 0 || index+9 > len(data) {
		return aviLAMEInfo{}
	}
	version := data[index : index+9]
	for len(version) > 4 && version[len(version)-1] == 0 {
		version = version[:len(version)-1]
	}
	for _, value := range version {
		if value < 0x20 || value > 0x7E {
			return aviLAMEInfo{}
		}
	}
	result := aviLAMEInfo{library: string(version)}
	result.frameCount, result.vbrBytes, result.tagged = aviXingInfo(data[:index])
	if !result.tagged {
		return result
	}
	if index+21 > len(data) {
		return result
	}
	method := data[index+9] & 0x0F
	lowpass := int(data[index+10])
	target := int64(data[index+20]) * 1000
	result.targetBitRate = target
	result.variable = method >= 2
	if lowpass == 0 || target == 0 || (method != 1 && method != 2) {
		return result
	}
	mode := "-b"
	if method == 2 {
		mode = "--abr"
	}
	lowpassText := strconv.FormatFloat(float64(lowpass)/10, 'f', -1, 64)
	result.settings = fmt.Sprintf("-m j -V 4 -q 2 -lowpass %s %s %d", lowpassText, mode, target/1000)
	return result
}

// aviXingInfo reads the big-endian frame and byte totals from the Xing or Info
// header that precedes a LAME encoder tag.
func aviXingInfo(data []byte) (uint32, uint32, bool) {
	for _, marker := range [][]byte{[]byte("Xing"), []byte("Info")} {
		index := bytes.LastIndex(data, marker)
		if index < 0 || index+8 > len(data) {
			continue
		}
		flags := binary.BigEndian.Uint32(data[index+4 : index+8])
		cursor := index + 8
		var frames uint32
		var streamBytes uint32
		if flags&1 != 0 && cursor+4 <= len(data) {
			frames = binary.BigEndian.Uint32(data[cursor : cursor+4])
			cursor += 4
		}
		if flags&2 != 0 && cursor+4 <= len(data) {
			streamBytes = binary.BigEndian.Uint32(data[cursor : cursor+4])
		}
		return frames, streamBytes, true
	}
	return 0, 0, false
}

// aviMP3DelaySeconds derives AVI MPEG-audio delay from the validated sync
// offset and stream-header rate, matching MediaInfo's RIFF parser merge.
func aviMP3DelaySeconds(stream *aviStream, frameOffset int) float64 {
	if stream == nil || stream.rate == 0 {
		return 0
	}
	return float64(frameOffset) / float64(stream.rate)
}

// findFirstMP3HeaderAt returns the first validated MPEG audio header and its
// byte offset, which AVI uses to derive stream delay.
func findFirstMP3HeaderAt(data []byte) (mp3HeaderInfo, int, bool) {
	for index := 0; index+4 <= len(data); index++ {
		header, ok := parseMP3Header(data[index : index+4])
		if !ok {
			continue
		}
		frameLength := mp3FrameLengthBytes(header)
		if frameLength <= 0 || index+frameLength+4 > len(data) {
			continue
		}
		next, ok := parseMP3Header(data[index+frameLength : index+frameLength+4])
		if ok && next.versionID == header.versionID && next.layerID == header.layerID && next.sampleRate == header.sampleRate {
			return header, index, true
		}
	}
	return mp3HeaderInfo{}, 0, false
}

// aviAACObjectType reads the WAVEFORMATEX AudioSpecificConfig used by AVI AAC
// streams and returns zero when the private data is absent or malformed.
func aviAACObjectType(extra []byte) int {
	objectType, _, _, _, ok := parseAACAudioSpecificConfig(extra)
	if !ok {
		return 0
	}
	return objectType
}

// probeAVIAC3 finds the first AC-3 syncframe, then accumulates the same bounded
// 404-frame statistics window reported by MediaInfo for AVI.
func probeAVIAC3(data []byte) (ac3Info, int, bool) {
	var result ac3Info
	firstOffset := -1
	frames := 0
	for offset := 0; offset+7 <= len(data) && frames < aviAC3StatsFrames; {
		frame, frameSize, ok := parseAC3Frame(data[offset:])
		if !ok || frameSize <= 0 {
			offset++
			continue
		}
		if firstOffset < 0 {
			firstOffset = offset
		}
		result.mergeFrame(frame)
		frames++
		offset += frameSize
	}
	if firstOffset < 0 {
		return ac3Info{}, 0, false
	}
	return result, firstOffset, true
}

// firstAC3SyncOffset returns the earliest raw AC-3 syncword offset, falling
// back to the first fully validated frame when no earlier syncword is present.
func firstAC3SyncOffset(data []byte, validatedOffset int) int {
	if offset := bytes.Index(data, []byte{0x0B, 0x77}); offset >= 0 {
		return offset
	}
	return validatedOffset
}

// applyAVIAC3Text projects AC-3 frame metadata and histogram statistics into
// MediaInfo-compatible raw text fields while JSON retains their raw extras.
func applyAVIAC3Text(builder *canonicalStreamBuilder, info ac3Info) {
	if builder == nil {
		return
	}
	if info.serviceKind != "" {
		builder.Text("Service kind", info.serviceKind)
	}
	if info.hasDialnorm {
		builder.Text("Dialog Normalization", formatDialnorm(info.dialnorm))
	}
	if info.hasCompr {
		builder.Text("compr", formatCompr(info.comprDB))
	}
	if info.hasDynrng {
		builder.Text("dynrng", formatCompr(info.dynrngDB))
	}
	if info.hasCmixlev {
		builder.Text("cmixlev", fmt.Sprintf("%.1f dB", info.cmixlevDB))
	}
	if info.hasSurmixlev {
		builder.Text("surmixlev", fmt.Sprintf("%.0f dB", info.surmixlevDB))
	}
	if average, minimum, maximum, ok := info.dialnormStats(); ok {
		builder.Text("dialnorm_Average", formatDialnorm(average))
		builder.Text("dialnorm_Minimum", formatDialnorm(minimum))
		if maximum != minimum {
			builder.Text("dialnorm_Maximum", formatDialnorm(maximum))
		}
	}
	if average, minimum, maximum, count, ok := info.comprStats(); ok {
		builder.Text("compr_Average", fmt.Sprintf("%.2f dB", average))
		builder.Text("compr_Minimum", fmt.Sprintf("%.2f dB", minimum))
		builder.Text("compr_Maximum", fmt.Sprintf("%.2f dB", maximum))
		builder.Text("compr_Count", strconv.Itoa(count))
	}
	if average, minimum, maximum, count, ok := info.dynrngStats(); ok {
		builder.Text("dynrng_Average", fmt.Sprintf("%.2f dB", average))
		builder.Text("dynrng_Minimum", fmt.Sprintf("%.2f dB", minimum))
		builder.Text("dynrng_Maximum", fmt.Sprintf("%.2f dB", maximum))
		builder.Text("dynrng_Count", strconv.Itoa(count))
	}
}

// decodeAVIText converts RIFF's Windows-1252 metadata to UTF-8 while replacing
// undefined control bytes consistently with MediaInfo.
func decodeAVIText(data []byte) string {
	data = bytes.TrimRight(data, "\x00")
	runes := make([]rune, 0, len(data))
	cp1252 := [...]rune{
		'€', '\uFFFD', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\uFFFD', 'Ž', '\uFFFD',
		'\uFFFD', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\uFFFD', 'ž', 'Ÿ',
	}
	for _, value := range data {
		switch {
		case value < 0x80:
			runes = append(runes, rune(value))
		case value < 0xA0:
			runes = append(runes, cp1252[value-0x80])
		default:
			runes = append(runes, rune(value))
		}
	}
	return strings.TrimSpace(string(runes))
}

// aviJUNKEncodedLibrary recognizes VirtualDub's encoder identity stored in a
// top-level JUNK padding chunk.
func aviJUNKEncodedLibrary(data []byte) string {
	const prefix = "VirtualDub build "
	index := bytes.Index(data, []byte(prefix))
	if index < 0 {
		return ""
	}
	return decodeAVIText(data[index:])
}

// aviAudioAlignment maps AVI block/sample sizing to MediaInfo's aligned versus
// split audio classification.
func aviAudioAlignment(stream *aviStream) string {
	if stream != nil && stream.audioTag == 0x2000 {
		if stream.delayBytes == 0 {
			return "Aligned"
		}
		return "Split"
	}
	if stream != nil && (stream.audioAlign > 1 || stream.sampleSize > 1) {
		return "Aligned"
	}
	return "Split"
}

func aviGeneralEncodedLibrary(writingApp string) string {
	// MediaInfo: for some applications (e.g., VirtualDubMod), Encoded_Library is derived
	// from the Writing application string.
	writingApp = strings.TrimSpace(writingApp)
	if strings.HasPrefix(writingApp, "VirtualDubMod") {
		if i := strings.IndexByte(writingApp, '('); i >= 0 {
			if j := strings.IndexByte(writingApp[i+1:], ')'); j >= 0 {
				inside := strings.TrimSpace(writingApp[i+1 : i+1+j])
				if inside != "" {
					return "VirtualDubMod " + inside
				}
			}
		}
	}
	if strings.HasPrefix(writingApp, "VirtualDub build ") {
		return writingApp
	}
	return ""
}

func isMP3CBRBitrate(bps int64) bool {
	// Common Layer III bitrates (bps). Used for AVI MP3 mode detection.
	switch bps {
	case 8000, 16000, 24000, 32000, 40000, 48000, 56000, 64000, 80000, 96000, 112000, 128000, 144000, 160000, 192000, 224000, 256000, 320000:
		return true
	default:
		return false
	}
}

func findFirstMP3Header(data []byte) (mp3HeaderInfo, bool) {
	for i := 0; i+4 <= len(data); i++ {
		if h, ok := parseMP3Header(data[i : i+4]); ok {
			return h, true
		}
	}
	return mp3HeaderInfo{}, false
}

func parseAVIStreamIndex(id string) (int, bool) {
	if len(id) != 4 {
		return 0, false
	}
	if id[0] < '0' || id[0] > '9' || id[1] < '0' || id[1] > '9' {
		return 0, false
	}
	return int(id[0]-'0')*10 + int(id[1]-'0'), true
}

func parseRIFFChunks(data []byte, fn func(id string, payload []byte)) {
	pos := 0
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		end := start + size
		if end > len(data) {
			return
		}
		fn(id, data[start:end])
		if size%2 == 1 {
			end++
		}
		pos = end
	}
}

func readAt(file io.ReadSeeker, offset int64, buf []byte) (int, error) {
	if readerAt, ok := file.(io.ReaderAt); ok {
		return readAtReaderAt(readerAt, offset, buf)
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}
	return io.ReadFull(file, buf)
}

func readAtReaderAt(readerAt io.ReaderAt, offset int64, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := readerAt.ReadAt(buf[total:], offset+int64(total))
		total += n
		if err != nil {
			if err == io.EOF && total == len(buf) {
				return total, nil
			}
			return total, err
		}
		if n == 0 {
			return total, io.EOF
		}
	}
	return total, nil
}

func fourCC(value uint32) string {
	b := []byte{byte(value), byte(value >> 8), byte(value >> 16), byte(value >> 24)}
	return string(b)
}

func formatYesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func trimNullString(data []byte) string {
	for i, b := range data {
		if b == 0x00 {
			return string(data[:i])
		}
	}
	return string(data)
}
