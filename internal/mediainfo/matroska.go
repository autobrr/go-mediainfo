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
	mkvIDChapters          = 0x1043A770
	mkvIDTag               = 0x7373
	mkvIDSimpleTag         = 0x67C8
	mkvIDTagName           = 0x45A3
	mkvIDTagString         = 0x4487
	mkvIDEditionEntry      = 0x45B9
	mkvIDChapterAtom       = 0xB6
	mkvIDChapterTimeStart  = 0x91
	mkvIDChapterDisplay    = 0x80
	mkvIDChapString        = 0x85
	mkvIDChapLanguage      = 0x437C
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
	mkvIDMasteringMetadata = 0x55D0
	mkvIDMasteringPrimRx   = 0x55D1
	mkvIDMasteringPrimRy   = 0x55D2
	mkvIDMasteringPrimGx   = 0x55D3
	mkvIDMasteringPrimGy   = 0x55D4
	mkvIDMasteringPrimBx   = 0x55D5
	mkvIDMasteringPrimBy   = 0x55D6
	mkvIDMasteringWhiteX   = 0x55D7
	mkvIDMasteringWhiteY   = 0x55D8
	mkvIDMasteringLumMax   = 0x55D9
	mkvIDMasteringLumMin   = 0x55DA
	mkvIDMaxCLL            = 0x55BC
	mkvIDMaxFALL           = 0x55BD
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
	scanSize := min(size, mkvMaxScan)
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
	if hdr := parseDolbyVisionConfigFromPrivate(buf); hdr != "" {
		for i := range info.Tracks {
			if info.Tracks[i].Kind == StreamVideo && findField(info.Tracks[i].Fields, "HDR format") == "" {
				info.Tracks[i].Fields = insertFieldBefore(info.Tracks[i].Fields, Field{Name: "HDR format", Value: hdr}, "Codec ID")
			}
		}
	}
	if info.SegmentSize == 0 && info.SegmentOffset > 0 && size > info.SegmentOffset {
		info.SegmentSize = size - info.SegmentOffset
	}
	if info.SegmentOffset > 0 && info.SegmentSize > 0 && info.TimecodeScale > 0 {
		audioProbes := map[uint64]*matroskaAudioProbe{}
		videoProbes := map[uint64]*matroskaVideoProbe{}
		for _, stream := range info.Tracks {
			if id := streamTrackNumber(stream); id > 0 {
				switch stream.Kind {
				case StreamAudio:
					format := findField(stream.Fields, "Format")
					if format != "AC-3" && format != "E-AC-3" {
						continue
					}
					probe := &matroskaAudioProbe{format: format}
					if format == "E-AC-3" {
						if stream.eac3Dec3.hasJOCComplex {
							probe.info.hasJOCComplex = true
							probe.info.jocComplexity = stream.eac3Dec3.jocComplexity
						}
						if stream.eac3Dec3.hasJOC {
							probe.info.hasJOC = true
						}
						probe.collect = true
						if opts.ParseSpeed < 1 {
							probe.targetFrames = matroskaEAC3QuickProbeFrames
						}
					}
					audioProbes[id] = probe
				case StreamVideo:
					format := findField(stream.Fields, "Format")
					if format == "HEVC" && stream.nalLengthSize > 0 {
						videoProbes[id] = &matroskaVideoProbe{
							codec:         format,
							nalLengthSize: stream.nalLengthSize,
						}
					}
				case StreamGeneral, StreamText, StreamImage, StreamMenu:
					continue
				}
			}
		}
		applyStats := opts.ParseSpeed >= 1 || size > mkvMaxScan
		needsScan := applyStats || len(audioProbes) > 0 || len(videoProbes) > 0
		if needsScan {
			if stats, ok := scanMatroskaClusters(r, info.SegmentOffset, info.SegmentSize, info.TimecodeScale, audioProbes, videoProbes, applyStats); ok {
				if applyStats {
					applyMatroskaStats(&info, stats, size)
				}
				applyMatroskaAudioProbes(&info, audioProbes)
				applyMatroskaVideoProbes(&info, videoProbes)
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
	var chaptersPayloads [][]byte
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
		if id == mkvIDChapters {
			chaptersPayloads = append(chaptersPayloads, buf[dataStart:dataEnd])
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
	if len(chaptersPayloads) > 0 {
		scale := info.TimecodeScale
		if scale == 0 {
			scale = 1000000
		}
		chapters := make([]matroskaChapter, 0, len(chaptersPayloads))
		for _, payload := range chaptersPayloads {
			chapters = append(chapters, parseMatroskaChapters(payload, scale)...)
		}
		if len(chapters) > 0 {
			menu := Stream{
				Kind:                StreamMenu,
				JSONRaw:             map[string]string{},
				JSONSkipStreamOrder: true,
				JSONSkipComputed:    true,
			}
			for i, chapter := range chapters {
				name := chapter.name
				if name == "" {
					name = fmt.Sprintf("Chapter %d", i+1)
				}
				if chapter.lang != "" {
					name = chapter.lang + ":" + name
				}
				menu.Fields = append(menu.Fields, Field{Name: formatMatroskaChapterTimeMs(chapter.startMs), Value: name})
			}
			menu.JSONRaw["extra"] = renderMatroskaMenuExtra(chapters)
			info.Tracks = append(info.Tracks, menu)
		}
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

type matroskaChapter struct {
	startMs int64
	name    string
	lang    string
}

func parseMatroskaChapters(buf []byte, timecodeScale uint64) []matroskaChapter {
	var chapters []matroskaChapter
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
		if id == mkvIDEditionEntry {
			chapters = append(chapters, parseMatroskaEditionEntry(buf[dataStart:dataEnd], timecodeScale)...)
		}
		pos = dataEnd
	}
	return chapters
}

func parseMatroskaEditionEntry(buf []byte, _ uint64) []matroskaChapter {
	var chapters []matroskaChapter
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
		if id == mkvIDChapterAtom {
			if chapter, ok := parseMatroskaChapterAtom(buf[dataStart:dataEnd]); ok {
				chapters = append(chapters, chapter)
			}
		}
		pos = dataEnd
	}
	return chapters
}

func parseMatroskaChapterAtom(buf []byte) (matroskaChapter, bool) {
	var chapter matroskaChapter
	var hasStart bool
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
		case mkvIDChapterTimeStart:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				chapter.startMs = int64(value) / 1_000_000
				hasStart = true
			}
		case mkvIDChapterDisplay:
			if name, lang := parseMatroskaChapterDisplay(buf[dataStart:dataEnd]); name != "" {
				chapter.name = name
				if chapter.lang == "" {
					chapter.lang = lang
				}
			}
		}
		pos = dataEnd
	}
	if hasStart {
		return chapter, true
	}
	return matroskaChapter{}, false
}

