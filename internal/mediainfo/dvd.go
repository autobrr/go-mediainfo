package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	dvdSectorSize    = 2048
	dvdMaxMuxBitRate = 10_080_000
)

const (
	dvdVideoAttrMenuOffset  = 0x0100
	dvdAudioCountMenuOffset = 0x0102
	dvdAudioAttrMenuOffset  = 0x0104
	dvdSubpicCountMenuOff   = 0x0154
	dvdVideoAttrVTSOffset   = 0x0200
	dvdAudioCountVTSOffset  = 0x0202
	dvdAudioAttrVTSOffset   = 0x0204
	dvdSubpicCountVTSOff    = 0x0254

	dvdPTTSRPTPointerOff = 0x00C8
	dvdPGCIPointerOff    = 0x00CC
)

// dvdInfo carries parsed DVD container, stream, and canonical General facts.
type dvdInfo struct {
	Container    ContainerInfo
	FileSize     int64
	General      []Field
	Streams      []Stream
	GeneralFacts canonicalStructuredFacts
	GeneralExtra *structuredNode
}

// dvdVideoAttrs stores decoded DVD video format, geometry, aspect, standard,
// and frame-rate facts.
type dvdVideoAttrs struct {
	Version     string
	Standard    string
	AspectRatio string
	Width       int
	Height      int
	FrameRate   float64
}

// dvdAudioAttrs stores decoded format, channel, sampling, and language facts
// for one DVD audio stream.
type dvdAudioAttrs struct {
	Format        string
	FormatInfo    string
	FormatProfile string
	Channels      int
	SampleRate    float64
	Language      string
	LanguageCode  string
	LanguageMore  string
	StreamID      int
}

// dvdSubpicAttrs stores decoded language metadata for one DVD subpicture
// stream.
type dvdSubpicAttrs struct {
	Language     string
	LanguageCode string
	LanguageMore string
	StreamID     int
}

// dvdMenuLists stores the audio and subtitle index lists emitted in a DVD
// menu stream's ordered extra object.
type dvdMenuLists struct {
	audio      string
	sub43      string
	subWide    string
	subLetter  string
	subPanScan string
}

// dvdProgram represents one retained VTS title program. Duration is in
// seconds, chapter offsets are in milliseconds, and firstSector orders titles.
type dvdProgram struct {
	duration    float64
	chapters    []int64
	firstSector uint32
}

type dvdPGCTimelineEntry struct {
	pos      int
	duration int64
}

// dvdBUPGeneralExtraNode builds the ordered extension warning emitted for a
// backup IFO file parsed through the DVD-Video path.
func dvdBUPGeneralExtraNode() structuredNode {
	warning := structuredNode{Kind: structuredArray, Array: []structuredNode{{
		Kind: structuredObject,
		Object: []structuredMember{{
			Key:   "GeneralCompliance",
			Value: structuredNode{Kind: structuredString, Text: "File name extension is not expected for this file format (actual BUP, expected ifo)"},
		}},
	}}}
	return structuredNode{Kind: structuredObject, Object: []structuredMember{
		{Key: "FileExtension_Invalid", Value: structuredNode{Kind: structuredString, Text: "ifo"}},
		{Key: "ConformanceWarnings", Value: warning},
	}}
}

