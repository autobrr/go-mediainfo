package mediainfo

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AnalyzeFile analyzes one local media file with the default MediaInfo-compatible options.
func AnalyzeFile(path string) (Report, error) {
	return AnalyzeFileWithOptions(path, defaultAnalyzeOptions())
}

// normalizedBitRateMode converts structured abbreviations to canonical text.
func normalizedBitRateMode(mode string) string {
	switch strings.ToUpper(mode) {
	case "VBR":
		return "Variable"
	case "CBR":
		return "Constant"
	default:
		return mode
	}
}

// streamBitRateMode returns one stream's normalized canonical bitrate mode.
func streamBitRateMode(stream Stream) string {
	mode, _ := canonicalSeedValue(stream, "BitRate_Mode")
	return normalizedBitRateMode(mode)
}

// overallBitRateModeForKind combines one stream kind's modes, with variable
// mode taking precedence over constant mode.
func overallBitRateModeForKind(streams []Stream, kind StreamKind) string {
	mode := ""
	for _, stream := range streams {
		if stream.Kind != kind {
			continue
		}
		candidate := streamBitRateMode(stream)
		if candidate == "" {
			continue
		}
		if candidate == "Variable" {
			return candidate
		}
		if mode == "" {
			mode = candidate
		}
	}
	return mode
}

// matroskaTextBitRateModeForKind combines only modes exposed by Matroska's
// text projection, preserving structured-only mode visibility.
func matroskaTextBitRateModeForKind(streams []Stream, kind StreamKind) string {
	mode := ""
	for _, stream := range streams {
		if stream.Kind != kind {
			continue
		}
		current := normalizedBitRateMode(matroskaStreamDisplay(stream, "Bit rate mode"))
		if current == "Variable" {
			return current
		}
		if current == "Constant" {
			mode = current
		}
	}
	return mode
}

// overallBitRatePrecisionExtra builds the ordered General precision object
// shared by TS and BDAV compatibility output.
func overallBitRatePrecisionExtra(minimum, maximum int64) structuredNode {
	return structuredObjectFromKVs([]jsonKV{
		{Key: "OverallBitRate_Precision_Min", Val: strconv.FormatInt(minimum, 10)},
		{Key: "OverallBitRate_Precision_Max", Val: strconv.FormatInt(maximum, 10)},
	})
}

// bdavIndexVersion returns the four-digit Blu-ray index version associated
// with a stream path, or an empty string when the stream lacks disc context.
func bdavIndexVersion(streamPath string) string {
	streamDir := filepath.Dir(streamPath)
	if !strings.EqualFold(filepath.Base(streamDir), "STREAM") {
		return ""
	}
	bdmvDir := filepath.Dir(streamDir)
	if !strings.EqualFold(filepath.Base(bdmvDir), "BDMV") {
		return ""
	}
	root, err := os.OpenRoot(bdmvDir)
	if err != nil {
		return ""
	}
	defer root.Close()
	index, err := root.Open("index.bdmv")
	if err != nil {
		return ""
	}
	defer index.Close()

	var signature [8]byte
	if _, err := io.ReadFull(index, signature[:]); err != nil || string(signature[:4]) != "INDX" {
		return ""
	}
	return string(signature[4:])
}

// bdavOverallBitRateMaximum returns MediaInfo's Blu-ray mux-rate ceiling.
// Disc index version is authoritative; stream-shape heuristics remain only for
// standalone M2TS files that have no BDMV/index.bdmv context.
func bdavOverallBitRateMaximum(streamPath string, hasHEVC, hasPCM bool, videoStreams, audioStreams, textStreams int) string {
	switch bdavIndexVersion(streamPath) {
	case "0300":
		return "109000000"
	case "0200":
		return "48000000"
	}
	if hasHEVC {
		if hasPCM || (videoStreams == 1 && audioStreams == 1 && textStreams == 0) {
			return "127900000"
		}
		return "109000000"
	}
	return "48000000"
}

// shouldApplyBDAVSizing reports whether MediaInfo derives BDAV stream sizes
// from the container bitrate. Video-only HEVC clips are eligible without audio
// sizing; HEVC clips with audio require every audio stream to be sized, and text
// streams disable projection when present.
func shouldApplyBDAVSizing(primaryVideoFormat string, audioCount, audioSizedCount int, textCounts ...int) bool {
	textCount := 0
	if len(textCounts) > 0 {
		textCount = textCounts[0]
	}
	return primaryVideoFormat != "HEVC" || textCount == 0 && (audioCount == 0 || audioSizedCount == audioCount)
}

