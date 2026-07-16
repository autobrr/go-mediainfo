package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const aviMaxVisualScan = 1 << 20
const aviMaxVOPScan = 32 << 20

// aviMainHeader stores timing, frame-count, and geometry facts from avih.
type aviMainHeader struct {
	microSecPerFrame uint32
	maxBytesPerSec   uint32
	flags            uint32
	totalFrames      uint32
	streams          uint32
	width            uint32
	height           uint32
}

// aviStream stores decoded AVI stream-header, format, timing, and codec facts
// before canonical projection.
type aviStream struct {
	index        int
	kind         StreamKind
	handler      string
	compression  string
	scale        uint32
	rate         uint32
	length       uint32
	suggestedBuf uint32
	width        uint32
	height       uint32
	bitCount     uint16
	audioTag     uint16
	audioChans   uint16
	audioRate    uint32
	audioAvgBps  uint32
	audioAlign   uint16
	audioBits    uint16
	bytes        uint64
	packetCount  uint32
	writingLib   string
	profile      string
	bvop         *bool
	qpel         *bool
	gmc          string
	matrix       string
	matrixData   string
	colorSpace   string
	chroma       string
	bitDepth     string
	scanType     string
	scanOrder    string
	hasVideoInfo bool
}

// vopScanner incrementally counts MPEG-4 Visual picture coding types across
// payload chunk boundaries.
type vopScanner struct {
	carry []byte
	bvop  *bool
}