// parseDVDVideo parses one IFO or BUP and, for title sets, aggregates matching
// VOB streams according to the supplied analysis options.
func parseDVDVideo(path string, file *os.File, size int64, opts AnalyzeOptions) (dvdInfo, bool) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return dvdInfo{}, false
	}
	data, err := readSizedFile(file, size)
	if err != nil || len(data) < 0x0206 {
		return dvdInfo{}, false
	}
	id := string(data[:12])
	isVTS := strings.HasPrefix(id, "DVDVIDEO-VTS")
	isVMG := strings.HasPrefix(id, "DVDVIDEO-VMG")
	if !isVTS && !isVMG {
		return dvdInfo{}, false
	}

	info := dvdInfo{}
	info.FileSize = size

	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	isBUP := ext == ".bup"
	var backupData []byte
	if isVTS && !isBUP {
		backupData = dvdMatchingBackupData(path)
	}
	// Title-set IFOs inside VIDEO_TS describe the logical program formed by the
	// sibling VTS_nn_1.VOB...VTS_nn_9.VOB files. BUP and VMG files remain
	// metadata-only: BUP is a navigation backup, while VMG describes menus.
	aggregateMode := isVTS && !isBUP && isDVDVideoTSPath(path)
	programMode := !aggregateMode

	var videoAttrs dvdVideoAttrs
	if isVMG {
		videoAttrs = parseDVDVideoAttrs(data, dvdVideoAttrMenuOffset)
	} else {
		videoAttrs = parseDVDVideoAttrs(data, dvdVideoAttrVTSOffset)
		if len(backupData) > 0 {
			videoAttrs = mergeDVDVideoAttrs(videoAttrs, parseDVDVideoAttrs(backupData, dvdVideoAttrVTSOffset))
		}
	}

	generalFields := []Field{}
	if isVMG {
		generalFields = append(generalFields, Field{Name: "Format profile", Value: "Menu"})
	} else if programMode {
		generalFields = append(generalFields, Field{Name: "Format profile", Value: "Program"})
	}
	if ext != "" {
		info.GeneralFacts.SetSame("FileExtension", strings.ToUpper(strings.TrimPrefix(ext, ".")))
	}

	var durationSeconds float64
	var ifoDurationSeconds float64
	var ifoBitRateDurationSeconds float64
	var chapterStarts []int64
	var menuStreams []Stream
	var programs []dvdProgram
	var pgcTableOffset int
	var audioAttrs []dvdAudioAttrs
	var subpicAttrs []dvdSubpicAttrs
	menuLists := dvdMenuLists{}
	menuListsKnown := false
	if isVTS {
		pttOffset := dvdPointer(data, dvdPTTSRPTPointerOff)
		pgcOffset := dvdPointer(data, dvdPGCIPointerOff)
		pgcTableOffset = pgcOffset
		if pttOffset > 0 && pgcOffset > 0 {
			durationSeconds, programs = parseDVDPrograms(data, pttOffset, pgcOffset)
			if len(programs) > 0 {
				chapterStarts = programs[0].chapters
			}
		}
		if len(programs) == 0 && len(backupData) > 0 {
			backupPTT := dvdPointer(backupData, dvdPTTSRPTPointerOff)
			backupPGC := dvdPointer(backupData, dvdPGCIPointerOff)
			if backupPTT > 0 && backupPGC > 0 {
				durationSeconds, programs = parseDVDPrograms(backupData, backupPTT, backupPGC)
				if len(programs) > 0 {
					data = backupData
					pttOffset = backupPTT
					pgcOffset = backupPGC
					pgcTableOffset = backupPGC
					chapterStarts = programs[0].chapters
				}
			}
		}
		ifoBitRateDurationSeconds = dvdPGCTimelineDuration(data, pgcOffset)
		if durationSeconds > 0 {
			info.Container.DurationSeconds = durationSeconds
			ifoDurationSeconds = durationSeconds
			generalFields = append(generalFields, Field{Name: "Duration", Value: formatDVDDuration(durationSeconds)})
		}
		audioAttrs = parseDVDAudioAttrs(data, dvdAudioCountVTSOffset, dvdAudioAttrVTSOffset)
		subpicAttrs = parseDVDSubpicAttrs(data, dvdSubpicCountVTSOff, dvdSubpicCountVTSOff+2)
		if len(backupData) > 0 {
			audioAttrs = mergeDVDAudioAttrs(audioAttrs, parseDVDAudioAttrs(backupData, dvdAudioCountVTSOffset, dvdAudioAttrVTSOffset))
			subpicAttrs = mergeDVDSubpicAttrs(subpicAttrs, parseDVDSubpicAttrs(backupData, dvdSubpicCountVTSOff, dvdSubpicCountVTSOff+2))
		}
		audioAttrs, subpicAttrs, menuLists, menuListsKnown = applyDVDPGCStreamControls(data, pttOffset, pgcOffset, videoAttrs, audioAttrs, subpicAttrs)
	} else if isVMG {
		audioAttrs = parseDVDAudioAttrs(data, dvdAudioCountMenuOffset, dvdAudioAttrMenuOffset)
		subpicAttrs = parseDVDSubpicAttrs(data, dvdSubpicCountMenuOff, dvdSubpicCountMenuOff+2)
	}

	streams := []Stream{}
	titleSetParsed := false
	payloadDurationSeconds := 0.0
	payloadBitRateDurationSeconds := 0.0
	payloadBitRateCorrected := false
	payloadFileSize := int64(0)
	if aggregateMode {
		if vobPaths, vobSize := dvdTitleSetVOBs(path); len(vobPaths) > 0 && vobSize > 0 {
			if len(backupData) == 0 && len(vobPaths) > 1 {
				vobPaths = vobPaths[:1]
				if firstInfo, err := os.Stat(vobPaths[0]); err == nil { //nolint:gosec // path came from root-bounded DVD grammar discovery
					vobSize = firstInfo.Size()
				}
			}
			aggregateSize := vobSize
			if ifoInfo, err := os.Stat(path); err == nil { //nolint:gosec // path is the caller-selected DVD analysis input; Stat only accounts its size
				aggregateSize += ifoInfo.Size()
			}
			if parsedInfo, parsedStreams, ok := ParseMPEGPSFiles(vobPaths, aggregateSize, mpegPSOptions{dvdExtras: true, dvdParsing: true, parseSpeed: opts.ParseSpeed}); ok {
				info.FileSize = aggregateSize
				streams = mergeDVDTitleSetStreams(parsedStreams, dvdTitleSetSource(base))
				streams = mergeDVDDeclaredStreams(streams, audioAttrs, subpicAttrs, ifoDurationSeconds, dvdTitleSetSource(base))
				payloadDurationSeconds = dvdPayloadCanonicalDuration(streams)
				if normalizeDVDConstantVideoClock(streams, ifoDurationSeconds) {
					payloadDurationSeconds = ifoDurationSeconds
				}
				payloadFileSize = vobSize
				payloadBitRateDurationSeconds, payloadBitRateCorrected = dvdTitleSetBitRateDuration(
					payloadFileSize, ifoBitRateDurationSeconds, ifoDurationSeconds, payloadDurationSeconds,
				)
				payloadBitRateCorrected = payloadBitRateCorrected && dvdHasConstantVideoBitRate(streams)
				deriveDVDPSVideoBitRateAndSize(streams, payloadFileSize, payloadBitRateDurationSeconds, payloadBitRateCorrected)
				titleSetParsed = len(streams) > 0
				if parsedInfo.DurationSeconds > 0 {
					info.Container.DurationSeconds = parsedInfo.DurationSeconds
					durationSeconds = parsedInfo.DurationSeconds
					generalFields = setFieldValue(generalFields, "Duration", formatDuration(durationSeconds))
				}
				if fps, ok := parseFPS(findStreamField(streams, StreamVideo, "Frame rate")); ok {
					generalFields = setFieldValue(generalFields, "Frame rate", formatFrameRate(fps))
					info.GeneralFacts.SetSame("FrameRate", formatJSONFloat(fps))
				}
				frameCount, streamSizeSum := dvdJSONStreamStats(streams)
				if frameCount != "" {
					info.GeneralFacts.SetSame("FrameCount", frameCount)
				}
				if streamSizeSum > 0 {
					remaining := payloadFileSize - streamSizeSum
					if remaining >= 0 {
						info.GeneralFacts.SetSame("StreamSize", strconv.FormatInt(remaining, 10))
					}
				}
			}
		}
	}
	if aggregateMode && !titleSetParsed {
		aggregateMode = false
		generalFields = append([]Field{{Name: "Format profile", Value: "Program"}}, generalFields...)
	}
	if aggregateMode && titleSetParsed && ifoDurationSeconds > 0 {
		info.Container.DurationSeconds = ifoDurationSeconds
		durationSeconds = ifoDurationSeconds
		generalFields = setFieldValue(generalFields, "Duration", formatDVDDuration(ifoDurationSeconds))
		for i := range streams {
			streams[i].Fields = setFieldValue(streams[i].Fields, "Duration", formatDVDDuration(ifoDurationSeconds))
			replaceCanonicalSeedProjection(
				&streams[i], "Duration",
				strconv.FormatFloat(ifoDurationSeconds*1000, 'f', -1, 64),
				formatJSONSeconds(ifoDurationSeconds),
				"Duration", formatDVDDuration(ifoDurationSeconds))
		}
	}
	if titleSetParsed && len(programs) > 1 {
		for i := range streams {
			if streams[i].Kind == StreamVideo {
				replaceCanonicalSeedFill(&streams[i], "Format_Settings_GOP", "Variable", "Format settings, GOP", "Variable")
				if scan, _ := canonicalSeedValue(streams[i], "ScanType"); scan == "Interlaced" {
					replaceCanonicalSeedFill(&streams[i], "Gop_OpenClosed", "Closed", "GOP, Open/Closed", "Closed")
					for _, name := range []fieldName{
						"colour_description_present", "colour_description_present_Source",
						"colour_primaries", "colour_primaries_Source",
						"transfer_characteristics", "transfer_characteristics_Source",
						"matrix_coefficients", "matrix_coefficients_Source",
					} {
						clearCanonicalSeedField(&streams[i], name, "")
					}
				} else {
					clearCanonicalSeedField(&streams[i], "Gop_OpenClosed", "")
				}
				clearCanonicalSeedField(&streams[i], "Gop_OpenClosed_FirstFrame", "")
				break
			}
		}
	}

	overallMode := "Variable"
	if titleSetParsed {
		for i := range streams {
			if streams[i].Kind != StreamVideo {
				continue
			}
			if mode, ok := canonicalSeedValue(streams[i], "BitRate_Mode"); ok && (mode == "CBR" || mode == "Constant") {
				overallMode = "Constant"
			}
			break
		}
	}
	generalFields = append(generalFields, Field{Name: "Overall bit rate mode", Value: overallMode})
	if info.Container.DurationSeconds > 0 && info.FileSize > 0 {
		bitRateDuration := info.Container.DurationSeconds
		bitRateSize := info.FileSize
		if titleSetParsed && payloadBitRateDurationSeconds > 0 {
			bitRateDuration = payloadBitRateDurationSeconds
			bitRateSize = payloadFileSize
		}
		overall := (float64(bitRateSize) * 8) / bitRateDuration
		generalFields = append(generalFields, Field{Name: "Overall bit rate", Value: formatBitrateSmall(overall)})
		info.GeneralFacts.SetSame("OverallBitRate", strconv.FormatInt(int64(overall+0.5), 10))
	}
	if videoAttrs.FrameRate > 0 && !titleSetParsed {
		generalFields = append(generalFields, Field{Name: "Frame rate", Value: formatFrameRate(videoAttrs.FrameRate)})
		if info.Container.DurationSeconds > 0 {
			frameCount := int64(info.Container.DurationSeconds*videoAttrs.FrameRate + 0.5)
			info.GeneralFacts.SetSame("FrameCount", strconv.FormatInt(frameCount, 10))
		}
	}

	if isBUP {
		generalFields = append(generalFields,
			Field{Name: "FileExtension_Invalid", Value: "ifo"},
			Field{Name: "Conformance warnings", Value: "Yes"},
			Field{Name: " General compliance", Value: "File name extension is not expected for this file format (actual BUP, expected ifo)"},
		)
		node := dvdBUPGeneralExtraNode()
		info.GeneralExtra = &node
	}

	info.General = generalFields
	if !titleSetParsed {
		videoDurationSeconds := durationSeconds
		videoFrameCount := int64(0)
		if videoAttrs.FrameRate > 0 && durationSeconds > 0 {
			videoFrameCount = int64(durationSeconds*videoAttrs.FrameRate + 0.5)
		}

		videoFields := []Field{}
		if videoAttrs.Version != "" {
			videoFields = append(videoFields, Field{Name: "Format", Value: "MPEG Video"})
			videoFields = append(videoFields, Field{Name: "Format version", Value: videoAttrs.Version})
		} else {
			videoFields = append(videoFields, Field{Name: "Format", Value: "MPEG Video"})
		}
		videoFields = append(videoFields, Field{Name: "ID", Value: "224 (0xE0)"})
		videoFields = append(videoFields, Field{Name: "Bit rate mode", Value: "Variable"})
		if videoDurationSeconds > 0 {
			videoFields = append(videoFields, Field{Name: "Duration", Value: formatDVDDuration(videoDurationSeconds)})
		}
		if videoAttrs.Width > 0 {
			videoFields = append(videoFields, Field{Name: "Width", Value: formatPixels(uint64(videoAttrs.Width))})
		}
		if videoAttrs.Height > 0 {
			videoFields = append(videoFields, Field{Name: "Height", Value: formatPixels(uint64(videoAttrs.Height))})
		}
		if videoAttrs.AspectRatio != "" {
			videoFields = append(videoFields, Field{Name: "Display aspect ratio", Value: videoAttrs.AspectRatio})
		}
		if videoAttrs.FrameRate > 0 {
			videoFields = append(videoFields, Field{Name: "Frame rate", Value: formatDVDFrameRate(videoAttrs.FrameRate)})
		}
		if videoAttrs.Standard != "" {
			videoFields = append(videoFields, Field{Name: "Standard", Value: videoAttrs.Standard})
		}
		videoFields = append(videoFields, Field{Name: "Compression mode", Value: "Lossy"})
		if aggregateMode && isVTS && !isBUP {
			if source := dvdTitleSetSource(base); source != "" {
				videoFields = append(videoFields, Field{Name: "Source", Value: source})
			}
		}
		videoFacts := &dvdStructuredFacts{}
		if videoDurationSeconds > 0 {
			videoFacts.Set(
				"Duration",
				strconv.FormatFloat(videoDurationSeconds*1000, 'f', -1, 64),
				formatJSONSeconds(videoDurationSeconds),
			)
		}
		if videoAttrs.Standard == "NTSC" {
			videoFacts.SetSame("FrameRate_Num", "29970")
			videoFacts.SetSame("FrameRate_Den", "1000")
		} else if videoAttrs.Standard == "PAL" {
			videoFacts.SetSame("FrameRate_Num", "25")
			videoFacts.SetSame("FrameRate_Den", "1")
		}
		if videoAttrs.AspectRatio != "" && videoAttrs.Width > 0 && videoAttrs.Height > 0 {
			if displayAspect, ok := parseRatioFloat(videoAttrs.AspectRatio); ok {
				pixelAspect := displayAspect / (float64(videoAttrs.Width) / float64(videoAttrs.Height))
				videoFacts.SetSame("PixelAspectRatio", formatJSONFloat(pixelAspect))
			}
		}
		if videoFrameCount > 0 {
			videoFacts.SetSame("FrameCount", strconv.FormatInt(videoFrameCount, 10))
		}
		videoFacts.SetSame("ID", "224")
		videoStream := buildCanonicalDVDVideoStream(videoFields, videoFacts, videoAttrs, videoDurationSeconds)
		streams = append(streams, videoStream)

		if len(audioAttrs) > 0 {
			for _, audio := range audioAttrs {
				audioFields := []Field{}
				if isVTS && audio.StreamID >= 0 {
					id := dvdAudioPrivateID(audio, audio.StreamID)
					audioFields = append(audioFields, Field{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", id, id)})
				}
				if audio.Format != "" {
					audioFields = append(audioFields, Field{Name: "Format", Value: audio.Format})
				}
				if audio.FormatInfo != "" {
					audioFields = append(audioFields, Field{Name: "Format/Info", Value: audio.FormatInfo})
				}
				if audio.FormatProfile != "" {
					audioFields = append(audioFields, Field{Name: "Format profile", Value: audio.FormatProfile})
				}
				if durationSeconds > 0 {
					audioFields = append(audioFields, Field{Name: "Duration", Value: formatDVDDuration(durationSeconds)})
				}
				if audio.Channels > 0 {
					audioFields = append(audioFields, Field{Name: "Channel(s)", Value: formatChannels(uint64(audio.Channels))})
				}
				if audio.SampleRate > 0 {
					audioFields = append(audioFields, Field{Name: "Sampling rate", Value: formatSampleRate(audio.SampleRate)})
				} else if audio.Format == "PCM" {
					audioFields = append(audioFields, Field{Name: "Sampling rate", Value: "0 Hz"})
				}
				if audio.Format != "PCM" {
					audioFields = append(audioFields, Field{Name: "Compression mode", Value: "Lossy"})
				}
				suppressLanguage := aggregateMode
				if audio.Language != "" && !suppressLanguage {
					audioFields = append(audioFields, Field{Name: "Language", Value: audio.Language})
				}
				if audio.LanguageMore != "" && !suppressLanguage {
					audioFields = append(audioFields, Field{Name: "Language, more info", Value: audio.LanguageMore})
				}
				if aggregateMode && isVTS && !isBUP {
					if source := dvdTitleSetSource(base); source != "" {
						audioFields = append(audioFields, Field{Name: "Source", Value: source})
					}
				}
				audioFacts := &dvdStructuredFacts{}
				if durationSeconds > 0 {
					audioFacts.Set(
						"Duration",
						strconv.FormatFloat(durationSeconds*1000, 'f', -1, 64),
						formatJSONSeconds(durationSeconds),
					)
				}
				if durationSeconds > 0 && audio.SampleRate > 0 {
					durationMs := int64(durationSeconds*1000 + 0.0005)
					sr := int64(audio.SampleRate + 0.5)
					samplingCount := (durationMs * sr) / 1000
					audioFacts.SetSame("SamplingCount", strconv.FormatInt(samplingCount, 10))
				}
				if audio.LanguageCode != "" && !suppressLanguage {
					audioFacts.SetSame("Language", audio.LanguageCode)
				}
				if audio.LanguageMore != "" && !suppressLanguage {
					audioFacts.SetSame("Language_More", audio.LanguageMore)
				}
				if isVTS && audio.StreamID >= 0 {
					audioFacts.SetSame("ID", fmt.Sprintf("189-%d", dvdAudioPrivateID(audio, audio.StreamID)))
				}
				audioStream := buildCanonicalDVDAudioStream(audioFields, audioFacts, audio, durationSeconds)
				streams = append(streams, audioStream)
			}
		}

		if len(subpicAttrs) > 0 {
			for _, subpic := range subpicAttrs {
				textFields := []Field{}
				if isVTS && subpic.StreamID >= 0 {
					id := 0x20 + subpic.StreamID
					textFields = append(textFields, Field{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", id, id)})
				}
				textFields = append(textFields, Field{Name: "Format", Value: "RLE"})
				textFields = append(textFields, Field{Name: "Format/Info", Value: "Run-length encoding"})
				textFields = append(textFields, Field{Name: "Bit depth", Value: "2 bits"})
				if subpic.Language != "" && !aggregateMode {
					textFields = append(textFields, Field{Name: "Language", Value: subpic.Language})
				}
				if subpic.LanguageMore != "" && !aggregateMode {
					textFields = append(textFields, Field{Name: "Language, more info", Value: subpic.LanguageMore})
				}
				textFacts := &dvdStructuredFacts{}
				if durationSeconds > 0 {
					textFacts.Set("Duration", strconv.FormatFloat(durationSeconds*1000, 'f', -1, 64), formatJSONSeconds(durationSeconds))
					textFields = append(textFields, Field{Name: "Duration", Value: formatDVDDuration(durationSeconds)})
				}
				if subpic.LanguageCode != "" && !aggregateMode {
					textFacts.SetSame("Language", subpic.LanguageCode)
				}
				if subpic.LanguageMore != "" && !aggregateMode {
					textFacts.SetSame("Language_More", subpic.LanguageMore)
				}
				if isVTS && subpic.StreamID >= 0 {
					textFacts.SetSame("ID", fmt.Sprintf("189-%d", 0x20+subpic.StreamID))
				}
				textStream := buildCanonicalDVDTextStream(textFields, textFacts, subpic)
				streams = append(streams, textStream)
			}
		}
	}

	if len(programs) == 0 && len(chapterStarts) > 0 && ifoDurationSeconds > 0 {
		programs = []dvdProgram{{duration: ifoDurationSeconds, chapters: chapterStarts}}
	}
	programDelays := dvdProgramStartDelays(data, pgcTableOffset, programs)
	for programIndex, program := range programs {
		if len(program.chapters) == 0 || program.duration <= 0 {
			continue
		}
		menuDelay := programDelays[programIndex]
		menuFields := []Field{{Name: "Duration", Value: formatDVDDuration(program.duration)}}
		menuRate := 25.0
		menuRateNum := "25"
		if videoAttrs.Standard == "NTSC" {
			menuRate = 30
			menuRateNum = "30"
		}
		menuFrameCount := int64(math.Round(program.duration * menuRate))
		menuFields = append(menuFields,
			Field{Name: "Delay", Value: formatDelayMs(int64(math.Round(menuDelay * 1000)))},
			Field{Name: "Frame rate", Value: formatFrameRate(menuRate)},
			Field{Name: "Frame count", Value: strconv.FormatInt(menuFrameCount, 10)},
		)
		for i, startMs := range program.chapters {
			menuFields = append(menuFields, Field{Name: formatDVDChapterTimeMs(startMs), Value: fmt.Sprintf("Chapter %d", i+1)})
		}
		if len(audioAttrs) > 0 {
			menuFields = append(menuFields, Field{Name: "List (Audio)", Value: dvdIndexList(len(audioAttrs))})
		}
		if len(subpicAttrs) > 0 {
			menuFields = append(menuFields, Field{Name: "List (Subtitles 4/3)", Value: dvdIndexList(len(subpicAttrs))})
			menuFields = append(menuFields, Field{Name: "List (Subtitles Wide)", Value: dvdZeroList(len(subpicAttrs))})
			menuFields = append(menuFields, Field{Name: "List (Subtitles Letterbox)", Value: dvdZeroList(len(subpicAttrs))})
			menuFields = append(menuFields, Field{Name: "List (Subtitles Pan&Scan)", Value: dvdZeroList(len(subpicAttrs))})
		}
		if aggregateMode && isVTS && !isBUP {
			if source := dvdTitleSetSource(base); source != "" {
				menuFields = append(menuFields, Field{Name: "Source", Value: source})
			}
		}
		menuFacts := &dvdStructuredFacts{}
		menuFacts.Set(
			"Duration",
			strconv.FormatFloat(program.duration*1000, 'f', -1, 64),
			formatJSONSeconds(program.duration),
		)
		menuFacts.Set("Delay", fmt.Sprintf("%.3f", menuDelay), fmt.Sprintf("%.3f", menuDelay))
		menuFacts.SetSame("FrameRate", formatJSONFloat(menuRate))
		menuFacts.SetSame("FrameRate_Num", menuRateNum)
		menuFacts.SetSame("FrameRate_Den", "1")
		menuFacts.SetSame("FrameCount", strconv.FormatInt(menuFrameCount, 10))
		if !menuListsKnown {
			menuLists = dvdMenuListsForAspect(videoAttrs.AspectRatio, len(audioAttrs), len(subpicAttrs))
		}
		menuExtra := dvdMenuExtraNode(program.chapters, menuLists)
		menu := buildCanonicalDVDMenuStream(menuFields, menuFacts, menuExtra)
		menuStreams = append(menuStreams, menu)
	}

	for i := range menuStreams {
		if aggregateMode {
			if source := dvdTitleSetSource(base); source != "" {
				appendCanonicalSeedObjectMembers(&menuStreams[i], "extra", []structuredMember{{
					Key:   "Source",
					Value: structuredNode{Kind: structuredString, Text: source},
				}})
			}
		}
		streams = append(streams, menuStreams[i])
	}

	for index := range streams {
		publishCanonicalProjectionPolicy(&streams[index])
	}
	info.Streams = streams
	return info, true
}

// dvdTitleSetBitRateDuration retains the bounded VOB timing used by
// MediaInfoLib's payload bitrate derivation. The independently parsed IFO
// duration remains the reported program duration; it does not replace the
// sampled payload clock used for bitrate and stream-size math.
func dvdTitleSetBitRateDuration(fileSize int64, ifoTimelineDuration, ifoDuration, payloadDuration float64) (float64, bool) {
	if payloadDuration > 0 {
		clockMismatch := fileSize > 0 && float64(fileSize)*8/payloadDuration > dvdMaxMuxBitRate
		return payloadDuration, clockMismatch
	}
	if ifoTimelineDuration > 0 {
		return ifoTimelineDuration, false
	}
	return ifoDuration, false
}

func dvdHasConstantVideoBitRate(streams []Stream) bool {
	for i := range streams {
		if streams[i].Kind != StreamVideo {
			continue
		}
		mode, _ := canonicalSeedValue(streams[i], "BitRate_Mode")
		return mode == "CBR" || mode == "Constant"
	}
	return false
}

// dvdStructuredFacts stages IFO facts without exposing JSON-shaped state to
// the DVD parser.
type dvdStructuredFacts = canonicalStructuredFacts

// buildCanonicalDVDVideoStream converts IFO video attributes into one direct
// canonical stream and its public compatibility snapshot.
func buildCanonicalDVDVideoStream(fields []Field, facts *dvdStructuredFacts, attrs dvdVideoAttrs, duration float64) Stream {
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("ID", facts.Canonical("ID"), "ID", findField(fields, "ID"))
	builder.Fill("Format", "MPEG Video", "Format", "MPEG Video")
	if attrs.Version != "" {
		builder.Fill("Format_Version", extractVersionNumber(attrs.Version), "Format version", attrs.Version)
	}
	builder.Fill("BitRate_Mode", "Variable", "Bit rate mode", "Variable")
	if duration > 0 {
		builder.Fill("Duration", strconv.FormatFloat(duration*1000, 'f', -1, 64), "Duration", findField(fields, "Duration"))
	}
	if attrs.Width > 0 {
		raw := strconv.Itoa(attrs.Width)
		builder.Fill("Width", raw, "Width", formatPixels(uint64(attrs.Width)))
	}
	if attrs.Height > 0 {
		raw := strconv.Itoa(attrs.Height)
		builder.Fill("Height", raw, "Height", formatPixels(uint64(attrs.Height)))
	}
	if attrs.AspectRatio != "" {
		if ratio, ok := parseRatioFloat(attrs.AspectRatio); ok {
			builder.Fill("DisplayAspectRatio", formatJSONFloat(ratio), "Display aspect ratio", attrs.AspectRatio)
		}
	}
	if attrs.FrameRate > 0 {
		builder.Fill("FrameRate", formatJSONFloat(attrs.FrameRate), "Frame rate", formatDVDFrameRate(attrs.FrameRate))
	}
	if attrs.Standard != "" {
		builder.Fill("Standard", attrs.Standard, "Standard", attrs.Standard)
	}
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	if source := findField(fields, "Source"); source != "" {
		builder.Text("Source", source)
	}
	facts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true, DVDOrder: true})
}

// buildCanonicalDVDAudioStream converts IFO audio attributes into one direct
// canonical stream and its public compatibility snapshot.
func buildCanonicalDVDAudioStream(fields []Field, facts *dvdStructuredFacts, attrs dvdAudioAttrs, duration float64) Stream {
	builder := newCanonicalStreamBuilder(StreamAudio)
	if id := findField(fields, "ID"); id != "" {
		builder.Fill("ID", firstNonEmpty(facts.Canonical("ID"), id), "ID", id)
	}
	if attrs.Format != "" {
		builder.Fill("Format", attrs.Format, "Format", attrs.Format)
	}
	if attrs.FormatInfo != "" {
		builder.Text("Format/Info", attrs.FormatInfo)
	}
	if attrs.FormatProfile != "" {
		builder.Fill("Format_Profile", attrs.FormatProfile, "Format profile", attrs.FormatProfile)
	}
	if duration > 0 {
		builder.Fill("Duration", strconv.FormatFloat(duration*1000, 'f', -1, 64), "Duration", findField(fields, "Duration"))
	}
	if attrs.Channels > 0 {
		builder.Fill("Channels", strconv.Itoa(attrs.Channels), "Channel(s)", formatChannels(uint64(attrs.Channels)))
	}
	if attrs.SampleRate > 0 {
		builder.Fill("SamplingRate", strconv.FormatInt(int64(attrs.SampleRate+0.5), 10), "Sampling rate", formatSampleRate(attrs.SampleRate))
	} else if attrs.Format == "PCM" {
		builder.Fill("SamplingRate", "0", "Sampling rate", "0 Hz")
	}
	if attrs.Format != "PCM" {
		builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	}
	if attrs.Language != "" {
		builder.Fill("Language", firstNonEmpty(facts.Canonical("Language"), attrs.Language), "Language", attrs.Language)
	}
	if attrs.LanguageMore != "" {
		builder.Fill("Language_More", attrs.LanguageMore, "Language, more info", attrs.LanguageMore)
	}
	if source := findField(fields, "Source"); source != "" {
		builder.Text("Source", source)
	}
	facts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true, DVDOrder: true})
}