// AnalyzeFileWithOptions analyzes one local media file with opts and returns
// filesystem or open errors. A numbered title VOB inside VIDEO_TS is analyzed
// with its matching title-set members as one logical input.
func AnalyzeFileWithOptions(path string, opts AnalyzeOptions) (Report, error) {
	opts = normalizeAnalyzeOptions(opts)
	stat, err := os.Stat(path)
	if err != nil {
		return Report{}, err
	}
	fileSize := stat.Size()
	var completeNameLast string

	header := make([]byte, maxSniffBytes)
	file, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer file.Close()

	n, _ := io.ReadFull(file, header)
	header = header[:n]

	format := DetectFormat(header, path)

	// MediaInfo CLI continuous file names behavior (File_TestContinuousFileNames=1) applies to both
	// MPEG-TS and BDAV (M2TS) streams.
	if opts.TestContinuousFileNames && (format == "MPEG-TS" || format == "BDAV") {
		if set, ok := detectContinuousFileSet(path); ok {
			completeNameLast = set.LastPath
			fileSize = set.TotalSize
		}
	}
	var dvdVOBPaths []string
	if format == "MPEG-PS" && isDVDTitleVOBPath(path) {
		if paths, total := dvdTitleSetVOBs(path); len(paths) > 1 && total > 0 {
			dvdVOBPaths = paths
			fileSize = total
			completeNameLast = paths[len(paths)-1]
		}
	}

	general := Stream{Kind: StreamGeneral}
	general.Fields = append(general.Fields,
		Field{Name: "Complete name", Value: path},
		Field{Name: "Format", Value: format},
		Field{Name: "File size", Value: formatBytes(fileSize)},
	)
	if completeNameLast != "" {
		general.Fields = appendFieldUnique(general.Fields, Field{Name: "CompleteName_Last", Value: completeNameLast})
	}
	generalBuilder := newCanonicalStreamBuilder(StreamGeneral)
	generalBuilder.Text("Complete name", path)
	generalBuilder.Fill("Format", format, "Format", format)
	generalBuilder.Fill("FileSize", strconv.FormatInt(fileSize, 10), "File size", formatBytes(fileSize))
	if completeNameLast != "" {
		generalBuilder.Text("CompleteName_Last", completeNameLast)
	}
	general.canonicalSeed = generalBuilder.Snapshot(canonicalStreamPolicy{}).canonicalSeed

	info := ContainerInfo{}
	streams := []Stream{}
	matroskaGoGeneralStreamSize := ""
	matroskaCanRetainGeneralStreamSize := false
	matroskaRetainedGeneral := matroskaRetainedGeneralPresence{}
	switch format {
	case "MPEG-4", "QuickTime":
		if parsed, ok := ParseMP4(file, stat.Size()); ok {
			info = parsed.Container
			mp4Duration := info.DurationSeconds
			for _, track := range parsed.Tracks {
				if track.DurationSeconds > mp4Duration+0.5 {
					mp4Duration = track.DurationSeconds
				}
				if track.EditDuration > mp4Duration+0.5 {
					mp4Duration = track.EditDuration
				}
			}
			info.DurationSeconds = mp4Duration
			generalFacts := &canonicalStructuredFacts{}
			mp4WritingApplication := ""
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
				switch field.Name {
				case "Title":
					generalFacts.SetSame("Title", field.Value)
					generalFacts.SetSame("Movie", field.Value)
				case "Album":
					generalFacts.SetSame("Album", field.Value)
				case "Comment":
					generalFacts.SetSame("Comment", field.Value)
				case "Writing application":
					mp4WritingApplication = field.Value
					if name, version, _ := splitWritingApplication(field.Value); (name == "DVDFab" && version != "") || exposeWritingApplicationComponents(name, version) {
						generalFacts.SetSame("Encoded_Application_Name", name)
						generalFacts.SetSame("Encoded_Application_Version", version)
					}
				}
			}
			if encoded := formatMP4UTCTime(parsed.MovieCreation); encoded != "" {
				general.Fields = appendFieldUnique(general.Fields, Field{Name: "Encoded date", Value: encoded})
				if tagged := formatMP4UTCTime(parsed.MovieModified); tagged != "" {
					general.Fields = appendFieldUnique(general.Fields, Field{Name: "Tagged date", Value: tagged})
				}
			}
			if info.DurationSeconds > 0 {
				// Preserve fractional seconds in JSON (text Duration drops ms for long runtimes).
				generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(info.DurationSeconds*1000)), 10), formatJSONSeconds(info.DurationSeconds))
			}
			if overallBitRate, ok := overallBitRateValue(stat.Size(), info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			if headerSize, dataSize, footerSize, mdatCount, moovBeforeMdat, ok := mp4TopLevelSizes(file, stat.Size()); ok {
				generalFacts.SetSame("HeaderSize", strconv.FormatInt(headerSize, 10))
				generalFacts.SetSame("DataSize", strconv.FormatInt(dataSize, 10))
				generalFacts.SetSame("FooterSize", strconv.FormatInt(footerSize, 10))
				if moovBeforeMdat {
					generalFacts.SetSame("IsStreamable", "Yes")
				} else {
					generalFacts.SetSame("IsStreamable", "No")
				}
				_ = mdatCount
			}
			var generalFrameCount string
			x264WritingLibrary, x264EncodingSettings := scanX264Info(file)
			x264Applied := false
			x264BitrateProcessed := false
			for _, track := range parsed.Tracks {
				builder := newCanonicalStreamBuilder(track.Kind)
				fields := []Field{}
				displayDuration := mp4PresentationDurationSeconds(track)
				sourceDuration := 0.0
				// tkhd carries the presentation duration. Retain mdhd as Source_* when
				// an edit offset exposes a distinct media timeline.
				if track.EditDuration > 0 && track.DurationSeconds > 0 {
					if math.Abs(track.EditDuration-track.DurationSeconds) > 0.0005 {
						if track.trackDurationTicks == 0 {
							displayDuration = track.EditDuration
						}
						if track.EditMediaTime > 0 {
							sourceDuration = track.DurationSeconds
						}
					}
				}
				if strings.HasPrefix(track.HandlerName, "BAMTech ") {
					if track.Kind == StreamAudio {
						displayDuration = info.DurationSeconds
					} else if track.trackDurationTicks == 0 {
						displayDuration = track.DurationSeconds
					}
				}
				if sourceDuration == 0 && track.EditMediaTime <= 0 && mp4HasDistinctSampleDuration(track) {
					sourceDuration = mp4SampleDurationSeconds(track)
				}
				if track.Kind == StreamAudio && (track.sampleEntryType == "ac-3" || track.sampleEntryType == "ec-3") && info.DurationSeconds > 0 && math.Abs(displayDuration-info.DurationSeconds) < 0.05 {
					displayDuration = info.DurationSeconds
				}
				if track.Kind == StreamMenu {
					streams = append(streams, buildMP4ChapterMenu(file, track))
					continue
				}
				if track.ID > 0 {
					value := strconv.FormatUint(uint64(track.ID), 10)
					fields = appendFieldUnique(fields, Field{Name: "ID", Value: value})
					builder.Fill("ID", value, "ID", value)
				}
				if track.Format != "" {
					fields = appendFieldUnique(fields, Field{Name: "Format", Value: track.Format})
					formatValue, additionalFeatures := splitAACFormat(track.Format)
					builder.Fill("Format", formatValue, "Format", track.Format)
					builder.Structured("Format_AdditionalFeatures", additionalFeatures)
				}
				// Generic handler names carry no user-facing information; retain all other
				// handler names as track titles regardless of stream kind.
				name := strings.TrimSpace(firstNonEmpty(track.trackTitle, track.HandlerName))
				if name != "" && name != "SoundHandler" && name != "VideoHandler" && name != "MetaHandler" && name != "SubtitleHandler" {
					fields = appendFieldUnique(fields, Field{Name: "Title", Value: name})
					builder.Fill("Title", name, "Title", name)
				}
				if track.LanguageCode != "" {
					code := normalizeLanguageCode(track.LanguageCode)
					if lang := formatLanguage(code); lang != "" {
						fields = appendFieldUnique(fields, Field{Name: "Language", Value: lang})
						builder.Fill("Language", code, "Language", lang)
					}
				}
				if track.Kind == StreamText && track.sampleEntryType != "" {
					builder.Fill("CodecID", track.sampleEntryType, "Codec ID", track.sampleEntryType)
					if track.HandlerType != "" {
						builder.Fill("MuxingMode", track.HandlerType, "Muxing mode", track.HandlerType)
					}
				}
				if len(track.canonicalSeed) > 0 {
					builder.ImportCanonicalSeed(track.canonicalSeed)
				}
				for _, field := range track.Fields {
					fields = appendFieldUnique(fields, field)
				}
				var bitrate float64
				trackFacts := newMP4StructuredFacts(track.canonicalSeed)
				var extraNode *structuredNode
				if track.Kind == StreamVideo {
					trackFacts.Set("Rotation", "0.000")
					builder.Structured("Rotation", "0.000")
				}
				if encoded := formatMP4UTCTime(track.CreationTime); encoded != "" {
					fields = appendFieldUnique(fields, Field{Name: "Encoded date", Value: encoded})
					builder.Fill("Encoded_Date", encoded, "Encoded date", encoded)
					if tagged := formatMP4UTCTime(track.ModificationTime); tagged != "" {
						fields = appendFieldUnique(fields, Field{Name: "Tagged date", Value: tagged})
						builder.Fill("Tagged_Date", tagged, "Tagged date", tagged)
					}
				}
				if track.LanguageCode != "" {
					code := normalizeLanguageCode(track.LanguageCode)
					if code != "" {
						trackFacts.Set("Language", code)
						builder.OverrideStructured("Language", code)
					}
				}
				if displayDuration > 0 {
					if track.SampleBytes > 0 {
						durationForBitrate := displayDuration
						if sourceDuration > 0 && !strings.HasPrefix(track.HandlerName, "BAMTech ") {
							durationForBitrate = sourceDuration
						}
						bitrate = (float64(track.SampleBytes) * 8) / durationForBitrate
					}
					fields = addStreamDuration(fields, displayDuration)
					// Preserve fractional seconds in JSON (text Duration drops ms for long runtimes).
					durationValue := formatJSONSeconds(displayDuration)
					trackFacts.Set("Duration", durationValue)
					builder.Fill("Duration", strconv.FormatFloat(displayDuration*1000, 'f', -1, 64), "Duration", formatDuration(displayDuration))
					builder.OverrideStructured("Duration", durationValue)
					if sourceDuration > 0 {
						fields = appendFieldUnique(fields, Field{Name: "Source duration", Value: formatDuration(sourceDuration)})
						sourceDurationValue := formatJSONSeconds(sourceDuration)
						trackFacts.Set("Source_Duration", sourceDurationValue)
						builder.Fill("Source_Duration", strconv.FormatFloat(sourceDuration*1000, 'f', -1, 64), "Source duration", formatDuration(sourceDuration))
						builder.OverrideStructured("Source_Duration", sourceDurationValue)
						if track.SampleDelta > 0 && track.LastSampleDelta > 0 && track.Timescale > 0 && track.LastSampleDelta != track.SampleDelta {
							diffSamples := int64(track.LastSampleDelta) - int64(track.SampleDelta)
							diffMs := int64(math.Round(float64(diffSamples) * 1000 / float64(track.Timescale)))
							if diffMs != 0 {
								fields = appendFieldUnique(fields, Field{Name: "Source_Duration_LastFrame", Value: strconv.FormatInt(diffMs, 10) + " ms"})
								lastFrameDuration := formatJSONSeconds(float64(diffMs) / 1000.0)
								trackFacts.Set("Source_Duration_LastFrame", lastFrameDuration)
								display := strconv.FormatInt(diffMs, 10) + " ms"
								builder.Fill("Source_Duration_LastFrame", strconv.FormatInt(diffMs, 10), "Source_Duration_LastFrame", display)
								builder.OverrideStructured("Source_Duration_LastFrame", lastFrameDuration)
							}
						}
					}
					if strings.HasPrefix(track.HandlerName, "BAMTech ") && track.SampleDelta > 0 && track.LastSampleDelta > 0 && track.Timescale > 0 && track.LastSampleDelta != track.SampleDelta {
						diffSamples := int64(track.LastSampleDelta) - int64(track.SampleDelta)
						diffMs := int64(math.Round(float64(diffSamples) * 1000 / float64(track.Timescale)))
						if diffMs != 0 {
							lastFrameDuration := formatJSONSeconds(float64(diffMs) / 1000.0)
							trackFacts.Set("Duration_LastFrame", lastFrameDuration)
							builder.Fill("Duration_LastFrame", strconv.FormatInt(diffMs, 10), "Duration_LastFrame/String", strconv.FormatInt(diffMs, 10)+"ms")
							builder.OverrideStructured("Duration_LastFrame", lastFrameDuration)
						} else if diffSamples < 0 {
							trackFacts.Set("Duration_LastFrame", "-0.000")
							builder.DirectStructured("Duration_LastFrame", "-0.000")
						}
					}
					if bitrate > 0 && findField(fields, "Bit rate") == "" && trackFacts.Canonical("BitRate") == "" {
						if track.Kind != StreamVideo {
							if mode := bitrateMode(bitrate); mode != "" {
								fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: mode})
								builder.Fill("BitRate_Mode", mode, "Bit rate mode", mode)
							}
						}
						fields = addStreamBitrate(fields, bitrate)
						// Match official mediainfo rounding for derived MP4 bitrates.
						// Observed: video/text use rounding, audio uses truncation.
						bitRateValue := ""
						if track.Kind == StreamVideo || track.Kind == StreamText {
							bitRateValue = strconv.FormatInt(int64(math.Round(bitrate)), 10)
						} else {
							bitRateValue = strconv.FormatInt(int64(math.Floor(bitrate)), 10)
						}
						trackFacts.Set("BitRate", bitRateValue)
						builder.Fill("BitRate", bitRateValue, "Bit rate", formatBitrate(bitrate))
					}
				}
				if track.SampleBytes > 0 {
					streamBytes := int64(track.SampleBytes)
					displaySamples := 0.0
					if sourceDuration > 0 && displayDuration > 0 {
						switch {
						case strings.HasPrefix(track.HandlerName, "BAMTech "):
							streamBytes = int64(track.SampleBytes)
							displaySamples = float64(track.SampleCount)
						case track.Kind == StreamAudio:
							// Official mediainfo trims edit lists by whole AAC frames for StreamSize.
							if track.SampleDelta > 0 && track.SampleCount > 0 && track.Timescale > 0 {
								displaySamples = (displayDuration * float64(track.Timescale)) / float64(track.SampleDelta)
								wantFrames := int64(math.Round(displaySamples))
								drop := int64(track.SampleCount) - wantFrames
								if drop > 0 && drop <= int64(len(track.SampleSizeTail)) {
									dropped := uint64(0)
									start := int64(len(track.SampleSizeTail)) - drop
									for i := start; i < int64(len(track.SampleSizeTail)); i++ {
										dropped += uint64(track.SampleSizeTail[i])
									}
									if track.SampleBytes > dropped {
										streamBytes = int64(track.SampleBytes - dropped)
									}
								} else {
									streamBytes = int64(math.Round(float64(track.SampleBytes) * displayDuration / sourceDuration))
								}
							} else {
								streamBytes = int64(math.Round(float64(track.SampleBytes) * displayDuration / sourceDuration))
							}
						case track.SampleDelta > 0 && track.SampleCount > 0 && track.Timescale > 0:
							displaySamples = (displayDuration * float64(track.Timescale)) / float64(track.SampleDelta)
							wantSamples := int64(math.Round(displaySamples))
							switch {
							case wantSamples >= int64(track.SampleCount):
								streamBytes = int64(track.SampleBytes)
							case displaySamples > 0:
								streamBytes = int64(math.Round(float64(track.SampleBytes) * displaySamples / float64(track.SampleCount)))
							case bitrate > 0:
								streamBytes = int64(math.Round((bitrate * displayDuration) / 8))
							}
						case bitrate > 0:
							streamBytes = int64(math.Round((bitrate * displayDuration) / 8))
						}
					}
					if streamSize := formatStreamSize(streamBytes, stat.Size()); streamSize != "" {
						fields = appendFieldUnique(fields, Field{Name: "Stream size", Value: streamSize})
					}
					if streamBytes > 0 {
						streamSizeValue := strconv.FormatInt(streamBytes, 10)
						trackFacts.Set("StreamSize", streamSizeValue)
						builder.Fill("StreamSize", streamSizeValue, "Stream size", formatStreamSize(streamBytes, stat.Size()))
					}
					// MP4 AAC: derive BitRate from StreamSize and the exact mdhd duration
					// instead of trusting ESDS/btrt average-rate declarations.
					if track.Kind == StreamAudio {
						if findField(fields, "Format") != "" && strings.Contains(findField(fields, "Format"), "AAC") {
							if track.DurationSeconds > 0 && streamBytes > 0 {
								measured := float64(streamBytes) * 8 / track.DurationSeconds
								derived := math.Floor(measured)
								existing, hasExisting := parseInt(trackFacts.Canonical("BitRate"))
								matchesMeasured := hasExisting && existing == int64(math.Round(measured))
								deriveMeasured := !matchesMeasured && (!hasExisting || existing <= 0 ||
									existing%1000 == 0 && existing%8000 != 0 || strings.HasPrefix(track.HandlerName, "BAMTech "))
								if derived > 0 && deriveMeasured {
									bitRateValue := strconv.FormatInt(int64(derived), 10)
									trackFacts.Set("BitRate", bitRateValue)
									fields = setFieldValue(fields, "Bit rate", formatBitrate(derived))
									builder.ReplaceText("Bit rate", formatBitrate(derived))
									builder.DirectStructured("BitRate", bitRateValue)
								}
							}
						}
					}
					if sourceDuration > 0 {
						if sourceSize := formatStreamSize(int64(track.SampleBytes), stat.Size()); sourceSize != "" {
							fields = appendFieldUnique(fields, Field{Name: "Source stream size", Value: sourceSize})
						}
						sourceStreamSize := strconv.FormatInt(int64(track.SampleBytes), 10)
						trackFacts.Set("Source_StreamSize", sourceStreamSize)
						builder.Fill("Source_StreamSize", sourceStreamSize, "Source stream size", formatStreamSize(int64(track.SampleBytes), stat.Size()))
						if track.Kind == StreamAudio {
							if displaySamples > 0 {
								frameCountValue := strconv.FormatInt(int64(math.Round(displaySamples)), 10)
								trackFacts.Set("FrameCount", frameCountValue)
								builder.Structured("FrameCount", frameCountValue)
							}
							if track.SampleCount > 0 {
								sourceFrameCount := strconv.FormatUint(track.SampleCount, 10)
								trackFacts.Set("Source_FrameCount", sourceFrameCount)
								builder.Structured("Source_FrameCount", sourceFrameCount)
							}
						}
					} else if track.Kind == StreamAudio && track.SampleCount > 0 && trackFacts.Get("FrameCount") == "" {
						// No edit list: MediaInfo reports AAC FrameCount from the MP4 sample table.
						frameCountValue := strconv.FormatUint(track.SampleCount, 10)
						trackFacts.Set("FrameCount", frameCountValue)
						builder.Structured("FrameCount", frameCountValue)
					}
				}
				if track.Kind == StreamVideo && track.SampleCount > 0 && displayDuration > 0 {
					frameRateMode := "Constant"
					if track.VariableDeltas {
						frameRateMode = "Variable"
					}
					fields = appendFieldUnique(fields, Field{Name: "Frame rate mode", Value: frameRateMode})
					builder.Fill("FrameRate_Mode", frameRateMode, "Frame rate mode", frameRateMode)
					rate := mp4FrameRate(track, displayDuration)
					if rate > 0 {
						numerator, denominator := rationalizeMP4FrameRate(track, rate)
						display := formatFrameRate(rate)
						if denominator > 1 {
							display = formatFrameRateRatio(uint32(numerator), uint32(denominator))
						}
						fields = appendFieldUnique(fields, Field{Name: "Frame rate", Value: display})
						builder.Fill("FrameRate", formatJSONFloat(rate), "Frame rate", display)
						if numerator > 0 && denominator > 0 {
							builder.Structured("FrameRate_Num", strconv.Itoa(numerator))
							builder.Structured("FrameRate_Den", strconv.Itoa(denominator))
						}
						if derivedBitRate := mp4VideoBitRate(track, rate); derivedBitRate > 0 && trackFacts.Canonical("BitRate_Mode") != "CBR" {
							bitrate = derivedBitRate
							bitRateValue := strconv.FormatInt(int64(math.Round(derivedBitRate)), 10)
							trackFacts.Set("BitRate", bitRateValue)
							fields = setFieldValue(fields, "Bit rate", formatBitrate(derivedBitRate))
							builder.ReplaceText("Bit rate", formatBitrate(derivedBitRate))
							builder.DirectStructured("BitRate", bitRateValue)
						}
						if track.VariableDeltas && track.Timescale > 0 {
							if track.MaximumSampleDelta > 0 {
								minimum := float64(track.Timescale) / float64(track.MaximumSampleDelta)
								builder.Structured("FrameRate_Minimum", formatJSONFloat(minimum))
								builder.Text("FrameRate_Minimum/String", fmt.Sprintf("%.3f fps", minimum))
							}
							if track.MinimumSampleDelta > 0 {
								maximum := float64(track.Timescale) / float64(track.MinimumSampleDelta)
								builder.Structured("FrameRate_Maximum", formatJSONFloat(maximum))
								builder.Text("FrameRate_Maximum/String", fmt.Sprintf("%.3f fps", maximum))
							}
						}
					}
					if track.Width > 0 && track.Height > 0 && track.SampleBytes > 0 {
						pixelBitrate := bitrate
						if pixelBitrate <= 0 {
							pixelBitrate = (float64(track.SampleBytes) * 8) / displayDuration
						}
						if bits := formatBitsPerPixelFrame(pixelBitrate, track.Width, track.Height, rate); bits != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
							builder.ReplaceText("Bits/(Pixel*Frame)", bits)
						}
					}
					frameCountValue := strconv.FormatUint(track.SampleCount, 10)
					trackFacts.Set("FrameCount", frameCountValue)
					builder.Structured("FrameCount", frameCountValue)
					// Original frame rate mode for MP4 is detected from the AVC bitstream (SPS VUI),
					// and is filled earlier in the pipeline when present.
					if generalFrameCount == "" {
						generalFrameCount = strconv.FormatUint(track.SampleCount, 10)
					}
				}
				if track.Kind == StreamText && track.SampleCount > 0 && displayDuration > 0 {
					frameCountValue := strconv.FormatUint(track.SampleCount, 10)
					builder.Structured("FrameCount", frameCountValue)
					rate := float64(track.SampleCount) / displayDuration
					if rate > 0 {
						builder.Fill("FrameRate", formatJSONFloat(rate), "Frame rate", formatFrameRate(rate))
					}
					if track.nonEmptySampleCount > 0 {
						value := strconv.FormatUint(track.nonEmptySampleCount, 10)
						builder.Fill("Events_Total", value, "Events_Total", value)
					}
					builder.Fill("Forced", "No", "Forced", "No")
				}
				if track.Width > 0 {
					builder.Fill("Width", strconv.FormatUint(track.Width, 10), "Width", formatPixels(track.Width))
				}
				if track.Height > 0 {
					builder.Fill("Height", strconv.FormatUint(track.Height, 10), "Height", formatPixels(track.Height))
				}
				if displayAspect := findField(fields, "Display aspect ratio"); displayAspect != "" && trackFacts.Canonical("DisplayAspectRatio") == "" {
					if value, ok := parseRatioFloat(displayAspect); ok {
						builder.Fill("DisplayAspectRatio", formatJSONFloat(value), "Display aspect ratio", displayAspect)
					}
				}
				// MP4 AC-3: probe first frame to match MediaInfo's codec details without scanning the whole file.
				if track.Kind == StreamAudio && findField(fields, "Codec ID") == "ac-3" &&
					track.FirstChunkOff > 0 && len(track.SampleSizeHead) > 0 {
					sz := int(track.SampleSizeHead[0])
					if sz > 0 && int64(track.FirstChunkOff) > 0 && int64(track.FirstChunkOff) < stat.Size() {
						if sz > 1<<16 {
							sz = 1 << 16
						}
						buf := make([]byte, sz)
						if _, err := file.ReadAt(buf, int64(track.FirstChunkOff)); err == nil || err == io.EOF {
							if ac3, _, ok := parseAC3Frame(buf); ok {
								if ac3.channels > 0 {
									fields = setFieldValue(fields, "Channel(s)", formatChannels(ac3.channels))
									builder.Fill("Channels", strconv.FormatUint(ac3.channels, 10), "Channel(s)", formatChannels(ac3.channels))
								}
								if ac3.layout != "" {
									fields = setFieldValue(fields, "Channel layout", ac3.layout)
									builder.Fill("ChannelLayout", ac3.layout, "Channel layout", ac3.layout)
								}
								if ac3.sampleRate > 0 {
									fields = setFieldValue(fields, "Sampling rate", formatSampleRate(ac3.sampleRate))
									builder.Fill("SamplingRate", strconv.FormatInt(int64(math.Round(ac3.sampleRate)), 10), "Sampling rate", formatSampleRate(ac3.sampleRate))
								}
								if ac3.frameRate > 0 && ac3.spf > 0 {
									fields = setFieldValue(fields, "Frame rate", formatAudioFrameRate(ac3.frameRate, ac3.spf))
									builder.Fill("FrameRate", formatJSONFloat(ac3.frameRate), "Frame rate", formatAudioFrameRate(ac3.frameRate, ac3.spf))
								}
								fields = setFieldValue(fields, "Commercial name", "Dolby Digital")
								builder.Fill("Format_Commercial_IfAny", "Dolby Digital", "Commercial name", "Dolby Digital")
								// Keep a human-readable string in text, but match official JSON ServiceKind short codes.
								if code := ac3ServiceKindCode(ac3.bsmod); code != "" {
									trackFacts.Set("ServiceKind", code)
								}
								trackFacts.Set("Format_Settings_Endianness", "Big")
								builder.Structured("Format_Settings_Endianness", "Big")
								if ac3.spf > 0 {
									samplesPerFrame := strconv.Itoa(ac3.spf)
									trackFacts.Set("SamplesPerFrame", samplesPerFrame)
									builder.Structured("SamplesPerFrame", samplesPerFrame)
								}
								if ac3.frameRate > 0 {
									frameRateValue := formatJSONFloat(ac3.frameRate)
									trackFacts.Set("FrameRate", frameRateValue)
									builder.OverrideStructured("FrameRate", frameRateValue)
								}
								if durStr := trackFacts.Get("Duration"); durStr != "" && ac3.frameRate > 0 {
									if duration, err := strconv.ParseFloat(durStr, 64); err == nil && duration > 0 {
										frameCount := int64(math.Round(duration * ac3.frameRate))
										if frameCount > 0 {
											frameCountValue := strconv.FormatInt(frameCount, 10)
											trackFacts.Set("FrameCount", frameCountValue)
											builder.OverrideStructured("FrameCount", frameCountValue)
											if ac3.spf > 0 {
												samplingCount := strconv.FormatInt(frameCount*int64(ac3.spf), 10)
												trackFacts.Set("SamplingCount", samplingCount)
												builder.Structured("SamplingCount", samplingCount)
											}
										}
									}
								}
								if extraNode == nil {
									extraFields := []jsonKV{}
									if ac3.bsid > 0 {
										extraFields = append(extraFields, jsonKV{Key: "bsid", Val: strconv.Itoa(ac3.bsid)})
									}
									if ac3.hasDialnorm {
										extraFields = append(extraFields, jsonKV{Key: "dialnorm", Val: strconv.Itoa(ac3.dialnorm)})
									}
									if ac3.acmod > 0 {
										extraFields = append(extraFields, jsonKV{Key: "acmod", Val: strconv.Itoa(ac3.acmod)})
									}
									if ac3.lfeon >= 0 {
										extraFields = append(extraFields, jsonKV{Key: "lfeon", Val: strconv.Itoa(ac3.lfeon)})
									}
									if avg, minVal, maxVal, ok := ac3.dialnormStats(); ok {
										extraFields = append(extraFields, jsonKV{Key: "dialnorm_Average", Val: strconv.Itoa(avg)})
										extraFields = append(extraFields, jsonKV{Key: "dialnorm_Minimum", Val: strconv.Itoa(minVal)})
										if maxVal != minVal {
											extraFields = append(extraFields, jsonKV{Key: "dialnorm_Maximum", Val: strconv.Itoa(maxVal)})
										}
									}
									if len(extraFields) > 0 {
										node := structuredObjectFromKVs(extraFields)
										extraNode = &node
										builder.OverrideStructuredNode("extra", node)
									}
								}
							}
						}
					}
				}
				if track.Kind == StreamAudio && (track.sampleEntryType == "ac-3" || track.sampleEntryType == "ec-3") {
					if ac3, ok := probeMP4AC3(file, track); ok {
						node := applyMP4AC3Probe(builder, trackFacts, ac3, track.Format)
						extraNode = &node
						if displayDuration > 0 && ac3.sampleRate > 0 {
							durationMS := math.Round(displayDuration * 1000)
							samplingCount := strconv.FormatInt(int64(math.Round(durationMS*ac3.sampleRate/1000)), 10)
							trackFacts.Set("SamplingCount", samplingCount)
							builder.OverrideStructured("SamplingCount", samplingCount)
						}
					}
				}
				if track.Kind == StreamVideo && track.Format == "HEVC" {
					applyMP4HEVCProbe(builder, probeMP4HEVC(file, track), track)
				}
				trackWritingLibrary := x264WritingLibrary
				trackEncodingSettings := x264EncodingSettings
				if track.Kind == StreamVideo && track.Format == "AVC" {
					probe := probeMP4AVC(file, track)
					if probe.writingLibrary != "" {
						trackWritingLibrary = probe.writingLibrary
					}
					if probe.settings != "" {
						trackEncodingSettings = probe.settings
					}
					if probe.timeCode != "" {
						if strings.HasPrefix(track.HandlerName, "BAMTech ") && len(probe.timeCode) >= 3 {
							probe.timeCode = probe.timeCode[:len(probe.timeCode)-3] + ";" + probe.timeCode[len(probe.timeCode)-2:]
						}
						fields = appendFieldUnique(fields, Field{Name: "Time code of first frame", Value: probe.timeCode})
						builder.Fill("TimeCode_FirstFrame", probe.timeCode, "Time code of first frame", probe.timeCode)
					}
					if probe.hasGOP {
						value := fmt.Sprintf("M=%d, N=%d", probe.gopM, probe.gopN)
						fields = appendFieldUnique(fields, Field{Name: "Format settings, GOP", Value: value})
						builder.Fill("Format_Settings_GOP", value, "Format settings, GOP", value)
					}
				}
				if track.Kind == StreamAudio && (track.sampleEntryType == "Opus" || track.sampleEntryType == "opus") {
					builder.Fill("Duration_FirstFrame", "0.001", "Duration_FirstFrame/String", "1ms")
				}
				if track.AlternateGroup > 0 {
					value := strconv.FormatUint(uint64(track.AlternateGroup), 10)
					fields = appendFieldUnique(fields, Field{Name: "Alternate group", Value: value})
					builder.Fill("AlternateGroup", value, "Alternate group", value)
					if track.Kind != StreamVideo {
						if track.Default {
							fields = appendFieldUnique(fields, Field{Name: "Default", Value: "Yes"})
							builder.Fill("Default", "Yes", "Default", "Yes")
						} else {
							fields = appendFieldUnique(fields, Field{Name: "Default", Value: "No"})
							builder.Fill("Default", "No", "Default", "No")
						}
					}
				}
				if track.Kind == StreamAudio && track.EditMediaTime > 0 && track.Timescale > 0 {
					delayMs := int64(math.Round(float64(track.EditMediaTime) * 1000 / float64(track.Timescale)))
					if delayMs != 0 {
						node := structuredObjectFromKVs([]jsonKV{
							{Key: "Source_Delay", Val: "-" + strconv.FormatInt(delayMs, 10)},
							{Key: "Source_Delay_Source", Val: "Container"},
						})
						extraNode = &node
					}
				}
				if !x264Applied && track.Kind == StreamVideo && findField(fields, "Format") == "AVC" && (trackWritingLibrary != "" || trackEncodingSettings != "") {
					if trackWritingLibrary != "" {
						fields = appendFieldUnique(fields, Field{Name: "Writing library", Value: trackWritingLibrary})
						encoded := trackWritingLibrary
						if strings.HasPrefix(encoded, "x264 ") && !strings.HasPrefix(encoded, "x264 - ") {
							encoded = "x264 - " + strings.TrimPrefix(encoded, "x264 ")
						}
						builder.Fill("Encoded_Library", encoded, "Writing library", trackWritingLibrary)
						if name, version := splitEncodedLibrary(encoded); name != "" {
							builder.Structured("Encoded_Library_Name", name)
							builder.Structured("Encoded_Library_Version", version)
						}
						if trackEncodingSettings == "" && strings.Contains(trackWritingLibrary, "Encoder") {
							trackFacts.Set("Encoded_Library_Name", trackWritingLibrary)
						}
					}
					if trackEncodingSettings != "" {
						fields = appendFieldUnique(fields, Field{Name: "Encoding settings", Value: trackEncodingSettings})
						builder.Fill("Encoded_Library_Settings", trackEncodingSettings, "Encoding settings", trackEncodingSettings)
					}
					x264Applied = true
				}
				// MPEG-4/QuickTime: when x264 settings provide a nominal bitrate that is close to the
				// container-derived bitrate, prefer it (matches official MediaInfo output).
				if !x264BitrateProcessed && track.Kind == StreamVideo && findField(fields, "Format") == "AVC" {
					x264BitrateProcessed = true
					enc := firstNonEmpty(trackEncodingSettings, findField(fields, "Encoding settings"))
					if x264Bps, ok := findX264Bitrate(enc); ok && x264Bps > 0 {
						if existingBps, hasExisting := parseInt(trackFacts.Canonical("BitRate")); hasExisting && existingBps > 0 {
							delta := math.Abs(float64(existingBps)-x264Bps) / x264Bps
							if delta < 0.05 {
								bitRateValue := strconv.FormatInt(int64(math.Round(x264Bps)), 10)
								trackFacts.Set("BitRate", bitRateValue)
								builder.ReplaceText("Bit rate", formatBitrate(x264Bps))
								builder.OverrideStructured("BitRate", bitRateValue)
								width := track.Width
								height := track.Height
								fps := 0.0
								if track.SampleCount > 0 && displayDuration > 0 {
									fps = float64(track.SampleCount) / displayDuration
								}
								if bits := formatBitsPerPixelFrame(x264Bps, width, height, fps); bits != "" {
									builder.ReplaceText("Bits/(Pixel*Frame)", bits)
								}
							} else {
								nominal := strconv.FormatInt(int64(math.Round(x264Bps)), 10)
								trackFacts.Set("BitRate_Nominal", nominal)
								builder.Fill("BitRate_Nominal", nominal, "Nominal bit rate", formatBitrate(x264Bps))
							}
						}
					}
				}
				if track.Kind == StreamAudio {
					channels := trackFacts.Canonical("Channels")
					if channels != "" && trackFacts.Get("ChannelPositions") == "" && trackFacts.Get("Channels_Original") == "" {
						if positions := channelPositionsFromCount(channels); positions != "" {
							trackFacts.Set("ChannelPositions", positions)
						}
					}
					audioFormat, _ := splitAACFormat(track.Format)
					switch track.sampleEntryType {
					case "ac-3", "ec-3":
						trackFacts.Set("BitRate_Mode", "CBR")
						builder.Fill("BitRate_Mode", "CBR", "Bit rate mode", "Constant")
					case "Opus", "opus":
						trackFacts.Set("BitRate_Mode", "VBR")
						builder.Fill("BitRate_Mode", "VBR", "Bit rate mode", "Variable")
						if displayDuration > 0 {
							trackFacts.Set("SamplingCount", strconv.FormatInt(int64(math.Round(displayDuration*48000)), 10))
						}
					}
					if strings.EqualFold(audioFormat, "AAC") {
						if trackFacts.Get("SamplesPerFrame") == "" {
							trackFacts.Set("SamplesPerFrame", "1024")
						}
						if displayDuration > 0 {
							if sampleRate, ok := parseInt(trackFacts.Canonical("SamplingRate")); ok && sampleRate > 0 {
								durationMS := math.Round(displayDuration * 1000)
								samplingCount := math.Round(durationMS * float64(sampleRate) / 1000)
								value := strconv.FormatInt(int64(samplingCount), 10)
								trackFacts.Set("SamplingCount", value)
								builder.OverrideStructured("SamplingCount", value)
							}
						}
					}
				}
				extraFields := []jsonKV{}
				if track.DurationSeconds > 0 && (mp4ShouldExposeMediaHeaderDuration(track) ||
					track.Kind == StreamVideo && strings.HasPrefix(mp4WritingApplication, "GPAC-")) {
					extraFields = append(extraFields, jsonKV{Key: "mdhd_Duration", Val: strconv.FormatInt(mp4RoundedDurationMilliseconds(track.DurationSeconds), 10)})
				}
				if track.Kind == StreamVideo {
					if len(track.chapterTrackRefs) > 0 {
						extraFields = append(extraFields, jsonKV{Key: "Menus", Val: strconv.FormatUint(uint64(track.chapterTrackRefs[0]), 10)})
					}
					if len(extraFields) > 0 {
						configuration := findField(fields, "Codec configuration box")
						if configuration != "" {
							if track.hasDolbyVision {
								configuration = "hvcC+dvvC"
							}
							extraFields = append(extraFields, jsonKV{Key: "CodecConfigurationBox", Val: configuration})
						}
					}
				}
				if len(extraFields) > 0 {
					node := structuredObjectFromKVs(extraFields)
					if extraNode != nil && extraNode.Kind == structuredObject {
						extraNode.Object = append(extraNode.Object, node.Object...)
					} else {
						extraNode = &node
					}
				}
				trackFacts.Apply(builder)
				if extraNode != nil {
					builder.OverrideStructuredNode("extra", *extraNode)
				}
				streams = append(streams, builder.Snapshot(canonicalStreamPolicy{}))
			}
			if len(parsed.Chapters) > 0 {
				menuBuilder := newCanonicalStreamBuilder(StreamMenu)
				extras := make([]jsonKV, 0, len(parsed.Chapters))
				for _, chapter := range parsed.Chapters {
					textKey := formatMP4ChapterTimeText(chapter.startMs)
					jsonKey := formatMP4ChapterTimeKey(chapter.startMs)
					menuBuilder.Text(textKey, chapter.title)
					extras = append(extras, jsonKV{Key: "_" + jsonKey, Val: chapter.title})
				}
				node := structuredObjectFromKVs(extras)
				menuBuilder.StructuredNode("extra", node)
				streams = append(streams, menuBuilder.Snapshot(canonicalStreamPolicy{SkipStreamOrder: true, SkipComputed: true}))
			}
			for _, stream := range streams {
				if mode, ok := canonicalSeedValue(stream, "BitRate_Mode"); ok && (mode == "VBR" || mode == "Variable") {
					generalFacts.SetSame("OverallBitRate_Mode", "VBR")
					break
				}
			}
			if generalFrameCount != "" {
				generalFacts.SetSame("FrameCount", generalFrameCount)
			}
			// MP4 General StreamSize: remaining bytes after summing track stream sizes.
			streamSizeSum := sumCanonicalStreamSizes(streams)
			if streamSize, ok := remainingStreamSizeValue(stat.Size(), streamSizeSum); ok {
				generalFacts.SetSame("StreamSize", streamSize)
			}
			generalFacts.ApplyToStream(&general)
			if len(parsed.generalExtra) > 0 {
				builder := newCanonicalStreamBuilder(StreamGeneral)
				builder.ImportCanonicalSeed(general.canonicalSeed)
				node := structuredObjectFromKVs(parsed.generalExtra)
				builder.StructuredNode("extra", node)
				augmented := builder.Snapshot(canonicalStreamPolicy{})
				general.canonicalSeed = augmented.canonicalSeed
			}
		}
	case "Matroska":
		if parsed, ok := parseMatroskaForAnalysis(file, stat.Size(), opts); ok {
			info = parsed.Container
			matroskaCanRetainGeneralStreamSize = len(parsed.attachments) == 0
			replaceCanonicalSeedFill(&general, "Format", format, "", "")
			var rawWritingApp string
			var rawWritingLibrary string
			for _, field := range parsed.General {
				if field.Name == "Unique ID" {
					uniqueID := strings.TrimSpace(field.Value)
					if idx := strings.IndexAny(uniqueID, " ("); idx >= 0 {
						uniqueID = strings.TrimSpace(uniqueID[:idx])
					}
					replaceCanonicalSeedFill(&general, "UniqueID", uniqueID, "", "")
				}
				if field.Name == "Writing application" {
					rawWritingApp = field.Value
					field.Value = normalizeWritingApplication(field.Value)
				}
				if field.Name == "Writing library" {
					rawWritingLibrary = field.Value
				}
				if field.Name == "Title" {
					replaceCanonicalSeedFill(&general, "Title", field.Value, "", "")
				}
				if field.Name == "Encoded date" {
					replaceCanonicalSeedFill(&general, "Encoded_Date", field.Value, "", "")
				}
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Tracks...)
			coverNames := []string{}
			coverMimes := []string{}
			coverTypes := []string{}
			for _, attachment := range parsed.attachmentInfo {
				imageStream, ok := matroskaAttachmentImageStream(attachment)
				if !ok {
					continue
				}
				streams = append(streams, imageStream)
				coverType := matroskaAttachmentCoverType(attachment)
				if coverType == "" {
					continue
				}
				coverNames = append(coverNames, firstNonEmpty(attachment.description, attachment.name))
				mime := attachment.mime
				if mime == "" {
					mime = matroskaAttachmentImageMIME(attachment.data)
				}
				coverMimes = append(coverMimes, mime)
				coverTypes = append(coverTypes, coverType)
			}
			if len(coverNames) > 0 {
				values := make([]string, len(coverNames))
				for i := range coverNames {
					values[i] = "Yes"
				}
				replaceMatroskaCanonicalJSONOnlyOverrides(&general, map[string]string{
					"Cover": strings.Join(values, " / "), "Cover_Description": strings.Join(coverNames, " / "),
					"Cover_Mime": strings.Join(coverMimes, " / "), "Cover_Type": strings.Join(coverTypes, " / "),
				})
			}
			applyLegacyMatroskaFrameRateRatio(rawWritingApp, streams)
			generalExtras := []jsonKV{}
			if value := findField(general.Fields, "ErrorDetectionType"); value != "" {
				generalExtras = append(generalExtras, jsonKV{Key: "ErrorDetectionType", Val: value})
			}
			if len(parsed.attachments) > 0 {
				generalExtras = append(generalExtras, jsonKV{Key: "Attachments", Val: strings.Join(parsed.attachments, " / ")})
			}
			if len(generalExtras) > 0 {
				node := structuredObjectFromKVs(generalExtras)
				appendCanonicalSeedObjectMembers(&general, "extra", node.Object)
			}
			if creationTime := formatMatroskaTagEncodedDate(parsed.generalTags["CREATION_TIME"]); creationTime != "" {
				if encodedDate := findField(general.Fields, "Encoded date"); encodedDate != "" {
					creationTime = encodedDate + " / " + creationTime
				}
				general.Fields = setFieldValue(general.Fields, "Encoded date", creationTime)
				replaceCanonicalSeedFill(&general, "Encoded_Date", creationTime, "", "")
			}
			if rawWritingApp != "" {
				replaceCanonicalSeedFill(&general, "Encoded_Application", rawWritingApp, "", "")
				if name, version, _ := splitWritingApplication(rawWritingApp); exposeWritingApplicationComponents(name, version) {
					replaceCanonicalSeedFill(&general, "Encoded_Application_Name", name, "", "")
					if version != "" {
						replaceCanonicalSeedFill(&general, "Encoded_Application_Version", version, "", "")
					}
				}
			}
			if rawWritingLibrary != "" {
				if encoder := parsed.generalTags["ENCODER"]; strings.HasPrefix(encoder, "Lavf") && !strings.Contains(rawWritingLibrary, encoder) {
					rawWritingLibrary += " / " + encoder
					general.Fields = setFieldValue(general.Fields, "Writing library", rawWritingLibrary)
				}
				replaceCanonicalSeedFill(&general, "Encoded_Library", rawWritingLibrary, "", "")
				if name, version, _ := splitWritingApplication(rawWritingLibrary); exposeWritingApplicationComponents(name, version) {
					replaceCanonicalSeedFill(&general, "Encoded_Library_Name", name, "", "")
					replaceCanonicalSeedFill(&general, "Encoded_Library_Version", version, "", "")
				}
			}
			applyMatroskaGeneralTags(&general, parsed.scopedTags.general)
			// MediaInfo emits the resolved Matroska title as both Title and Movie.
			canonicalTitle, _ := canonicalSeedValue(general, "Title")
			if title := canonicalTitle; title != "" {
				replaceCanonicalSeedFill(&general, "Title", title, "", "")
				replaceCanonicalSeedFill(&general, "Movie", title, "", "")
			}
			generalValues := map[string]string{"IsStreamable": "Yes"}
			if info.DurationSeconds > 0 {
				projectedDuration := formatJSONFloat(info.DurationSeconds + 1e-9)
				if canonicalDuration, ok := decimalSecondsToMilliseconds(projectedDuration); ok {
					replaceCanonicalSeedProjection(&general, "Duration", canonicalDuration, projectedDuration, "", "")
				}
			}
			if overallBitRate, ok := overallBitRateValue(stat.Size(), info.DurationSeconds); ok {
				generalValues["OverallBitRate"] = overallBitRate
			}
			for _, stream := range streams {
				if strings.HasPrefix(matroskaStreamScalar(stream, "CodecID"), "D_WEBVTT/") {
					generalValues["IsStreamable"] = "No"
					break
				}
			}
			replaceMatroskaCanonicalOverrides(&general, generalValues)
			overallModeField := overallBitRateModeForKind(streams, StreamVideo)
			// Variable audio makes the complete payload variable even when video is CBR.
			audioModeField := overallBitRateModeForKind(streams, StreamAudio)
			if overallModeField == "" || audioModeField == "Variable" {
				overallModeField = audioModeField
			}
			if overallModeField == "Variable" {
				textMode := matroskaTextBitRateModeForKind(streams, StreamVideo)
				audioTextMode := matroskaTextBitRateModeForKind(streams, StreamAudio)
				if textMode == "" || audioTextMode == "Variable" {
					textMode = audioTextMode
				}
				if textMode == "Variable" {
					general.Fields = appendFieldUnique(general.Fields, Field{Name: "Overall bit rate mode", Value: textMode})
				}
				replaceCanonicalSeedFill(&general, "OverallBitRate_Mode", mapBitrateMode(overallModeField), "", "")
			}
			for _, stream := range streams {
				if stream.Kind != StreamVideo {
					continue
				}
				frameCount, _ := canonicalSeedValue(stream, "FrameCount")
				if frameCount != "" {
					replaceCanonicalSeedFill(&general, "FrameCount", frameCount, "", "")
				}
				break
			}
			goNominalBitRate := make([]bool, len(streams))
			for i := range streams {
				if streams[i].Kind != StreamVideo {
					continue
				}
				goNominalBitRate[i] = matroskaStreamDisplay(streams[i], "Bit rate") == "" && matroskaStreamScalar(streams[i], "BitRate") == ""
			}
			deriveMatroskaVideoBitRateAndSize(general, streams, stat.Size())
			// MediaInfo prefers x264 settings for bitrate/VBV constraints when available.
			for i := range streams {
				if streams[i].Kind != StreamVideo {
					continue
				}
				enc := matroskaStreamDisplay(streams[i], "Encoding settings")
				if enc == "" {
					continue
				}
				if !matroskaVideoHasX264Settings(streams[i]) {
					continue
				}
				x264Bitrate, x264HasBitrate := findX264Bitrate(enc)
				if goNominalBitRate[i] && x264HasBitrate && x264Bitrate > 0 {
					fillMatroskaRetainedJSON(&streams[i], "BitRate_Nominal", strconv.FormatInt(int64(math.Round(x264Bitrate)), 10))
				}
				if x264HasBitrate && x264Bitrate > 0 {
					// Match official mediainfo: when a container-derived bitrate exists, prefer x264's
					// nominal bitrate if it's very close, and do not emit BitRate_Nominal.
					existingBps := int64(0)
					if parsed, ok := parseInt(matroskaStreamScalar(streams[i], "BitRate")); ok && parsed > 0 {
						existingBps = parsed
					}
					if existingBps > 0 {
						delta := math.Abs(float64(existingBps)-x264Bitrate) / x264Bitrate
						constantTarget := strings.Contains(enc, "rc=2pass") || strings.Contains(enc, "rc=cbr")
						// Constant targets tolerate muxing overhead, but a larger mismatch
						// identifies a trimmed or remuxed payload whose measured rate wins.
						if delta < 0.02 || constantTarget && delta < 0.05 {
							replaceCanonicalSeedFill(&streams[i], "BitRate", strconv.FormatInt(int64(math.Round(x264Bitrate)), 10), "Bit rate", formatBitrate(x264Bitrate))
							// A rate absent before remainder derivation remains an explicit x264
							// nominal target even when the derived average converges on it.
							if !goNominalBitRate[i] {
								clearCanonicalSeedField(&streams[i], "BitRate_Nominal", "Nominal bit rate")
							}
						} else if constantTarget {
							replaceCanonicalSeedFill(&streams[i], "BitRate_Nominal", strconv.FormatInt(int64(math.Round(x264Bitrate)), 10), "Nominal bit rate", formatBitrate(x264Bitrate))
						}
					}
				}

				if x264HasBitrate && x264Bitrate > 0 &&
					matroskaStreamDisplay(streams[i], "Nominal bit rate") == "" &&
					matroskaStreamDisplay(streams[i], "Bit rate") == "" &&
					matroskaStreamScalar(streams[i], "BitRate") == "" {
					replaceCanonicalSeedFill(&streams[i], "BitRate_Nominal", strconv.FormatInt(int64(math.Round(x264Bitrate)), 10), "Nominal bit rate", formatBitrate(x264Bitrate))
				}
				// MediaInfo reports VBV constraints only when HRD signaling is enabled.
				hrdEnabled, _ := matroskaX264HRDState(enc)
				if hrdEnabled {
					if maxKbps, ok := findX264VbvMaxrate(enc); ok && maxKbps > 0 {
						fillMatroskaRetainedJSON(&streams[i], "BitRate_Maximum", strconv.FormatInt(int64(math.Round(maxKbps*1000)), 10))
					}
					if bufKbps, ok := findX264VbvBufsize(enc); ok && bufKbps > 0 {
						fillMatroskaRetainedJSON(&streams[i], "BufferSize", strconv.FormatInt(int64(math.Round(bufKbps*1000)), 10))
					}
				}
				if hrdEnabled {
					if maxKbps, ok := findX264VbvMaxrate(enc); ok && maxKbps > 0 {
						maxBps := maxKbps * 1000
						if matroskaStreamDisplay(streams[i], "Maximum bit rate") == "" && matroskaStreamScalar(streams[i], "BitRate_Maximum") == "" {
							replaceCanonicalSeedFill(&streams[i], "BitRate_Maximum", strconv.FormatInt(int64(math.Round(maxBps)), 10), "Maximum bit rate", formatBitrate(maxBps))
						}
					}
					if bufKbps, ok := findX264VbvBufsize(enc); ok && bufKbps > 0 {
						bufBps := bufKbps * 1000
						if matroskaStreamScalar(streams[i], "BufferSize") == "" {
							replaceCanonicalSeedFill(&streams[i], "BufferSize", strconv.FormatInt(int64(math.Round(bufBps)), 10), "", "")
						}
					}
				}
				bitRate, _ := canonicalSeedValue(streams[i], "BitRate")
				nominal, _ := canonicalSeedValue(streams[i], "BitRate_Nominal")
				mode, _ := canonicalSeedValue(streams[i], "BitRate_Mode")
				if bitRate != "" && nominal != "" && (bitRate == nominal || strings.TrimSpace(mode) == "") {
					clearCanonicalSeedField(&streams[i], "BitRate_Nominal", "Nominal bit rate")
				}
			}
			streamSizeSum := sumCanonicalStreamSizes(streams)
			clearCanonicalSeedField(&general, "StreamSize", "")
			if matroskaPayloadStreamSizesKnown(streams) {
				if remainingSize, ok := remainingStreamSizeValue(stat.Size(), streamSizeSum); ok {
					replaceMatroskaCanonicalJSONOnlyOverrides(&general, map[string]string{"StreamSize": remainingSize})
				}
			}
			_, matroskaRetainedGeneral.streamSize = projectedCanonicalSeedValue(general, "StreamSize")
			_, matroskaRetainedGeneral.overallBitRateMode = projectedCanonicalSeedValue(general, "OverallBitRate_Mode")
			applyMatroskaWriterRules(rawWritingApp, &general, streams)
		}
	case "MPEG-TS":
		if parsedInfo, parsedStreams, generalFields, ok := ParseMPEGTS(file, stat.Size(), opts.ParseSpeed); ok {
			info = parsedInfo
			generalFacts := &canonicalStructuredFacts{}
			var generalExtra *structuredNode
			if completeNameLast != "" {
				generalFacts.SetSame("FileSize", strconv.FormatInt(fileSize, 10))
			}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			// MediaInfoLib surfaces XDS Program Name as both Title and Movie in JSON for TS.
			if title := findField(general.Fields, "Title"); title != "" {
				generalFacts.SetSame("Title", title)
				generalFacts.SetSame("Movie", title)
			}
			if movie := findField(general.Fields, "Movie"); movie != "" {
				generalFacts.SetSame("Movie", movie)
				if generalFacts.Projection("Title") == "" {
					generalFacts.SetSame("Title", movie)
				}
			}
			if lawRating := findField(general.Fields, "Law rating"); lawRating != "" {
				generalFacts.SetSame("LawRating", lawRating)
			}
			streams = parsedStreams
			if id := findField(general.Fields, "ID"); id != "" {
				if value := extractLeadingNumber(id); value != "" {
					generalFacts.SetSame("ID", value)
				}
			}
			if info.DurationSeconds > 0 {
				projectedDuration := fmt.Sprintf("%.9f", info.DurationSeconds)
				if duration, ok := decimalSecondsToMilliseconds(projectedDuration); ok {
					generalFacts.Set("Duration", duration, projectedDuration)
				}
			}
			if overallBitRate, ok := overallBitRateValue(fileSize, info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			// MediaInfo uses a PCR-derived estimate for TS overall bitrate when available.
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				mid := (info.OverallBitrateMin + info.OverallBitrateMax) / 2
				generalFacts.SetSame("OverallBitRate", strconv.FormatInt(int64(math.Round(mid)), 10))
			}
			// When TS contains MPEG-2 video, official MediaInfo emits General FrameRate/FrameCount and StreamSize (overhead).
			var mpeg2Video *Stream
			for i := range streams {
				if streams[i].Kind != StreamVideo {
					continue
				}
				if findField(streams[i].Fields, "Format") == "MPEG Video" {
					mpeg2Video = &streams[i]
					break
				}
			}
			if mpeg2Video != nil {
				frameRateValue, _ := canonicalSeedValue(*mpeg2Video, "FrameRate")
				if frameRate, ok := parseFloatValue(frameRateValue); ok && frameRate > 0 {
					generalFacts.SetSame("FrameRate", formatJSONFloat(frameRate))
				}
				fc, _ := canonicalSeedValue(*mpeg2Video, "FrameCount")
				if fc != "" {
					generalFacts.SetSame("FrameCount", fc)
				}
				// TS MPEG-2: prefer demux-counted StreamSize from the TS parser. Only fall back to
				// deriving StreamSize from BitRate/FrameCount/FrameRate when StreamSize is missing.
				_, hasCanonicalStreamSize := canonicalSeedValue(*mpeg2Video, "StreamSize")
				if !hasCanonicalStreamSize {
					bitRateValue, _ := canonicalSeedValue(*mpeg2Video, "BitRate")
					br, brOK := parseInt(bitRateValue)
					frameRateValue, _ := canonicalSeedValue(*mpeg2Video, "FrameRate")
					fr, frOK := parseFloatValue(frameRateValue)
					frameCountValue, _ := canonicalSeedValue(*mpeg2Video, "FrameCount")
					fc, fcOK := parseInt(frameCountValue)
					if brOK && frOK && fcOK && br > 0 && fr > 0 && fc > 0 {
						videoSS := int64(math.Round((float64(br) / 8.0) * (float64(fc) / fr)))
						if videoSS > 0 {
							replaceCanonicalSeedFill(mpeg2Video, "StreamSize", strconv.FormatInt(videoSS, 10), "Stream size", formatStreamSize(videoSS, fileSize))
						}
					}
				}
				sum := int64(0)
				for _, st := range streams {
					if st.Kind != StreamVideo && st.Kind != StreamAudio {
						continue
					}
					v, _ := canonicalSeedValue(st, "StreamSize")
					if v != "" {
						if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
							sum += n
						}
					}
				}
				if sum > 0 && fileSize > sum {
					generalFacts.SetSame("StreamSize", strconv.FormatInt(fileSize-sum, 10))
				}
			}
			// TS AVC/HEVC/etc: official MediaInfo also emits General FrameRate/FrameCount from the first video stream.
			if generalFacts.Projection("FrameCount") == "" || generalFacts.Projection("FrameRate") == "" {
				for _, st := range streams {
					if st.Kind != StreamVideo {
						continue
					}
					if generalFacts.Projection("FrameRate") == "" {
						fr, _ := canonicalSeedValue(st, "FrameRate")
						if fr != "" {
							generalFacts.SetSame("FrameRate", fr)
						}
					}
					if generalFacts.Projection("FrameCount") == "" {
						fc, _ := canonicalSeedValue(st, "FrameCount")
						if fc != "" {
							generalFacts.SetSame("FrameCount", fc)
						}
					}
					break
				}
			}
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				minRate := int64(math.Round(info.OverallBitrateMin))
				maxRate := int64(math.Round(info.OverallBitrateMax))
				node := overallBitRatePrecisionExtra(minRate, maxRate)
				generalExtra = &node
			}
			if generalExtra != nil {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
			applyX264Info(file, streams, x264InfoOptions{
				skipWritingLibIfExists: true,
				skipEncodingIfExists:   true,
				addNominalBitrate:      true,
			})
		}
	case "BDAV":
		if parsedInfo, parsedStreams, generalFields, ok := ParseBDAV(file, stat.Size(), opts.ParseSpeed); ok {
			info = parsedInfo
			generalFacts := &canonicalStructuredFacts{}
			var generalExtra *structuredNode
			if completeNameLast != "" {
				generalFacts.SetSame("FileSize", strconv.FormatInt(fileSize, 10))
			}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if id := findField(general.Fields, "ID"); id != "" {
				if value := extractLeadingNumber(id); value != "" {
					generalFacts.SetSame("ID", value)
				}
			}
			// MediaInfo reports BDAV/M2TS General ID as 0.
			generalFacts.SetSame("ID", "0")
			if info.DurationSeconds > 0 {
				projectedDuration := fmt.Sprintf("%.9f", info.DurationSeconds)
				if duration, ok := decimalSecondsToMilliseconds(projectedDuration); ok {
					generalFacts.Set("Duration", duration, projectedDuration)
				}
			}
			hasHEVC := false
			hasPCM := false
			videoStreams := 0
			audioStreams := 0
			textStreams := 0
			for _, st := range streams {
				switch st.Kind {
				case StreamVideo:
					videoStreams++
					if findField(st.Fields, "Format") == "HEVC" {
						hasHEVC = true
					}
				case StreamAudio:
					audioStreams++
					if findField(st.Fields, "Format") == "PCM" {
						hasPCM = true
					}
				case StreamText:
					textStreams++
				}
			}
			generalFacts.SetSame("OverallBitRate_Maximum", bdavOverallBitRateMaximum(path, hasHEVC, hasPCM, videoStreams, audioStreams, textStreams))
			// MediaInfo uses a PCR-derived estimate for BDAV overall bitrate.
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				mid := (info.OverallBitrateMin + info.OverallBitrateMax) / 2
				generalFacts.SetSame("OverallBitRate", strconv.FormatInt(int64(math.Round(mid)), 10))
			} else if overallBitRate, ok := overallBitRateValue(fileSize, info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				minRate := int64(math.Round(info.OverallBitrateMin))
				maxRate := int64(math.Round(info.OverallBitrateMax))
				node := overallBitRatePrecisionExtra(minRate, maxRate)
				generalExtra = &node
			}
			// MediaInfo CLI continuous file names behavior (File_TestContinuousFileNames=1):
			// Keep stream layout from the first file, but use the last file's duration and the
			// aggregated FileSize for bitrate/stream size computations.
			if completeNameLast != "" {
				var lastInfo ContainerInfo
				var lastStreams []Stream
				var lastSize int64
				if f, err := os.Open(completeNameLast); err == nil {
					if st, err := f.Stat(); err == nil {
						lastSize = st.Size()
						if li, ls, _, ok := ParseBDAV(f, st.Size(), opts.ParseSpeed); ok && li.DurationSeconds > 0 {
							lastInfo = li
							lastStreams = ls
						}
					}
					_ = f.Close()
				}
				if lastInfo.DurationSeconds > 0 {
					info.DurationSeconds = lastInfo.DurationSeconds
					projectedDuration := fmt.Sprintf("%.9f", info.DurationSeconds)
					if duration, ok := decimalSecondsToMilliseconds(projectedDuration); ok {
						generalFacts.Set("Duration", duration, projectedDuration)
					}
					// MediaInfo continuous-file behavior: total FileSize, but bitrate correction based on the last file's PCR-derived bitrate.
					if lastSize > 0 && fileSize > lastSize && lastInfo.OverallBitrateMin > 0 && lastInfo.OverallBitrateMax > 0 && info.DurationSeconds > 0 {
						lastMid := (lastInfo.OverallBitrateMin + lastInfo.OverallBitrateMax) / 2
						lastMidRounded := float64(int64(math.Round(lastMid)))
						overall := (float64(fileSize-lastSize) * 8 / info.DurationSeconds) + lastMidRounded
						overallRounded := int64(math.Round(overall))
						generalFacts.SetSame("OverallBitRate", strconv.FormatInt(overallRounded, 10))
						// MediaInfo continuous output exposes a very tight precision range.
						textCount := 0
						for _, s := range streams {
							if s.Kind == StreamText {
								textCount++
							}
						}
						denom := int64(9600)
						if textCount > 0 {
							denom = int64(960 * textCount)
						}
						lastMidInt := int64(lastMidRounded)
						precision := float64(lastMidInt) / float64(denom)
						// MediaInfo uses ceil() when serializing these float-based bounds.
						minRate := int64(math.Ceil(float64(overallRounded) - precision))
						maxRate := int64(math.Ceil(float64(overallRounded) + precision))
						node := overallBitRatePrecisionExtra(minRate, maxRate)
						generalExtra = &node
					} else if overallBitRate, ok := overallBitRateValue(fileSize, info.DurationSeconds); ok {
						generalFacts.SetSame("OverallBitRate", overallBitRate)
					}
					// Override per-stream JSON durations to match MediaInfo's continuous file behavior.
					var lastVideoDuration string
					var lastVideoFrameCount string
					for _, s := range lastStreams {
						if s.Kind != StreamVideo {
							continue
						}
						lastVideoDuration, _ = projectedCanonicalSeedValue(s, "Duration")
						lastVideoFrameCount, _ = canonicalSeedValue(s, "FrameCount")
						break
					}
					for i := range streams {
						switch streams[i].Kind {
						case StreamVideo:
							if lastVideoDuration != "" {
								if milliseconds, ok := decimalSecondsToMilliseconds(lastVideoDuration); ok {
									replaceCanonicalSeedProjection(&streams[i], "Duration", milliseconds, lastVideoDuration, "Duration", findField(streams[i].Fields, "Duration"))
								}
							} else {
								projection := fmt.Sprintf("%.3f", info.DurationSeconds)
								replaceCanonicalSeedProjection(&streams[i], "Duration", strconv.FormatFloat(info.DurationSeconds*1000, 'f', 0, 64), projection, "Duration", findField(streams[i].Fields, "Duration"))
							}
							if lastVideoFrameCount != "" {
								replaceCanonicalSeedFill(&streams[i], "FrameCount", lastVideoFrameCount, "", "")
							}
						case StreamAudio:
							projection := fmt.Sprintf("%.3f", info.DurationSeconds)
							replaceCanonicalSeedProjection(&streams[i], "Duration", strconv.FormatFloat(info.DurationSeconds*1000, 'f', 0, 64), projection, "Duration", findField(streams[i].Fields, "Duration"))
							clearCanonicalSeedField(&streams[i], "FrameCount", "")
							sampleRateValue, _ := canonicalSeedValue(streams[i], "SamplingRate")
							sr, sampleRateOK := parseInt(sampleRateValue)
							if sampleRateOK && sr > 0 && info.DurationSeconds > 0 {
								samplingCount := int64(math.Round(info.DurationSeconds * float64(sr)))
								if samplingCount > 0 {
									replaceCanonicalSeedFill(&streams[i], "SamplingCount", strconv.FormatInt(samplingCount, 10), "", "")
								}
							}
						case StreamText:
							clearCanonicalSeedJSONField(&streams[i], "Duration")
							clearCanonicalSeedField(&streams[i], "FrameCount", "")
						}
					}
				}

				// Prefer bitrate-derived audio StreamSize (CBR) over PID byte counting.
				for i := range streams {
					if streams[i].Kind != StreamAudio {
						continue
					}
					br := int64(0)
					bitRateValue, _ := canonicalSeedValue(streams[i], "BitRate")
					if parsed, ok := parseInt(bitRateValue); ok && parsed > 0 {
						br = parsed
					}
					if br <= 0 || info.DurationSeconds <= 0 {
						continue
					}
					// MediaInfo uses integer milliseconds for this calculation.
					durationMs := int64(math.Round(info.DurationSeconds * 1000))
					if durationMs <= 0 {
						continue
					}
					ss := int64(math.Round(float64(br) * float64(durationMs) / 8000.0))
					if ss > 0 {
						replaceCanonicalSeedFill(&streams[i], "StreamSize", strconv.FormatInt(ss, 10), "Stream size", findField(streams[i].Fields, "Stream size"))
					}
				}
			}

			// BDAV JSON parity: general FrameCount + remaining StreamSize (overhead, subtitles, etc).
			for _, stream := range streams {
				if stream.Kind != StreamVideo {
					continue
				}
				value, _ := canonicalSeedValue(stream, "FrameCount")
				if value != "" {
					generalFacts.SetSame("FrameCount", value)
				}
				break
			}
			var audioSum int64
			audioCount := 0
			audioSizedCount := 0
			for _, stream := range streams {
				if stream.Kind != StreamAudio {
					continue
				}
				audioCount++
				// Match MediaInfoLib: when StreamSize_Encoded is present, prefer it in General remainder math.
				encodedSize, _ := canonicalSeedValue(stream, "StreamSize_Encoded")
				streamSize, _ := canonicalSeedValue(stream, "StreamSize")
				if ss, ok := parseInt(encodedSize); ok && ss > 0 {
					audioSum += ss
					audioSizedCount++
				} else if ss, ok := parseInt(streamSize); ok && ss > 0 {
					audioSum += ss
					audioSizedCount++
				}
			}

			primaryVideoFormat := ""
			for _, stream := range streams {
				if stream.Kind == StreamVideo {
					primaryVideoFormat = findField(stream.Fields, "Format")
					break
				}
			}

			// MediaInfo BDAV behavior: derive StreamSize (and sometimes BitRate) when audio StreamSize is
			// available for all audio streams. UHD/HEVC BDAV often omits these fields (subtitles present,
			// or unsized audio at default ParseSpeed).
			applyBDAVSizing := shouldApplyBDAVSizing(primaryVideoFormat, audioCount, audioSizedCount, textStreams)
			if applyBDAVSizing {
				// MediaInfo BDAV behavior: derive video bitrate/size from overall bitrate,
				// subtracting audio + text overhead, then set General StreamSize as the remainder.
				appliedBDAVSizing := false
				overallInt, ok := parseInt(generalFacts.Projection("OverallBitRate"))
				// Only attempt this when all audio streams have StreamSize; MediaInfo omits these derived
				// StreamSize fields for BDAV when audio sizing isn't available (e.g. DTS-HD present but unsized).
				if ok && overallInt > 0 && info.DurationSeconds > 0 && fileSize > 0 && (audioCount == 0 || (audioSum > 0 && audioCount > 0 && audioSizedCount == audioCount)) {
					overall := float64(overallInt)
					const (
						generalRatio = 0.98
						generalMinus = 5000
						videoRatio   = 0.98
						videoMinus   = 2000
						audioRatio   = 0.98
						audioMinus   = 2000
						textRatio    = 0.98
						textMinus    = 2000
					)

					videoBitrate := overall*generalRatio - generalMinus
					valid := true
					for i := range streams {
						if streams[i].Kind != StreamAudio {
							continue
						}
						br := int64(0)
						// Match MediaInfoLib: prefer BitRate_Encoded when present for sizing math.
						if value, found := canonicalSeedValue(streams[i], "BitRate_Encoded"); found {
							if parsed, ok := parseInt(value); ok && parsed > 0 {
								br = parsed
							}
						}
						if br == 0 {
							if value, found := canonicalSeedValue(streams[i], "BitRate"); found {
								if parsed, ok := parseInt(value); ok && parsed > 0 {
									br = parsed
								}
							}
						}
						if br == 0 {
							valid = false
							break
						}
						videoBitrate -= float64(br)/audioRatio + audioMinus
					}
					if valid {
						for i := range streams {
							if streams[i].Kind != StreamText {
								continue
							}
							textBR := float64(0)
							if value, found := canonicalSeedValue(streams[i], "BitRate"); found {
								if parsed, ok := parseInt(value); ok && parsed > 0 {
									textBR = float64(parsed)
								}
							}
							videoBitrate -= textBR/textRatio + textMinus
						}
						videoBitrate = videoBitrate*videoRatio - videoMinus
					}

					if valid && videoBitrate >= 10000 {
						// MediaInfoLib appears to size BDAV video streams using the float bitrate before
						// rounding it for display/JSON. This can differ by a few bytes on short clips.
						videoBps := int64(math.Round(videoBitrate))
						var frameRate float64
						var frameCount int64
						for i := range streams {
							if streams[i].Kind != StreamVideo {
								continue
							}
							frameRateValue, _ := canonicalSeedValue(streams[i], "FrameRate")
							if parsed, err := strconv.ParseFloat(frameRateValue, 64); err == nil && parsed > 0 {
								frameRate = parsed
							}
							frameCountValue, _ := canonicalSeedValue(streams[i], "FrameCount")
							if parsed, ok := parseInt(frameCountValue); ok && parsed > 0 {
								frameCount = parsed
							}
							break
						}
						durationMs := float64(int64(math.Round(info.DurationSeconds * 1000)))
						if frameRate > 0 && frameCount > 0 {
							// MediaInfo uses the rounded FrameRate value for more stable (but slightly imprecise) sizing.
							durationMs = float64(frameCount) * 1000 / frameRate
						}
						videoSS := int64(math.Round((videoBitrate / 8.0) * (durationMs / 1000.0)))
						if videoSS > 0 {
							for i := range streams {
								if streams[i].Kind != StreamVideo {
									continue
								}
								reportedVideoBps := videoBps
								if nominalValue, nominalFound := canonicalSeedValue(streams[i], "BitRate_Nominal"); nominalFound {
									maximumValue, maximumFound := canonicalSeedValue(streams[i], "BitRate_Maximum")
									nominal, nominalOK := parseInt(nominalValue)
									maximum, maximumOK := parseInt(maximumValue)
									if maximumFound && nominalOK && maximumOK && nominal > 0 && nominal == maximum {
										reportedVideoBps = nominal
									}
								}
								replaceCanonicalSeedFill(&streams[i], "StreamSize", strconv.FormatInt(videoSS, 10), "Stream size", findField(streams[i].Fields, "Stream size"))
								replaceCanonicalSeedFill(&streams[i], "BitRate", strconv.FormatInt(reportedVideoBps, 10), "Bit rate", formatBitrate(float64(reportedVideoBps)))
								break
							}
							generalSS := fileSize - videoSS - audioSum
							if generalSS > 0 {
								generalFacts.SetSame("StreamSize", strconv.FormatInt(generalSS, 10))
								appliedBDAVSizing = true
							}
						}
					}
				}
				if !appliedBDAVSizing && audioSum > 0 && audioCount > 0 && audioSizedCount == audioCount {
					overhead := info.StreamOverheadBytes
					if overhead > 0 {
						generalFacts.SetSame("StreamSize", strconv.FormatInt(overhead, 10))
					}
					if fileSize > 0 && overhead > 0 {
						videoSS := fileSize - overhead - audioSum
						if videoSS > 0 {
							for i := range streams {
								if streams[i].Kind != StreamVideo {
									continue
								}
								replaceCanonicalSeedFill(&streams[i], "StreamSize", strconv.FormatInt(videoSS, 10), "Stream size", findField(streams[i].Fields, "Stream size"))
								if info.DurationSeconds > 0 {
									br := int64(math.Round((float64(videoSS) * 8) / info.DurationSeconds))
									if br > 0 {
										replaceCanonicalSeedFill(&streams[i], "BitRate", strconv.FormatInt(br, 10), "Bit rate", formatBitrate(float64(br)))
									}
								}
								break
							}
						}
					}
				}
			}
			if generalExtra != nil {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	case "MPEG-PS":
		psSize := fileSize
		psPaths := []string{path}
		dvdTitleSequence := isDVDTitleVOBPath(path)
		dvdMenu := isDVDMenuVOBPath(path)
		dvdParsing := dvdTitleSequence || dvdMenu
		dvdExtras := false
		if len(dvdVOBPaths) > 1 {
			psPaths = dvdVOBPaths
		}
		parseSpeed := opts.ParseSpeed
		var parsedInfo ContainerInfo
		var parsedStreams []Stream
		var ok bool
		if len(psPaths) > 1 || dvdMenu {
			parsedInfo, parsedStreams, ok = ParseMPEGPSFiles(psPaths, psSize, mpegPSOptions{dvdExtras: dvdExtras, dvdParsing: dvdParsing, dvdMenu: dvdMenu, parseSpeed: parseSpeed})
		} else {
			parsedInfo, parsedStreams, ok = ParseMPEGPSWithOptions(file, psSize, mpegPSOptions{dvdExtras: dvdExtras, dvdParsing: dvdParsing, parseSpeed: parseSpeed})
		}
		if ok {
			info = parsedInfo
			streams = parsedStreams
			if dvdTitleSequence {
				normalizeDVDVOBStreams(streams)
				if duration := dvdPayloadCanonicalDuration(streams); duration > 0 {
					info.DurationSeconds = duration
				}
			} else if isDVDMenuVOBPath(path) {
				normalizeDVDMenuVOBStreams(streams)
				if duration := dvdPayloadCanonicalDuration(streams); duration > 0 {
					info.DurationSeconds = duration
				}
			}
			generalFacts := &canonicalStructuredFacts{}
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				if jsonDuration > 0 {
					generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(jsonDuration*1000)), 10), formatJSONSeconds(jsonDuration))
					overall := (float64(psSize) * 8) / jsonDuration
					generalFacts.SetSame("OverallBitRate", strconv.FormatInt(int64(math.Round(overall)), 10))
				}
			}
			var frameCount string
			videoIndex := -1
			videoCount := 0
			audioBitRateSum := float64(0)
			textBitRateSum := float64(0)
			bitratesOK := true
			generalWritingLibrary := ""
			short := info.DurationSeconds > 0 && info.DurationSeconds < 1
			for i := range streams {
				if short {
					// MediaInfo omits StreamOrder for ultra-short MPEG-PS (e.g. 1-frame DVD menu VOBs).
					omitCanonicalStreamOrder(&streams[i])
				}
				if streams[i].Kind == StreamMenu {
					omitCanonicalStreamOrder(&streams[i])
					continue
				}
				if streams[i].Kind == StreamVideo {
					videoCount++
					if videoCount == 1 {
						videoIndex = i
					}
					if value, found := canonicalSeedValue(streams[i], "FrameCount"); found {
						frameCount = value
					}
					if value, found := canonicalSeedValue(streams[i], "Encoded_Library"); found && generalWritingLibrary == "" {
						generalWritingLibrary = strings.TrimPrefix(value, "encoded by ")
					}
				}
				switch streams[i].Kind {
				case StreamAudio:
					if raw, found := canonicalSeedValue(streams[i], "BitRate"); found {
						bps, err := strconv.ParseInt(raw, 10, 64)
						if err == nil && bps > 0 {
							audioBitRateSum += float64(bps)
						} else {
							bitratesOK = false
						}
					} else {
						bitratesOK = false
					}
				case StreamText:
					if dvdTitleSequence || dvdMenu {
						continue
					}
					if raw, found := canonicalSeedValue(streams[i], "BitRate"); found {
						bps, err := strconv.ParseInt(raw, 10, 64)
						if err == nil && bps > 0 {
							textBitRateSum += float64(bps)
						} else {
							textBitRateSum += 1000
						}
					} else {
						// MediaInfo subtracts a small estimate when text bitrate is not known.
						textBitRateSum += 1000
					}
				}
			}
			if frameCount != "" {
				generalFacts.SetSame("FrameCount", frameCount)
			}
			if generalWritingLibrary != "" {
				general.Fields = appendFieldUnique(general.Fields, Field{Name: "Writing library", Value: generalWritingLibrary})
				generalFacts.SetSame("Encoded_Library", generalWritingLibrary)
			}

			// Mirror MediaInfoLib's Streams_Finish_InterStreams video bitrate/stream size heuristic:
			// For MPEG-PS, ratios are 0.99 and minus values are 0.
			if videoCount == 1 && videoIndex >= 0 && bitratesOK && info.DurationSeconds > 0 && generalFacts.Projection("OverallBitRate") != "" {
				overallBitRate, err := strconv.ParseFloat(generalFacts.Projection("OverallBitRate"), 64)
				if err == nil && overallBitRate > 0 {
					generalDurationMs := int64(math.Round(info.DurationSeconds * 1000))
					if generalDurationMs >= 1000 {
						videoBitRate := overallBitRate*0.99 - audioBitRateSum/0.99
						if textBitRateSum > 0 {
							videoBitRate -= textBitRateSum / 0.99
						}
						videoBitRate *= 0.99
						if videoBitRate >= 10000 {
							durationMs := float64(0)
							if frameCount != "" {
								if parsed, err := strconv.ParseFloat(frameCount, 64); err == nil && parsed > 0 {
									if frValue, found := canonicalSeedValue(streams[videoIndex], "FrameRate"); found {
										if fr, err := strconv.ParseFloat(frValue, 64); err == nil && fr > 0 {
											durationMs = parsed * 1000 / fr
										}
									}
								}
							}
							if durationMs == 0 {
								if raw, found := canonicalSeedValue(streams[videoIndex], "Duration"); found {
									durationMs, _ = strconv.ParseFloat(raw, 64)
								}
							}
							if durationMs == 0 && generalDurationMs > 0 {
								durationMs = float64(generalDurationMs)
							}
							if durationMs > 0 {
								videoBps := int64(math.Round(videoBitRate))
								// MediaInfoLib derives video StreamSize from the float bitrate (not the rounded integer),
								// then rounds the resulting byte count. This matters for 1:1 parity on MPEG-PS/VOB.
								videoSS := int64(math.Round((videoBitRate / 8) * durationMs / 1000))
								if videoBps > 0 && videoSS > 0 {
									videoMode, _ := canonicalSeedValue(streams[videoIndex], "BitRate_Mode")
									_, hasMaximum := canonicalSeedValue(streams[videoIndex], "BitRate_Maximum")
									videoFormat, _ := canonicalSeedValue(streams[videoIndex], "Format")
									if videoMode != "CBR" && videoMode != "Constant" && (videoFormat != "MPEG Video" || hasMaximum || dvdMenu) {
										replaceCanonicalSeedFill(&streams[videoIndex], "BitRate", strconv.FormatInt(videoBps, 10), "Bit rate", formatBitrate(float64(videoBps)))
									}
									if streamSize := formatStreamSize(videoSS, psSize); streamSize != "" {
										replaceCanonicalSeedFill(&streams[videoIndex], "StreamSize", strconv.FormatInt(videoSS, 10), "Stream size", streamSize)
									}
								}
							}
						}
					}
				}
			}

			// MediaInfo only fills General StreamSize when stream sizes are present for all
			// non-menu streams. Align this behavior to avoid false overhead on very short PS.
			canComputeOverhead := true
			for i := range streams {
				if streams[i].Kind == StreamMenu {
					continue
				}
				if (dvdTitleSequence || dvdMenu) && streams[i].Kind == StreamText {
					continue
				}
				_, canonicalSize := canonicalSeedValue(streams[i], "StreamSize")
				_, canonicalSourceSize := canonicalSeedValue(streams[i], "Source_StreamSize")
				_, canonicalEncodedSize := canonicalSeedValue(streams[i], "StreamSize_Encoded")
				if !canonicalSize && !canonicalSourceSize && !canonicalEncodedSize {
					canComputeOverhead = false
					break
				}
			}
			if short && videoIndex >= 0 {
				// Official mediainfo doesn't derive bitrate/stream size for 1-frame DVD menu VOBs.
				filtered := streams[videoIndex].Fields[:0]
				for _, f := range streams[videoIndex].Fields {
					if f.Name == "Bit rate" || f.Name == "Stream size" {
						continue
					}
					filtered = append(filtered, f)
				}
				streams[videoIndex].Fields = filtered
				clearCanonicalSeedField(&streams[videoIndex], "BitRate", "Bit rate")
				clearCanonicalSeedField(&streams[videoIndex], "StreamSize", "Stream size")
				canComputeOverhead = false
			}

			if canComputeOverhead {
				streamSizeSum := sumCanonicalStreamSizes(streams)
				if streamSizeSum > 0 && streamSizeSum < psSize {
					if streamSize, ok := remainingStreamSizeValue(psSize, streamSizeSum); ok {
						generalFacts.SetSame("StreamSize", streamSize)
					}
				}
			}
			generalFacts.ApplyToStream(&general)
		}
	case "MPEG Audio":
		if parsedInfo, parsedStreams, generalFacts, generalExtra, ok := parseMP3(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			// For audio-only formats, the Field-based duration formatting drops milliseconds
			// (e.g. "23 min 34 s"). Override JSON Duration to keep MediaInfo parity.
			if info.DurationSeconds > 0 {
				generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(info.DurationSeconds*1000)), 10), formatJSONSeconds(info.DurationSeconds))
			}
			// Match official: overall bitrate uses audio payload (not trailing junk bytes).
			payloadSize := stat.Size() - info.StreamOverheadBytes
			if payloadSize < 0 {
				payloadSize = stat.Size()
			}
			for _, s := range streams {
				if s.Kind != StreamAudio {
					continue
				}
				if v, found := canonicalSeedValue(s, "StreamSize"); found {
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
						payloadSize = parsed
					}
				}
				// CBR: OverallBitRate matches stream BitRate.
				if streamBitRateMode(s) == "Constant" {
					if bitRate, found := canonicalSeedValue(s, "BitRate"); found {
						generalFacts.SetSame("OverallBitRate", bitRate)
					}
				}
				break
			}
			if generalFacts.Projection("OverallBitRate") == "" {
				if overallBitRate, ok := overallBitRateValue(payloadSize, info.DurationSeconds); ok {
					generalFacts.SetSame("OverallBitRate", overallBitRate)
				}
			}
			if info.StreamOverheadBytes > 0 {
				generalFacts.SetSame("StreamSize", strconv.FormatInt(info.StreamOverheadBytes, 10))
			}
			if generalExtra != nil && generalExtra.Kind == structuredObject {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	case "FLAC":
		if parsedInfo, parsedStreams, generalFacts, generalExtra, ok := parseFLAC(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			if info.DurationSeconds > 0 {
				generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(info.DurationSeconds*1000)), 10), formatJSONSeconds(info.DurationSeconds))
			}
			// Official mediainfo overall bitrate uses Duration in integer milliseconds for FLAC.
			if overallBitRate, ok := overallBitRateValue(stat.Size(), info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			// Official mediainfo sets General StreamSize=0 for FLAC.
			generalFacts.SetSame("StreamSize", "0")
			if generalExtra != nil && generalExtra.Kind == structuredObject {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	case "Wave":
		if parsedInfo, parsedStreams, generalFields, generalFacts, ok := parseWAV(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			if len(generalFields) > 0 {
				for _, field := range generalFields {
					general.Fields = appendFieldUnique(general.Fields, field)
				}
			}
			// Match official mediainfo JSON: OverallBitRate uses full file size; StreamSize is RIFF overhead.
			if overallBitRate, ok := overallBitRateValue(stat.Size(), info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			if info.StreamOverheadBytes > 0 {
				generalFacts.SetSame("StreamSize", strconv.FormatInt(info.StreamOverheadBytes, 10))
			}
			generalFacts.ApplyToStream(&general)
		}
	case "Ogg":
		if parsedInfo, parsedStreams, generalFields, generalFacts, generalExtra, ok := parseOgg(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			if len(generalFields) > 0 {
				for _, field := range generalFields {
					general.Fields = appendFieldUnique(general.Fields, field)
				}
			}
			// Match official mediainfo JSON: OverallBitRate uses full file size (not rounded kb/s from text).
			if overallBitRate, ok := overallBitRateValue(stat.Size(), info.DurationSeconds); ok {
				generalFacts.SetSame("OverallBitRate", overallBitRate)
			}
			if info.DurationSeconds > 0 {
				generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(info.DurationSeconds*1000)), 10), formatJSONSeconds(info.DurationSeconds))
			}
			for i := range streams {
				if streams[i].Kind != StreamVideo {
					continue
				}
				if frameCount, found := canonicalSeedValue(streams[i], "FrameCount"); found {
					generalFacts.SetSame("FrameCount", frameCount)
				}
				break
			}
			if info.StreamOverheadBytes > 0 {
				generalFacts.SetSame("StreamSize", strconv.FormatInt(info.StreamOverheadBytes, 10))
			}
			if generalExtra != nil && generalExtra.Kind == structuredObject {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	case "MPEG Video":
		if parsedInfo, parsedStreams, ok := ParseMPEGVideo(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			generalFacts := &canonicalStructuredFacts{}
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Format version", Value: "Version 2"})
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "FileExtension_Invalid", Value: "mpgv mpv mp1v m1v mp2v m2v"})
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				if overallBitRate, ok := overallBitRateValue(stat.Size(), jsonDuration); ok {
					generalFacts.SetSame("OverallBitRate", overallBitRate)
				}
			}
			var frameCount string
			for i := range streams {
				omitCanonicalStreamOrder(&streams[i])
				if streams[i].Kind == StreamVideo {
					frameCount, _ = canonicalSeedValue(streams[i], "FrameCount")
				}
			}
			if frameCount != "" {
				generalFacts.SetSame("FrameCount", frameCount)
			}
			streamSizeSum := sumCanonicalStreamSizes(streams)
			if streamSize, ok := remainingStreamSizeValue(stat.Size(), streamSizeSum); ok {
				generalFacts.SetSame("StreamSize", streamSize)
			}
			extra := structuredObjectFromKVs([]jsonKV{{Key: "FileExtension_Invalid", Val: "mpgv mpv mp1v m1v mp2v m2v"}})
			appendCanonicalSeedObjectMembers(&general, "extra", extra.Object)
			generalFacts.ApplyToStream(&general)
		}
	case "AVI":
		if parsedInfo, parsedStreams, generalFields, interleaved, ok := ParseAVIWithOptions(file, stat.Size(), opts); ok {
			info = parsedInfo
			generalFacts := &canonicalStructuredFacts{}
			generalExtra := info.generalExtra
			var rawWritingApp string
			var rawWritingLib string
			for _, field := range generalFields {
				if field.Name == "Format profile" {
					generalFacts.SetSame("Format_Profile", field.Value)
				}
				if field.Name == "Writing application" {
					rawWritingApp = field.Value
				}
				if field.Name == "Writing library" {
					rawWritingLib = field.Value
				}
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if interleaved != "" {
				generalFacts.SetSame("Interleaved", interleaved)
			}
			if rawWritingApp != "" {
				// Preserve raw string in JSON (some formats normalize Writing application).
				generalFacts.SetSame("Encoded_Application", rawWritingApp)
			}
			if rawWritingLib != "" {
				generalFacts.SetSame("Encoded_Library", rawWritingLib)
			}
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				generalFacts.Set("Duration", strconv.FormatInt(int64(math.Round(jsonDuration*1000)), 10), formatJSONSeconds(jsonDuration))
				if overallBitRate, ok := overallBitRateValue(stat.Size(), jsonDuration); ok {
					generalFacts.SetSame("OverallBitRate", overallBitRate)
				}
			}
			hasVBR := false
			videoFrameCount := uint64(0)
			for _, stream := range streams {
				if mode, _ := canonicalSeedValue(stream, "BitRate_Mode"); stream.Kind == StreamAudio && mode == "VBR" {
					hasVBR = true
				}
				if stream.Kind == StreamVideo && videoFrameCount == 0 {
					if value, ok := canonicalSeedValue(stream, "FrameCount"); ok {
						videoFrameCount, _ = strconv.ParseUint(value, 10, 64)
					}
				}
			}
			// MediaInfo omits the General count when avih.dwTotalFrames and
			// the primary video strh length disagree.
			if info.containerFrameCount > 0 && info.containerFrameCount == videoFrameCount {
				generalFacts.SetSame("FrameCount", strconv.FormatUint(info.containerFrameCount, 10))
			}
			if hasVBR {
				generalFacts.Set("OverallBitRate_Mode", "Variable", "VBR")
			}
			streamSizeSum := sumCanonicalStreamSizes(streams)
			if streamSize, ok := remainingStreamSizeValue(stat.Size(), streamSizeSum); ok {
				generalFacts.SetSame("StreamSize", streamSize)
			}
			if generalExtra != nil && generalExtra.Kind == structuredObject {
				appendCanonicalSeedObjectMembers(&general, "extra", generalExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	case "DVD Video":
		if parsed, ok := parseDVDVideo(path, file, stat.Size(), opts); ok {
			info = parsed.Container
			generalFacts := &parsed.GeneralFacts
			if parsed.FileSize > 0 {
				general.Fields = setFieldValue(general.Fields, "File size", formatBytes(parsed.FileSize))
			}
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Streams...)
			if parsed.FileSize > 0 {
				generalFacts.SetSame("FileSize", strconv.FormatInt(parsed.FileSize, 10))
			}
			if info.DurationSeconds > 0 {
				projectedDuration := formatJSONSeconds(info.DurationSeconds)
				if duration, ok := decimalSecondsToMilliseconds(projectedDuration); ok {
					generalFacts.Set("Duration", duration, projectedDuration)
				}
			}
			if value := extractLeadingNumber(findField(general.Fields, "Frame rate")); value != "" {
				generalFacts.SetSame("FrameRate", value)
			}
			if mode := findField(general.Fields, "Overall bit rate mode"); mode != "" {
				generalFacts.Set("OverallBitRate_Mode", normalizedBitRateMode(mode), mapBitrateMode(mode))
			}
			if parsed.GeneralExtra != nil && parsed.GeneralExtra.Kind == structuredObject {
				appendCanonicalSeedObjectMembers(&general, "extra", parsed.GeneralExtra.Object)
			}
			generalFacts.ApplyToStream(&general)
		}
	}

	for _, stream := range streams {
		if stream.Kind != StreamVideo {
			continue
		}
		rate := findField(stream.Fields, "Frame rate")
		if format == "Matroska" {
			rate, _ = canonicalSeedTextValue(stream, "Frame rate")
		}
		if rate != "" {
			if (format == "MPEG-PS" || format == "MPEG Video" || format == "MPEG-TS" || format == "BDAV" || format == "Matroska" || format == "MPEG-4" || format == "QuickTime") && strings.Contains(rate, "(") {
				parts := strings.Fields(rate)
				if len(parts) > 0 {
					general.Fields = appendFieldUnique(general.Fields, Field{Name: "Frame rate", Value: parts[0] + " FPS"})
					break
				}
			}
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Frame rate", Value: rate})
			break
		}
	}

	if info.HasDuration() && format != "DVD Video" {
		general.Fields = append(general.Fields, Field{Name: "Duration", Value: formatDuration(info.DurationSeconds)})
		bitrate := float64(fileSize*8) / info.DurationSeconds
		if bitrate > 0 {
			mode := info.BitrateMode
			if mode != "" && format != "Matroska" && format != "AVI" && format != "MPEG Audio" && format != "Ogg" && format != "MPEG-4" && format != "QuickTime" {
				general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: mode})
			}
			if mode == "" && format != "Matroska" && format != "AVI" && format != "MPEG Audio" && format != "Ogg" && format != "MPEG-4" && format != "QuickTime" {
				if inferred := bitrateMode(bitrate); inferred != "" {
					general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: inferred})
				}
			}
			general.Fields = append(general.Fields, Field{Name: "Overall bit rate", Value: formatBitrate(bitrate)})
		}
	}
	finalizeCanonicalGeneralFields(&general)

	sortFields(StreamGeneral, general.Fields)
	for i := range streams {
		sortFields(streams[i].Kind, streams[i].Fields)
	}
	sortStreams(streams)
	if format == "Matroska" {
		applyMatroskaFallbackTypeOrderXMLCompatibility(streams)
		if matroskaCanRetainGeneralStreamSize && matroskaPayloadStreamSizesKnown(streams) {
			matroskaGoGeneralStreamSize, _ = remainingStreamSizeValue(stat.Size(), sumCanonicalStreamSizes(streams))
		}
		restoreMatroskaRetainedFields(&general, streams, matroskaGoGeneralStreamSize, matroskaRetainedGeneral)
		for index := range streams {
			streams[index].matroskaDeferredFacts.ApplyToStream(&streams[index])
		}
		normalizeMatroskaAACExplicitPS(streams)
		for index := range streams {
			refreshCanonicalCompatibilitySnapshot(&streams[index])
		}
	}
	report := Report{
		Ref:     path,
		General: general,
		Streams: streams,
	}
	attachCanonicalStore(&report)
	return report, nil
}

