package mediainfo

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const dvdSectorSize = 2048

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
	Format       string
	FormatInfo   string
	Channels     int
	SampleRate   float64
	Language     string
	LanguageCode string
}

// dvdSubpicAttrs stores decoded language metadata for one DVD subpicture
// stream.
type dvdSubpicAttrs struct {
	Language     string
	LanguageCode string
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
	// MediaInfo reports VTS IFO files from IFO metadata; it does not aggregate
	// payload details by scanning sibling VOB files.
	aggregateMode := false
	programMode := !aggregateMode

	var videoAttrs dvdVideoAttrs
	if isVMG {
		videoAttrs = parseDVDVideoAttrs(data, dvdVideoAttrMenuOffset)
	} else {
		videoAttrs = parseDVDVideoAttrs(data, dvdVideoAttrVTSOffset)
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
	var chapterStarts []int64
	var menuStream *Stream
	var audioAttrs []dvdAudioAttrs
	var subpicAttrs []dvdSubpicAttrs
	if isVTS {
		pttOffset := dvdPointer(data, dvdPTTSRPTPointerOff)
		pgcOffset := dvdPointer(data, dvdPGCIPointerOff)
		if pttOffset > 0 && pgcOffset > 0 {
			durationSeconds, chapterStarts = parseDVDChapters(data, pttOffset, pgcOffset)
		}
		if durationSeconds > 0 {
			info.Container.DurationSeconds = durationSeconds
			ifoDurationSeconds = durationSeconds
			generalFields = append(generalFields, Field{Name: "Duration", Value: formatDVDDuration(durationSeconds)})
		}
		audioAttrs = parseDVDAudioAttrs(data, dvdAudioCountVTSOffset, dvdAudioAttrVTSOffset)
		subpicAttrs = parseDVDSubpicAttrs(data, dvdSubpicCountVTSOff, dvdSubpicCountVTSOff+2)
	} else if isVMG {
		audioAttrs = parseDVDAudioAttrs(data, dvdAudioCountMenuOffset, dvdAudioAttrMenuOffset)
		subpicAttrs = parseDVDSubpicAttrs(data, dvdSubpicCountMenuOff, dvdSubpicCountMenuOff+2)
	}

	streams := []Stream{}
	titleSetParsed := false
	if aggregateMode {
		if vobPaths, vobSize := dvdTitleSetVOBs(path); len(vobPaths) > 0 && vobSize > 0 {
			info.FileSize = vobSize
			if ifoInfo, err := os.Stat(path); err == nil {
				info.FileSize += ifoInfo.Size()
			}
			if parsedInfo, parsedStreams, ok := ParseMPEGPSFiles(vobPaths, info.FileSize, mpegPSOptions{dvdExtras: true, dvdParsing: true, parseSpeed: opts.ParseSpeed}); ok {
				streams = mergeDVDTitleSetStreams(parsedStreams, dvdTitleSetSource(base))
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
					remaining := info.FileSize - streamSizeSum
					if remaining >= 0 {
						info.GeneralFacts.SetSame("StreamSize", strconv.FormatInt(remaining, 10))
					}
				}
			}
		}
	}
	if aggregateMode && titleSetParsed && ifoDurationSeconds > 0 {
		info.Container.DurationSeconds = ifoDurationSeconds
		durationSeconds = ifoDurationSeconds
		generalFields = setFieldValue(generalFields, "Duration", formatDVDDuration(ifoDurationSeconds))
		for i := range streams {
			streams[i].Fields = setFieldValue(streams[i].Fields, "Duration", formatDVDDuration(ifoDurationSeconds))
			replaceCanonicalSeedLegacyProjection(
				&streams[i],
				"Duration",
				strconv.FormatFloat(ifoDurationSeconds*1000, 'f', -1, 64),
				formatJSONSeconds(ifoDurationSeconds),
				"Duration",
				formatDVDDuration(ifoDurationSeconds),
			)
		}
	}

	generalFields = append(generalFields, Field{Name: "Overall bit rate mode", Value: "Variable"})
	if info.Container.DurationSeconds > 0 && info.FileSize > 0 {
		overall := (float64(info.FileSize) * 8) / info.Container.DurationSeconds
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
			if videoFrameCount > 0 {
				videoDurationSeconds = float64(videoFrameCount) / videoAttrs.FrameRate
			}
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
			for i, audio := range audioAttrs {
				audioFields := []Field{}
				if isVTS {
					if aggregateMode {
						audioFields = append(audioFields, Field{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", 128+i, 0x80+i)})
					} else {
						audioFields = append(audioFields, Field{Name: "ID", Value: fmt.Sprintf("%d (0x%X)", 128+i, 0x80+i)})
					}
				}
				if audio.Format != "" {
					audioFields = append(audioFields, Field{Name: "Format", Value: audio.Format})
				}
				if audio.FormatInfo != "" {
					audioFields = append(audioFields, Field{Name: "Format/Info", Value: audio.FormatInfo})
				}
				if durationSeconds > 0 {
					audioFields = append(audioFields, Field{Name: "Duration", Value: formatDVDDuration(durationSeconds)})
				}
				if audio.Channels > 0 {
					audioFields = append(audioFields, Field{Name: "Channel(s)", Value: formatChannels(uint64(audio.Channels))})
				}
				if audio.SampleRate > 0 {
					audioFields = append(audioFields, Field{Name: "Sampling rate", Value: formatSampleRate(audio.SampleRate)})
				}
				audioFields = append(audioFields, Field{Name: "Compression mode", Value: "Lossy"})
				suppressLanguage := aggregateMode
				if audio.Language != "" && !suppressLanguage {
					audioFields = append(audioFields, Field{Name: "Language", Value: audio.Language})
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
				if isVTS {
					if aggregateMode {
						audioFacts.SetSame("ID", fmt.Sprintf("189-%d", 128+i))
					} else {
						audioFacts.SetSame("ID", fmt.Sprintf("%d (0x%X)", 128+i, 0x80+i))
					}
				}
				audioStream := buildCanonicalDVDAudioStream(audioFields, audioFacts, audio, durationSeconds)
				streams = append(streams, audioStream)
			}
		}

		if len(subpicAttrs) > 0 {
			for i, subpic := range subpicAttrs {
				textFields := []Field{}
				if isVTS {
					if aggregateMode {
						textFields = append(textFields, Field{Name: "ID", Value: fmt.Sprintf("189 (0xBD)-%d (0x%X)", 32+i, 0x20+i)})
					} else {
						textFields = append(textFields, Field{Name: "ID", Value: fmt.Sprintf("%d (0x%X)", 32, 0x20)})
					}
				}
				textFields = append(textFields, Field{Name: "Format", Value: "RLE"})
				textFields = append(textFields, Field{Name: "Format/Info", Value: "Run-length encoding"})
				textFields = append(textFields, Field{Name: "Bit depth", Value: "2 bits"})
				if subpic.Language != "" && !aggregateMode {
					textFields = append(textFields, Field{Name: "Language", Value: subpic.Language})
				}
				textFacts := &dvdStructuredFacts{}
				if subpic.LanguageCode != "" && !aggregateMode {
					textFacts.SetSame("Language", subpic.LanguageCode)
				}
				if isVTS {
					if aggregateMode {
						textFacts.SetSame("ID", fmt.Sprintf("189-%d", 32+i))
					} else {
						textFacts.SetSame("ID", fmt.Sprintf("%d (0x%X)", 32, 0x20))
					}
				}
				textStream := buildCanonicalDVDTextStream(textFields, textFacts, subpic)
				streams = append(streams, textStream)
			}
		}
	}

	if len(chapterStarts) > 0 && ifoDurationSeconds > 0 {
		menuFields := []Field{{Name: "Duration", Value: formatDVDDuration(ifoDurationSeconds)}}
		for i, startMs := range chapterStarts {
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
			strconv.FormatFloat(ifoDurationSeconds*1000, 'f', -1, 64),
			formatJSONSeconds(ifoDurationSeconds),
		)
		menuExtra := dvdMenuExtraNode(chapterStarts, dvdMenuListsFromCounts(len(audioAttrs), len(subpicAttrs)))
		menu := buildCanonicalDVDMenuStream(menuFields, menuFacts, menuExtra)
		menuStream = &menu
	}

	if menuStream != nil {
		if aggregateMode {
			if source := dvdTitleSetSource(base); source != "" {
				appendCanonicalSeedLegacyObjectMembers(menuStream, "extra", []structuredMember{{
					Key:   "Source",
					Value: structuredNode{Kind: structuredString, Text: source},
				}})
			}
		}
		streams = append(streams, *menuStream)
	}

	for index := range streams {
		publishCanonicalProjectionPolicy(&streams[index])
	}
	info.Streams = streams
	return info, true
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
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
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
	if duration > 0 {
		builder.Fill("Duration", strconv.FormatFloat(duration*1000, 'f', -1, 64), "Duration", findField(fields, "Duration"))
	}
	if attrs.Channels > 0 {
		builder.Fill("Channels", strconv.Itoa(attrs.Channels), "Channel(s)", formatChannels(uint64(attrs.Channels)))
	}
	if attrs.SampleRate > 0 {
		builder.Fill("SamplingRate", strconv.FormatInt(int64(attrs.SampleRate+0.5), 10), "Sampling rate", formatSampleRate(attrs.SampleRate))
	}
	builder.Fill("Compression_Mode", "Lossy", "Compression mode", "Lossy")
	if attrs.Language != "" {
		builder.Fill("Language", firstNonEmpty(facts.Canonical("Language"), attrs.Language), "Language", attrs.Language)
	}
	if source := findField(fields, "Source"); source != "" {
		builder.Text("Source", source)
	}
	facts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
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
	builder.Fill("BitDepth", "2", "Bit depth", "2 bits")
	if attrs.Language != "" {
		builder.Fill("Language", firstNonEmpty(facts.Canonical("Language"), attrs.Language), "Language", attrs.Language)
	}
	facts.Apply(builder)
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
}

// buildCanonicalDVDMenuStream converts chapter/list fields and ordered menu
// extras into one direct canonical stream and its compatibility snapshot.
func buildCanonicalDVDMenuStream(fields []Field, facts *dvdStructuredFacts, extra structuredNode) Stream {
	builder := newCanonicalStreamBuilder(StreamMenu)
	for _, field := range fields {
		if field.Name == "Duration" {
			builder.Fill("Duration", facts.Canonical("Duration"), "Duration", field.Value)
			continue
		}
		builder.Text(field.Name, field.Value)
	}
	facts.Apply(builder)
	builder.OverrideStructuredNode("extra", extra)
	builder.MarkLegacyJSONRaw("extra", structuredNodeText(extra))
	return builder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true})
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
		lang := dvdTrimLang(data[off+2 : off+4])
		sampleCode := (b1 >> 4) & 0x03
		sampleRate := dvdAudioSampleRate(sampleCode)
		channels := int(b1&0x07) + 1
		langCode := normalizeLanguageCode(lang)
		attrs = append(attrs, dvdAudioAttrs{
			Format:       format,
			FormatInfo:   formatInfo,
			Channels:     channels,
			SampleRate:   sampleRate,
			Language:     formatLanguage(lang),
			LanguageCode: langCode,
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
		})
	}
	return attrs
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
		return "LPCM", "Linear PCM"
	case 4:
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
		return 96000
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