// buildCanonicalDVDTextStream converts IFO subpicture attributes into one
// direct canonical stream and its public compatibility snapshot.
func buildCanonicalDVDTextStream(fields []Field, facts *dvdStructuredFacts, attrs dvdSubpicAttrs) Stream {
	builder := newCanonicalStreamBuilder(StreamText)
	if id := findField(fields, "ID"); id != "" {
		builder.Fill("ID", firstNonEmpty(facts.Canonical("ID"), id), "ID", id)
	}
	builder.Fill("Format", "RLE", "Format", "RLE")
	builder.Text("Format/Info", "Run-length encoding")
	if duration := facts.Canonical("Duration"); duration != "" {
		builder.Fill("Duration", duration, "Duration", findField(fields, "Duration"))
	}
	builder.Fill("BitDepth", "2", "Bit depth", "2 bits")
	if attrs.Language != "" {
		builder.Fill("Language", firstNonEmpty(facts.Canonical("Language"), attrs.Language), "Language", attrs.Language)
	}
	if attrs.LanguageMore != "" {
		builder.Fill("Language_More", attrs.LanguageMore, "Language, more info", attrs.LanguageMore)
	}
	facts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true, DVDOrder: true})
}

// buildCanonicalDVDMenuStream converts chapter/list fields and ordered menu
// extras into one direct canonical stream and its compatibility snapshot.
func buildCanonicalDVDMenuStream(fields []Field, facts *dvdStructuredFacts, extra structuredNode) Stream {
	builder := newCanonicalStreamBuilder(StreamMenu)
	for _, field := range fields {
		switch field.Name {
		case "Duration":
			builder.Fill("Duration", facts.Canonical("Duration"), "Duration", field.Value)
		case "Delay":
			builder.Fill("Delay", facts.Canonical("Delay"), "Delay", field.Value)
		case "Frame rate":
			builder.Fill("FrameRate", facts.Canonical("FrameRate"), "Frame rate", field.Value)
		case "Frame count":
			builder.Fill("FrameCount", facts.Canonical("FrameCount"), "Frame count", field.Value)
		default:
			builder.Text(field.Name, field.Value)
		}
	}
	facts.Apply(builder)
	builder.OverrideStructuredNode("extra", extra)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true, DVDOrder: true})
}

// readSizedFile reads exactly size bytes from the start of file after
// validating the requested allocation.
func readSizedFile(file *os.File, size int64) ([]byte, error) {
	if size <= 0 {
		return io.ReadAll(file)
	}
	if size > int64(^uint(0)>>1) {
		return io.ReadAll(file)
	}
	buf := make([]byte, int(size))
	n, err := io.ReadFull(file, buf)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return buf[:n], nil
		}
		return nil, err
	}
	return buf[:n], nil
}

// parseDVDVideoAttrs decodes one bounds-checked DVD video attribute record.
func parseDVDVideoAttrs(data []byte, offset int) dvdVideoAttrs {
	if offset+2 > len(data) {
		return dvdVideoAttrs{}
	}
	b0 := data[offset]
	b1 := data[offset+1]
	coding := (b0 >> 6) & 0x03
	standardCode := (b0 >> 4) & 0x03
	aspectCode := (b0 >> 2) & 0x03
	resCode := (b1 >> 3) & 0x03

	attrs := dvdVideoAttrs{}
	switch coding {
	case 1:
		attrs.Version = "Version 2"
	case 0:
		attrs.Version = "Version 1"
	}

	switch standardCode {
	case 0:
		attrs.Standard = "NTSC"
		attrs.FrameRate = 29.97
	case 1:
		attrs.Standard = "PAL"
		attrs.FrameRate = 25.0
	}

	switch aspectCode {
	case 0:
		attrs.AspectRatio = "4:3"
	case 3:
		attrs.AspectRatio = "16:9"
	}

	width := 0
	switch attrs.Standard {
	case "PAL":
		switch resCode {
		case 0:
			width = 720
			attrs.Height = 576
		case 1:
			width = 704
			attrs.Height = 576
		case 2:
			width = 352
			attrs.Height = 576
		case 3:
			width = 352
			attrs.Height = 288
		}
	case "NTSC":
		switch resCode {
		case 0:
			width = 720
			attrs.Height = 480
		case 1:
			width = 704
			attrs.Height = 480
		case 2:
			width = 352
			attrs.Height = 480
		case 3:
			width = 352
			attrs.Height = 240
		}
	}
	attrs.Width = width
	return attrs
}

// parseDVDAudioAttrs decodes the bounded DVD audio attribute table at the
// supplied count and entry offsets.
func parseDVDAudioAttrs(data []byte, countOffset int, attrOffset int) []dvdAudioAttrs {
	if countOffset+2 > len(data) || attrOffset >= len(data) {
		return nil
	}
	count := dvdAttrCount(data, countOffset)
	if count <= 0 {
		return nil
	}
	attrs := []dvdAudioAttrs{}
	for i := range count {
		off := attrOffset + i*8
		if off+8 > len(data) {
			break
		}
		b0 := data[off]
		b1 := data[off+1]
		code := (b0 >> 5) & 0x07
		format, formatInfo := dvdAudioFormat(code)
		formatProfile := ""
		switch code {
		case 2:
			formatProfile = "Version 1"
		case 3:
			formatProfile = "Version 2"
		}
		lang := dvdTrimLang(data[off+2 : off+4])
		sampleCode := (b1 >> 4) & 0x03
		sampleRate := dvdAudioSampleRate(sampleCode)
		channels := int(b1&0x07) + 1
		langCode := normalizeLanguageCode(lang)
		attrs = append(attrs, dvdAudioAttrs{
			Format:        format,
			FormatInfo:    formatInfo,
			FormatProfile: formatProfile,
			Channels:      channels,
			SampleRate:    sampleRate,
			Language:      formatLanguage(lang),
			LanguageCode:  langCode,
			LanguageMore:  dvdAudioLanguageMore(data[off+5]),
			StreamID:      i,
		})
	}
	return attrs
}

// parseDVDSubpicAttrs decodes the bounded DVD subpicture attribute table at
// the supplied count and entry offsets.
func parseDVDSubpicAttrs(data []byte, countOffset int, attrOffset int) []dvdSubpicAttrs {
	if countOffset+2 > len(data) || attrOffset >= len(data) {
		return nil
	}
	count := dvdAttrCount(data, countOffset)
	if count <= 0 {
		return nil
	}
	attrs := []dvdSubpicAttrs{}
	for i := range count {
		off := attrOffset + i*6
		if off+6 > len(data) {
			break
		}
		lang := dvdTrimLang(data[off+2 : off+4])
		attrs = append(attrs, dvdSubpicAttrs{
			Language:     formatLanguage(lang),
			LanguageCode: normalizeLanguageCode(lang),
			LanguageMore: dvdLanguageMore(data[off+5]),
			StreamID:     i,
		})
	}
	return attrs
}

// dvdLanguageMore maps a DVD language-extension code to its MediaInfo label.
func dvdLanguageMore(code byte) string {
	switch code {
	case 1:
		return "Normal"
	case 2:
		return "For visually impaired"
	case 3:
		return "Director's comments"
	case 4:
		return "Alternate director's comments"
	case 6:
		return "Large"
	default:
		return ""
	}
}

// dvdAudioLanguageMore maps a DVD audio language-extension code, treating the
// audio-specific code 1 as having no additional language description.
func dvdAudioLanguageMore(code byte) string {
	if code == 1 {
		return ""
	}
	return dvdLanguageMore(code)
}

// dvdAudioFormat maps a DVD audio coding-mode code to format and descriptive
// information labels.
func dvdAudioFormat(code byte) (string, string) {
	switch code {
	case 0:
		return "AC-3", "Audio Coding 3"
	case 2:
		return "MPEG Audio", "MPEG Audio"
	case 3:
		return "MPEG Audio", "MPEG Audio"
	case 4:
		return "PCM", "Linear PCM"
	case 6:
		return "DTS", "Digital Theater Systems"
	default:
		return "", ""
	}
}

// dvdAudioSampleRate maps a DVD audio sampling-frequency code to hertz.
func dvdAudioSampleRate(code byte) float64 {
	switch code {
	case 0:
		return 48000
	case 1:
		return 0
	default:
		return 0
	}
}

// dvdPointer resolves a bounds-checked sector pointer to a byte offset.
func dvdPointer(data []byte, offset int) int {
	if offset+4 > len(data) {
		return 0
	}
	sector := binary.BigEndian.Uint32(data[offset : offset+4])
	if sector == 0 {
		return 0
	}
	pos := int(sector) * dvdSectorSize
	if pos <= 0 || pos >= len(data) {
		return 0
	}
	return pos
}

// dvdAttrCount reads a bounds-checked big-endian attribute count.
func dvdAttrCount(data []byte, offset int) int {
	if offset+2 > len(data) {
		return 0
	}
	return int(binary.BigEndian.Uint16(data[offset : offset+2]))
}

// dvdTrimLang trims NUL and space padding from a DVD language code.
func dvdTrimLang(raw []byte) string {
	return strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
}

// dvdIndexList returns a zero-based sequence joined in DVD menu-list form.
func dvdIndexList(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for i := range count {
		values[i] = strconv.Itoa(i)
	}
	return strings.Join(values, " / ")
}

// dvdZeroList returns count zero indexes joined in DVD menu-list form.
func dvdZeroList(count int) string {
	if count <= 0 {
		return ""
	}
	values := make([]string, count)
	for i := range count {
		values[i] = "0"
	}
	return strings.Join(values, " / ")
}

// dvdMenuListsFromCounts builds MediaInfo-compatible menu stream index lists
// from decoded audio and subpicture counts.
func dvdMenuListsFromCounts(audioCount, subpicCount int) dvdMenuLists {
	return dvdMenuLists{
		audio:      dvdIndexList(audioCount),
		sub43:      dvdIndexList(subpicCount),
		subWide:    dvdZeroList(subpicCount),
		subLetter:  dvdZeroList(subpicCount),
		subPanScan: dvdZeroList(subpicCount),
	}
}

// dvdMenuListsForAspect builds menu stream index lists using the subtitle
// display modes selected by the title set's video aspect ratio.
func dvdMenuListsForAspect(aspect string, audioCount, subpicCount int) dvdMenuLists {
	lists := dvdMenuLists{audio: dvdIndexList(audioCount)}
	if aspect == "16:9" {
		lists.sub43 = dvdZeroList(subpicCount)
		lists.subWide = dvdIndexList(subpicCount)
		lists.subLetter = dvdIndexList(subpicCount)
		lists.subPanScan = dvdZeroList(subpicCount)
		return lists
	}
	lists.sub43 = dvdIndexList(subpicCount)
	lists.subWide = dvdZeroList(subpicCount)
	lists.subLetter = dvdZeroList(subpicCount)
	lists.subPanScan = dvdZeroList(subpicCount)
	return lists
}