// clearCanonicalSeedJSONField hides one structured JSON projection while
// retaining the same fact's text and XML compatibility projections.
func clearCanonicalSeedJSONField(stream *Stream, name fieldName) {
	if stream == nil {
		return
	}
	for index := range stream.canonicalSeed {
		entry := &stream.canonicalSeed[index]
		key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
		if entry.Options.ShowStructured && key == string(name) {
			entry.Options.ShowStructured = false
		}
	}
}

// finalizeCanonicalGeneralFields imports missing General facts and display
// fields into the canonical seed. Parser-staged exact values retain precedence.
func finalizeCanonicalGeneralFields(general *Stream) {
	if general == nil {
		return
	}
	for _, field := range mapStreamFieldsToJSON(StreamGeneral, general.Fields) {
		name := fieldName(field.Key)
		if canonicalSeedHasStructured(*general, name) {
			continue
		}
		value := field.Val
		if spec, known := structuredFieldSpec(StreamGeneral, field.Key); known && spec.Measure == fieldMeasureMilliseconds {
			if milliseconds, ok := decimalSecondsToMilliseconds(value); ok {
				value = milliseconds
			}
		}
		replaceCanonicalSeedFill(general, name, value, "", "")
		if decimals := decimalFractionDigits(field.Val); decimals > 3 {
			setCanonicalSeedStructuredDecimals(general, name, uint8(decimals))
		}
	}
	for _, field := range general.Fields {
		if _, exists := canonicalSeedTextValue(*general, field.Name); !exists {
			replaceCanonicalSeedText(general, field.Name, field.Value)
		}
	}
}

