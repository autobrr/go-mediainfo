package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
)

const (
	mkvIDEBML              = 0x1A45DFA3
	mkvIDSegment           = 0x18538067
	mkvIDInfo              = 0x1549A966
	mkvIDCluster           = 0x1F43B675
	mkvIDSegmentUID        = 0x73A4
	mkvIDTimecodeScale     = 0x2AD7B1
	mkvIDDuration          = 0x4489
	mkvIDMuxingApp         = 0x4D80
	mkvIDWritingApp        = 0x5741
	mkvIDErrorDetection    = 0x6BAA
	mkvIDTracks            = 0x1654AE6B
	mkvIDTags              = 0x1254C367
	mkvIDTag               = 0x7373
	mkvIDSimpleTag         = 0x67C8
	mkvIDTagName           = 0x45A3
	mkvIDTagString         = 0x4487
	mkvIDTrackEntry        = 0xAE
	mkvIDTrackNumber       = 0xD7
	mkvIDTrackUID          = 0x73C5
	mkvIDTrackType         = 0x83
	mkvIDTrackName         = 0x536E
	mkvIDTrackLanguage     = 0x22B59C
	mkvIDTrackLanguageIETF = 0x22B59D
	mkvIDTrackOffset       = 0x537F
	mkvIDCodecID           = 0x86
	mkvIDCodecPrivate      = 0x63A2
	mkvIDCodecName         = 0x258688
	mkvIDDefaultDuration   = 0x23E383
	mkvIDFlagDefault       = 0x88
	mkvIDFlagForced        = 0x55AA
	mkvIDTrackVideo        = 0xE0
	mkvIDTrackAudio        = 0xE1
	mkvIDBitRate           = 0x6264
	mkvIDPixelWidth        = 0xB0
	mkvIDPixelHeight       = 0xBA
	mkvIDDisplayWidth      = 0x54B0
	mkvIDDisplayHeight     = 0x54BA
	mkvIDDisplayUnit       = 0x54B2
	mkvIDAspectRatioType   = 0x54B3
	mkvIDPixelCropTop      = 0x54AA
	mkvIDPixelCropBottom   = 0x54BB
	mkvIDPixelCropLeft     = 0x54CC
	mkvIDPixelCropRight    = 0x54DD
	mkvIDColour            = 0x55B0
	mkvIDRange             = 0x55B9
	mkvIDColourPrimaries   = 0x55BB
	mkvIDTransferChar      = 0x55BA
	mkvIDMatrixCoeffs      = 0x55B3
	mkvIDSamplingRate      = 0xB5
	mkvIDChannels          = 0x9F
	mkvIDDocType           = 0x4282
	mkvIDDocTypeVersion    = 0x4287
	mkvIDTimecode          = 0xE7
	mkvIDSimpleBlock       = 0xA3
	mkvIDBlockGroup        = 0xA0
	mkvIDBlock             = 0xA1
	mkvIDBlockDuration     = 0x9B
	mkvIDCRC32             = 0xBF
	mkvMaxScan             = int64(4 << 20)
)

const matroskaEAC3QuickProbeFrames = 1113

type MatroskaInfo struct {
	Container     ContainerInfo
	General       []Field
	Tracks        []Stream
	SegmentOffset int64
	SegmentSize   int64
	TimecodeScale uint64
}

func ParseMatroska(r io.ReaderAt, size int64) (MatroskaInfo, bool) {
	return ParseMatroskaWithOptions(r, size, defaultAnalyzeOptions())
}