// dvdAudioPrivateID maps an AC-3 logical stream index to its DVD private-stream
// substream ID. Other formats retain their logical stream index.
func dvdAudioPrivateID(attrs dvdAudioAttrs, streamID int) int {
	if attrs.Format == "AC-3" {
		return 0x80 + streamID
	}
	return streamID
}

// applyDVDPGCStreamControls applies the referenced PGC's enabled audio and
// subpicture mappings and returns their menu lists. The boolean reports whether
// a valid PGC control table was available.
func applyDVDPGCStreamControls(data []byte, pttOffset, pgcOffset int, video dvdVideoAttrs, audio []dvdAudioAttrs, subpics []dvdSubpicAttrs) ([]dvdAudioAttrs, []dvdSubpicAttrs, dvdMenuLists, bool) {
	base := dvdReferencedPGCBase(data, pttOffset, pgcOffset)
	if base < 0 || base+0x9C > len(data) {
		return audio, subpics, dvdMenuListsForAspect(video.AspectRatio, len(audio), len(subpics)), false
	}
	audioValues := make([]int, 0, len(audio))
	for i := range audio {
		audio[i].StreamID = -1
		if i >= 8 {
			continue
		}
		control := binary.BigEndian.Uint16(data[base+0x0C+i*2 : base+0x0E+i*2])
		if control&0x8000 == 0 {
			continue
		}
		streamID := int((control >> 8) & 0x07)
		audio[i].StreamID = streamID
		audioValues = append(audioValues, streamID)
	}
	if len(audio) <= 4 {
		for i := range audio {
			if audio[i].StreamID < 0 && audio[i].Format == "AC-3" {
				audio[i].StreamID = i
			}
		}
	}

	sub43 := make([]int, 0, len(subpics))
	subWide := make([]int, 0, len(subpics))
	subLetter := make([]int, 0, len(subpics))
	subPan := make([]int, 0, len(subpics))
	for i := range subpics {
		subpics[i].StreamID = -1
		if i >= 32 {
			continue
		}
		off := base + 0x1C + i*4
		control := binary.BigEndian.Uint32(data[off : off+4])
		if control&0x80000000 == 0 {
			continue
		}
		id43 := int((control >> 24) & 0x7F)
		idWide := int((control >> 16) & 0x7F)
		idLetter := int((control >> 8) & 0x7F)
		idPan := int(control & 0x7F)
		sub43 = append(sub43, id43)
		subWide = append(subWide, idWide)
		subLetter = append(subLetter, idLetter)
		subPan = append(subPan, idPan)
		if video.AspectRatio == "16:9" {
			subpics[i].StreamID = idWide
		} else {
			subpics[i].StreamID = 0
		}
	}
	return audio, subpics, dvdMenuLists{
		audio:      dvdJoinIndexes(audioValues),
		sub43:      dvdJoinIndexes(sub43),
		subWide:    dvdJoinIndexes(subWide),
		subLetter:  dvdJoinIndexes(subLetter),
		subPanScan: dvdJoinIndexes(subPan),
	}, true
}

// dvdJoinIndexes formats explicit stream indexes in DVD menu-list form.
func dvdJoinIndexes(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return strings.Join(parts, " / ")
}

// dvdReferencedPGCBase resolves the PGC referenced by the first title's first
// PTT entry. It returns -1 for malformed or out-of-range tables.
func dvdReferencedPGCBase(data []byte, pttOffset, pgcOffset int) int {
	if pttOffset < 0 || pttOffset+12 > len(data) || pgcOffset < 0 || pgcOffset+16 > len(data) {
		return -1
	}
	pttStart := pttOffset + int(binary.BigEndian.Uint32(data[pttOffset+8:pttOffset+12]))
	if pttStart < 0 || pttStart+2 > len(data) {
		return -1
	}
	pgcn := int(binary.BigEndian.Uint16(data[pttStart : pttStart+2]))
	pgcCount := int(binary.BigEndian.Uint16(data[pgcOffset : pgcOffset+2]))
	if pgcn < 1 || pgcn > pgcCount {
		return -1
	}
	entry := pgcOffset + 8 + (pgcn-1)*8
	if entry+8 > len(data) {
		return -1
	}
	base := pgcOffset + int(binary.BigEndian.Uint32(data[entry+4:entry+8]))
	if base < 0 || base >= len(data) {
		return -1
	}
	return base
}

// parseDVDPrograms decodes VTS titles from the PTT and PGC tables, retains
// presentation-length programs, and returns their summed duration in seconds.
func parseDVDPrograms(data []byte, pttOffset, pgcOffset int) (float64, []dvdProgram) {
	if pttOffset+12 > len(data) || pgcOffset+8 > len(data) {
		return 0, nil
	}
	titleCount := int(binary.BigEndian.Uint16(data[pttOffset : pttOffset+2]))
	lastByte := int(binary.BigEndian.Uint32(data[pttOffset+4 : pttOffset+8]))
	if titleCount <= 0 || pttOffset+8+titleCount*4 > len(data) || lastByte < 8 {
		return 0, nil
	}
	tableEnd := min(pttOffset+lastByte+1, len(data))
	programs := make([]dvdProgram, 0, titleCount)
	var maxDuration float64
	for title := range titleCount {
		relStart := int(binary.BigEndian.Uint32(data[pttOffset+8+title*4 : pttOffset+12+title*4]))
		start := pttOffset + relStart
		end := tableEnd
		if title+1 < titleCount {
			relEnd := int(binary.BigEndian.Uint32(data[pttOffset+12+title*4 : pttOffset+16+title*4]))
			end = min(pttOffset+relEnd, tableEnd)
		}
		if start < pttOffset || start >= end || end > len(data) {
			continue
		}
		entries := make([][2]uint16, 0, (end-start)/4)
		for pos := start; pos+4 <= end; pos += 4 {
			pgcn := binary.BigEndian.Uint16(data[pos : pos+2])
			pgn := binary.BigEndian.Uint16(data[pos+2 : pos+4])
			if pgcn > 0 && pgn > 0 {
				entries = append(entries, [2]uint16{pgcn, pgn})
			}
		}
		program, ok := parseDVDProgramEntries(data, pgcOffset, entries)
		if !ok {
			continue
		}
		programs = append(programs, program)
		maxDuration = max(maxDuration, program.duration)
	}
	selected := programs[:0]
	var totalMs int64
	for _, program := range programs {
		// VTS PTT tables commonly expose short branch/angle PGCs beside the
		// presentation titles. MediaInfo retains presentation PGCs whose playback
		// time is at least half of the longest title in the title set.
		if program.duration*2 < maxDuration {
			continue
		}
		selected = append(selected, program)
		totalMs += int64(math.Round(program.duration * 1000))
	}
	return float64(totalMs) / 1000, selected
}

// parseDVDProgramEntries resolves one title's PTT entries to its PGC duration,
// chapter offsets, and first playback sector.
func parseDVDProgramEntries(data []byte, pgcOffset int, entries [][2]uint16) (dvdProgram, bool) {
	if len(entries) == 0 || pgcOffset+8 > len(data) {
		return dvdProgram{}, false
	}
	pgcCount := int(binary.BigEndian.Uint16(data[pgcOffset : pgcOffset+2]))
	pgcn := int(entries[0][0])
	if pgcn < 1 || pgcn > pgcCount {
		return dvdProgram{}, false
	}
	entry := pgcOffset + 8 + (pgcn-1)*8
	if entry+8 > len(data) {
		return dvdProgram{}, false
	}
	if data[entry]&0x80 == 0 {
		return dvdProgram{}, false
	}
	base := pgcOffset + int(binary.BigEndian.Uint32(data[entry+4:entry+8]))
	if base+0xEA > len(data) {
		return dvdProgram{}, false
	}
	duration := float64(dvdTicksToMillisecondsFloor(dvdTimeToTicks(data[base+4:base+8]))) / 1000
	programCount := int(data[base+2])
	cellCount := int(data[base+3])
	if programCount <= 0 || cellCount <= 0 {
		return dvdProgram{duration: duration}, duration > 0
	}
	programMapStart := base + int(binary.BigEndian.Uint16(data[base+0xE6:base+0xE8]))
	cellPlayStart := base + int(binary.BigEndian.Uint16(data[base+0xE8:base+0xEA]))
	if programMapStart+programCount > len(data) || cellPlayStart >= len(data) {
		return dvdProgram{duration: duration}, duration > 0
	}
	programMap := data[programMapStart : programMapStart+programCount]
	firstSector := uint32(0)
	firstProgram := int(entries[0][1])
	if firstProgram >= 1 && firstProgram <= len(programMap) {
		firstCell := int(programMap[firstProgram-1]) - 1
		sectorOff := cellPlayStart + firstCell*0x18 + 8
		if firstCell >= 0 && sectorOff+4 <= len(data) {
			firstSector = binary.BigEndian.Uint32(data[sectorOff : sectorOff+4])
		}
	}
	cellTicks := make([]int64, 0, cellCount)
	for i := range cellCount {
		cell := cellPlayStart + i*0x18
		if cell+8 > len(data) {
			break
		}
		cellTicks = append(cellTicks, dvdTimeToTicks(data[cell+4:cell+8]))
	}
	chapters := make([]int64, 0, len(entries))
	for _, ptt := range entries {
		if int(ptt[0]) != pgcn || ptt[1] < 1 || int(ptt[1]) > len(programMap) {
			continue
		}
		cellIndex := int(programMap[int(ptt[1])-1]) - 1
		if cellIndex < 0 || cellIndex > len(cellTicks) {
			continue
		}
		var ticks int64
		for i := range cellIndex {
			ticks += cellTicks[i]
		}
		chapters = append(chapters, dvdTicksToMilliseconds(ticks))
	}
	return dvdProgram{duration: duration, chapters: chapters, firstSector: firstSector}, duration > 0
}

// dvdPGCTimeline returns valid PGCs in playback-sector order. A later PGC with
// the same first sector replaces the earlier entry, matching DVD menu-delay
// accounting while avoiding duplicate branch/angle timelines.
func dvdPGCTimeline(data []byte, pgcOffset int) []dvdPGCTimelineEntry {
	if pgcOffset <= 0 || pgcOffset+8 > len(data) {
		return nil
	}
	titles := map[uint32]dvdPGCTimelineEntry{}
	pgcCount := int(binary.BigEndian.Uint16(data[pgcOffset : pgcOffset+2]))
	for pgc := range pgcCount {
		entry := pgcOffset + 8 + pgc*8
		if entry+8 > len(data) {
			break
		}
		base := pgcOffset + int(binary.BigEndian.Uint32(data[entry+4:entry+8]))
		if base+0xEA > len(data) {
			continue
		}
		duration := dvdTimeToTicks(data[base+4 : base+8])
		cellCount := int(data[base+3])
		if duration <= 0 || cellCount <= 0 {
			continue
		}
		cellStart := base + int(binary.BigEndian.Uint16(data[base+0xE8:base+0xEA]))
		lastCell := cellStart + (cellCount-1)*0x18
		if cellStart < 0 || cellStart+12 > len(data) || lastCell < cellStart || lastCell+24 > len(data) {
			continue
		}
		firstSector := binary.BigEndian.Uint32(data[cellStart+8 : cellStart+12])
		lastSector := binary.BigEndian.Uint32(data[lastCell+20 : lastCell+24])
		if lastSector == 0 {
			continue
		}
		// MediaInfo stores these in a sector-keyed map, so a later PGC with
		// the same first sector replaces the earlier title-offset record.
		titles[firstSector] = dvdPGCTimelineEntry{pos: pgc, duration: duration}
	}
	sectors := make([]uint32, 0, len(titles))
	for sector := range titles {
		sectors = append(sectors, sector)
	}
	slices.Sort(sectors)
	timeline := make([]dvdPGCTimelineEntry, 0, len(sectors))
	for _, sector := range sectors {
		timeline = append(timeline, titles[sector])
	}
	return timeline
}

// dvdPGCTimelineDuration returns the complete unique PGC timeline in seconds.
// It is the duration counterpart to aggregate title-VOB size accounting.
func dvdPGCTimelineDuration(data []byte, pgcOffset int) float64 {
	var total int64
	for _, title := range dvdPGCTimeline(data, pgcOffset) {
		total += title.duration
	}
	return float64(total) / 90000
}

// dvdProgramStartDelays derives program delays in seconds from the complete
// sector-ordered PGC timeline.
func dvdProgramStartDelays(data []byte, pgcOffset int, programs []dvdProgram) []float64 {
	delays := make([]float64, len(programs))
	var elapsed int64
	for _, title := range dvdPGCTimeline(data, pgcOffset) {
		// MediaInfo writes through the original pre-prune menu position.
		// Positions beyond the retained menu slice are ignored by Fill().
		if title.pos >= 0 && title.pos < len(delays) {
			delays[title.pos] = math.Round(float64(elapsed)/90) / 1000
		}
		elapsed += title.duration
	}
	return delays
}