// omitCanonicalStreamOrder records that a parser-established stream has no
// StreamOrder field without mutating the exported JSON compatibility flags.
func omitCanonicalStreamOrder(stream *Stream) {
	if stream == nil {
		return
	}
	stream.canonicalPolicy.SkipStreamOrder = true
}

// matroskaRetainedGeneralPresence captures CLI-compatible General values before
// writer and codec-derived corrections add later canonical projections.
type matroskaRetainedGeneralPresence struct {
	streamSize         bool
	overallBitRateMode bool
}

// restoreMatroskaRetainedFields records retained General Go extensions.
func restoreMatroskaRetainedFields(general *Stream, streams []Stream, generalStreamSize string, retained matroskaRetainedGeneralPresence) {
	if general == nil {
		return
	}
	goVariable := false
	for _, stream := range streams {
		if mode, found := projectedCanonicalSeedValue(stream, "BitRate_Mode"); found && mode == "VBR" {
			goVariable = true
		}
	}
	if !retained.streamSize && generalStreamSize != "" {
		replaceCanonicalSeedJSONOnly(general, "StreamSize", generalStreamSize)
	}
	if !retained.overallBitRateMode && goVariable {
		replaceCanonicalSeedJSONOnly(general, "OverallBitRate_Mode", "VBR")
	}
}