func ParseMatroskaWithOptions(r io.ReaderAt, size int64, opts AnalyzeOptions) (MatroskaInfo, bool) {
	opts = normalizeAnalyzeOptions(opts)
	scanSize := size
	if scanSize > mkvMaxScan {
		scanSize = mkvMaxScan
	}
	if scanSize <= 0 {
		return MatroskaInfo{}, false
	}

	buf := make([]byte, scanSize)
	if _, err := r.ReadAt(buf, 0); err != nil && err != io.EOF {
		return MatroskaInfo{}, false
	}

	info, ok := parseMatroska(buf)
	if !ok {
		return MatroskaInfo{}, false
	}
	if info.SegmentSize == 0 && info.SegmentOffset > 0 && size > info.SegmentOffset {
		info.SegmentSize = size - info.SegmentOffset
	}
	if info.SegmentOffset > 0 && info.SegmentSize > 0 && info.TimecodeScale > 0 {
		probes := map[uint64]*matroskaAudioProbe{}
		for _, stream := range info.Tracks {
			if stream.Kind != StreamAudio {
				continue
			}
			format := findField(stream.Fields, "Format")
			if format != "AC-3" && format != "E-AC-3" {
				continue
			}
			if id := streamTrackNumber(stream); id > 0 {
				probe := &matroskaAudioProbe{format: format}
				if format == "E-AC-3" {
					probe.collect = true
					if opts.ParseSpeed < 1 {
						probe.targetFrames = matroskaEAC3QuickProbeFrames
					}
				}
				probes[id] = probe
			}
		}
		applyStats := opts.ParseSpeed >= 1 || size > mkvMaxScan
		needsScan := applyStats || len(probes) > 0
		if needsScan {
			if stats, ok := scanMatroskaClusters(r, info.SegmentOffset, info.SegmentSize, info.TimecodeScale, probes); ok {
				if applyStats {
					applyMatroskaStats(&info, stats, size)
				}
				applyMatroskaAudioProbes(&info, probes)
			}
		}
	}
	return info, true
}

func parseMatroska(buf []byte) (MatroskaInfo, bool) {
	pos := 0
	var headerFields []Field
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDEBML {
			headerFields = parseMatroskaHeader(buf[dataStart:dataEnd])
		}
		if id == mkvIDSegment {
			if info, ok := parseMatroskaSegment(buf[dataStart:dataEnd]); ok {
				if len(headerFields) > 0 {
					info.General = append(headerFields, info.General...)
				}
				info.SegmentOffset = int64(dataStart)
				if size != unknownVintSize {
					info.SegmentSize = int64(size)
				}
				return info, true
			}
		}
		pos = dataEnd
	}
	return MatroskaInfo{}, false
}

func parseMatroskaSegment(buf []byte) (MatroskaInfo, bool) {
	info := MatroskaInfo{}
	var encoders []string
	var segmentFields []Field
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDInfo {
			if segInfo, ok := parseMatroskaInfo(buf[dataStart:dataEnd]); ok {
				info.Container.DurationSeconds = segInfo.Duration
				info.TimecodeScale = segInfo.TimecodeScale
				info.General = append(info.General, segInfo.Fields...)
			}
		}
		if id == mkvIDErrorDetection {
			if label := matroskaErrorDetectionLabel(buf[dataStart:dataEnd]); label != "" {
				segmentFields = append(segmentFields, Field{Name: "ErrorDetectionType", Value: label})
			}
		}
		if id == mkvIDTracks {
			if tracks, ok := parseMatroskaTracks(buf[dataStart:dataEnd], info.Container.DurationSeconds); ok {
				info.Tracks = append(info.Tracks, tracks...)
			}
		}
		if id == mkvIDTags {
			encoders = append(encoders, parseMatroskaEncoders(buf[dataStart:dataEnd])...)
		}
		pos = dataEnd
	}
	if len(encoders) > 0 && len(info.Tracks) > 0 {
		applyMatroskaEncoders(info.Tracks, encoders)
	}
	if len(segmentFields) > 0 {
		info.General = append(info.General, segmentFields...)
	}
	if findField(info.General, "ErrorDetectionType") == "" && matroskaHasCRC(buf) {
		info.General = append(info.General, Field{Name: "ErrorDetectionType", Value: "Per level 1"})
	}
	if info.Container.HasDuration() || len(info.Tracks) > 0 {
		return info, true
	}
	return MatroskaInfo{}, false
}

func parseMatroskaHeader(buf []byte) []Field {
	pos := 0
	var fields []Field
	var docTypeVersion uint64
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDDocTypeVersion {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				docTypeVersion = value
			}
		}
		pos = dataEnd
	}
	if docTypeVersion > 0 {
		fields = append(fields, Field{Name: "Format version", Value: fmt.Sprintf("Version %d", docTypeVersion)})
	}
	return fields
}

func formatSegmentUID(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	value := new(big.Int).SetBytes(payload)
	hex := fmt.Sprintf("%X", payload)
	return fmt.Sprintf("%s (0x%s)", value.String(), hex)
}

type matroskaSegmentInfo struct {
	Duration      float64
	TimecodeScale uint64
	Fields        []Field
}