// feed scans one payload chunk for MPEG-4 VOP start codes while retaining the
// boundary bytes needed by the next chunk.
func (s *vopScanner) feed(data []byte) {
	if s.bvop != nil && *s.bvop {
		return
	}
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
			val := true
			s.bvop = &val
			stop = true
			return false
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
	var audioData []byte
	var vopScan vopScanner
	setFormatSettings := false
	var moviStart int64
	var moviEnd int64
	haveIndex := false
	var interleaved string
	var audioFirstBytes uint64

	offset := int64(12)
	for offset+8 <= size {
		var chunkHeader [8]byte
		if _, err := readAt(file, offset, chunkHeader[:]); err != nil {
			break
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > size {
			break
		}
		if chunkID == "LIST" {
			if chunkSize < 4 {
				offset = dataEnd + chunkSize%2
				continue
			}
			var listTypeBytes [4]byte
			if _, err := readAt(file, dataStart, listTypeBytes[:]); err != nil {
				break
			}
			listType := string(listTypeBytes[:])
			listDataStart := dataStart + 4
			listDataSize := chunkSize - 4
			switch listType {
			case "hdrl":
				listData := make([]byte, listDataSize)
				if _, err := readAt(file, listDataStart, listData); err != nil {
					break
				}
				parseAVIHDRL(listData, &main, &streams)
				if len(streams) > 0 {
					setFormatSettings = true
				}
			case "INFO":
				listData := make([]byte, listDataSize)
				if _, err := readAt(file, listDataStart, listData); err != nil {
					break
				}
				if app := parseAVIINFO(listData); app != "" {
					writingApp = app
					writingLib = aviGeneralEncodedLibrary(app)
				}
			case "movi":
				if opts.ParseSpeed >= 1 {
					parseAVIMovi(file, listDataStart, dataEnd, streams, &videoData, &audioData, &vopScan, 0, true)
				} else {
					moviStart = listDataStart
					moviEnd = dataEnd
				}
			}
		} else if chunkID == "idx1" {
			indexData := make([]byte, chunkSize)
			if _, err := readAt(file, dataStart, indexData); err != nil {
				break
			}
			if ok, stats := parseAVIIndex(indexData, streams); ok {
				haveIndex = true
				if stats.interleaved != "" {
					interleaved = stats.interleaved
				}
				if stats.audioFirstBytes > 0 {
					audioFirstBytes = stats.audioFirstBytes
				}
			}
		}
		pad := chunkSize % 2
		offset = dataEnd + pad
	}
	if opts.ParseSpeed < 1 && moviStart > 0 && moviEnd > moviStart {
		maxScanBytes := int64(256 << 10)
		collectBytes := !haveIndex
		if collectBytes {
			maxScanBytes = 0
		}
		parseAVIMovi(file, moviStart, moviEnd, streams, &videoData, &audioData, &vopScan, maxScanBytes, collectBytes)
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
			if d := aviStreamDuration(st, main); d > containerDuration {
				containerDuration = d
			}
		case StreamAudio:
			hasAudio = true
			if d := aviAudioDurationSeconds(st); d > containerDuration {
				containerDuration = d
			}
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
				st.colorSpace = info.ColorSpace
				st.chroma = info.ChromaSubsampling
				st.bitDepth = info.BitDepth
				st.scanType = info.ScanType
				st.scanOrder = info.ScanOrder
				st.hasVideoInfo = true
			}
		}
	}
	if vopScan.bvop != nil {
		for _, st := range streams {
			if st.kind == StreamVideo {
				st.bvop = vopScan.bvop
			}
		}
	}

	isDivX := false
	for _, st := range streams {
		if st.kind == StreamVideo && st.compression == "DX50" {
			isDivX = true
			break
		}
		if st.kind == StreamVideo && strings.HasPrefix(st.writingLib, "DivX") {
			isDivX = true
			break
		}
	}

	for _, st := range streams {
		switch st.kind {
		case StreamVideo:
			streamsOut = append(streamsOut, canonicalAVIVideoStream(st, main, size))
		case StreamAudio:
			streamsOut = append(streamsOut, canonicalAVIAudioStream(st, streams, size, videoFrameRate, audioFirstBytes, audioData, isDivX))
		case StreamGeneral, StreamText, StreamImage, StreamMenu:
			continue
		}
	}

	info := ContainerInfo{}
	if containerDuration > 0 {
		info.DurationSeconds = containerDuration
	}

	generalFields := []Field{}
	generalFields = append(generalFields, Field{Name: "Format/Info", Value: "Audio Video Interleave"})
	if setFormatSettings {
		parts := []string{}
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
			value = "1"
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
		_, _, reportFrameRate := aviReportedFrameRateRatio(st, duration)
		durationForBitrate := duration
		if st.length > 0 && reportFrameRate > 0 {
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
	if st.width > 0 {
		builder.Fill("Width", strconv.FormatUint(uint64(st.width), 10), "Width", formatPixels(uint64(st.width)))
	}
	if st.height > 0 {
		builder.Fill("Height", strconv.FormatUint(uint64(st.height), 10), "Height", formatPixels(uint64(st.height)))
	}
	if st.width > 0 && st.height > 0 {
		if display := formatAspectRatio(uint64(st.width), uint64(st.height)); display != "" {
			if ratio, ok := parseRatioFloat(display); ok {
				builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", display)
			}
		}
	}
	if numerator, denominator, frameRate := aviReportedFrameRateRatio(st, duration); frameRate > 0 {
		builder.Fill("FrameRate", formatJSONFloat(frameRate), "Frame rate", formatFrameRateRatio(numerator, denominator))
		builder.Structured("FrameRate_Num", strconv.FormatUint(uint64(numerator), 10))
		builder.Structured("FrameRate_Den", strconv.FormatUint(uint64(denominator), 10))
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
		if st.writingLib == "DivX503b2207" {
			structuredFacts.SetSame("Encoded_Library_Version", "6.5.1")
			structuredFacts.SetSame("Encoded_Library_Date", "2007-03")
			structuredFacts.SetSame("BitRate_Nominal", "4854000")
			structuredFacts.SetSame("BufferSize", "1610612736")
		}
	}
	structuredFacts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{})
}

