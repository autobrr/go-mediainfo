package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"
)

const (
	mkvIDEBML            = 0x1A45DFA3
	mkvIDSegment         = 0x18538067
	mkvIDInfo            = 0x1549A966
	mkvIDSegmentUID      = 0x73A4
	mkvIDTimecodeScale   = 0x2AD7B1
	mkvIDDuration        = 0x4489
	mkvIDMuxingApp       = 0x4D80
	mkvIDWritingApp      = 0x5741
	mkvIDTracks          = 0x1654AE6B
	mkvIDTags            = 0x1254C367
	mkvIDTag             = 0x7373
	mkvIDSimpleTag       = 0x67C8
	mkvIDTagName         = 0x45A3
	mkvIDTagString       = 0x4487
	mkvIDTrackEntry      = 0xAE
	mkvIDTrackNumber     = 0xD7
	mkvIDTrackType       = 0x83
	mkvIDCodecID         = 0x86
	mkvIDCodecPrivate    = 0x63A2
	mkvIDCodecName       = 0x258688
	mkvIDDefaultDuration = 0x23E383
	mkvIDFlagDefault     = 0x88
	mkvIDFlagForced      = 0x55AA
	mkvIDTrackVideo      = 0xE0
	mkvIDTrackAudio      = 0xE1
	mkvIDBitRate         = 0x6264
	mkvIDPixelWidth      = 0xB0
	mkvIDPixelHeight     = 0xBA
	mkvIDDisplayWidth    = 0x54B0
	mkvIDDisplayHeight   = 0x54BA
	mkvIDColour          = 0x55B0
	mkvIDRange           = 0x55B9
	mkvIDSamplingRate    = 0xB5
	mkvIDChannels        = 0x9F
	mkvIDDocType         = 0x4282
	mkvIDDocTypeVersion  = 0x4287
	mkvMaxScan           = int64(4 << 20)
)

type MatroskaInfo struct {
	Container ContainerInfo
	General   []Field
	Tracks    []Stream
}

func ParseMatroska(r io.ReaderAt, size int64) (MatroskaInfo, bool) {
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
				info.General = append(info.General, segInfo.Fields...)
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
	Duration float64
	Fields   []Field
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
	if findField(fields, "ErrorDetectionType") == "" {
		fields = append(fields, Field{Name: "ErrorDetectionType", Value: "Per level 1"})
	}
	return matroskaSegmentInfo{Duration: seconds, Fields: fields}, true
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
	var codecID string
	var codecPrivate []byte
	var codecName string
	var videoWidth uint64
	var videoHeight uint64
	var displayWidth uint64
	var displayHeight uint64
	var colorRange string
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
		if id == mkvIDBitRate {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				bitRate = value
			}
		}
		if id == mkvIDDefaultDuration {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				defaultDuration = value
			}
		}
		if id == mkvIDTrackVideo {
			width, height, displayW, displayH, rangeValue := parseMatroskaVideo(buf[dataStart:dataEnd])
			if width > 0 {
				videoWidth = width
			}
			if height > 0 {
				videoHeight = height
			}
			if displayW > 0 {
				displayWidth = displayW
			}
			if displayH > 0 {
				displayHeight = displayH
			}
			if rangeValue != "" {
				colorRange = rangeValue
			}
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
	if info := mapMatroskaFormatInfo(format); info != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: info})
	}
	if kind == StreamAudio && aacProfile == "LC" {
		fields = append(fields, Field{Name: "Format/Info", Value: "Advanced Audio Codec Low Complexity"})
	}
	if kind == StreamVideo && codecID == "V_MPEG4/ISO/AVC" && len(codecPrivate) > 0 {
		_, avcFields := parseAVCConfig(codecPrivate)
		fields = append(fields, avcFields...)
	}
	if kind == StreamVideo {
		if videoWidth > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(videoWidth)})
		}
		if videoHeight > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(videoHeight)})
		}
		aspectW := videoWidth
		aspectH := videoHeight
		if displayWidth > 0 {
			aspectW = displayWidth
		}
		if displayHeight > 0 {
			aspectH = displayHeight
		}
		if ar := formatAspectRatio(aspectW, aspectH); ar != "" {
			fields = append(fields, Field{Name: "Display aspect ratio", Value: ar})
		}
		if defaultDuration > 0 {
			rate := 1e9 / float64(defaultDuration)
			fields = append(fields, Field{Name: "Frame rate mode", Value: "Constant"})
			fields = append(fields, Field{Name: "Frame rate", Value: formatFrameRate(rate)})
			if segmentDuration > 0 {
				frameDuration := float64(defaultDuration) / 1e9
				frameCount := math.Floor(segmentDuration / frameDuration)
				if frameCount > 0 {
					fields = addStreamDuration(fields, frameCount*frameDuration)
				}
			}
		}
		if bitRate > 0 {
			fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(float64(bitRate))})
			if defaultDuration > 0 && videoWidth > 0 && videoHeight > 0 {
				rate := 1e9 / float64(defaultDuration)
				if bits := formatBitsPerPixelFrame(float64(bitRate), videoWidth, videoHeight, rate); bits != "" {
					fields = append(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
				}
			}
		}
		if colorRange != "" {
			fields = append(fields, Field{Name: "Color range", Value: colorRange})
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
	return Stream{Kind: kind, Fields: fields}, true
}

func parseMatroskaVideo(buf []byte) (uint64, uint64, uint64, uint64, string) {
	pos := 0
	var width uint64
	var height uint64
	var displayWidth uint64
	var displayHeight uint64
	var colorRange string
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
		if id == mkvIDPixelWidth {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				width = value
			}
		}
		if id == mkvIDPixelHeight {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				height = value
			}
		}
		if id == mkvIDDisplayWidth {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				displayWidth = value
			}
		}
		if id == mkvIDDisplayHeight {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				displayHeight = value
			}
		}
		if id == mkvIDColour {
			if value := parseMatroskaColorRange(buf[dataStart:dataEnd]); value != "" {
				colorRange = value
			}
		}
		pos = dataEnd
	}
	return width, height, displayWidth, displayHeight, colorRange
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

func parseMatroskaColorRange(buf []byte) string {
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
		if id == mkvIDRange {
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				switch value {
				case 1:
					return "Limited"
				case 2:
					return "Full"
				}
			}
		}
		pos = dataEnd
	}
	return ""
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