func parseMatroskaInfo(buf []byte) (matroskaSegmentInfo, bool) {
	timecodeScale := uint64(1000000)
	var durationValue float64
	var hasDuration bool
	var fields []Field

	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		payload := buf[dataStart:dataEnd]
		switch id {
		case mkvIDTimecodeScale:
			if value, ok := readUnsigned(payload); ok {
				timecodeScale = value
			}
		case mkvIDDuration:
			if value, ok := readFloat(payload); ok {
				durationValue = value
				hasDuration = true
			}
		case mkvIDSegmentUID:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Unique ID", Value: formatSegmentUID(payload)})
			}
		case mkvIDWritingApp:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Writing application", Value: string(payload)})
			}
		case mkvIDMuxingApp:
			if len(payload) > 0 {
				fields = append(fields, Field{Name: "Writing library", Value: string(payload)})
			}
		case mkvIDErrorDetection:
			if label := matroskaErrorDetectionLabel(payload); label != "" {
				fields = append(fields, Field{Name: "ErrorDetectionType", Value: label})
			}
		}
		pos = dataEnd
	}

	if !hasDuration {
		return matroskaSegmentInfo{}, false
	}
	seconds := durationValue * float64(timecodeScale) / 1e9
	if seconds <= 0 {
		return matroskaSegmentInfo{}, false
	}
	return matroskaSegmentInfo{Duration: seconds, TimecodeScale: timecodeScale, Fields: fields}, true
}

func parseMatroskaTracks(buf []byte, segmentDuration float64) ([]Stream, bool) {
	entries := []Stream{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackEntry {
			if stream, ok := parseMatroskaTrackEntry(buf[dataStart:dataEnd], segmentDuration); ok {
				entries = append(entries, stream)
			}
		}
		pos = dataEnd
	}
	return entries, len(entries) > 0
}