// parseDVDChapters decodes title playback duration and chapter-start ticks
// from the PTT and PGC tables.
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
// rounded milliseconds.
func dvdTicksToMilliseconds(ticks int64) int64 {
	if ticks <= 0 {
		return 0
	}
	// MediaInfo DVD durations are truncated to integer milliseconds (no rounding).
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0
	}
	var total int64
	paths := []string{}
	for _, entry := range entries {
		name := entry.Name()
		upper := strings.ToUpper(name)
		if !strings.HasPrefix(upper, prefix) || !strings.HasSuffix(upper, ".VOB") {
			continue
		}
		if strings.HasSuffix(upper, "_0.VOB") {
			continue
		}
		info, err := entry.Info()
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

// mergeDVDTitleSetStreams deduplicates title-set streams while appending their
// common VOB source to each retained canonical extra object.
func mergeDVDTitleSetStreams(streams []Stream, source string) []Stream {
	if len(streams) == 0 {
		return streams
	}
	hasAudio := false
	for _, stream := range streams {
		if stream.Kind == StreamAudio {
			hasAudio = true
			break
		}
	}
	audioIndex := 0
	out := []Stream{}
	for i := range streams {
		stream := streams[i]
		if stream.Kind == StreamMenu {
			continue
		}
		if stream.Kind == StreamAudio {
			replaceCanonicalSeedLegacyFill(&stream, "StreamOrder", strconv.Itoa(audioIndex), "", "")
			audioIndex++
		}
		if stream.Kind == StreamVideo && hasAudio {
			omitCanonicalStreamOrder(&stream)
		}
		if stream.Kind == StreamText {
			omitCanonicalStreamOrder(&stream)
		}
		if source != "" {
			replaceCanonicalSeedText(&stream, "Source", source)
			appendCanonicalSeedLegacyObjectMembers(&stream, "extra", []structuredMember{{
				Key:   "Source",
				Value: structuredNode{Kind: structuredString, Text: source},
			}})
		}
		out = append(out, stream)
	}
	return out
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