func parseMatroskaChapterDisplay(buf []byte) (string, string) {
	pos := 0
	var name string
	var lang string
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
		case mkvIDChapString:
			name = strings.TrimRight(string(buf[dataStart:dataEnd]), "\x00")
		case mkvIDChapLanguage:
			lang = normalizeLanguageCode(strings.TrimSpace(string(buf[dataStart:dataEnd])))
		}
		pos = dataEnd
	}
	return name, lang
}

func formatMatroskaChapterTimeMs(msTotal int64) string {
	if msTotal < 0 {
		msTotal = 0
	}
	h := msTotal / (3600 * 1000)
	msTotal -= h * 3600 * 1000
	m := msTotal / (60 * 1000)
	msTotal -= m * 60 * 1000
	s := msTotal / 1000
	ms := msTotal - s*1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func renderMatroskaMenuExtra(chapters []matroskaChapter) string {
	fields := make([]jsonKV, 0, len(chapters))
	for i, chapter := range chapters {
		name := chapter.name
		if name == "" {
			name = fmt.Sprintf("Chapter %d", i+1)
		}
		key := "_" + strings.NewReplacer(":", "_", ".", "_").Replace(formatMatroskaChapterTimeMs(chapter.startMs))
		fields = append(fields, jsonKV{Key: key, Val: name})
	}
	return renderJSONObject(fields, false)
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
	var trackBitRate bool
	var flagDefault *bool
	var flagForced *bool
	var nalLengthSize int
	var hdrFormat string
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
				trackBitRate = true
			} else if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				bitRate = uint64(math.Round(value))
				trackBitRate = true
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
	displayLanguage := trackLanguage
	if displayLanguage == "" {
		displayLanguage = trackLanguageIETF
	}
	kind, format := mapMatroskaCodecID(codecID, trackType)
	if kind == "" {
		return Stream{}, false
	}
	var dec3Info eac3Dec3Info
	if kind == StreamAudio && format == "E-AC-3" && len(codecPrivate) > 0 {
		if info, ok := parseEAC3Dec3(codecPrivate); ok {
			dec3Info = info
		}
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
		fields = append(fields, Field{Name: "ID", Value: strconv.FormatUint(trackNumber, 10)})
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
	if kind == StreamVideo && codecID == "V_MPEGH/ISO/HEVC" && len(codecPrivate) > 0 {
		_, hevcFields, hevcInfo := parseHEVCConfig(codecPrivate)
		fields = append(fields, hevcFields...)
		nalLengthSize = hevcInfo.nalLengthSize
		if dv := parseDolbyVisionConfigFromPrivate(codecPrivate); dv != "" {
			hdrFormat = dv
		}
		if hdrFormat == "" {
			if dv := parseDolbyVisionConfigFromPrivate(buf); dv != "" {
				hdrFormat = dv
			}
		}
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
		bitRateNominal := trackBitRate || (spsInfo.HasBitRateCBR && spsInfo.BitRateCBR)
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
		if videoInfo.displayUnit != 0 {
			displayWidth = 0
			displayHeight = 0
		}
		if storedWidth > 0 {
			fields = append(fields, Field{Name: "Width", Value: formatPixels(storedWidth)})
		}
		if storedHeight > 0 {
			fields = append(fields, Field{Name: "Height", Value: formatPixels(storedHeight)})
		}
		aspectW := storedWidth
		aspectH := storedHeight
		if displayWidth > 0 && displayHeight > 0 {
			aspectW = displayWidth
			aspectH = displayHeight
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
			if bitRateNominal {
				fields = append(fields, Field{Name: "Nominal bit rate", Value: formatBitrate(float64(bitRate))})
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Constant"})
			} else {
				fields = append(fields, Field{Name: "Maximum bit rate", Value: formatBitrate(float64(bitRate))})
				fields = append(fields, Field{Name: "Bit rate mode", Value: "Variable"})
			}
			if defaultDuration > 0 && storedWidth > 0 && storedHeight > 0 {
				rate := 1e9 / float64(defaultDuration)
				if bits := formatBitsPerPixelFrame(float64(bitRate), storedWidth, storedHeight, rate); bits != "" {
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
		if videoInfo.masteringPresent {
			if videoInfo.masteringPrimaries != "" && findField(fields, "Mastering display color primaries") == "" {
				fields = append(fields, Field{Name: "Mastering display color primaries", Value: videoInfo.masteringPrimaries})
			}
			if videoInfo.masteringLuminanceMax > 0 && videoInfo.masteringLuminanceMin > 0 && findField(fields, "Mastering display luminance") == "" {
				fields = append(fields, Field{Name: "Mastering display luminance", Value: formatMasteringLuminance(videoInfo.masteringLuminanceMin, videoInfo.masteringLuminanceMax)})
			}
		}
		if videoInfo.maxCLL > 0 && findField(fields, "Maximum Content Light Level") == "" {
			fields = append(fields, Field{Name: "Maximum Content Light Level", Value: fmt.Sprintf("%d cd/m2", videoInfo.maxCLL)})
		}
		if videoInfo.maxFALL > 0 && findField(fields, "Maximum Frame-Average Light Level") == "" {
			fields = append(fields, Field{Name: "Maximum Frame-Average Light Level", Value: fmt.Sprintf("%d cd/m2", videoInfo.maxFALL)})
		}
		if hdrFormat != "" && findField(fields, "HDR format") == "" {
			fields = insertFieldBefore(fields, Field{Name: "HDR format", Value: hdrFormat}, "Codec ID")
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
	languageCode := normalizeLanguageCode(trackLanguageIETF)
	if languageCode == "" {
		languageCode = normalizeLanguageCode(trackLanguage)
	}
	if language := formatLanguage(displayLanguage); language != "" {
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
		jsonExtras["UniqueID"] = strconv.FormatUint(trackUID, 10)
	}
	if bitRate > 0 {
		bitRateNominal := trackBitRate || (spsInfo.HasBitRateCBR && spsInfo.BitRateCBR)
		if bitRateNominal {
			jsonExtras["BitRate_Nominal"] = strconv.FormatUint(bitRate, 10)
		} else {
			jsonExtras["BitRate_Maximum"] = strconv.FormatUint(bitRate, 10)
		}
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
			jsonExtras["Stored_Width"] = strconv.FormatUint(storedWidth, 10)
		}
		if storedHeight == displayHeight && displayHeight > 0 && codecID == "V_MPEG4/ISO/AVC" {
			if displayHeight%16 != 0 {
				storedHeight = ((displayHeight + 15) / 16) * 16
			}
		}
		if storedHeight > 0 && displayHeight > 0 && storedHeight != displayHeight {
			jsonExtras["Stored_Height"] = strconv.FormatUint(storedHeight, 10)
		}
		if spsInfo.HasFixedFrameRate && !spsInfo.FixedFrameRate {
			if findField(fields, "Frame rate mode") == "Constant" {
				jsonExtras["FrameRate_Mode_Original"] = "VFR"
			}
		}
		if videoInfo.colorRange != "" || videoInfo.colorPrimaries != "" || videoInfo.transferCharacteristics != "" || videoInfo.matrixCoefficients != "" {
			colorSource := "Container"
			hasStream := matroskaHasStreamColor(videoInfo)
			hasContainer := matroskaHasContainerColor(videoInfo)
			if hasStream && hasContainer {
				colorSource = "Container / Stream"
			} else if hasStream {
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
	return Stream{Kind: kind, Fields: fields, JSON: jsonExtras, eac3Dec3: dec3Info, nalLengthSize: nalLengthSize}, true
}

type matroskaVideoInfo struct {
	pixelWidth              uint64
	pixelHeight             uint64
	displayWidth            uint64
	displayHeight           uint64
	displayUnit             uint64
	aspectRatioType         uint64
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
	masteringPrimaries      string
	masteringLuminanceMin   float64
	masteringLuminanceMax   float64
	masteringPresent        bool
	maxCLL                  uint64
	maxFALL                 uint64
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
		case mkvIDDisplayUnit:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.displayUnit = value
			}
		case mkvIDAspectRatioType:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.aspectRatioType = value
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
			colour := parseMatroskaColour(buf[dataStart:dataEnd])
			if colour.rangeValue != "" {
				info.colorRange = colour.rangeValue
				info.colorRangeSource = "Container"
			}
			if colour.primaries != "" {
				info.colorPrimaries = colour.primaries
				info.colorPrimariesSource = "Container"
			}
			if colour.transfer != "" {
				info.transferCharacteristics = colour.transfer
				info.transferSource = "Container"
			}
			if colour.matrix != "" {
				info.matrixCoefficients = colour.matrix
				info.matrixSource = "Container"
			}
			if colour.masteringPresent {
				info.masteringPresent = true
				info.masteringLuminanceMax = colour.masteringLuminanceMax
				info.masteringLuminanceMin = colour.masteringLuminanceMin
				info.masteringPrimaries = colour.masteringPrimaries
			}
			if colour.maxCLL > 0 {
				info.maxCLL = colour.maxCLL
			}
			if colour.maxFALL > 0 {
				info.maxFALL = colour.maxFALL
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

type matroskaColourInfo struct {
	rangeValue            string
	primaries             string
	transfer              string
	matrix                string
	masteringPrimaries    string
	masteringLuminanceMin float64
	masteringLuminanceMax float64
	masteringPresent      bool
	maxCLL                uint64
	maxFALL               uint64
}

func parseMatroskaColour(buf []byte) matroskaColourInfo {
	info := matroskaColourInfo{}
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
		case mkvIDRange:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				switch value {
				case 1:
					info.rangeValue = "Limited"
				case 2:
					info.rangeValue = "Full"
				}
			}
		case mkvIDColourPrimaries:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.primaries = matroskaColorPrimariesName(value)
			}
		case mkvIDTransferChar:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.transfer = matroskaTransferName(value)
			}
		case mkvIDMatrixCoeffs:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.matrix = matroskaMatrixName(value)
			}
		case mkvIDMasteringMetadata:
			mastering := parseMatroskaMasteringMetadata(buf[dataStart:dataEnd])
			if mastering.present {
				info.masteringPresent = true
				info.masteringLuminanceMax = mastering.luminanceMax
				info.masteringLuminanceMin = mastering.luminanceMin
			}
		case mkvIDMaxCLL:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.maxCLL = value
			}
		case mkvIDMaxFALL:
			if value, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.maxFALL = value
			}
		}
		pos = dataEnd
	}
	if info.masteringPresent && info.primaries != "" {
		info.masteringPrimaries = info.primaries
	}
	return info
}

type matroskaMasteringInfo struct {
	luminanceMin float64
	luminanceMax float64
	present      bool
}

func parseMatroskaMasteringMetadata(buf []byte) matroskaMasteringInfo {
	info := matroskaMasteringInfo{}
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
		case mkvIDMasteringLumMax:
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.luminanceMax = value
				info.present = true
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.luminanceMax = float64(valueInt)
				info.present = true
			}
		case mkvIDMasteringLumMin:
			if value, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.luminanceMin = value
				info.present = true
			} else if valueInt, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.luminanceMin = float64(valueInt)
				info.present = true
			}
		case mkvIDMasteringPrimRx, mkvIDMasteringPrimRy, mkvIDMasteringPrimGx, mkvIDMasteringPrimGy, mkvIDMasteringPrimBx, mkvIDMasteringPrimBy, mkvIDMasteringWhiteX, mkvIDMasteringWhiteY:
			if _, ok := readFloat(buf[dataStart:dataEnd]); ok {
				info.present = true
			} else if _, ok := readUnsigned(buf[dataStart:dataEnd]); ok {
				info.present = true
			}
		}
		pos = dataEnd
	}
	return info
}