func parseMatroskaTrackEntry(buf []byte, segmentDuration float64) (Stream, bool) {
	pos := 0
	var trackType uint64
	var trackNumber uint64
	var trackUID uint64
	var trackName string
	var trackLanguage string
	var trackLanguageIETF string
	var trackOffset int64
	var hasTrackOffset bool
	var codecID string
	var codecPrivate []byte
	var codecName string
	var videoInfo matroskaVideoInfo
	var spsInfo h264SPSInfo
	var audioChannels uint64
	var audioSampleRate float64
	var defaultDuration uint64
	var bitRate uint64
	var flagDefault *bool
	var flagForced *bool
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTrackType {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackType = value
			}
		}
		if id == mkvIDTrackNumber {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackNumber = value
			}
		}
		if id == mkvIDTrackUID {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				trackUID = value
			}
		}
		if id == mkvIDTrackName {
			trackName = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDTrackLanguage {
			trackLanguage = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDTrackLanguageIETF {
			trackLanguageIETF = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDCodecID {
			codecID = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDCodecPrivate {
			codecPrivate = buf[dataStart:dataEnd]
		}
		if id == mkvIDCodecName {
			codecName = string(buf[dataStart:dataEnd])
		}
		if id == mkvIDFlagDefault {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				v := value != 0
				flagDefault = &v
			}
		}
		if id == mkvIDFlagForced {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				v := value != 0
				flagForced = &v
			}
		}
		if id == mkvIDTrackOffset {
			if value, ok := readSigned(buf[dataStart:dataEnd]); ok {
				trackOffset = value
				hasTrackOffset = true
			}
		}
		if id == mkvIDBitRate {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				bitRate = value
			} else if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				bitRate = uint64(math.Round(value))
			}
		}
		if id == mkvIDDefaultDuration {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				defaultDuration = value
			}
		}
		if id == mkvIDTrackVideo {
			videoInfo = parseMatroskaVideo(buf[dataStart:dataEnd])
		}
		if id == mkvIDTrackAudio {
			channels, sampleRate := parseMatroskaAudio(buf[dataStart:dataEnd])
			if channels > 0 {
				audioChannels = channels
			}
			if sampleRate > 0 {
				audioSampleRate = sampleRate
			}
		}
		pos = dataEnd
	}
	if trackLanguageIETF != "" {
		trackLanguage = trackLanguageIETF
	}
	kind, format := mapMatroskaCodecID(codecID, trackType)
	if kind == "" {
		return Stream{}, false
	}
	aacProfile := ""
	aacObjType := 0
	if kind == StreamAudio && format == "AAC" && len(codecPrivate) > 0 {
		aacProfile, aacObjType = parseAACProfileFromASC(codecPrivate)
		if aacProfile != "" {
			format = "AAC " + aacProfile
		}
		if codecID == "A_AAC" && aacObjType > 0 {
			codecID = fmt.Sprintf("A_AAC-%d", aacObjType)
		}
	}
	fields := []Field{{Name: "Format", Value: format}}
	if trackNumber > 0 {
		fields = append(fields, Field{Name: "ID", Value: fmt.Sprintf("%d", trackNumber)})
	}
	if codecID != "" {
		fields = append(fields, Field{Name: "Codec ID", Value: codecID})
	}
	if codecID == "S_TEXT/UTF8" {
		fields = append(fields, Field{Name: "Codec ID/Info", Value: "UTF-8 Plain Text"})
	}
	if info := mapMatroskaFormatInfo(format); info != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: info})
	}
	if kind == StreamAudio && format == "E-AC-3" {
		fields = append(fields, Field{Name: "Commercial name", Value: "Dolby Digital Plus"})
	}
	if kind == StreamAudio && aacProfile == "LC" {
		fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
	}
	if kind == StreamVideo && codecID == "V_MPEG4/ISO/AVC" && len(codecPrivate) > 0 {
		_, avcFields, avcInfo := parseAVCConfig(codecPrivate)
		fields = append(fields, avcFields...)
		spsInfo = avcInfo
	}
	if spsInfo.CodedWidth > 0 {
		videoInfo.codedWidth = spsInfo.CodedWidth
	}
	if spsInfo.CodedHeight > 0 {
		videoInfo.codedHeight = spsInfo.CodedHeight
	}
	if videoInfo.colorRange == "" && spsInfo.HasColorRange {
		videoInfo.colorRange = spsInfo.ColorRange
		videoInfo.colorRangeSource = "Stream"
	}
	if videoInfo.colorPrimaries == "" && spsInfo.ColorPrimaries != "" {
		videoInfo.colorPrimaries = spsInfo.ColorPrimaries
		videoInfo.colorPrimariesSource = "Stream"
	}
	if videoInfo.transferCharacteristics == "" && spsInfo.TransferCharacteristics != "" {
		videoInfo.transferCharacteristics = spsInfo.TransferCharacteristics
		videoInfo.transferSource = "Stream"
	}
	if videoInfo.matrixCoefficients == "" && spsInfo.MatrixCoefficients != "" {
		videoInfo.matrixCoefficients = spsInfo.MatrixCoefficients
		videoInfo.matrixSource = "Stream"
	}
	if bitRate == 0 && spsInfo.HasBitRate && spsInfo.BitRate > 0 {
		bitRate = uint64(spsInfo.BitRate)
	}
	if kind == StreamVideo {
		storedWidth := videoInfo.pixelWidth
		storedHeight := videoInfo.pixelHeight
		if videoInfo.codedWidth > 0 {
			storedWidth = videoInfo.codedWidth
		}
		if videoInfo.codedHeight > 0 {
			storedHeight = videoInfo.codedHeight
		}
		displayWidth := videoInfo.displayWidth
		displayHeight := videoInfo.displayHeight
		if displayWidth == 0 && storedWidth > 0 {
			crop := videoInfo.cropLeft + videoInfo.cropRight
			if crop > 0 && crop < storedWidth {
				displayWidth = storedWidth - crop
			} else {
				displayWidth = storedWidth
			}
		}
		if displayHeight == 0 && storedHeight > 0 {
			crop := videoInfo.cropTop + videoInfo.cropBottom
			if crop > 0 && crop < storedHeight {
				displayHeight = storedHeight - crop
			} else {
				displayHeight = storedHeight
			}
		}
		if displayWidth > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(displayWidth)})
		}
		if displayHeight > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(displayHeight)})
		}
		aspectW := displayWidth
		aspectH := displayHeight
		if aspectW == 0 {
			aspectW = storedWidth
		}
		if aspectH == 0 {
			aspectH = storedHeight
		}
		if ar := formatAspectRatio(aspectW, aspectH); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
		if defaultDuration > 0 {
			rate := 1e9 / float64(defaultDuration)
			fields = append(fields, Field{Name: "Frame rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRateWithRatio(rate)})
			if segmentDuration > 0 {
				frameDuration := float64(defaultDuration) / 1e9
				frameCount := math.Floor(segmentDuration / frameDuration)
				if frameCount > 0 {
					fields = addStreamDuration(fields, frameCount*frameDuration)
				}
			}
		}
		if bitRate > 0 {
			nominalKbps := int64(math.Round(float64(bitRate) / 1000))
			fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrateKbps(nominalKbps)})
			if defaultDuration > 0 && displayWidth > 0 && displayHeight > 0 {
				rate := 1e9 / float64(defaultDuration)
				if bits := formatBitsPerPixelFrame(float64(bitRate), displayWidth, displayHeight, rate); bits != "" {
					fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
				}
			}
		}
		if videoInfo.colorRange != "" && findField(fields, "Color range") == "" {
			fields = append(fields, Field{Name: "Color range", Value: videoInfo.colorRange})
		}
		if videoInfo.colorPrimaries != "" && findField(fields, "Color primaries") == "" {
			fields = append(fields, Field{Name: "Color primaries", Value: videoInfo.colorPrimaries})
		}
		if videoInfo.transferCharacteristics != "" && findField(fields, "Transfer characteristics") == "" {
			fields = append(fields, Field{Name: "Transfer characteristics", Value: videoInfo.transferCharacteristics})
		}
		if videoInfo.matrixCoefficients != "" && findField(fields, "Matrix coefficients") == "" {
			fields = append(fields, Field{Name: "Matrix coefficients", Value: videoInfo.matrixCoefficients})
		}
		if findField(fields, "Color space") == "" && (videoInfo.colorRange != "" || videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "") {
			if matroskaHasStreamColor(videoInfo) {
				fields = append(fields, Field{Name: "Color space", Value: "YUV"})
			}
		}
	}
	if kind == StreamAudio {
		if audioChannels > 0 {
			fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(audioChannels)})
			if layout := channelLayout(audioChannels); layout != "" {
				fields = append(fields, Field{Name: "Channel layout", Value: layout})
			}
		}
		if audioSampleRate > 0 {
			fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(audioSampleRate)})
			if format == "AAC LC" {
				frameRate := audioSampleRate / 1024.0
				fields = append(fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%.3f FPS (1024 SPF)", frameRate)})
			}
		}
		if bitRate > 0 {
			fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Bit rate", Value: formatBitrate(float64(bitRate))})
		}
		if segmentDuration > 0 {
			fields = addStreamDuration(fields, segmentDuration)
		}
		if format == "AAC LC" {
			fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
		}
		if codecName != "" && strings.Contains(codecName, "Lavc") {
			fields = append(fields, Field{Name: "Writing library", Value: codecName})
		}
	}
	defaultValue := true
	if flagDefault != nil {
		defaultValue = *flagDefault
	}
	if defaultValue {
		fields = append(fields, Field{Name: "Default", Value: "Yes"})
	} else {
		fields = append(fields, Field{Name: "Default", Value: "No"})
	}
	forcedValue := false
	if flagForced != nil {
		forcedValue = *flagForced
	}
	if forcedValue {
		fields = append(fields, Field{Name: "Forced", Value: "Yes"})
	} else {
		fields = append(fields, Field{Name: "Forced", Value: "No"})
	}
	languageCode := normalizeLanguageCode(trackLanguage)
	if language := formatLanguage(trackLanguage); language != "" {
		fields = insertFieldBefore(fields, Field{Name: "Language", Value: language}, "Default")
	}
	if trackName != "" {
		before := "Default"
		if languageCode != "" {
			before = "Language"
		}
		fields = insertFieldBefore(fields, Field{Name: "Title", Value: trackName}, before)
	}
	jsonExtras := map[string]string{}
	if trackUID > 0 {
		jsonExtras["UniqueID"] = fmt.Sprintf("%d", trackUID)
	}
	if languageCode != "" {
		jsonExtras["Language"] = languageCode
	}
	delaySeconds := 0.0
	if hasTrackOffset {
		delaySeconds = float64(trackOffset) / 1e9
	}
	if kind == StreamVideo || kind == StreamAudio {
		delay := fmt.Sprintf("%.3f", delaySeconds)
		jsonExtras["Delay"] = delay
		jsonExtras["Delay_Source"] = "Container"
		if kind == StreamAudio {
			jsonExtras["Video_Delay"] = delay
		}
	}
	if kind == StreamVideo {
		storedWidth := videoInfo.pixelWidth
		storedHeight := videoInfo.pixelHeight
		displayWidth := videoInfo.displayWidth
		displayHeight := videoInfo.displayHeight
		if displayWidth == 0 && storedWidth > 0 {
			crop := videoInfo.cropLeft + videoInfo.cropRight
			if crop > 0 && crop < storedWidth {
				displayWidth = storedWidth - crop
			} else {
				displayWidth = storedWidth
			}
		}
		if displayHeight == 0 && storedHeight > 0 {
			crop := videoInfo.cropTop + videoInfo.cropBottom
			if crop > 0 && crop < storedHeight {
				displayHeight = storedHeight - crop
			} else {
				displayHeight = storedHeight
			}
		}
		if storedWidth > 0 && displayWidth > 0 && storedWidth != displayWidth {
			jsonExtras["Stored_Width"] = fmt.Sprintf("%d", storedWidth)
		}
		if storedHeight == displayHeight && displayHeight > 0 && codecID == "V_MPEG4/ISO/AVC" {
			if displayHeight%16 != 0 {
				storedHeight = ((displayHeight + 15) / 16) * 16
			}
		}
		if storedHeight > 0 && displayHeight > 0 && storedHeight != displayHeight {
			jsonExtras["Stored_Height"] = fmt.Sprintf("%d", storedHeight)
		}
		if spsInfo.HasFixedFrameRate && !spsInfo.FixedFrameRate {
			if findField(fields, "Frame rate mode") == "Constant" {
				jsonExtras["FrameRate_Mode_Original"] = "VFR"
			}
		}
		if videoInfo.colorRange != "" || videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "" {
			colorSource := "Container"
			if matroskaHasStreamColor(videoInfo) {
				colorSource = "Stream"
			}
			jsonExtras["colour_description_present"] = "Yes"
			jsonExtras["colour_description_present_Source"] = colorSource
			if videoInfo.colorRange != "" {
				jsonExtras["colour_range"] = videoInfo.colorRange
				jsonExtras["colour_range_Source"] = matroskaColorSource(videoInfo.colorRangeSource, colorSource)
			}
			if videoInfo.colorPrimaries != "" {
				jsonExtras["colour_primaries"] = videoInfo.colorPrimaries
				jsonExtras["colour_primaries_Source"] = matroskaColorSource(videoInfo.colorPrimariesSource, colorSource)
			}
			if videoInfo.transferCharacteristics != "" {
				jsonExtras["transfer_characteristics"] = videoInfo.transferCharacteristics
				jsonExtras["transfer_characteristics_Source"] = matroskaColorSource(videoInfo.transferSource, colorSource)
			}
			if videoInfo.matrixCoefficients != "" {
				jsonExtras["matrix_coefficients"] = videoInfo.matrixCoefficients
				jsonExtras["matrix_coefficients_Source"] = matroskaColorSource(videoInfo.matrixSource, colorSource)
			}
		}
		if spsInfo.HasBufferSize && spsInfo.BufferSize > 0 {
			jsonExtras["BufferSize"] = strconv.FormatInt(spsInfo.BufferSize, 10)
		}
	}
	durationSeconds := 0.0
	if kind == StreamVideo {
		if defaultDuration > 0 && segmentDuration > 0 {
			frameDuration := float64(defaultDuration) / 1e9
			frameCount := math.Floor(segmentDuration / frameDuration)
			if frameCount > 0 {
				durationSeconds = frameCount * frameDuration
			}
		}
		if durationSeconds == 0 && segmentDuration > 0 {
			durationSeconds = segmentDuration
		}
	}
	if kind == StreamAudio && segmentDuration > 0 {
		durationSeconds = segmentDuration
	}
	if durationSeconds > 0 {
		if kind == StreamVideo {
			durationSeconds = math.Ceil(durationSeconds*1000) / 1000
		}
		jsonExtras["Duration"] = fmt.Sprintf("%.9f", durationSeconds)
	}
	return Stream{Kind: kind, Fields: fields, JSON: jsonExtras}, true
}