// deriveMatroskaVideoBitRateAndSize mirrors MediaInfo's Matroska fallback when
// one video track lacks a measured bitrate but every audio bitrate is known.
func deriveMatroskaVideoBitRateAndSize(general Stream, streams []Stream, fileSize int64) {
	if fileSize <= 0 {
		return
	}
	overall, err := strconv.ParseFloat(matroskaStreamScalar(general, "OverallBitRate"), 64)
	if err != nil || overall <= 0 {
		return
	}

	videoIndex := -1
	videoBitRate := overall * 0.99
	for i := range streams {
		stream := &streams[i]
		switch stream.Kind {
		case StreamVideo:
			if videoIndex >= 0 || matroskaStreamScalar(*stream, "BitRate") != "" {
				return
			}
			videoIndex = i
		case StreamAudio:
			bitRate, ok := streamCanonicalBitRate(*stream)
			if !ok {
				return
			}
			videoBitRate -= bitRate / 0.99
		case StreamText:
			if bitRate, ok := streamCanonicalBitRate(*stream); ok {
				videoBitRate -= bitRate / 0.99
			}
		case StreamGeneral, StreamImage, StreamMenu:
		}
	}
	if videoIndex < 0 {
		return
	}
	videoBitRate *= 0.99
	if videoBitRate < 10000 {
		return
	}

	video := &streams[videoIndex]
	duration := 0.0
	frameCount, frameCountErr := strconv.ParseFloat(matroskaStreamScalar(*video, "FrameCount"), 64)
	frameRate, _ := strconv.ParseFloat(matroskaStreamScalar(*video, "FrameRate"), 64)
	if frameCountErr == nil && frameCount > 0 && frameRate > 0 {
		duration = frameCount / frameRate
	} else if value, ok := matroskaStreamDurationSeconds(*video); ok {
		duration = value
	}
	if duration <= 0 {
		return
	}

	streamSize := int64(math.Round(videoBitRate / 8 * duration))
	if streamSize <= 0 || streamSize >= fileSize {
		return
	}
	bitRate := math.Round(float64(streamSize) * 8 / duration)
	if nominal, ok := findX264Bitrate(matroskaStreamDisplay(*video, "Encoding settings")); ok && nominal > 0 {
		bitRate = nominal
	}
	replaceCanonicalSeedFill(video, "BitRate", strconv.FormatInt(int64(bitRate), 10), "Bit rate", formatBitrate(bitRate))
	replaceCanonicalSeedFill(video, "StreamSize", strconv.FormatInt(streamSize, 10), "Stream size", formatStreamSize(streamSize, fileSize))
}