func parseDVDChapters(data []byte, pttOffset int, pgcOffset int) (float64, []int64) {
	if pttOffset+8 > len(data) || pgcOffset+8 > len(data) {
		return 0, nil
	}
	pttCount := int(binary.BigEndian.Uint16(data[pttOffset : pttOffset+2]))
	if pttCount == 0 {
		return 0, nil
	}
	pttEnd := int(binary.BigEndian.Uint32(data[pttOffset+4 : pttOffset+8]))
	pttStart := int(binary.BigEndian.Uint32(data[pttOffset+8 : pttOffset+12]))
	if pttStart == 0 || pttEnd <= 0 {
		return 0, nil
	}
	pttStart += pttOffset
	pttEnd += pttOffset + 1
	if pttStart >= len(data) || pttEnd > len(data) || pttEnd <= pttStart {
		return 0, nil
	}
	entries := []struct {
		pgcn uint16
		pgn  uint16
	}{}
	for pos := pttStart; pos+4 <= pttEnd; pos += 4 {
		pgcn := binary.BigEndian.Uint16(data[pos : pos+2])
		pgn := binary.BigEndian.Uint16(data[pos+2 : pos+4])
		if pgcn == 0 || pgn == 0 {
			continue
		}
		entries = append(entries, struct {
			pgcn uint16
			pgn  uint16
		}{pgcn: pgcn, pgn: pgn})
	}
	if len(entries) == 0 {
		return 0, nil
	}

	pgcCount := int(binary.BigEndian.Uint16(data[pgcOffset : pgcOffset+2]))
	if pgcCount == 0 {
		return 0, nil
	}
	pgcn := int(entries[0].pgcn)
	if pgcn < 1 || pgcn > pgcCount {
		return 0, nil
	}
	pgcEntryOff := pgcOffset + 8 + (pgcn-1)*8
	if pgcEntryOff+8 > len(data) {
		return 0, nil
	}
	pgcOffsetRel := int(binary.BigEndian.Uint32(data[pgcEntryOff+4 : pgcEntryOff+8]))
	pgcBase := pgcOffset + pgcOffsetRel
	if pgcBase+0x00EA > len(data) {
		return 0, nil
	}

	durationTicks := dvdTimeToTicks(data[pgcBase+4 : pgcBase+8])
	durationMs := dvdTicksToMilliseconds(durationTicks)
	duration := float64(durationMs) / 1000.0
	programCount := int(data[pgcBase+2])
	cellCount := int(data[pgcBase+3])
	if programCount == 0 || cellCount == 0 {
		return duration, nil
	}

	progMapOff := int(binary.BigEndian.Uint16(data[pgcBase+0x00E6 : pgcBase+0x00E8]))
	cellPlayOff := int(binary.BigEndian.Uint16(data[pgcBase+0x00E8 : pgcBase+0x00EA]))
	progMapStart := pgcBase + progMapOff
	cellPlayStart := pgcBase + cellPlayOff
	if progMapStart+programCount > len(data) || cellPlayStart >= len(data) {
		return duration, nil
	}

	programMap := data[progMapStart : progMapStart+programCount]
	cellTimes := make([]int64, 0, cellCount)
	for i := range cellCount {
		entryStart := cellPlayStart + i*0x18
		if entryStart+8 > len(data) {
			break
		}
		cellTimes = append(cellTimes, dvdTimeToTicks(data[entryStart+4:entryStart+8]))
	}

	starts := []int64{}
	for _, entry := range entries {
		if entry.pgcn != uint16(pgcn) {
			continue
		}
		pgn := int(entry.pgn)
		if pgn < 1 || pgn > len(programMap) {
			continue
		}
		cellIdx := int(programMap[pgn-1]) - 1
		if cellIdx < 0 || cellIdx > len(cellTimes) {
			continue
		}
		var startTicks int64
		for i := 0; i < cellIdx && i < len(cellTimes); i++ {
			startTicks += cellTimes[i]
		}
		starts = append(starts, dvdTicksToMilliseconds(startTicks))
	}
	return duration, starts
}

// dvdTimeToTicks decodes a four-byte DVD playback time into 90 kHz clock
// ticks, including its PAL or NTSC frame component.
func dvdTimeToTicks(b []byte) int64 {
	if len(b) < 4 {
		return 0
	}
	h := dvdBCD(b[0])
	m := dvdBCD(b[1])
	s := dvdBCD(b[2])
	frame := dvdBCD(b[3] & 0x3F)
	fpsCode := (b[3] >> 6) & 0x03
	ticks := int64(h*3600+m*60+s) * 90000
	switch fpsCode {
	case 1:
		ticks += int64(frame) * 3600
	case 3:
		ticks += int64(frame) * 3000
	}
	return ticks
}

// dvdTicksToMilliseconds converts non-negative 90 kHz DVD clock ticks to
// nearest milliseconds after tick sums have been accumulated.
func dvdTicksToMilliseconds(ticks int64) int64 {
	if ticks <= 0 {
		return 0
	}
	return (ticks*1000 + 45000) / 90000
}

// dvdTicksToMillisecondsFloor converts non-negative 90 kHz DVD clock ticks to
// milliseconds by truncating the fractional millisecond.
func dvdTicksToMillisecondsFloor(ticks int64) int64 {
	if ticks <= 0 {
		return 0
	}
	return (ticks * 1000) / 90000
}

// dvdBCD decodes one packed binary-coded decimal byte.
func dvdBCD(v byte) int {
	return int((v>>4)*10 + (v & 0x0F))
}

// formatDVDChapterTimeMs formats a non-negative chapter offset as an
// hour-based millisecond timestamp.
func formatDVDChapterTimeMs(msTotal int64) string {
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

// formatDVDDuration formats positive DVD durations at MediaInfo's minute-level
// precision, falling back to the common formatter below one minute.
func formatDVDDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	totalMinutes := int(seconds / 60)
	if totalMinutes <= 0 {
		return formatDuration(seconds)
	}
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	if hours > 0 {
		return fmt.Sprintf("%d h %d min", hours, minutes)
	}
	return fmt.Sprintf("%d min", minutes)
}

// formatDVDFrameRate formats a positive DVD frame rate with its recognized
// NTSC rational when applicable.
func formatDVDFrameRate(rate float64) string {
	if rate <= 0 {
		return ""
	}
	if rate > 29.0 && rate < 30.0 {
		return formatFrameRateRatio(29970, 1000)
	}
	return formatFrameRateWithRatio(rate)
}

// dvdMenuExtraNode builds the ordered chapter and stream-list object retained
// by DVD Menu streams.
func dvdMenuExtraNode(chapterStarts []int64, lists dvdMenuLists) structuredNode {
	fields := []jsonKV{}
	for i, startMs := range chapterStarts {
		key := "_" + strings.NewReplacer(":", "_", ".", "_").Replace(formatDVDChapterTimeMs(startMs))
		fields = append(fields, jsonKV{Key: key, Val: fmt.Sprintf("Chapter %d", i+1)})
	}
	if lists.audio != "" {
		fields = append(fields, jsonKV{Key: "List_Audio", Val: lists.audio})
	}
	if lists.sub43 != "" {
		fields = append(fields, jsonKV{Key: "List_Subtitles_4_3", Val: lists.sub43})
	}
	if lists.subWide != "" {
		fields = append(fields, jsonKV{Key: "List_Subtitles_Wide", Val: lists.subWide})
	}
	if lists.subLetter != "" {
		fields = append(fields, jsonKV{Key: "List_Subtitles_Letterbox", Val: lists.subLetter})
	}
	if lists.subPanScan != "" {
		fields = append(fields, jsonKV{Key: "List_Subtitles_PanScan", Val: lists.subPanScan})
	}
	return structuredObjectFromKVs(fields)
}

// dvdTitleSetSource derives the first title VOB name from a title-set IFO
// basename.
func dvdTitleSetSource(base string) string {
	upper := strings.ToUpper(base)
	if strings.HasPrefix(upper, "VTS_") && strings.HasSuffix(upper, ".IFO") {
		parts := strings.SplitN(upper, "_", 3)
		if len(parts) >= 2 {
			return fmt.Sprintf("VTS_%s_1.VOB", parts[1])
		}
	}
	return ""
}

// isDVDVideoTSPath reports whether path is directly inside a VIDEO_TS directory.
func isDVDVideoTSPath(path string) bool {
	return strings.EqualFold(filepath.Base(filepath.Dir(filepath.Clean(path))), "VIDEO_TS")
}

// isDVDTitleVOBPath reports whether path names a numbered title VOB directly
// inside a VIDEO_TS directory.
func isDVDTitleVOBPath(path string) bool {
	if !isDVDVideoTSPath(path) {
		return false
	}
	base := strings.ToUpper(filepath.Base(path))
	if len(base) != len("VTS_00_0.VOB") || !strings.HasPrefix(base, "VTS_") || !strings.HasSuffix(base, ".VOB") {
		return false
	}
	return base[4] >= '0' && base[4] <= '9' && base[5] >= '0' && base[5] <= '9' &&
		base[6] == '_' && base[7] >= '1' && base[7] <= '9'
}

// isDVDMenuVOBPath reports whether path names a VMG or title-set menu VOB
// directly inside a VIDEO_TS directory.
func isDVDMenuVOBPath(path string) bool {
	if !isDVDVideoTSPath(path) {
		return false
	}
	name := strings.ToUpper(filepath.Base(path))
	if name == "VIDEO_TS.VOB" {
		return true
	}
	return len(name) == len("VTS_00_0.VOB") && strings.HasPrefix(name, "VTS_") &&
		name[4] >= '0' && name[4] <= '9' && name[5] >= '0' && name[5] <= '9' &&
		name[6:] == "_0.VOB"
}

// dvdTitleSetVOBs returns the sorted title VOB paths associated with an IFO and
// their combined size, excluding the title-set menu VOB.
func dvdTitleSetVOBs(path string) ([]string, int64) {
	dir := filepath.Dir(path)
	base := strings.ToUpper(filepath.Base(path))
	if !strings.HasPrefix(base, "VTS_") {
		return nil, 0
	}
	parts := strings.SplitN(base, "_", 3)
	if len(parts) < 2 {
		return nil, 0
	}
	prefix := fmt.Sprintf("VTS_%s_", parts[1])
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, 0
	}
	var total int64
	paths := []string{}
	for _, entry := range entries {
		name := entry.Name()
		upper := strings.ToUpper(name)
		if !isDVDTitleMemberName(upper, prefix) {
			continue
		}
		info, err := root.Stat(name)
		if err != nil {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
		total += info.Size()
	}
	sort.Slice(paths, func(i, j int) bool {
		return dvdVOBIndex(paths[i]) < dvdVOBIndex(paths[j])
	})
	return paths, total
}

// isDVDTitleMemberName reports whether name is an exact numbered title VOB for
// prefix. The menu member with index zero is excluded.
func isDVDTitleMemberName(name, prefix string) bool {
	return len(name) == len(prefix)+len("1.VOB") && strings.HasPrefix(name, prefix) &&
		name[len(prefix)] >= '1' && name[len(prefix)] <= '9' && strings.HasSuffix(name, ".VOB")
}

// dvdMatchingBackupData reads only the exact sibling backup selected by DVD
// naming rules. The size cap keeps malformed or unrelated files bounded.
func dvdMatchingBackupData(path string) []byte {
	dir := filepath.Dir(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil
	}
	defer root.Close()
	want := strings.TrimSuffix(strings.ToUpper(filepath.Base(path)), ".IFO") + ".BUP"
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.ToUpper(entry.Name()) != want {
			continue
		}
		info, err := root.Stat(entry.Name())
		if err != nil || info.Size() < 12 || info.Size() > 16<<20 {
			return nil
		}
		file, err := root.Open(entry.Name())
		if err != nil {
			return nil
		}
		data, readErr := io.ReadAll(io.LimitReader(file, info.Size()+1))
		_ = file.Close()
		if readErr != nil || int64(len(data)) != info.Size() || string(data[:12]) != "DVDVIDEO-VTS" {
			return nil
		}
		return data
	}
	return nil
}

// mergeDVDVideoAttrs fills missing primary video attributes from the matching
// BUP attributes without replacing values decoded from the IFO.
func mergeDVDVideoAttrs(primary, backup dvdVideoAttrs) dvdVideoAttrs {
	if primary.Version == "" {
		primary.Version = backup.Version
	}
	if primary.Standard == "" {
		primary.Standard = backup.Standard
	}
	if primary.AspectRatio == "" {
		primary.AspectRatio = backup.AspectRatio
	}
	if primary.Width == 0 {
		primary.Width = backup.Width
	}
	if primary.Height == 0 {
		primary.Height = backup.Height
	}
	if primary.FrameRate == 0 {
		primary.FrameRate = backup.FrameRate
	}
	return primary
}

// mergeDVDAudioAttrs fills missing primary audio attributes positionally from
// the matching BUP while preserving all decoded IFO values.
func mergeDVDAudioAttrs(primary, backup []dvdAudioAttrs) []dvdAudioAttrs {
	if len(primary) == 0 {
		return backup
	}
	for i := range min(len(primary), len(backup)) {
		if primary[i].Format == "" {
			primary[i].Format = backup[i].Format
		}
		if primary[i].FormatInfo == "" {
			primary[i].FormatInfo = backup[i].FormatInfo
		}
		if primary[i].FormatProfile == "" {
			primary[i].FormatProfile = backup[i].FormatProfile
		}
		if primary[i].Channels == 0 {
			primary[i].Channels = backup[i].Channels
		}
		if primary[i].SampleRate == 0 {
			primary[i].SampleRate = backup[i].SampleRate
		}
		if primary[i].Language == "" {
			primary[i].Language = backup[i].Language
			primary[i].LanguageCode = backup[i].LanguageCode
		}
		if primary[i].LanguageMore == "" {
			primary[i].LanguageMore = backup[i].LanguageMore
		}
	}
	return primary
}

// mergeDVDSubpicAttrs fills missing primary subpicture language attributes
// positionally from the matching BUP while preserving decoded IFO values.
func mergeDVDSubpicAttrs(primary, backup []dvdSubpicAttrs) []dvdSubpicAttrs {
	if len(primary) == 0 {
		return backup
	}
	for i := range min(len(primary), len(backup)) {
		if primary[i].Language == "" {
			primary[i].Language = backup[i].Language
			primary[i].LanguageCode = backup[i].LanguageCode
		}
		if primary[i].LanguageMore == "" {
			primary[i].LanguageMore = backup[i].LanguageMore
		}
	}
	return primary
}

// mergeDVDTitleSetStreams deduplicates title-set streams while appending their
// common VOB source to each retained canonical extra object.
func mergeDVDTitleSetStreams(streams []Stream, source string) []Stream {
	if len(streams) == 0 {
		return streams
	}
	hasAudio := false
	hasCaption := false
	for _, stream := range streams {
		if stream.Kind == StreamAudio {
			hasAudio = true
		}
		if stream.Kind == StreamText {
			format, _ := canonicalSeedValue(stream, "Format")
			if format == "EIA-608" {
				hasCaption = true
			}
		}
	}
	out := []Stream{}
	for i := range streams {
		stream := streams[i]
		if stream.Kind == StreamMenu {
			continue
		}
		if stream.Kind == StreamAudio {
			format, _ := canonicalSeedValue(stream, "Format")
			if format == "MPEG Audio" {
				omitCanonicalStreamOrder(&stream)
			} else {
				stream.canonicalPolicy.SkipStreamOrder = false
				replaceCanonicalSeedFill(&stream, "StreamOrder", "0", "", "")
			}
		}
		if stream.Kind == StreamVideo && hasAudio {
			omitCanonicalStreamOrder(&stream)
		}
		if stream.Kind == StreamText {
			format, _ := canonicalSeedValue(stream, "Format")
			if format == "RLE" {
				stream.canonicalPolicy.SkipStreamOrder = false
				replaceCanonicalSeedFill(&stream, "StreamOrder", "0", "", "")
			} else {
				omitCanonicalStreamOrder(&stream)
			}
		}
		if source != "" {
			replaceCanonicalSeedText(&stream, "Source", source)
			appendCanonicalSeedObjectMembers(&stream, "extra", []structuredMember{{
				Key:   "Source",
				Value: structuredNode{Kind: structuredString, Text: source},
			}})

		}
		out = append(out, stream)
	}
	sortDVDPayloadStreams(out)
	cleanupDVDTextStreams(out, false, hasCaption)
	normalizeDVDVideoFields(out)
	normalizeDVDPayloadDelay(out)
	suppressDVDShortVideoScan(out)
	expandDVDPrimaryAudioDuration(out, true)
	clearDVDShortAudioTiming(out)
	extendDVDShortAC3Frame(out)
	alignDVDHeaderAudioStreams(out)
	normalizeDVDPCMVideoClock(out)
	markDVDProjectionOrder(out)
	return out
}