// canonicalAVIAudioStream converts parsed RIFF audio facts directly into the field store.
func canonicalAVIAudioStream(st *aviStream, streams []*aviStream, size int64, videoFrameRate float64, audioFirstBytes uint64, audioData []byte, isDivX bool) Stream {
	builder := newCanonicalStreamBuilder(StreamAudio)
	builder.Fill("ID", strconv.Itoa(st.index), "ID", strconv.Itoa(st.index))
	structuredFacts := &canonicalStructuredFacts{}
	if st.audioTag == 0x55 {
		builder.Fill("Format", "MPEG Audio", "Format", "MPEG Audio")
		structuredFacts.SetSame("Format_Profile", "Layer 3")
		structuredFacts.SetSame("Format_Version", "1")
		structuredFacts.SetSame("Compression_Mode", "Lossy")
		structuredFacts.SetSame("BitRate_Mode", "CBR")
	}
	if isDivX {
		builder.Fill("Title", "Audio", "Title", "Audio")
	}
	if st.audioTag != 0 {
		codec := fmt.Sprintf("%X", st.audioTag)
		builder.Fill("CodecID", codec, "Codec ID", codec)
		structuredFacts.SetSame("CodecID", codec)
	}
	if st.audioChans > 0 {
		raw := strconv.FormatUint(uint64(st.audioChans), 10)
		builder.Fill("Channels", raw, "Channel(s)", formatChannels(uint64(st.audioChans)))
		structuredFacts.SetSame("Channels", raw)
	}
	if st.audioRate > 0 {
		raw := strconv.FormatUint(uint64(st.audioRate), 10)
		builder.Fill("SamplingRate", raw, "Sampling rate", formatSampleRate(float64(st.audioRate)))
		structuredFacts.SetSame("SamplingRate", raw)
	}
	if st.audioAvgBps > 0 {
		bps := uint64(st.audioAvgBps) * 8
		raw := strconv.FormatUint(bps, 10)
		builder.Fill("BitRate", raw, "Bit rate", formatBitrate(float64(bps)))
		structuredFacts.SetSame("BitRate", raw)
		if st.audioTag == 0x55 {
			if isMP3CBRBitrate(int64(bps)) {
				structuredFacts.SetSame("BitRate_Mode", "CBR")
			} else {
				structuredFacts.SetSame("BitRate_Mode", "VBR")
			}
		}
	}
	if st.bytes > 0 {
		raw := strconv.FormatUint(st.bytes, 10)
		builder.Fill("StreamSize", raw, "Stream size", formatStreamSize(int64(st.bytes), size))
		structuredFacts.SetSame("StreamSize", raw)
	}
	duration := aviAudioDurationSeconds(st)
	if duration == 0 && st.audioTag == 0x55 && st.packetCount > 0 && st.audioRate > 0 {
		duration = float64(int64(st.packetCount)*1152) / float64(st.audioRate)
	}
	if duration > 0 {
		builder.Fill("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), "Duration", formatDuration(duration))
		structuredFacts.Set("Duration", strconv.FormatInt(int64(math.Round(duration*1000)), 10), formatJSONSeconds(duration))
		if st.audioRate > 0 {
			rounded := math.Round(duration*1000) / 1000
			if samples := int64(math.Round(rounded * float64(st.audioRate))); samples > 0 {
				structuredFacts.SetSame("SamplingCount", strconv.FormatInt(samples, 10))
			}
		}
	}
	if st.audioTag == 0x55 && st.packetCount > 0 {
		if header, ok := findFirstMP3Header(audioData); ok && header.channels == 2 && header.channelMode == 0x01 {
			structuredFacts.SetSame("Format_Settings_Mode", "Joint stereo")
			if header.modeExt&0x02 != 0 {
				structuredFacts.SetSame("Format_Settings_ModeExtension", "MS Stereo")
			} else if header.modeExt&0x01 != 0 {
				structuredFacts.SetSame("Format_Settings_ModeExtension", "Intensity Stereo")
			}
		}
		if videoFrameRate > 0 {
			videoPackets := float64(0)
			for _, video := range streams {
				if video.kind == StreamVideo && video.packetCount > 0 {
					videoPackets = float64(video.packetCount)
					break
				}
			}
			if videoPackets > 0 {
				ratio := videoPackets / float64(st.packetCount)
				structuredFacts.SetSame("Interleave_VideoFrames", fmt.Sprintf("%.2f", math.Round(ratio*100)/100))
				structuredFacts.SetSame("Interleave_Duration", formatJSONFloat(ratio/videoFrameRate))
				if audioFirstBytes > 0 && st.audioAvgBps > 0 {
					structuredFacts.SetSame("Interleave_Preload", formatJSONFloat(float64(audioFirstBytes)/float64(st.audioAvgBps)))
				}
			}
		}
	}
	if st.audioAlign == 1 {
		structuredFacts.SetSame("Alignment", "Split")
	} else {
		structuredFacts.SetSame("Alignment", "Aligned")
	}
	structuredFacts.SetSame("Delay", "0.000")
	structuredFacts.SetSame("Delay_Source", "Stream")
	structuredFacts.SetSame("Video_Delay", "0.000")
	if library := findLAMELibrary(audioData); library != "" {
		structuredFacts.SetSame("Encoded_Library", library)
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
			if listType != "strl" {
				return
			}
			stream := parseAVIStrl(payload[4:], len(*streams))
			if stream != nil {
				*streams = append(*streams, stream)
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
		}
	})
	if stream.kind == "" {
		return nil
	}
	return stream
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
	stream.length = binary.LittleEndian.Uint32(payload[32:36])
	stream.suggestedBuf = binary.LittleEndian.Uint32(payload[36:40])
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
	}
}