type matroskaVideoInfo struct {
	pixelWidth              uint64
	pixelHeight             uint64
	displayWidth            uint64
	displayHeight           uint64
	codedWidth              uint64
	codedHeight             uint64
	cropTop                 uint64
	cropBottom              uint64
	cropLeft                uint64
	cropRight               uint64
	colorRange              string
	colorRangeSource        string
	colorPrimaries          string
	colorPrimariesSource    string
	transferCharacteristics string
	transferSource          string
	matrixCoefficients      string
	matrixSource            string
}

func parseMatroskaVideo(buf []byte) matroskaVideoInfo {
	info := matroskaVideoInfo{}
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDPixelWidth:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.pixelWidth = value
			}
		case mkvIDPixelHeight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.pixelHeight = value
			}
		case mkvIDDisplayWidth:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayWidth = value
			}
		case mkvIDDisplayHeight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayHeight = value
			}
		case mkvIDPixelCropTop:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropTop = value
			}
		case mkvIDPixelCropBottom:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropBottom = value
			}
		case mkvIDPixelCropLeft:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropLeft = value
			}
		case mkvIDPixelCropRight:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.cropRight = value
			}
		case mkvIDColour:
			rangeValue, primaries, transfer, matrix := parseMatroskaColour(buf[dataStart:dataEnd])
			if rangeValue != "" {
				info.colorRange = rangeValue
				info.colorRangeSource = "Container"
			}
			if primaries != "" {
				info.colorPrimaries = primaries
				info.colorPrimariesSource = "Container"
			}
			if transfer != "" {
				info.transferCharacteristics = transfer
				info.transferSource = "Container"
			}
			if matrix != "" {
				info.matrixCoefficients = matrix
				info.matrixSource = "Container"
			}
		}
		pos = dataEnd
	}
	return info
}