// markDVDProjectionOrder selects DVD-specific renderer ordering and moves the
// Source extra member to the end of each stream's ordered extra object.
func markDVDProjectionOrder(streams []Stream) {
	for i := range streams {
		streams[i].canonicalPolicy.DVDOrder = true
		node := canonicalSeedStructuredNode(&streams[i], "extra")
		if node == nil || node.Kind != structuredObject {
			continue
		}
		var source *structuredMember
		for _, member := range node.Object {
			if member.Key == "Source" {
				memberCopy := member
				source = &memberCopy
				break
			}
		}
		if source != nil {
			removeCanonicalSeedObjectMember(&streams[i], "extra", "Source")
			appendCanonicalSeedObjectMembers(&streams[i], "extra", []structuredMember{*source})
		}
	}
}

// deriveDVDPSVideoBitRateAndSize derives the sole video stream's bitrate and
// size from aggregate file size, authoritative logical duration, and known
// audio bitrates.
func deriveDVDPSVideoBitRateAndSize(streams []Stream, fileSize int64, duration float64, correctedDuration bool) {
	if fileSize <= 0 || duration <= 0 {
		return
	}
	videoIndex := -1
	audioBitRate := 0.0
	for i := range streams {
		switch streams[i].Kind {
		case StreamVideo:
			if videoIndex >= 0 {
				return
			}
			videoIndex = i
		case StreamAudio:
			value, ok := canonicalSeedValue(streams[i], "BitRate")
			if !ok {
				if _, discovered := canonicalSeedValue(streams[i], "FirstPacketOrder"); discovered {
					return
				}
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || parsed <= 0 {
				return
			}
			audioBitRate += parsed
		case StreamText:
		case StreamGeneral, StreamImage, StreamMenu:
		}
	}
	if videoIndex < 0 {
		return
	}
	overall := math.Round(float64(fileSize) * 8 / duration)
	bitRate := overall*0.99 - audioBitRate/0.99
	bitRate *= 0.99
	if bitRate < 10000 {
		return
	}
	videoDuration := dvdCanonicalNumber(streams[videoIndex], "Duration") / 1000
	if frameCount := dvdCanonicalNumber(streams[videoIndex], "FrameCount"); frameCount > 0 {
		if frameRate := dvdCanonicalNumber(streams[videoIndex], "FrameRate"); frameRate > 0 {
			videoDuration = frameCount / frameRate
		}
	}
	if videoDuration <= 0 {
		return
	}
	streamSize := int64(math.Round(bitRate / 8 * videoDuration))
	mode, _ := canonicalSeedValue(streams[videoIndex], "BitRate_Mode")
	headerRate := dvdCanonicalNumber(streams[videoIndex], "BitRate")
	correctedConstantClock := correctedDuration && (mode == "CBR" || mode == "Constant") && headerRate > 0
	if correctedConstantClock {
		if headerRateValue, ok := canonicalSeedValue(streams[videoIndex], "BitRate"); ok {
			replaceCanonicalSeedFill(&streams[videoIndex], "BitRate_Maximum", headerRateValue, "Maximum bit rate", formatBitrate(headerRate))
		}
		if value, _ := canonicalSeedValue(streams[videoIndex], "colour_primaries"); value == "BT.470 BG" {
			replaceCanonicalSeedFill(&streams[videoIndex], "colour_primaries", "BT.601 PAL", "", "")
		}
		if value, _ := canonicalSeedValue(streams[videoIndex], "transfer_characteristics"); value == "Gamma 2.8" {
			replaceCanonicalSeedFill(&streams[videoIndex], "transfer_characteristics", "BT.470 System B/G", "", "")
		}
		if value, _ := canonicalSeedValue(streams[videoIndex], "matrix_coefficients"); value == "BT.470 BG" {
			replaceCanonicalSeedFill(&streams[videoIndex], "matrix_coefficients", "BT.470 System B/G", "", "")
		}
		if streams[videoIndex].dvdMPEG2IntraDCFirst > 0 {
			replaceCanonicalSeedObjectValues(&streams[videoIndex], "extra", map[string]string{
				"intra_dc_precision": strconv.Itoa(streams[videoIndex].dvdMPEG2IntraDCFirst),
			})
		}
	}
	if mode != "CBR" && mode != "Constant" || correctedConstantClock {
		replaceCanonicalSeedFill(&streams[videoIndex], "BitRate", strconv.FormatInt(int64(math.Round(bitRate)), 10), "Bit rate", formatBitrate(bitRate))
	}
	replaceCanonicalSeedFill(&streams[videoIndex], "StreamSize", strconv.FormatInt(streamSize, 10), "Stream size", formatStreamSize(streamSize, fileSize))
}

// normalizeDVDConstantVideoClock aligns a constant-bitrate video's frame count
// to a compatible IFO duration. It reports whether the duration was accepted.
func normalizeDVDConstantVideoClock(streams []Stream, duration float64) bool {
	if duration <= 0 {
		return false
	}
	for i := range streams {
		if streams[i].Kind != StreamVideo {
			continue
		}
		mode, _ := canonicalSeedValue(streams[i], "BitRate_Mode")
		if mode != "CBR" && mode != "Constant" {
			return false
		}
		payloadDuration := dvdCanonicalNumber(streams[i], "Duration") / 1000
		if payloadDuration <= 0 || math.Abs(payloadDuration-duration) > max(1, duration*0.01) {
			return false
		}
		frameRate := dvdCanonicalNumber(streams[i], "FrameRate")
		if frameRate > 0 {
			frameCount := int64(math.Round(duration * frameRate))
			if existing := int64(dvdCanonicalNumber(streams[i], "FrameCount")); existing > 0 && existing != frameCount {
				difference := existing - frameCount
				if difference < 0 {
					difference = -difference
				}
				if difference <= 1 {
					return false
				}
			}
			replaceCanonicalSeedFill(&streams[i], "FrameCount", strconv.FormatInt(frameCount, 10), "", "")
		}
		return true
	}
	return false
}

// sortDVDPayloadStreams orders DVD payload streams by kind, then by the
// MediaInfo-compatible duration, subtitle, and stream-ID rules for that kind.
func sortDVDPayloadStreams(streams []Stream) {
	sort.SliceStable(streams, func(i, j int) bool {
		if streams[i].Kind != streams[j].Kind {
			return streams[i].Kind < streams[j].Kind
		}
		if streams[i].Kind == StreamAudio {
			leftDuration := dvdCanonicalNumber(streams[i], "Duration")
			rightDuration := dvdCanonicalNumber(streams[j], "Duration")
			if leftDuration != rightDuration {
				return leftDuration > rightDuration
			}
		}
		if streams[i].Kind == StreamText {
			leftFormat, _ := canonicalSeedValue(streams[i], "Format")
			rightFormat, _ := canonicalSeedValue(streams[j], "Format")
			leftID := dvdCanonicalNumber(streams[i], "ID")
			rightID := dvdCanonicalNumber(streams[j], "ID")
			if leftFormat != "RLE" {
				leftID = 32.5
			}
			if rightFormat != "RLE" {
				rightID = 32.5
			}
			if leftID != rightID {
				return leftID < rightID
			}
		}
		return dvdCanonicalNumber(streams[i], "ID") < dvdCanonicalNumber(streams[j], "ID")
	})
}

// normalizeDVDVOBStreams applies standalone title-VOB ordering, timing, and
// projection rules to parsed payload streams.
func normalizeDVDVOBStreams(streams []Stream) {
	hasCaption := false
	for _, stream := range streams {
		format, _ := canonicalSeedValue(stream, "Format")
		if stream.Kind == StreamText && format == "EIA-608" {
			hasCaption = true
			break
		}
	}
	for i := range streams {
		switch streams[i].Kind {
		case StreamAudio:
			format, _ := canonicalSeedValue(streams[i], "Format")
			if format == "MPEG Audio" {
				omitCanonicalStreamOrder(&streams[i])
			} else {
				streams[i].canonicalPolicy.SkipStreamOrder = false
				replaceCanonicalSeedFill(&streams[i], "StreamOrder", "0", "", "")
			}
		case StreamText:
			format, _ := canonicalSeedValue(streams[i], "Format")
			if format == "RLE" {
				streams[i].canonicalPolicy.SkipStreamOrder = false
				replaceCanonicalSeedFill(&streams[i], "StreamOrder", "0", "", "")
			} else {
				omitCanonicalStreamOrder(&streams[i])
			}
		case StreamGeneral, StreamVideo, StreamImage, StreamMenu:
		}
	}
	sortDVDPayloadStreams(streams)
	cleanupDVDTextStreams(streams, true, hasCaption)
	normalizeDVDVideoFields(streams)
	normalizeDVDPayloadDelay(streams)
	suppressDVDShortVideoScan(streams)
	expandDVDPrimaryAudioDuration(streams, false)
	clearDVDShortAudioTiming(streams)
	extendDVDShortAC3Frame(streams)
	normalizeDVDPCMVideoClock(streams)
	markDVDProjectionOrder(streams)
}

// normalizeDVDMenuVOBStreams removes title-only facts and applies menu-VOB
// timing and projection rules to parsed payload streams.
func normalizeDVDMenuVOBStreams(streams []Stream) {
	for i := range streams {
		switch streams[i].Kind {
		case StreamVideo:
			for _, field := range []struct {
				canonical fieldName
				text      string
			}{
				{"Format_Settings_PictureStructure", "Format settings, Picture structure"},
				{"Delay_DropFrame", ""},
				{"Delay_Original", "Original delay"},
				{"Delay_Original_DropFrame", ""},
				{"Delay_Original_Source", ""},
				{"TimeCode_FirstFrame", "Time code of first frame"},
				{"TimeCode_Source", "Time code source"},
				{"Gop_OpenClosed", "GOP, Open/Closed"},
				{"Gop_OpenClosed_FirstFrame", "GOP, Open/Closed of first frame"},
			} {
				clearCanonicalSeedField(&streams[i], field.canonical, field.text)
			}
			clearCanonicalSeedField(&streams[i], "BitRate_Maximum", "Maximum bit rate")
		case StreamAudio:
			streams[i].canonicalPolicy.SkipStreamOrder = false
			replaceCanonicalSeedFill(&streams[i], "StreamOrder", "0", "", "")
		case StreamText:
			streams[i].canonicalPolicy.SkipStreamOrder = false
			replaceCanonicalSeedFill(&streams[i], "StreamOrder", "0", "", "")
			clearDVDPayloadTextTiming(&streams[i])
		case StreamGeneral, StreamImage, StreamMenu:
		}
	}
	normalizeDVDVideoFields(streams)
	suppressDVDShortVideoScan(streams)
	markDVDProjectionOrder(streams)
}

// cleanupDVDTextStreams removes DVD subtitle and caption timing fields that
// MediaInfo omits for standalone, duplicate, or mixed-caption projections.
func cleanupDVDTextStreams(streams []Stream, standalone, hasCaption bool) {
	seenRLE := false
	firstRLEDelay := 0.0
	firstRLEVideoDelay := 0.0
	firstRLETiming := false
	for i := range streams {
		if streams[i].Kind != StreamText {
			continue
		}
		format, _ := canonicalSeedValue(streams[i], "Format")
		switch format {
		case "RLE":
			if standalone {
				clearCanonicalSeedField(&streams[i], "Duration", "Duration")
			}
			delay, hasDelay := canonicalSeedValue(streams[i], "Delay")
			videoDelay, hasVideoDelay := canonicalSeedValue(streams[i], "Video_Delay")
			parsedDelay, delayErr := strconv.ParseFloat(delay, 64)
			parsedVideoDelay, videoDelayErr := strconv.ParseFloat(videoDelay, 64)
			validTiming := hasDelay && hasVideoDelay && delayErr == nil && videoDelayErr == nil && math.Abs(parsedVideoDelay) < 60
			if !seenRLE && validTiming {
				firstRLEDelay = parsedDelay
				firstRLEVideoDelay = parsedVideoDelay
				firstRLETiming = true
			}
			keepTiming := validTiming && (!seenRLE || (!hasCaption && firstRLETiming && math.Abs(parsedDelay-firstRLEDelay) < 0.001 && math.Abs(parsedVideoDelay-firstRLEVideoDelay) < 0.001))
			if !keepTiming {
				for _, name := range []fieldName{"Delay", "Delay_Source", "Video_Delay"} {
					clearCanonicalSeedField(&streams[i], name, "")
				}
			}
			seenRLE = true
		case "EIA-608":
			clearCanonicalSeedField(&streams[i], "Duration_Start2End", "Duration of the visible content")
			clearCanonicalSeedField(&streams[i], "Duration_End", "End time")
		default:
		}
	}
}

// extendDVDShortAC3Frame accounts for MediaInfo's validation frame on short
// frame-aligned AC-3 streams and updates dependent counts and stream size.
func extendDVDShortAC3Frame(streams []Stream) {
	validationFrames := int64(1)
	for i := range streams {
		if streams[i].Kind == StreamVideo && dvdCanonicalNumber(streams[i], "FrameCount") == 1 {
			validationFrames = 2
			break
		}
	}
	for i := range streams {
		if streams[i].Kind != StreamAudio {
			continue
		}
		format, _ := canonicalSeedValue(streams[i], "Format")
		durationMs := dvdCanonicalNumber(streams[i], "Duration")
		if format != "AC-3" || durationMs <= 0 || durationMs >= 60000 {
			continue
		}
		if samplingCount := dvdCanonicalNumber(streams[i], "SamplingCount"); samplingCount > 0 && int64(samplingCount)%1536 != 0 {
			// A non-frame-aligned count already includes MediaInfo's partial
			// boundary payload and must not receive the validation frame.
			continue
		}
		durationMs += float64(32 * validationFrames)
		replaceCanonicalSeedFill(&streams[i], "Duration", strconv.FormatInt(int64(math.Round(durationMs)), 10), "Duration", formatDuration(durationMs/1000))
		for _, name := range []fieldName{"SamplingCount", "FrameCount", "StreamSize"} {
			value, ok := canonicalSeedValue(streams[i], name)
			if !ok {
				continue
			}
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				continue
			}
			switch name {
			case "SamplingCount":
				parsed += 1536 * validationFrames
			case "FrameCount":
				parsed += validationFrames
			case "StreamSize":
				if bitRate := dvdCanonicalNumber(streams[i], "BitRate"); bitRate > 0 {
					parsed += int64(math.Round(bitRate * 0.032 / 8 * float64(validationFrames)))
				}
			}
			replaceCanonicalSeedFill(&streams[i], name, strconv.FormatInt(parsed, 10), "", "")
		}
	}
}