func matroskaColorPrimariesName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470M"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "Film"
	case 9:
		return "BT.2020"
	case 10:
		return "SMPTE ST 428-1"
	default:
		return ""
	}
}

func matroskaTransferName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "BT.470M"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "Linear"
	case 9:
		return "Log"
	case 10:
		return "Log Sqrt"
	case 11:
		return "IEC 61966-2-4"
	case 12:
		return "BT.1361"
	case 13:
		return "IEC 61966-2-1"
	case 14:
		return "BT.2020 10-bit"
	case 15:
		return "BT.2020 12-bit"
	case 16:
		return "PQ"
	case 17:
		return "SMPTE ST 428-1"
	case 18:
		return "HLG"
	default:
		return ""
	}
}

func matroskaMatrixName(value uint64) string {
	switch value {
	case 1:
		return "BT.709"
	case 4:
		return "FCC"
	case 5:
		return "BT.470BG"
	case 6:
		return "SMPTE 170M"
	case 7:
		return "SMPTE 240M"
	case 8:
		return "YCgCo"
	case 9:
		return "BT.2020 non-constant"
	case 10:
		return "BT.2020 constant"
	default:
		return ""
	}
}

func formatMasteringLuminance(minVal, maxVal float64) string {
	return fmt.Sprintf("min: %s cd/m2, max: %s cd/m2", formatHDRLuminance(minVal), formatHDRLuminance(maxVal))
}

func formatHDRLuminance(value float64) string {
	switch {
	case value >= 100:
		return fmt.Sprintf("%.0f", value)
	case value >= 10:
		return fmt.Sprintf("%.1f", value)
	case value >= 1:
		return fmt.Sprintf("%.2f", value)
	default:
		return fmt.Sprintf("%.4f", value)
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

func matroskaHasContainerColor(info matroskaVideoInfo) bool {
	return info.colorRangeSource == "Container" ||
		info.colorPrimariesSource == "Container" ||
		info.transferSource == "Container" ||
		info.matrixSource == "Container"
}

func matroskaColorSource(value string, fallback string) string {
	if strings.Contains(fallback, "/") {
		return fallback
	}
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
	for i := range length {
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
	for i := range 8 {
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