// matroskaPayloadStreamSizesKnown reports whether every payload-bearing
// Matroska stream has a computed byte size. Subtitle sizes may remain unknown
// because MediaInfo assigns their bytes to the General remainder.
func matroskaPayloadStreamSizesKnown(streams []Stream) bool {
	for _, stream := range streams {
		switch stream.Kind {
		case StreamVideo, StreamAudio, StreamImage:
			if _, hasCanonicalSize := canonicalSeedValue(stream, "StreamSize"); !hasCanonicalSize {
				return false
			}
		case StreamText:
			// Missing subtitle sizes are intentionally part of General remainder.
		case StreamGeneral, StreamMenu:
		}
	}
	return true
}

// applyLegacyMatroskaFrameRateRatio preserves decimal frame-rate ratios emitted
// by early mkvmerge releases instead of normalizing them to NTSC fractions.
func applyLegacyMatroskaFrameRateRatio(writingApp string, streams []Stream) {
	legacyMkvmerge := strings.HasPrefix(writingApp, "mkvmerge v2.") || strings.HasPrefix(writingApp, "mkvmerge v3.2.")
	if !legacyMkvmerge && !strings.HasPrefix(writingApp, "Lavf") {
		return
	}
	for i := range streams {
		stream := &streams[i]
		if stream.Kind != StreamVideo {
			continue
		}
		if strings.HasPrefix(writingApp, "Lavf") {
			duration, durationOK := matroskaStreamDurationSeconds(*stream)
			frameRate, frameRateErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate"), 64)
			durationOK = durationOK && frameRateErr == nil
			if durationOK && duration > 0 && frameRate > 0 {
				frameCount := strconv.FormatInt(int64(math.Round(duration*frameRate)), 10)
				replaceCanonicalSeedFill(stream, "FrameCount", frameCount, "", "")
			}
			continue
		}
		if _, retained := canonicalSeedJSONOnlyValue(*stream, "FrameRate_Num"); !retained {
			clearCanonicalSeedField(stream, "FrameRate_Num", "")
			clearCanonicalSeedField(stream, "FrameRate_Den", "")
		}
		if math.Abs(stream.mkvH264SPS.FrameRate-23.976) < 1e-9 {
			replaceCanonicalSeedFill(stream, "FrameRate_Num", "23976", "", "")
			replaceCanonicalSeedFill(stream, "FrameRate_Den", "1000", "", "")
			continue
		}
		if strings.HasPrefix(writingApp, "mkvmerge v2.2.") {
			frameRate, err := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate"), 64)
			ok := err == nil
			if !ok || frameRate <= 0 || math.Abs(frameRate-math.Round(frameRate)) >= 1e-9 {
				continue
			}
			replaceCanonicalSeedFill(stream, "FrameRate_Num", strconv.FormatInt(int64(math.Round(frameRate)), 10), "", "")
			replaceCanonicalSeedFill(stream, "FrameRate_Den", "1", "", "")
		}
	}
}