func parseMatroskaAudio(buf []byte) (uint64, float64) {
	pos := 0
	var channels uint64
	var sampleRate float64
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDChannels {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				channels = value
			}
		}
		if id == mkvIDSamplingRate {
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				sampleRate = value
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				sampleRate = float64(valueInt)
			}
		}
		pos = dataEnd
	}
	return channels, sampleRate
}

func parseMatroskaColour(buf []byte) (string, string, string, string) {
	pos := 0
	var rangeValue string
	var primaries string
	var transfer string
	var matrix string
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		switch id {
		case mkvIDRange:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				switch value {
				case 1:
					rangeValue = "Limited"
				case 2:
					rangeValue = "Full"
				}
			}
		case mkvIDColourPrimaries:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				primaries = matroskaColorName(value)
			}
		case mkvIDTransferChar:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				transfer = matroskaColorName(value)
			}
		case mkvIDMatrixCoeffs:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				matrix = matroskaColorName(value)
			}
		}
		pos = dataEnd
	}
	return rangeValue, primaries, transfer, matrix
}

func matroskaColorName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	default:
		return ""
	}
}

func matroskaErrorDetectionType(value uint64) string {
	switch value {
	case 1:
		return "Per level 1"
	case 2:
		return "Per level 2"
	case 3:
		return "Per level 3"
	default:
		return ""
	}
}