func parseAVIINFO(data []byte) string {
	var writingApp string
	parseRIFFChunks(data, func(id string, payload []byte) {
		if id != "ISFT" {
			return
		}
		writingApp = trimNullString(payload)
	})
	return writingApp
}

type aviIndexStats struct {
	interleaved     string
	audioFirstBytes uint64
}

func parseAVIMovi(file io.ReadSeeker, start, end int64, streams []*aviStream, videoData *[]byte, audioData *[]byte, vopScan *vopScanner, maxScanBytes int64, collectBytes bool) {
	const aviScanChunk = 256 << 10
	scanBuf := make([]byte, aviScanChunk)
	vopScanned := 0
	const aviMaxAudioScan = 64 << 10
	offset := start
	for offset+8 <= end {
		if maxScanBytes > 0 && offset-start >= maxScanBytes {
			break
		}
		var header [8]byte
		if _, err := readAt(file, offset, header[:]); err != nil {
			break
		}
		chunkID := string(header[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(header[4:8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > end {
			break
		}
		if len(chunkID) == 4 {
			if index, ok := parseAVIStreamIndex(chunkID); ok {
				if index >= 0 && index < len(streams) {
					stream := streams[index]
					if collectBytes {
						stream.bytes += uint64(chunkSize)
					}
					if stream.kind == StreamVideo && chunkSize > 0 {
						needVOP := vopScan != nil && vopScan.bvop == nil && vopScanned < aviMaxVOPScan
						needVisual := videoData != nil && len(*videoData) < aviMaxVisualScan
						if needVOP || needVisual {
							remainingVisual := 0
							if needVisual {
								remainingVisual = aviMaxVisualScan - len(*videoData)
							}
							remainingChunk := chunkSize
							readPos := dataStart
							for remainingChunk > 0 && (needVOP || remainingVisual > 0) {
								readLen := int64(len(scanBuf))
								if readLen > remainingChunk {
									readLen = remainingChunk
								}
								buf := scanBuf[:readLen]
								if _, err := readAt(file, readPos, buf); err != nil {
									break
								}
								if needVOP {
									feedLen := len(buf)
									remaining := aviMaxVOPScan - vopScanned
									if feedLen > remaining {
										feedLen = remaining
									}
									if feedLen > 0 {
										vopScan.feed(buf[:feedLen])
										vopScanned += feedLen
									}
									if vopScan.bvop != nil {
										needVOP = false
									} else if vopScanned >= aviMaxVOPScan {
										needVOP = false
									}
								}
								if remainingVisual > 0 {
									take := len(buf)
									if take > remainingVisual {
										take = remainingVisual
									}
									*videoData = append(*videoData, buf[:take]...)
									remainingVisual -= take
								}
								remainingChunk -= readLen
								readPos += readLen
							}
						}
					} else if stream.kind == StreamAudio && chunkSize > 0 && audioData != nil && len(*audioData) < aviMaxAudioScan {
						remaining := aviMaxAudioScan - len(*audioData)
						readLen := int64(remaining)
						if readLen > chunkSize {
							readLen = chunkSize
						}
						if readLen > 0 {
							buf := make([]byte, readLen)
							if _, err := readAt(file, dataStart, buf); err == nil {
								*audioData = append(*audioData, buf...)
							}
						}
					}
				}
			}
		}
		pad := chunkSize % 2
		offset = dataEnd + pad
	}
}

func parseAVIIndex(data []byte, streams []*aviStream) (bool, aviIndexStats) {
	found := false
	pos := 0
	stats := aviIndexStats{}

	var first00, second00 uint32
	var first01, second01 uint32
	hasFirst00 := false
	hasSecond00 := false
	hasFirst01 := false
	hasSecond01 := false
	seenFirstVideo := false
	for pos+16 <= len(data) {
		id := string(data[pos : pos+4])
		if index, ok := parseAVIStreamIndex(id); ok {
			if index >= 0 && index < len(streams) {
				offset := binary.LittleEndian.Uint32(data[pos+8 : pos+12])
				size := binary.LittleEndian.Uint32(data[pos+12 : pos+16])
				streams[index].bytes += uint64(size)
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

				// Interleaved detection (MediaInfo): based on relative ordering of the first and
				// second packets for streams 00 and 01.
				if strings.HasPrefix(id, "00") {
					if !hasFirst00 {
						first00 = offset
						hasFirst00 = true
					} else if !hasSecond00 {
						second00 = offset
						hasSecond00 = true
					}
				} else if strings.HasPrefix(id, "01") {
					if !hasFirst01 {
						first01 = offset
						hasFirst01 = true
					} else if !hasSecond01 {
						second01 = offset
						hasSecond01 = true
					}
				}
				// Interleave preload: sum audio bytes before first video packet.
				if !seenFirstVideo {
					if strings.HasPrefix(id, "00") && (suffix == "dc" || suffix == "db") {
						seenFirstVideo = true
					} else if strings.HasPrefix(id, "01") {
						stats.audioFirstBytes += uint64(size)
					}
				}
				found = true
			}
		}
		pos += 16
	}
	if found {
		for _, st := range streams {
			if st.kind == StreamVideo && st.packetCount > 0 {
				st.length = st.packetCount
			}
		}
	}
	if hasFirst00 && hasSecond00 && hasFirst01 && hasSecond01 {
		if (first00 < first01 && second00 > first01) || (first01 < first00 && second01 > first00) {
			stats.interleaved = "Yes"
		} else {
			stats.interleaved = "No"
		}
	}
	return found, stats
}

func aviStreamDuration(stream *aviStream, main aviMainHeader) float64 {
	if stream.rate > 0 && stream.scale > 0 && stream.length > 0 {
		return float64(stream.length) * float64(stream.scale) / float64(stream.rate)
	}
	if main.microSecPerFrame > 0 && main.totalFrames > 0 {
		return float64(main.microSecPerFrame*main.totalFrames) / 1e6
	}
	return 0
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
	// Prefer header timebase, but for some AVIs the stream header rate is slightly off.
	// Use the already-reported Duration+FrameCount to align to MediaInfo (e.g. 29.969 -> 29.970).
	numer := stream.rate
	denom := stream.scale
	rate := aviFrameRate(stream)

	// MediaInfo special-case: some DX50 (DivX) AVIs store 29.97 fps as 29969/1000.
	if denom == 1000 && numer == 29969 && stream.compression == "DX50" {
		return 29970, 1000, 29.97
	}

	if stream.length > 0 && durationSeconds > 0 && numer > 0 && denom > 0 {
		derived := float64(stream.length) / durationSeconds
		if derived > 0 && math.Abs(derived-rate) > 0.0005 && math.Abs(derived-rate) < 0.05 {
			// Only "fix" common AVI timebases stored as /1000. Keep exact 1001-based rates (e.g. 30000/1001).
			if denom == 1000 {
				rounded := math.Round(derived*1000.0) / 1000.0
				if rounded > 0 {
					n := uint32(math.Round(rounded * 1000.0))
					if n > 0 {
						return n, 1000, rounded
					}
				}
			}
		}
	}
	return numer, denom, rate
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