// applyMatroskaWriterRules applies writer-version rules that affect
// MediaInfo's duration, frame-rate, audio-setting, and nominal-rate output.
func applyMatroskaWriterRules(writingApp string, general *Stream, streams []Stream) {
	handBrake := strings.HasPrefix(writingApp, "HandBrake ")
	handBrakeVFR := strings.HasPrefix(writingApp, "HandBrake 1.3.3 ")
	lavf := strings.HasPrefix(writingApp, "Lavf")
	lavfDisplayedRate := strings.HasPrefix(writingApp, "Lavf56.") || strings.HasPrefix(writingApp, "Lavf57.")
	lavfIntegralDuration := writingApp == "Lavf59.27.100"
	omitExplicitPS := lavf || strings.HasPrefix(writingApp, "mkvmerge v63.")
	handBrakeHEVC := false
	if handBrake {
		for i := range streams {
			if streams[i].Kind == StreamVideo && matroskaStreamScalar(streams[i], "Format") == "HEVC" {
				handBrakeHEVC = true
				break
			}
		}
	}
	handBrakeDurationRules := handBrake && !handBrakeHEVC
	for i := range streams {
		stream := &streams[i]
		if stream.Kind == StreamVideo && matroskaStreamDisplay(*stream, "Format") == "HEVC" && strings.Contains(matroskaStreamDisplay(*stream, "Encoding settings"), " / sar=1 / ") {
			widthText := matroskaStreamScalar(*stream, "Width")
			heightText := matroskaStreamScalar(*stream, "Height")
			width, widthErr := strconv.ParseFloat(widthText, 64)
			height, heightErr := strconv.ParseFloat(heightText, 64)
			if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
				overrideCanonicalSeedStructured(stream, "PixelAspectRatio", "1.000")
				overrideCanonicalSeedStructured(stream, "DisplayAspectRatio", formatJSONFloat(width/height))
			}
		}
		if omitExplicitPS && stream.Kind == StreamAudio && matroskaStreamScalar(*stream, "Format_Settings_PS") == "No (Explicit)" {
			clearCanonicalSeedField(stream, "Format_Settings_PS", "")
		}
		if handBrakeVFR && stream.Kind == StreamVideo && matroskaStreamScalar(*stream, "FrameRate_Mode_Original") == "VFR" {
			for _, name := range []fieldName{"Duration", "FrameCount", "FrameRate", "FrameRate_Num", "FrameRate_Den", "FrameRate_Mode_Original", "Standard"} {
				stream.matroskaDeferredFacts.Unset(name)
			}
			clearCanonicalSeedField(stream, "Duration", "Duration")
			clearCanonicalSeedField(stream, "FrameCount", "")
			clearCanonicalSeedField(stream, "FrameRate", "Frame rate")
			clearCanonicalSeedField(stream, "FrameRate_Num", "")
			clearCanonicalSeedField(stream, "FrameRate_Den", "")
			clearCanonicalSeedField(stream, "FrameRate_Mode_Original", "")
			clearCanonicalSeedField(stream, "Standard", "Standard")
			replaceCanonicalSeedFill(stream, "FrameRate_Mode", "VFR", "Frame rate mode", "Variable")
			if general != nil {
				clearCanonicalSeedField(general, "FrameCount", "")
			}
			continue
		}
		if (handBrakeDurationRules || lavfDisplayedRate) && stream.Kind == StreamVideo && !stream.mkvTagDuration {
			frameCount, frameCountErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameCount"), 64)
			frameRateText := matroskaStreamScalar(*stream, "FrameRate")
			frameRate, frameRateErr := strconv.ParseFloat(frameRateText, 64)
			if frameCountErr == nil && frameCount > 0 && frameRateErr == nil && frameRate > 0 {
				duration := frameCount / frameRate
				value := fmt.Sprintf("%.3f", duration)
				replaceMatroskaDurationProjection(stream, value, 3)
			}
			if handBrake {
				clearCanonicalSeedField(stream, "Standard", "Standard")
			}
		}
		if lavf && stream.Kind == StreamVideo {
			if writingApp == "Lavf58.45.100" {
				clearCanonicalSeedField(stream, "Standard", "Standard")
			}
			frameRateText := matroskaStreamScalar(*stream, "FrameRate")
			if frameRate, err := strconv.ParseFloat(frameRateText, 64); err == nil && frameRate > 0 && math.Abs(frameRate-math.Round(frameRate)) < 1e-9 && matroskaStreamScalar(*stream, "FrameRate_Mode_Original") != "VFR" {
				replaceCanonicalSeedFill(stream, "FrameRate_Num", strconv.FormatInt(int64(math.Round(frameRate)), 10), "", "")
				replaceCanonicalSeedFill(stream, "FrameRate_Den", "1", "", "")
			}
		}
		if (handBrakeDurationRules || lavfDisplayedRate || lavfIntegralDuration) && (stream.Kind == StreamVideo || stream.Kind == StreamAudio) && !stream.mkvTagDuration {
			durationText, _ := projectedCanonicalSeedValue(*stream, "Duration")
			if duration, err := strconv.ParseFloat(durationText, 64); err == nil && duration > 0 {
				value := fmt.Sprintf("%.3f", duration)
				replaceMatroskaDurationProjection(stream, value, 3)
			}
		}
		if lavfIntegralDuration && stream.Kind == StreamVideo {
			if bitRate := matroskaStreamScalar(*stream, "BitRate"); bitRate != "" && matroskaStreamScalar(*stream, "StreamSize") == "" {
				if parsed, err := strconv.ParseFloat(bitRate, 64); err == nil && parsed > 0 {
					replaceCanonicalSeedFill(stream, "BitRate_Nominal", bitRate, "Nominal bit rate", formatBitrate(parsed))
					stream.matroskaDeferredFacts.Unset("BitRate")
					clearCanonicalSeedField(stream, "BitRate", "Bit rate")
				}
			}
		}
	}
	normalizeMatroskaDeclaredFrameRates(streams)
	normalizeMatroskaMPEG4VisualSettings(streams)
}