func matroskaErrorDetectionLabel(payload []byte) string {
	if value, ok := readUnsigned(payload); ok {
		if label := matroskaErrorDetectionType(value); label != "" {
			return label
		}
	}
	if len(payload) > 0 {
		return string(payload)
	}
	return ""
}

func matroskaHasCRC(buf []byte) bool {
	return matroskaScanForCRC(buf, 0, len(buf))
}

func matroskaScanForCRC(buf []byte, start int, end int) bool {
	pos := start
	for pos < end {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			return false
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			return false
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDCRC32 {
			return true
		}
		if matroskaIsMasterID(id) {
			if matroskaScanForCRC(buf, dataStart, dataEnd) {
				return true
			}
		}
		pos = dataEnd
	}
	return false
}

func matroskaIsMasterID(id uint64) bool {
	switch id {
	case mkvIDSegment, mkvIDInfo, mkvIDTracks, mkvIDTags, mkvIDTag, mkvIDSimpleTag,
		mkvIDTrackEntry, mkvIDTrackVideo, mkvIDTrackAudio, mkvIDColour, mkvIDCluster, mkvIDBlockGroup:
		return true
	default:
		return false
	}
}

func matroskaHasStreamColor(info matroskaVideoInfo) bool {
	return info.colorRangeSource == "Stream" ||
		info.colorPrimariesSource == "Stream" ||
		info.transferSource == "Stream" ||
		info.matrixSource == "Stream"
}

func matroskaColorSource(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func parseMatroskaEncoders(buf []byte) []string {
	var encoders []string
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDTag {
			encoders = append(encoders, parseMatroskaTagEncoders(buf[dataStart:dataEnd])...)
		}
		pos = dataEnd
	}
	return encoders
}