// alignDVDHeaderAudioStreams normalizes one-frame MPEG-2 headers and extends
// shorter audio sampling counts to the longest header-level audio stream.
func alignDVDHeaderAudioStreams(streams []Stream) {
	headerVideo := false
	maxSamplingCount := int64(0)
	for i := range streams {
		if streams[i].Kind == StreamVideo && dvdCanonicalNumber(streams[i], "FrameCount") == 1 {
			headerVideo = true
			replaceCanonicalSeedFill(&streams[i], "ScanType", "Interlaced", "Scan type", "Interlaced")
			replaceCanonicalSeedFill(&streams[i], "ScanOrder", "TFF", "Scan order", "Top Field First")
			replaceCanonicalSeedFill(&streams[i], "Gop_OpenClosed", "Closed", "GOP, Open/Closed", "Closed")
			clearCanonicalSeedField(&streams[i], "Gop_OpenClosed_FirstFrame", "GOP, Open/Closed of first frame")
			if streams[i].dvdMPEG2MaxBitRate > 0 {
				replaceCanonicalSeedFill(&streams[i], "BitRate_Maximum", strconv.FormatInt(streams[i].dvdMPEG2MaxBitRate, 10), "Maximum bit rate", formatBitrate(float64(streams[i].dvdMPEG2MaxBitRate)))
			}
		}
		if streams[i].Kind == StreamAudio {
			maxSamplingCount = max(maxSamplingCount, int64(dvdCanonicalNumber(streams[i], "SamplingCount")))
		}
	}
	if !headerVideo || maxSamplingCount <= 0 {
		return
	}
	for i := range streams {
		if streams[i].Kind != StreamAudio {
			continue
		}
		count := int64(dvdCanonicalNumber(streams[i], "SamplingCount"))
		if count <= 0 || count >= maxSamplingCount {
			continue
		}
		replaceCanonicalSeedFill(&streams[i], "SamplingCount", strconv.FormatInt(maxSamplingCount, 10), "", "")
		clearCanonicalSeedField(&streams[i], "FrameCount", "")
		samplingRate := dvdCanonicalNumber(streams[i], "SamplingRate")
		bitRate := dvdCanonicalNumber(streams[i], "BitRate")
		if samplingRate > 0 && bitRate > 0 {
			size := int64(math.Round(float64(maxSamplingCount) / samplingRate * bitRate / 8))
			replaceCanonicalSeedFill(&streams[i], "StreamSize", strconv.FormatInt(size, 10), "Stream size", formatStreamSize(size, 0))
		}
		insertCanonicalSeedObjectMembersBefore(&streams[i], "extra", "Source", []structuredMember{
			{Key: "SamplingCount_Source", Value: structuredNode{Kind: structuredString, Text: "General_Duration"}},
			{Key: "Duration_Source", Value: structuredNode{Kind: structuredString, Text: "General_Duration"}},
		})
	}
}

// normalizeDVDPCMVideoClock derives the DVD video duration and frame count from
// a parsed PCM stream when MediaInfo treats that audio clock as authoritative.
func normalizeDVDPCMVideoClock(streams []Stream) {
	pcmDuration := 0.0
	for _, stream := range streams {
		format, _ := canonicalSeedValue(stream, "Format")
		if stream.Kind == StreamAudio && format == "PCM" {
			pcmDuration = dvdCanonicalNumber(stream, "Duration")
			break
		}
	}
	if pcmDuration <= 0 {
		return
	}
	for i := range streams {
		if streams[i].Kind != StreamVideo {
			continue
		}
		duration := pcmDuration + 7
		replaceCanonicalSeedFill(&streams[i], "Duration", strconv.FormatFloat(duration, 'f', -1, 64), "Duration", formatDuration(duration/1000))
		frameRate := dvdCanonicalNumber(streams[i], "FrameRate")
		if frameRate > 0 {
			frames := int64(math.Round(duration / 1000 * frameRate))
			replaceCanonicalSeedFill(&streams[i], "FrameCount", strconv.FormatInt(frames, 10), "", "")
		}
		break
	}
}

// suppressDVDShortVideoScan removes scan type and order from DVD videos shorter
// than three seconds.
func suppressDVDShortVideoScan(streams []Stream) {
	for i := range streams {
		if streams[i].Kind != StreamVideo {
			continue
		}
		if duration := dvdCanonicalNumber(streams[i], "Duration"); duration > 0 && duration < 3000 {
			clearCanonicalSeedField(&streams[i], "ScanType", "Scan type")
			clearCanonicalSeedField(&streams[i], "ScanOrder", "Scan order")
		}
	}
}

// dvdPayloadCanonicalDuration returns the longest credible video or discovered
// audio duration in seconds, preferring count-derived clocks when consistent.
func dvdPayloadCanonicalDuration(streams []Stream) float64 {
	duration := 0.0
	for _, stream := range streams {
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
			continue
		}
		if stream.Kind == StreamAudio {
			if _, discovered := canonicalSeedValue(stream, "FirstPacketOrder"); !discovered {
				continue
			}
		}
		streamDuration := dvdCanonicalNumber(stream, "Duration") / 1000
		countDuration := 0.0
		switch stream.Kind {
		case StreamVideo:
			frameRate := dvdCanonicalNumber(stream, "FrameRate")
			if numerator := dvdCanonicalNumber(stream, "FrameRate_Num"); numerator > 0 {
				if denominator := dvdCanonicalNumber(stream, "FrameRate_Den"); denominator > 0 {
					frameRate = numerator / denominator
				}
			}
			if frameCount := dvdCanonicalNumber(stream, "FrameCount"); frameCount > 0 && frameRate > 0 {
				countDuration = frameCount / frameRate
			}
		case StreamAudio:
			if samplingCount := dvdCanonicalNumber(stream, "SamplingCount"); samplingCount > 0 {
				if samplingRate := dvdCanonicalNumber(stream, "SamplingRate"); samplingRate > 0 {
					countDuration = samplingCount / samplingRate
				}
			}
		case StreamGeneral, StreamText, StreamImage, StreamMenu:
		}
		if countDuration > 0 && (streamDuration <= 0 || math.Abs(streamDuration-countDuration) <= max(1, streamDuration*0.01)) {
			streamDuration = math.Round(countDuration*1000) / 1000
		}
		duration = max(duration, streamDuration)
	}
	return duration
}

// normalizeDVDVideoFields canonicalizes DVD MPEG-2 aspect, color, scan, GOP,
// time-code, and delay fields for MediaInfo projection parity.
func normalizeDVDVideoFields(streams []Stream) {
	for i := range streams {
		if streams[i].Kind != StreamVideo {
			continue
		}
		width := dvdCanonicalNumber(streams[i], "Width")
		height := dvdCanonicalNumber(streams[i], "Height")
		display := dvdCanonicalNumber(streams[i], "DisplayAspectRatio")
		if width > 0 && height > 0 && display > 0 {
			if math.Abs(display-4.0/3.0) < 0.01 {
				display = 4.0 / 3.0
			} else if math.Abs(display-16.0/9.0) < 0.01 {
				display = 16.0 / 9.0
			}
			pixel := display / (width / height)
			replaceCanonicalSeedFill(&streams[i], "PixelAspectRatio", fmt.Sprintf("%.3f", pixel), "", "")
		}
		switch height {
		case 576:
			replaceCanonicalSeedFill(&streams[i], "Standard", "PAL", "Standard", "PAL")
			if value, _ := canonicalSeedValue(streams[i], "colour_primaries"); value == "BT.470 BG" {
				replaceCanonicalSeedFill(&streams[i], "colour_primaries", "BT.601 PAL", "", "")
			}
			if value, _ := canonicalSeedValue(streams[i], "transfer_characteristics"); value == "Gamma 2.8" {
				replaceCanonicalSeedFill(&streams[i], "transfer_characteristics", "BT.470 System B/G", "", "")
			}
			if value, _ := canonicalSeedValue(streams[i], "matrix_coefficients"); value == "BT.470 BG" {
				replaceCanonicalSeedFill(&streams[i], "matrix_coefficients", "BT.470 System B/G", "", "")
			}
		case 480:
			replaceCanonicalSeedFill(&streams[i], "Standard", "NTSC", "Standard", "NTSC")
			if frameRate := dvdCanonicalNumber(streams[i], "FrameRate"); math.Abs(frameRate-24000.0/1001.0) < 0.01 {
				if scanOrder, _ := canonicalSeedValue(streams[i], "ScanOrder"); scanOrder == "2:3 Pulldown" {
					clearCanonicalSeedField(&streams[i], "Standard", "Standard")
				}
			}
		}
		if scan, _ := canonicalSeedValue(streams[i], "ScanType"); scan == "Interlaced" {
			if picture, _ := canonicalSeedValue(streams[i], "Format_Settings_PictureStructure"); picture == "Frame" {
				if _, hasOrder := canonicalSeedValue(streams[i], "ScanOrder"); !hasOrder {
					replaceCanonicalSeedFill(&streams[i], "ScanOrder", "BFF", "Scan order", "Bottom Field First")
				}
			}
		}
		if gop, ok := canonicalSeedValue(streams[i], "Format_Settings_GOP"); ok && strings.HasPrefix(gop, "N=") {
			if bvop, _ := canonicalSeedValue(streams[i], "Format_Settings_BVOP"); bvop == "Yes" {
				replaceCanonicalSeedFill(&streams[i], "Format_Settings_GOP", "M=3, "+gop, "Format settings, GOP", "M=3, "+gop)
			}
		}
		if timeCode, _ := canonicalSeedValue(streams[i], "TimeCode_FirstFrame"); len(timeCode) == 11 && (timeCode[8] == ':' || timeCode[8] == ';') {
			hour, hourErr := strconv.Atoi(timeCode[0:2])
			minute, minuteErr := strconv.Atoi(timeCode[3:5])
			second, secondErr := strconv.Atoi(timeCode[6:8])
			frame, frameErr := strconv.Atoi(timeCode[9:11])
			frameRate := dvdCanonicalNumber(streams[i], "FrameRate")
			original := float64(hour*3600 + minute*60 + second)
			if hourErr == nil && minuteErr == nil && secondErr == nil && frameErr == nil && frameRate > 0 {
				original += float64(frame) / frameRate
				if scanOrder, _ := canonicalSeedValue(streams[i], "ScanOrder"); scanOrder == "2:3 Pulldown" && frame == 0 && original >= 3600 {
					original--
				}
				if timeCode[8] == ';' {
					original = math.Round(original*1000) / 1000
				} else {
					original = math.Floor(original*1000+1e-9) / 1000
				}
				replaceCanonicalSeedFill(&streams[i], "Delay_Original", fmt.Sprintf("%.3f", original), "Original delay", formatDelayMs(int64(original*1000)))
			}
			if timeCode[8] == ':' && second == 59 && frame == 0 && frameRate > 0 {
				delay := dvdCanonicalNumber(streams[i], "Delay")
				if delay > 0 {
					delay -= 2 / frameRate
					replaceCanonicalSeedFill(&streams[i], "Delay", fmt.Sprintf("%.9f", delay), "Delay", formatDelayMs(int64(math.Round(delay*1000))))
				}
			}
		}
	}
}

// normalizeDVDPayloadDelay applies the two-frame DVD timing correction when an
// audio stream proves the video clock is offset by exactly that amount.
func normalizeDVDPayloadDelay(streams []Stream) {
	videoIndex := -1
	frameRate := 0.0
	for i := range streams {
		if streams[i].Kind == StreamVideo {
			videoIndex = i
			frameRate = dvdCanonicalNumber(streams[i], "FrameRate")
			if numerator := dvdCanonicalNumber(streams[i], "FrameRate_Num"); numerator > 0 {
				if denominator := dvdCanonicalNumber(streams[i], "FrameRate_Den"); denominator > 0 {
					frameRate = numerator / denominator
				}
			}
			break
		}
	}
	if videoIndex < 0 || frameRate <= 0 {
		return
	}
	if delay := dvdCanonicalNumber(streams[videoIndex], "Delay"); delay < 0.1 {
		return
	}
	correction := 0.0
	for i := range streams {
		if streams[i].Kind != StreamAudio {
			continue
		}
		if _, ok := canonicalSeedValue(streams[i], "Video_Delay"); !ok {
			continue
		}
		videoDelay := dvdCanonicalNumber(streams[i], "Video_Delay")
		if videoDelay < 0 && math.Abs(math.Abs(videoDelay)-2/frameRate) < 0.002 {
			correction = -2 / frameRate
		}
		break
	}
	if correction == 0 {
		return
	}
	if delay, ok := canonicalSeedValue(streams[videoIndex], "Delay"); ok {
		parsed, err := strconv.ParseFloat(delay, 64)
		if err == nil {
			parsed += correction
			replaceCanonicalSeedFill(&streams[videoIndex], "Delay", fmt.Sprintf("%.9f", parsed), "Delay", formatDelayMs(int64(math.Round(parsed*1000))))
		}
	}
	if frameCount := int64(dvdCanonicalNumber(streams[videoIndex], "FrameCount")); frameCount > 1 && frameCount < 10000 {
		frameCount += int64(math.Round(-correction * frameRate))
		replaceCanonicalSeedFill(&streams[videoIndex], "FrameCount", strconv.FormatInt(frameCount, 10), "", "")
	}
	for i := range streams {
		if streams[i].Kind != StreamAudio && streams[i].Kind != StreamText {
			continue
		}
		value, ok := canonicalSeedValue(streams[i], "Video_Delay")
		if !ok {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			continue
		}
		parsed -= correction
		if math.Abs(parsed) < 0.002 {
			parsed = 0
		}
		replaceCanonicalSeedFill(&streams[i], "Video_Delay", fmt.Sprintf("%.3f", parsed), "Delay relative to video", formatDelayMs(int64(math.Round(parsed*1000))))
	}
}

// clearDVDShortAudioTiming removes audio delay fields that MediaInfo suppresses
// for absent or short DVD timing observations.
func clearDVDShortAudioTiming(streams []Stream) {
	for i := range streams {
		if streams[i].Kind != StreamAudio {
			continue
		}
		duration := dvdCanonicalNumber(streams[i], "Duration")
		delay := dvdCanonicalNumber(streams[i], "Delay")
		if delay > 0 && !(duration > 0 && duration < 60000 && delay > 1000) {
			continue
		}
		for _, name := range []fieldName{"Delay", "Delay_Source", "Video_Delay"} {
			clearCanonicalSeedField(&streams[i], name, "")
		}
	}
}