// normalizeMatroskaDeclaredFrameRates resolves small measured-rate drift by
// using a surviving CFR numerator/denominator pair when the container also
// records an original rate. This replaces a former TrackUID-specific fix.
func normalizeMatroskaDeclaredFrameRates(streams []Stream) {
	for i := range streams {
		stream := &streams[i]
		mode := strings.ToUpper(matroskaStreamScalar(*stream, "FrameRate_Mode"))
		if stream.Kind != StreamVideo || mode != "CFR" && mode != "CONSTANT" || matroskaStreamScalar(*stream, "FrameRate_Original") == "" {
			continue
		}
		numerator, numeratorErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate_Num"), 64)
		denominator, denominatorErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate_Den"), 64)
		current, currentErr := strconv.ParseFloat(matroskaStreamScalar(*stream, "FrameRate"), 64)
		if numeratorErr != nil || denominatorErr != nil || currentErr != nil || numerator <= 0 || denominator <= 0 || current <= 0 {
			continue
		}
		declared := numerator / denominator
		if math.Abs(current-declared) > 0.01 {
			continue
		}
		replaceCanonicalSeedFill(stream, "FrameRate", fmt.Sprintf("%.3f", declared), "Frame rate", formatFrameRate(declared))
	}
}

// normalizeMatroskaMPEG4VisualSettings keeps the numeric structured GMC value
// separate from its human-readable "No" spelling, matching the field schema
// without relying on SegmentUID or TrackUID.
func normalizeMatroskaMPEG4VisualSettings(streams []Stream) {
	for i := range streams {
		stream := &streams[i]
		if stream.Kind != StreamVideo || matroskaStreamScalar(*stream, "Format") != "MPEG-4 Visual" {
			continue
		}
		gmc := matroskaStreamScalar(*stream, "Format_Settings_GMC")
		if strings.HasPrefix(gmc, "No") || strings.HasPrefix(matroskaStreamDisplay(*stream, "Format settings, GMC"), "No") {
			replaceCanonicalSeedFill(stream, "Format_Settings_GMC", "0", "Format settings, GMC", "No")
		}
	}
}

// replaceMatroskaDurationProjection stores canonical milliseconds while
// retaining the exact seconds precision required by structured renderers.
func replaceMatroskaDurationProjection(stream *Stream, seconds string, decimals uint8) {
	milliseconds, ok := decimalSecondsToMilliseconds(seconds)
	if !ok {
		return
	}
	replaceCanonicalSeedProjection(stream, "Duration", milliseconds, seconds, "", "")
	setCanonicalSeedStructuredDecimals(stream, "Duration", decimals)
}

// removeMatroskaCanonicalField removes one fact from both canonical
// projections; the public adapter later publishes the removal.
func removeMatroskaCanonicalField(stream *Stream, key string, field string) {
	clearCanonicalSeedField(stream, fieldName(key), field)
}

// replaceMatroskaCanonicalOverrides stores parsed canonical scalars in stable
// key order.
func replaceMatroskaCanonicalOverrides(stream *Stream, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		replaceCanonicalSeedFill(stream, fieldName(key), values[key], "", "")
	}
}

// replaceMatroskaCanonicalJSONOnlyOverrides stores parsed structured-only facts
// that must remain absent from text and XML projections.
func replaceMatroskaCanonicalJSONOnlyOverrides(stream *Stream, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		name := fieldName(key)
		replaceCanonicalSeedJSONOnly(stream, name, values[key])
	}
}

// replaceMatroskaCanonicalJSONOnlyProjection stores one JSON-only fact with
// exact structured precision when canonical units and projection units differ.
func replaceMatroskaCanonicalJSONOnlyProjection(stream *Stream, name fieldName, raw, projection string) {
	if stream == nil || name == "" || raw == "" || projection == "" {
		return
	}
	replaceCanonicalSeedJSONOnly(stream, name, raw)
	setCanonicalSeedStructuredDecimals(stream, name, uint8(decimalFractionDigits(projection)))
}

// setMatroskaJSONExtras replaces selected canonical extra members in place and
// appends missing members deterministically without disturbing encounter order.
func setMatroskaJSONExtras(stream *Stream, values map[string]string) {
	if stream == nil || len(values) == 0 {
		return
	}
	existing := map[string]struct{}{}
	if node := canonicalSeedStructuredNode(stream, "extra"); node != nil && node.Kind == structuredObject {
		for _, member := range node.Object {
			existing[member.Key] = struct{}{}
		}
	}
	missing := make([]string, 0, len(values))
	for key, value := range values {
		if value == "" {
			continue
		}
		if _, found := existing[key]; !found {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	members := make([]structuredMember, 0, len(missing))
	for _, key := range missing {
		members = append(members, structuredMember{Key: key, Value: structuredNode{Kind: structuredString, Text: values[key]}})
	}
	appendCanonicalSeedObjectMembers(stream, "extra", members)
	replaceCanonicalSeedObjectValues(stream, "extra", values)
}

// streamCanonicalBitRate returns a stream's encoded bitrate when present, then
// its ordinary bitrate, matching MediaInfo's stream-size fallback precedence.
func streamCanonicalBitRate(stream Stream) (float64, bool) {
	for _, key := range []string{"BitRate_Encoded", "BitRate"} {
		value, _ := canonicalSeedValue(stream, fieldName(key))
		if value != "" {
			bitRate, err := strconv.ParseFloat(value, 64)
			if err == nil && bitRate > 0 {
				return bitRate, true
			}
		}
	}
	return 0, false
}

func AnalyzeFiles(paths []string) ([]Report, int, error) {
	return AnalyzeFilesWithOptions(paths, defaultAnalyzeOptions())
}

func AnalyzeFilesWithOptions(paths []string, opts AnalyzeOptions) ([]Report, int, error) {
	expanded, err := expandPaths(paths)
	if err != nil {
		return nil, 0, err
	}
	reports := make([]Report, 0, len(expanded))
	for _, path := range expanded {
		report, err := AnalyzeFileWithOptions(path, opts)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path, err)
		}
		reports = append(reports, report)
	}
	return reports, len(reports), nil
}

func expandPaths(paths []string) ([]string, error) {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			expanded = append(expanded, path)
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			expanded = append(expanded, filepath.Join(path, name))
		}
	}
	return expanded, nil
}

func parsePixels(value string) (uint64, bool) {
	parsedValue := extractLeadingNumber(value)
	if parsedValue == "" {
		return 0, false
	}
	parsed, err := strconv.ParseUint(parsedValue, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseFPS(value string) (float64, bool) {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, false
	}
	for _, part := range parts[1:] {
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
			ratio := strings.TrimSuffix(strings.TrimPrefix(part, "("), ")")
			if ratio != "" {
				pieces := strings.Split(ratio, "/")
				if len(pieces) == 2 {
					num, numErr := strconv.ParseFloat(pieces[0], 64)
					den, denErr := strconv.ParseFloat(pieces[1], 64)
					if numErr == nil && denErr == nil && den > 0 {
						return num / den, true
					}
				}
			}
		}
	}
	return parsed, true
}

// matroskaVideoHasX264Settings reports whether Matroska encoder metadata is
// strong enough to apply x264-only bitrate and VBV rules. A positive x264 or
// libx264 library wins, an explicit x265/libx265 library rejects the settings,
// and settings-only streams are accepted only when they contain x264-specific
// option keys.
func matroskaVideoHasX264Settings(stream Stream) bool {
	writingLib := strings.TrimSpace(matroskaStreamDisplay(stream, "Writing library"))
	if matroskaWritingLibraryIsX264(writingLib) {
		return true
	}
	if matroskaWritingLibraryIsX265(writingLib) {
		return false
	}
	return matroskaEncodingSettingsLookX264(matroskaStreamDisplay(stream, "Encoding settings"))
}

// matroskaX264HRDState reports exact enabled and disabled nal_hrd options from
// normalized x264 settings. Disabled wins when malformed settings conflict.
func matroskaX264HRDState(encoding string) (enabled, disabled bool) {
	for token := range strings.SplitSeq(encoding, "/") {
		switch strings.TrimSpace(token) {
		case "nal_hrd=none", "nal_hrd=0":
			disabled = true
		case "nal_hrd=vbr", "nal_hrd=cbr":
			enabled = true
		}
	}
	return enabled && !disabled, disabled
}

func matroskaWritingLibraryIsX264(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return hasCodecLibraryToken(lower, "x264") || hasCodecLibraryToken(lower, "libx264")
}

func matroskaWritingLibraryIsX265(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return hasCodecLibraryToken(lower, "x265") || hasCodecLibraryToken(lower, "libx265")
}

// hasCodecLibraryToken matches an encoder token at the start of a library
// string without accepting adjacent names such as "x264foo".
func hasCodecLibraryToken(value, token string) bool {
	if !strings.HasPrefix(value, token) {
		return false
	}
	if len(value) == len(token) {
		return true
	}
	return !isASCIILetterDigit(value[len(token)])
}

func isASCIILetterDigit(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

// matroskaEncodingSettingsLookX264 recognizes option keys emitted by x264 that
// are not enough to identify x265 settings on their own.
func matroskaEncodingSettingsLookX264(value string) bool {
	lower := strings.ToLower(value)
	keys := [...]string{
		"cabac=", "ref=", "deblock=", "analyse=", "subme=", "psy_rd=",
		"mixed_ref=", "me_range=", "chroma_me=", "8x8dct=", "cqm=",
		"deadzone=", "fast_pskip=", "b_pyramid=", "b_adapt=", "weightb=",
		"mbtree=", "keyint_min=", "scenecut=", "intra_refresh=",
	}
	for _, key := range keys {
		if strings.Contains(lower, key) {
			return true
		}
	}
	return false
}

// x264InfoOptions controls which inferred x264 metadata may update an existing
// canonical video stream.
type x264InfoOptions struct {
	skipWritingLibIfExists bool
	skipEncodingIfExists   bool
	addNominalBitrate      bool
	addBitsPerPixel        bool
}

// applyX264Info scans bounded stream data for x264 metadata and applies the
// discovered library, settings, bitrate, and density facts to the video stream.
func applyX264Info(file io.ReadSeeker, streams []Stream, opts x264InfoOptions) {
	writingLib, encoding := scanX264Info(file)
	if writingLib == "" && encoding == "" {
		return
	}
	applyX264Values(streams, writingLib, encoding, opts)
}

// scanX264Info reads the bounded head and tail probes used to discover AVC
// writing-library and encoder-setting strings, restoring the reader to start.
func scanX264Info(file io.ReadSeeker) (string, string) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", ""
	}
	defer func() { _, _ = file.Seek(0, io.SeekStart) }()
	// MP4 can embed x264 strings inside the first few MB of mdat, not just moov/udta.
	sniff := make([]byte, 4<<20)
	n, _ := io.ReadFull(file, sniff)
	writingLib, encoding := findX264Info(sniff[:n])
	if writingLib == "" && encoding == "" {
		writingLib = findH264WritingLibrary(sniff[:n])
	}
	if writingLib == "" && encoding == "" {
		// MP4 often stores writing-library strings late in the moov/udta metadata.
		if end, err := file.Seek(0, io.SeekEnd); err == nil && end > 0 {
			start := end - int64(len(sniff))
			if start < 0 {
				start = 0
			}
			if _, err := file.Seek(start, io.SeekStart); err == nil {
				n2, _ := file.Read(sniff)
				if n2 > 0 {
					writingLib, encoding = findX264Info(sniff[:n2])
					if writingLib == "" && encoding == "" {
						writingLib = findH264WritingLibrary(sniff[:n2])
					}
				}
			}
		}
	}
	return writingLib, encoding
}

// applyX264Values applies discovered AVC encoder metadata to the first matching
// video stream while preserving each container's existing fill policy.
func applyX264Values(streams []Stream, writingLib, encoding string, opts x264InfoOptions) {
	for i := range streams {
		if streams[i].Kind != StreamVideo || findField(streams[i].Fields, "Format") != "AVC" {
			continue
		}
		if writingLib != "" {
			if !opts.skipWritingLibIfExists || matroskaStreamDisplay(streams[i], "Writing library") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: writingLib})
				if len(streams[i].canonicalSeed) > 0 {
					encodedLibrary := canonicalEncodedLibrary(writingLib)
					replaceCanonicalSeedFill(&streams[i], "Encoded_Library", encodedLibrary, "Writing library", writingLib)
					if name, version := splitEncodedLibrary(encodedLibrary); name != "" {
						replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Name", name, "", "")
						replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Version", version, "", "")
					}
				}
			}
			// Some encoders (non-x264) expect Encoded_Library_Name to mirror the full string.
			if encoding == "" && strings.Contains(writingLib, "Encoder") {
				replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Name", writingLib, "", "")
			}
		}
		if encoding != "" {
			if !opts.skipEncodingIfExists || matroskaStreamDisplay(streams[i], "Encoding settings") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: encoding})
				if len(streams[i].canonicalSeed) > 0 {
					replaceCanonicalSeedFill(&streams[i], "Encoded_Library_Settings", encoding, "Encoding settings", encoding)
				}
			}
		}
		if opts.addNominalBitrate && encoding != "" && matroskaStreamDisplay(streams[i], "Nominal bit rate") == "" {
			if bitrate, ok := findX264Bitrate(encoding); ok {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
				if len(streams[i].canonicalSeed) > 0 {
					replaceCanonicalSeedFill(&streams[i], "BitRate_Nominal", strconv.FormatInt(int64(math.Round(bitrate)), 10), "Nominal bit rate", formatBitrate(bitrate))
				}
				if opts.addBitsPerPixel {
					width, _ := strconv.ParseUint(matroskaStreamScalar(streams[i], "Width"), 10, 64)
					height, _ := strconv.ParseUint(matroskaStreamScalar(streams[i], "Height"), 10, 64)
					fps, _ := strconv.ParseFloat(matroskaStreamScalar(streams[i], "FrameRate"), 64)
					if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
						streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
					}
				}
			}
		}
		break
	}
}