func parseMatroskaTagEncoders(buf []byte) []string {
	var encoders []string
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		if id == mkvIDSimpleTag {
			if encoder := parseMatroskaSimpleTagEncoder(buf[dataStart:dataEnd]); encoder != "" {
				encoders = append(encoders, encoder)
			}
		}
		pos = dataEnd
	}
	return encoders
}

func parseMatroskaSimpleTagEncoder(buf []byte) string {
	var name string
	var value string
	pos := 0
	for pos < len(buf) {
		id, idLen, ok := readVintID(buf, pos)
		if !ok {
			break
		}
		size, sizeLen, ok := readVintSize(buf, pos+idLen)
		if !ok {
			break
		}
		dataStart := pos + idLen + sizeLen
		dataEnd := dataStart + int(size)
		if size == unknownVintSize || dataEnd > len(buf) {
			dataEnd = len(buf)
		}
		payload := buf[dataStart:dataEnd]
		if id == mkvIDTagName {
			name = string(payload)
		}
		if id == mkvIDTagString {
			value = string(payload)
		}
		pos = dataEnd
	}
	if name == "ENCODER" && value != "" {
		return value
	}
	return ""
}

func applyMatroskaEncoders(streams []Stream, encoders []string) {
	if len(encoders) == 0 {
		return
	}
	audioEncoder := selectEncoder(encoders, "aac")
	if audioEncoder != "" {
		for i := range streams {
			if streams[i].Kind != StreamAudio {
				continue
			}
			if findField(streams[i].Fields, "Writing library") != "" {
				continue
			}
			streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: audioEncoder})
		}
	}
}

func selectEncoder(encoders []string, token string) string {
	token = strings.ToLower(token)
	for _, encoder := range encoders {
		lower := strings.ToLower(encoder)
		if strings.Contains(lower, token) {
			return encoder
		}
	}
	return ""
}

func parseAACProfileFromASC(payload []byte) (string, int) {
	if len(payload) == 0 {
		return "", 0
	}
	objType := int(payload[0] >> 3)
	if objType == 31 && len(payload) > 1 {
		objType = 32 + int((payload[0]&0x07)<<3|payload[1]>>5)
	}
	if objType <= 0 {
		return "", 0
	}
	return mapAACProfile(objType), objType
}

const unknownVintSize = ^uint64(0)

func readVintID(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	var value uint64
	for i := 0; i < length; i++ {
		value = (value << 8) | uint64(buf[pos+i])
	}
	return value, length, true
}

func readVintSize(buf []byte, pos int) (uint64, int, bool) {
	if pos >= len(buf) {
		return 0, 0, false
	}
	first := buf[pos]
	length := vintLength(first)
	if length == 0 || pos+length > len(buf) {
		return 0, 0, false
	}
	mask := byte(0xFF >> length)
	value := uint64(first & mask)
	for i := 1; i < length; i++ {
		value = (value << 8) | uint64(buf[pos+i])
	}
	if value == (uint64(1)<<(uint(length*7)))-1 {
		return unknownVintSize, length, true
	}
	return value, length, true
}

func vintLength(first byte) int {
	for i := 0; i < 8; i++ {
		if first&(1<<(7-uint(i))) != 0 {
			return i + 1
		}
	}
	return 0
}

func readUnsigned(buf []byte) (uint64, bool) {
	if len(buf) == 0 || len(buf) > 8 {
		return 0, false
	}
	var value uint64
	for _, b := range buf {
		value = (value << 8) | uint64(b)
	}
	return value, true
}

func readSigned(buf []byte) (int64, bool) {
	if len(buf) == 0 || len(buf) > 8 {
		return 0, false
	}
	var value int64
	for _, b := range buf {
		value = (value << 8) | int64(b)
	}
	if buf[0]&0x80 != 0 {
		value -= 1 << (uint(len(buf)) * 8)
	}
	return value, true
}

func readFloat(buf []byte) (float64, bool) {
	if len(buf) == 4 {
		bits := binary.BigEndian.Uint32(buf)
		return float64(math.Float32frombits(bits)), true
	}
	if len(buf) == 8 {
		bits := binary.BigEndian.Uint64(buf)
		return math.Float64frombits(bits), true
	}
	return 0, false
}