// clearDVDPayloadTextTiming removes all payload-derived timing from a DVD text stream.
func clearDVDPayloadTextTiming(stream *Stream) {
	for _, name := range []fieldName{"Duration", "Delay", "Delay_Source", "Video_Delay"} {
		clearCanonicalSeedField(stream, name, "")
	}
}

// expandDVDPrimaryAudioDuration extends the best-observed audio stream to a
// much longer video duration and recomputes dependent sampling and size facts.
func expandDVDPrimaryAudioDuration(streams []Stream, addSources bool) {
	videoDuration := 0.0
	for _, stream := range streams {
		if stream.Kind != StreamVideo {
			continue
		}
		if value, ok := canonicalSeedValue(stream, "Duration"); ok {
			videoDuration, _ = strconv.ParseFloat(value, 64)
		}
		break
	}
	if videoDuration < 60000 {
		return
	}
	candidate := -1
	maxSamplingCount := int64(0)
	for i := range streams {
		if streams[i].Kind != StreamAudio {
			continue
		}
		value, _ := canonicalSeedValue(streams[i], "SamplingCount")
		count, _ := strconv.ParseInt(value, 10, 64)
		if count > maxSamplingCount {
			candidate = i
			maxSamplingCount = count
		}
	}
	if candidate < 0 {
		return
	}
	currentValue, _ := canonicalSeedValue(streams[candidate], "Duration")
	currentDuration, _ := strconv.ParseFloat(currentValue, 64)
	if currentDuration <= 0 || videoDuration <= currentDuration*10 {
		return
	}
	videoDurationSeconds := videoDuration / 1000
	samplingRateValue, _ := canonicalSeedValue(streams[candidate], "SamplingRate")
	samplingRate, _ := strconv.ParseInt(samplingRateValue, 10, 64)
	bitRateValue, _ := canonicalSeedValue(streams[candidate], "BitRate")
	bitRate, _ := strconv.ParseInt(bitRateValue, 10, 64)
	replaceCanonicalSeedFill(&streams[candidate], "Duration", strconv.FormatFloat(videoDuration, 'f', -1, 64), "Duration", formatDuration(videoDurationSeconds))
	replaceCanonicalSeedFill(&streams[candidate], "Video_Delay", "0.000", "Delay relative to video", "0 ms")
	clearCanonicalSeedField(&streams[candidate], "FrameCount", "")
	if samplingRate > 0 {
		count := int64(math.Round(videoDurationSeconds * float64(samplingRate)))
		replaceCanonicalSeedFill(&streams[candidate], "SamplingCount", strconv.FormatInt(count, 10), "", "")
	}
	if bitRate > 0 {
		size := int64(math.Round(float64(bitRate) * videoDurationSeconds / 8))
		replaceCanonicalSeedFill(&streams[candidate], "StreamSize", strconv.FormatInt(size, 10), "Stream size", formatStreamSize(size, 0))
	}
	if addSources {
		appendCanonicalSeedObjectMembers(&streams[candidate], "extra", []structuredMember{
			{Key: "SamplingCount_Source", Value: structuredNode{Kind: structuredString, Text: "General_Duration"}},
			{Key: "Duration_Source", Value: structuredNode{Kind: structuredString, Text: "General_Duration"}},
		})
	}
}

// dvdCanonicalNumber parses a stream's canonical numeric value. Composite DVD
// IDs use their final component.
func dvdCanonicalNumber(stream Stream, name fieldName) float64 {
	value, _ := canonicalSeedValue(stream, name)
	if name == "ID" {
		if dash := strings.LastIndexByte(value, '-'); dash >= 0 {
			value = value[dash+1:]
		}
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

// overlayDVDDeclaredLanguages fills payload audio and subtitle language facts
// by exact DVD PES/substream identity, never by positional stream order.
func overlayDVDDeclaredLanguages(streams []Stream, audio []dvdAudioAttrs, subpics []dvdSubpicAttrs) {
	for i := range streams {
		id, ok := canonicalSeedValue(streams[i], "ID")
		if !ok {
			continue
		}
		switch streams[i].Kind {
		case StreamAudio:
			index := dvdDeclaredAudioIndex(id, audio)
			if index >= 0 && index < len(audio) && audio[index].LanguageCode != "" {
				replaceCanonicalSeedFill(&streams[i], "Language", audio[index].LanguageCode, "Language", audio[index].Language)
				if audio[index].LanguageMore != "" {
					replaceCanonicalSeedFill(&streams[i], "Language_More", audio[index].LanguageMore, "Language, more info", audio[index].LanguageMore)
				}
			}
		case StreamText:
			subID, ok := dvdPrivateSubstreamID(id)
			if !ok {
				continue
			}
			index := subID - 0x20
			if index >= 0 && index < len(subpics) && subpics[index].LanguageCode != "" {
				replaceCanonicalSeedFill(&streams[i], "Language", subpics[index].LanguageCode, "Language", subpics[index].Language)
				if subpics[index].LanguageMore != "" {
					replaceCanonicalSeedFill(&streams[i], "Language_More", subpics[index].LanguageMore, "Language, more info", subpics[index].LanguageMore)
				}
			}
		case StreamGeneral, StreamVideo, StreamImage, StreamMenu:
		}
	}
}

// dvdDeclaredAudioIndex returns the declared audio entry matching a payload
// stream ID, or -1 when no exact PES/substream identity matches.
func dvdDeclaredAudioIndex(id string, audio []dvdAudioAttrs) int {
	pesID, subID, hasSubID, ok := dvdPayloadStreamIdentity(id)
	if !ok {
		return -1
	}
	for index, attrs := range audio {
		if attrs.StreamID < 0 {
			continue
		}
		wantPESID, wantSubID, wantSubIDPresent := dvdAudioPayloadIdentity(attrs)
		if pesID == wantPESID && hasSubID == wantSubIDPresent && (!hasSubID || subID == wantSubID) {
			return index
		}
	}
	return -1
}

// dvdAudioPayloadIdentity maps declared DVD audio attributes to their PES and,
// when applicable, private-stream substream identity.
func dvdAudioPayloadIdentity(attrs dvdAudioAttrs) (pesID, subID int, hasSubID bool) {
	switch attrs.Format {
	case "MPEG Audio":
		return 0xC0 + attrs.StreamID, 0, false
	case "AC-3":
		return 0xBD, 0x80 + attrs.StreamID, true
	case "DTS":
		return 0xBD, 0x88 + attrs.StreamID, true
	case "PCM":
		return 0xBD, 0xA0 + attrs.StreamID, true
	default:
		return -1, -1, false
	}
}

// dvdPayloadStreamIdentity parses a canonical DVD stream ID into its PES and
// optional private-stream substream identity.
func dvdPayloadStreamIdentity(id string) (pesID, subID int, hasSubID, ok bool) {
	parts := strings.SplitN(id, "-", 2)
	parse := func(part string) (int, bool) {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			return 0, false
		}
		value, err := strconv.Atoi(fields[0])
		return value, err == nil
	}
	pesID, ok = parse(parts[0])
	if !ok || len(parts) == 1 {
		return pesID, 0, false, ok
	}
	subID, ok = parse(parts[1])
	return pesID, subID, true, ok
}

// mergeDVDDeclaredStreams retains payload streams while filling declared DVD
// identities that a bounded VOB scan did not encounter. Existing VOB facts
// always win; synthesized streams contain only IFO-owned attributes.
func mergeDVDDeclaredStreams(streams []Stream, audio []dvdAudioAttrs, subpics []dvdSubpicAttrs, duration float64, source string) []Stream {
	used := make([]bool, len(streams))
	result := make([]Stream, 0, max(len(streams), 1+len(audio)+len(subpics)))
	for i := range streams {
		if streams[i].Kind == StreamVideo {
			result = append(result, streams[i])
			used[i] = true
		}
	}
	appendKind := func(kind StreamKind, target int, makeMissing func(int) Stream) {
		count := 0
		for i := range streams {
			if !used[i] && streams[i].Kind == kind {
				result = append(result, streams[i])
				used[i] = true
				count++
			}
		}
		for index := count; index < target; index++ {
			result = append(result, makeMissing(index))
		}
	}
	appendKind(StreamAudio, len(audio), func(index int) Stream {
		return buildDVDDeclaredAudioStream(audio[index], duration, source)
	})
	appendKind(StreamText, len(subpics), func(index int) Stream {
		return buildDVDDeclaredTextStream(subpics[index], duration, source)
	})
	for i := range streams {
		if !used[i] {
			result = append(result, streams[i])
		}
	}
	return result
}

// buildDVDDeclaredAudioStream constructs the minimal canonical audio stream
// available from IFO-owned attributes when no VOB payload stream was observed.
func buildDVDDeclaredAudioStream(audio dvdAudioAttrs, duration float64, source string) Stream {
	id := dvdAudioPrivateID(audio, audio.StreamID)
	fields := []Field{}
	if audio.StreamID >= 0 {
		fields = append(fields, Field{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", id, id)})
	}
	if audio.Format != "" {
		fields = append(fields, Field{Name: "Format", Value: audio.Format})
	}
	if audio.FormatInfo != "" {
		fields = append(fields, Field{Name: "Format/Info", Value: audio.FormatInfo})
	}
	if audio.FormatProfile != "" {
		fields = append(fields, Field{Name: "Format profile", Value: audio.FormatProfile})
	}
	if duration > 0 {
		fields = append(fields, Field{Name: "Duration", Value: formatDVDDuration(duration)})
	}
	if audio.Channels > 0 {
		fields = append(fields, Field{Name: "Channel(s)", Value: formatChannels(uint64(audio.Channels))})
	}
	if audio.SampleRate > 0 {
		fields = append(fields, Field{Name: "Sampling rate", Value: formatSampleRate(audio.SampleRate)})
	}
	if audio.Format != "PCM" {
		fields = append(fields, Field{Name: "Compression mode", Value: "Lossy"})
	}
	if audio.Language != "" {
		fields = append(fields, Field{Name: "Language", Value: audio.Language})
	}
	if audio.LanguageMore != "" {
		fields = append(fields, Field{Name: "Language, more info", Value: audio.LanguageMore})
	}
	facts := &dvdStructuredFacts{}
	if audio.StreamID >= 0 {
		facts.SetSame("ID", fmt.Sprintf("189-%d", id))
	}
	if duration > 0 {
		facts.Set("Duration", strconv.FormatFloat(duration*1000, 'f', -1, 64), formatJSONSeconds(duration))
	}
	if audio.SampleRate > 0 && duration > 0 {
		facts.SetSame("SamplingCount", strconv.FormatInt(int64(duration*audio.SampleRate), 10))
	}
	if audio.LanguageCode != "" {
		facts.SetSame("Language", audio.LanguageCode)
	}
	if audio.LanguageMore != "" {
		facts.SetSame("Language_More", audio.LanguageMore)
	}
	stream := buildCanonicalDVDAudioStream(fields, facts, audio, duration)
	appendCanonicalSeedObjectMembers(&stream, "extra", []structuredMember{{Key: "Source", Value: structuredNode{Kind: structuredString, Text: source}}})
	publishCanonicalProjectionPolicy(&stream)
	return stream
}

// buildDVDDeclaredTextStream constructs the minimal canonical subpicture stream
// available from IFO-owned attributes when no VOB payload stream was observed.
func buildDVDDeclaredTextStream(subpic dvdSubpicAttrs, duration float64, source string) Stream {
	id := 0x20 + subpic.StreamID
	fields := []Field{
		{Name: "Format", Value: "RLE"},
		{Name: "Format/Info", Value: "Run-length encoding"},
		{Name: "Bit depth", Value: "2 bits"},
	}
	if subpic.StreamID >= 0 {
		fields = append([]Field{{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", id, id)}}, fields...)
	}
	if duration > 0 {
		fields = append(fields, Field{Name: "Duration", Value: formatDVDDuration(duration)})
	}
	if subpic.Language != "" {
		fields = append(fields, Field{Name: "Language", Value: subpic.Language})
	}
	if subpic.LanguageMore != "" {
		fields = append(fields, Field{Name: "Language, more info", Value: subpic.LanguageMore})
	}
	facts := &dvdStructuredFacts{}
	if subpic.StreamID >= 0 {
		facts.SetSame("ID", fmt.Sprintf("189-%d", id))
	}
	if duration > 0 {
		facts.Set("Duration", strconv.FormatFloat(duration*1000, 'f', -1, 64), formatJSONSeconds(duration))
	}
	if subpic.LanguageCode != "" {
		facts.SetSame("Language", subpic.LanguageCode)
	}
	if subpic.LanguageMore != "" {
		facts.SetSame("Language_More", subpic.LanguageMore)
	}
	stream := buildCanonicalDVDTextStream(fields, facts, subpic)
	appendCanonicalSeedObjectMembers(&stream, "extra", []structuredMember{{Key: "Source", Value: structuredNode{Kind: structuredString, Text: source}}})
	publishCanonicalProjectionPolicy(&stream)
	return stream
}

// dvdPrivateSubstreamID parses the private-stream component from a canonical
// DVD ID. It rejects IDs outside the 189-prefixed private stream.
func dvdPrivateSubstreamID(id string) (int, bool) {
	const prefix = "189-"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	end := len(id)
	if index := strings.IndexByte(id[len(prefix):], ' '); index >= 0 {
		end = len(prefix) + index
	}
	value, err := strconv.Atoi(id[len(prefix):end])
	return value, err == nil
}

// dvdVOBIndex extracts the trailing numeric title-set index from a VOB path.
func dvdVOBIndex(path string) int {
	name := strings.ToUpper(filepath.Base(path))
	if !strings.HasSuffix(name, ".VOB") {
		return 0
	}
	name = strings.TrimSuffix(name, ".VOB")
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return 0
	}
	value, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return value
}

// dvdJSONStreamStats returns the canonical video frame count and total
// canonical payload size used by DVD General derivation.
func dvdJSONStreamStats(streams []Stream) (string, int64) {
	var frameCount string
	for _, stream := range streams {
		if stream.Kind == StreamVideo {
			frameCount, _ = canonicalSeedValue(stream, "FrameCount")
		}
	}
	streamSizeSum := sumCanonicalStreamSizes(streams)
	return frameCount, streamSizeSum
}

func findStreamField(streams []Stream, kind StreamKind, name string) string {
	for _, stream := range streams {
		if stream.Kind != kind {
			continue
		}
		if value := findField(stream.Fields, name); value != "" {
			return value
		}
	}
	return ""
}
