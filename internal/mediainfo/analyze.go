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

func AnalyzeFile(path string) (Report, error) {
	return AnalyzeFileWithOptions(path, defaultAnalyzeOptions())
}

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

	if opts.TestContinuousFileNames && (format == "BDAV" || format == "MPEG-TS") {
		if set, ok := detectContinuousFileSet(path); ok {
			completeNameLast = set.LastPath
			fileSize = set.TotalSize
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
		if general.JSON == nil {
			general.JSON = map[string]string{}
		}
		// Override JSON FileSize (default comes from report.Ref on disk).
		general.JSON["FileSize"] = strconv.FormatInt(fileSize, 10)
	}

	info := ContainerInfo{}
	streams := []Stream{}
	switch format {
	case "MPEG-4", "QuickTime":
		if parsed, ok := ParseMP4(file, stat.Size()); ok {
			info = parsed.Container
			general.JSON = map[string]string{}
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			setOverallBitRate(general.JSON, stat.Size(), info.DurationSeconds)
			if headerSize, dataSize, footerSize, mdatCount, moovBeforeMdat, ok := mp4TopLevelSizes(file, stat.Size()); ok {
				general.JSON["HeaderSize"] = strconv.FormatInt(headerSize, 10)
				general.JSON["DataSize"] = strconv.FormatInt(dataSize, 10)
				general.JSON["FooterSize"] = strconv.FormatInt(footerSize, 10)
				if moovBeforeMdat {
					general.JSON["IsStreamable"] = "Yes"
				} else {
					general.JSON["IsStreamable"] = "No"
				}
				_ = mdatCount
			}
			var generalFrameCount string
			for _, track := range parsed.Tracks {
				fields := []Field{}
				displayDuration := track.DurationSeconds
				sourceDuration := 0.0
				if track.EditDuration > 0 && track.DurationSeconds > 0 {
					if math.Abs(track.EditDuration-track.DurationSeconds) > 0.0005 {
						displayDuration = track.EditDuration
						sourceDuration = track.DurationSeconds
					}
				}
				if track.ID > 0 {
					fields = appendFieldUnique(fields, Field{Name: "ID", Value: strconv.FormatUint(uint64(track.ID), 10)})
				}
				if track.Format != "" {
					fields = appendFieldUnique(fields, Field{Name: "Format", Value: track.Format})
				}
				for _, field := range track.Fields {
					fields = appendFieldUnique(fields, field)
				}
				var bitrate float64
				jsonExtras := map[string]string{}
				jsonRaw := map[string]string{}
				if len(track.JSON) > 0 {
					for k, v := range track.JSON {
						jsonExtras[k] = v
					}
				}
				if track.Kind == StreamVideo {
					jsonExtras["Rotation"] = "0.000"
				}
				if displayDuration > 0 {
					if track.SampleBytes > 0 {
						durationForBitrate := displayDuration
						if sourceDuration > 0 {
							durationForBitrate = sourceDuration
						}
						bitrate = (float64(track.SampleBytes) * 8) / durationForBitrate
					}
					fields = addStreamDuration(fields, displayDuration)
					if sourceDuration > 0 {
						fields = appendFieldUnique(fields, Field{Name: "Source duration", Value: formatDuration(sourceDuration)})
						if track.SampleDelta > 0 && track.LastSampleDelta > 0 && track.Timescale > 0 {
							if track.LastSampleDelta != track.SampleDelta {
								diffSamples := int64(track.LastSampleDelta) - int64(track.SampleDelta)
								diffMs := int64(math.Round(float64(diffSamples) * 1000 / float64(track.Timescale)))
								if diffMs != 0 {
									fields = appendFieldUnique(fields, Field{Name: "Source_Duration_LastFrame", Value: strconv.FormatInt(diffMs, 10) + " ms"})
								}
							}
						}
					}
					if bitrate > 0 && !(track.Kind == StreamAudio && findField(fields, "Bit rate") != "") {
						if track.Kind != StreamVideo {
							if mode := bitrateMode(bitrate); mode != "" {
								fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: mode})
							}
						}
						fields = addStreamBitrate(fields, bitrate)
						// Match official mediainfo rounding for derived MP4 bitrates.
						jsonExtras["BitRate"] = strconv.FormatInt(int64(math.Floor(bitrate)), 10)
					}
				}
				if track.SampleBytes > 0 {
					streamBytes := int64(track.SampleBytes)
					displaySamples := 0.0
					if sourceDuration > 0 && displayDuration > 0 {
						if track.Kind == StreamAudio {
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
						} else if track.SampleDelta > 0 && track.SampleCount > 0 && track.Timescale > 0 {
							displaySamples = (displayDuration * float64(track.Timescale)) / float64(track.SampleDelta)
							if displaySamples > 0 {
								streamBytes = int64(math.Round(float64(track.SampleBytes) * displaySamples / float64(track.SampleCount)))
							} else if bitrate > 0 {
								streamBytes = int64(math.Round((bitrate * displayDuration) / 8))
							}
						} else if bitrate > 0 {
							streamBytes = int64(math.Round((bitrate * displayDuration) / 8))
						}
					}
					if streamSize := formatStreamSize(streamBytes, stat.Size()); streamSize != "" {
						fields = appendFieldUnique(fields, Field{Name: "Stream size", Value: streamSize})
					}
					if streamBytes > 0 {
						jsonExtras["StreamSize"] = strconv.FormatInt(streamBytes, 10)
					}
					if sourceDuration > 0 {
						if sourceSize := formatStreamSize(int64(track.SampleBytes), stat.Size()); sourceSize != "" {
							fields = appendFieldUnique(fields, Field{Name: "Source stream size", Value: sourceSize})
						}
						jsonExtras["Source_StreamSize"] = strconv.FormatInt(int64(track.SampleBytes), 10)
						if track.Kind == StreamAudio {
							if displaySamples > 0 {
								jsonExtras["FrameCount"] = strconv.FormatInt(int64(math.Round(displaySamples)), 10)
							}
							if track.SampleCount > 0 {
								jsonExtras["Source_FrameCount"] = strconv.FormatUint(track.SampleCount, 10)
							}
						}
					}
				}
				if track.Kind == StreamVideo && track.SampleCount > 0 && displayDuration > 0 {
					fields = appendFieldUnique(fields, Field{Name: "Frame rate mode", Value: "Constant"})
					rate := float64(track.SampleCount) / displayDuration
					if rate > 0 {
						fields = appendFieldUnique(fields, Field{Name: "Frame rate", Value: formatFrameRate(rate)})
					}
					if track.Width > 0 && track.Height > 0 && track.SampleBytes > 0 {
						pixelBitrate := (float64(track.SampleBytes) * 8) / displayDuration
						if bits := formatBitsPerPixelFrame(pixelBitrate, track.Width, track.Height, rate); bits != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
						}
					}
					jsonExtras["FrameCount"] = strconv.FormatUint(track.SampleCount, 10)
					jsonExtras["FrameRate_Mode_Original"] = "VFR"
					if generalFrameCount == "" {
						generalFrameCount = strconv.FormatUint(track.SampleCount, 10)
					}
				}
				if track.Default && track.Kind != StreamVideo {
					fields = appendFieldUnique(fields, Field{Name: "Default", Value: "Yes"})
				}
				if track.AlternateGroup > 0 {
					fields = appendFieldUnique(fields, Field{Name: "Alternate group", Value: strconv.FormatUint(uint64(track.AlternateGroup), 10)})
				}
				if track.Kind == StreamAudio && track.EditMediaTime > 0 && track.Timescale > 0 {
					delayMs := int64(math.Round(float64(track.EditMediaTime) * 1000 / float64(track.Timescale)))
					if delayMs != 0 {
						jsonRaw["extra"] = "{\"Source_Delay\":\"-" + strconv.FormatInt(delayMs, 10) + "\",\"Source_Delay_Source\":\"Container\"}"
					}
				}
				streams = append(streams, Stream{Kind: track.Kind, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw})
			}
			if generalFrameCount != "" {
				general.JSON["FrameCount"] = generalFrameCount
			}
			applyX264Info(file, streams, x264InfoOptions{})
			// MP4 General StreamSize: remaining bytes after summing track stream sizes.
			streamSizeSum := sumStreamSizes(streams, true)
			setRemainingStreamSize(general.JSON, stat.Size(), streamSizeSum)
		}
	case "Matroska":
		if parsed, ok := ParseMatroskaWithOptions(file, stat.Size(), opts); ok {
			info = parsed.Container
			general.JSON = map[string]string{}
			var rawWritingApp string
			for _, field := range parsed.General {
				if field.Name == "Writing application" {
					rawWritingApp = field.Value
					field.Value = normalizeWritingApplication(field.Value)
				}
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Tracks...)
			if len(parsed.attachments) > 0 {
				if general.JSONRaw == nil {
					general.JSONRaw = map[string]string{}
				}
				// Match official JSON: attachments are nested under General.extra.Attachments.
				general.JSONRaw["extra"] = "{\"Attachments\":\"" + strings.Join(parsed.attachments, " / ") + "\"}"
			}
			if rawWritingApp != "" {
				general.JSON["Encoded_Application"] = rawWritingApp
			}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = formatJSONFloat(info.DurationSeconds)
			}
			setOverallBitRate(general.JSON, stat.Size(), info.DurationSeconds)
			general.JSON["IsStreamable"] = "Yes"
			streamSizeSum := sumStreamSizes(streams, true)
			// Official mediainfo does not expose large Matroska overhead as General StreamSize when
			// it's dominated by attachments (fonts).
			if len(parsed.attachments) == 0 {
				setRemainingStreamSize(general.JSON, stat.Size(), streamSizeSum)
			}
			overallModeField := ""
			for _, stream := range streams {
				if stream.Kind != StreamVideo {
					continue
				}
				if mode := findField(stream.Fields, "Bit rate mode"); mode != "" {
					overallModeField = mode
					break
				}
				if stream.JSON != nil {
					if mode := stream.JSON["BitRate_Mode"]; mode != "" {
						switch strings.ToUpper(mode) {
						case "VBR":
							overallModeField = "Variable"
						case "CBR":
							overallModeField = "Constant"
						default:
							overallModeField = mode
						}
						break
					}
				}
			}
			if overallModeField == "Variable" {
				general.Fields = appendFieldUnique(general.Fields, Field{Name: "Overall bit rate mode", Value: overallModeField})
				general.JSON["OverallBitRate_Mode"] = mapBitrateMode(overallModeField)
			}
			for _, stream := range streams {
				if stream.Kind != StreamVideo {
					continue
				}
				if frameCount := stream.JSON["FrameCount"]; frameCount != "" {
					general.JSON["FrameCount"] = frameCount
				} else {
					duration, durOk := parseDurationSeconds(findField(stream.Fields, "Duration"))
					fps, fpsOk := parseFPS(findField(stream.Fields, "Frame rate"))
					if durOk && fpsOk {
						general.JSON["FrameCount"] = strconv.Itoa(int(math.Round(duration * fps)))
					}
				}
				break
			}
			applyX264Info(file, streams, x264InfoOptions{
				skipWritingLibIfExists: true,
				skipEncodingIfExists:   true,
				addNominalBitrate:      true,
				addBitsPerPixel:        false,
			})
			// MediaInfo prefers x264 settings for bitrate/VBV constraints when available.
			for i := range streams {
				if streams[i].Kind != StreamVideo {
					continue
				}
				enc := findField(streams[i].Fields, "Encoding settings")
				if enc == "" {
					continue
				}
				if bitrate, ok := findX264Bitrate(enc); ok && bitrate > 0 && findField(streams[i].Fields, "Nominal bit rate") == "" {
					streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
					if streams[i].JSON == nil {
						streams[i].JSON = map[string]string{}
					}
					streams[i].JSON["BitRate_Nominal"] = strconv.FormatInt(int64(math.Round(bitrate)), 10)
				}
				// MediaInfo reports VBV constraints only when HRD signaling is enabled.
				if !strings.Contains(enc, "nal_hrd=none") {
					if maxKbps, ok := findX264VbvMaxrate(enc); ok && maxKbps > 0 {
						maxBps := maxKbps * 1000
						streams[i].Fields = setFieldValue(streams[i].Fields, "Maximum bit rate", formatBitrate(maxBps))
						if streams[i].JSON == nil {
							streams[i].JSON = map[string]string{}
						}
						streams[i].JSON["BitRate_Maximum"] = strconv.FormatInt(int64(math.Round(maxBps)), 10)
					}
					if bufKbps, ok := findX264VbvBufsize(enc); ok && bufKbps > 0 {
						bufBps := bufKbps * 1000
						if streams[i].JSON == nil {
							streams[i].JSON = map[string]string{}
						}
						if streams[i].JSON["BufferSize"] == "" {
							streams[i].JSON["BufferSize"] = strconv.FormatInt(int64(math.Round(bufBps)), 10)
						}
					}
				}
			}
		}
	case "MPEG-TS":
		if parsedInfo, parsedStreams, generalFields, ok := ParseMPEGTS(file, stat.Size()); ok {
			info = parsedInfo
			general.JSON = map[string]string{}
			general.JSONRaw = map[string]string{}
			if completeNameLast != "" {
				general.JSON["FileSize"] = strconv.FormatInt(fileSize, 10)
			}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if id := findField(general.Fields, "ID"); id != "" {
				if value := extractLeadingNumber(id); value != "" {
					general.JSON["ID"] = value
				}
			}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = fmt.Sprintf("%.9f", info.DurationSeconds)
			}
			setOverallBitRate(general.JSON, fileSize, info.DurationSeconds)
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				minRate := int64(math.Round(info.OverallBitrateMin))
				maxRate := int64(math.Round(info.OverallBitrateMax))
				general.JSONRaw["extra"] = "{\"OverallBitRate_Precision_Min\":\"" + strconv.FormatInt(minRate, 10) + "\",\"OverallBitRate_Precision_Max\":\"" + strconv.FormatInt(maxRate, 10) + "\"}"
			}
			applyX264Info(file, streams, x264InfoOptions{
				addNominalBitrate: true,
			})
		}
	case "BDAV":
		if parsedInfo, parsedStreams, generalFields, ok := ParseBDAV(file, stat.Size()); ok {
			info = parsedInfo
			general.JSON = map[string]string{}
			general.JSONRaw = map[string]string{}
			if completeNameLast != "" {
				general.JSON["FileSize"] = strconv.FormatInt(fileSize, 10)
			}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if id := findField(general.Fields, "ID"); id != "" {
				if value := extractLeadingNumber(id); value != "" {
					general.JSON["ID"] = value
				}
			}
			// MediaInfo reports BDAV/M2TS General ID as 0.
			general.JSON["ID"] = "0"
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = fmt.Sprintf("%.9f", info.DurationSeconds)
			}
			// Blu-ray max mux rate (per MediaInfo output).
			general.JSON["OverallBitRate_Maximum"] = "48000000"
			// MediaInfo uses a PCR-derived estimate for BDAV overall bitrate.
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				mid := (info.OverallBitrateMin + info.OverallBitrateMax) / 2
				general.JSON["OverallBitRate"] = strconv.FormatInt(int64(math.Round(mid)), 10)
			} else {
				setOverallBitRate(general.JSON, fileSize, info.DurationSeconds)
			}
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				minRate := int64(math.Round(info.OverallBitrateMin))
				maxRate := int64(math.Round(info.OverallBitrateMax))
				general.JSONRaw["extra"] = "{\"OverallBitRate_Precision_Min\":\"" + strconv.FormatInt(minRate, 10) + "\",\"OverallBitRate_Precision_Max\":\"" + strconv.FormatInt(maxRate, 10) + "\"}"
			}
			applyX264Info(file, streams, x264InfoOptions{
				addNominalBitrate: true,
			})

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
						if li, ls, _, ok := ParseBDAV(f, st.Size()); ok && li.DurationSeconds > 0 {
							lastInfo = li
							lastStreams = ls
						}
					}
					_ = f.Close()
				}
				if lastInfo.DurationSeconds > 0 {
					info.DurationSeconds = lastInfo.DurationSeconds
					general.JSON["Duration"] = fmt.Sprintf("%.9f", info.DurationSeconds)
					// MediaInfo continuous-file behavior: total FileSize, but bitrate correction based on the last file's PCR-derived bitrate.
					if lastSize > 0 && fileSize > lastSize && lastInfo.OverallBitrateMin > 0 && lastInfo.OverallBitrateMax > 0 && info.DurationSeconds > 0 {
						lastMid := (lastInfo.OverallBitrateMin + lastInfo.OverallBitrateMax) / 2
						lastMidRounded := float64(int64(math.Round(lastMid)))
						overall := (float64(fileSize-lastSize) * 8 / info.DurationSeconds) + lastMidRounded
						overallRounded := int64(math.Round(overall))
						general.JSON["OverallBitRate"] = strconv.FormatInt(overallRounded, 10)
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
						general.JSONRaw["extra"] = "{\"OverallBitRate_Precision_Min\":\"" + strconv.FormatInt(minRate, 10) + "\",\"OverallBitRate_Precision_Max\":\"" + strconv.FormatInt(maxRate, 10) + "\"}"
					} else {
						setOverallBitRate(general.JSON, fileSize, info.DurationSeconds)
					}
					// Override per-stream JSON durations to match MediaInfo's continuous file behavior.
					var lastVideoDuration string
					var lastVideoFrameCount string
					for _, s := range lastStreams {
						if s.Kind != StreamVideo || s.JSON == nil {
							continue
						}
						lastVideoDuration = s.JSON["Duration"]
						lastVideoFrameCount = s.JSON["FrameCount"]
						break
					}
					for i := range streams {
						if streams[i].JSON == nil {
							continue
						}
						switch streams[i].Kind {
						case StreamVideo:
							if lastVideoDuration != "" {
								streams[i].JSON["Duration"] = lastVideoDuration
							} else {
								streams[i].JSON["Duration"] = fmt.Sprintf("%.3f", info.DurationSeconds)
							}
							if lastVideoFrameCount != "" {
								streams[i].JSON["FrameCount"] = lastVideoFrameCount
							}
						case StreamAudio:
							streams[i].JSON["Duration"] = fmt.Sprintf("%.3f", info.DurationSeconds)
							delete(streams[i].JSON, "FrameCount")
							if sr, ok := parseSampleRate(findField(streams[i].Fields, "Sampling rate")); ok && sr > 0 && info.DurationSeconds > 0 {
								samplingCount := int64(math.Round(info.DurationSeconds * float64(sr)))
								if samplingCount > 0 {
									streams[i].JSON["SamplingCount"] = strconv.FormatInt(samplingCount, 10)
								}
							}
						case StreamText:
							delete(streams[i].JSON, "Duration")
							delete(streams[i].JSON, "FrameCount")
						}
					}
				}

				// Prefer bitrate-derived audio StreamSize (CBR) over PID byte counting.
				for i := range streams {
					if streams[i].Kind != StreamAudio || streams[i].JSON == nil {
						continue
					}
					br := int64(0)
					if parsed, ok := parseInt(streams[i].JSON["BitRate"]); ok && parsed > 0 {
						br = parsed
					} else if parsed, ok := parseBitrateBps(findField(streams[i].Fields, "Bit rate")); ok && parsed > 0 {
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
						streams[i].JSON["StreamSize"] = strconv.FormatInt(ss, 10)
					}
				}
			}

			// BDAV JSON parity: general FrameCount + remaining StreamSize (overhead, subtitles, etc).
			for _, stream := range streams {
				if stream.Kind != StreamVideo {
					continue
				}
				if stream.JSON != nil {
					if value := stream.JSON["FrameCount"]; value != "" {
						general.JSON["FrameCount"] = value
					}
				}
				if general.JSON["FrameCount"] == "" {
					if count, ok := frameCountFromFields(stream.Fields); ok {
						general.JSON["FrameCount"] = count
					}
				}
				break
			}
			var audioSum int64
			for _, stream := range streams {
				if stream.Kind != StreamAudio || stream.JSON == nil {
					continue
				}
				if ss, ok := parseInt(stream.JSON["StreamSize"]); ok && ss > 0 {
					audioSum += ss
				}
			}

			if completeNameLast != "" {
				// MediaInfo's continuous-file behavior for BDAV: derive video bitrate/size from overall bitrate,
				// subtracting audio + text overhead, then set General StreamSize as the remainder.
				overallInt, ok := parseInt(general.JSON["OverallBitRate"])
				if ok && overallInt > 0 && info.DurationSeconds > 0 && fileSize > 0 {
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
						if streams[i].JSON != nil {
							if parsed, ok := parseInt(streams[i].JSON["BitRate"]); ok && parsed > 0 {
								br = parsed
							}
						}
						if br == 0 {
							if parsed, ok := parseBitrateBps(findField(streams[i].Fields, "Bit rate")); ok && parsed > 0 {
								br = parsed
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
							if streams[i].JSON != nil {
								if parsed, ok := parseInt(streams[i].JSON["BitRate"]); ok && parsed > 0 {
									textBR = float64(parsed)
								}
							}
							if textBR == 0 {
								if parsed, ok := parseBitrateBps(findField(streams[i].Fields, "Bit rate")); ok && parsed > 0 {
									textBR = float64(parsed)
								}
							}
							videoBitrate -= textBR/textRatio + textMinus
						}
						videoBitrate = videoBitrate*videoRatio - videoMinus
					}

					if valid && videoBitrate >= 10000 {
						videoBps := int64(math.Round(videoBitrate))
						var frameRate float64
						var frameCount int64
						for i := range streams {
							if streams[i].Kind != StreamVideo || streams[i].JSON == nil {
								continue
							}
							if parsed, err := strconv.ParseFloat(streams[i].JSON["FrameRate"], 64); err == nil && parsed > 0 {
								frameRate = parsed
							} else if parsed, ok := parseFPS(findField(streams[i].Fields, "Frame rate")); ok && parsed > 0 {
								frameRate = parsed
							}
							if parsed, ok := parseInt(streams[i].JSON["FrameCount"]); ok && parsed > 0 {
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
								if streams[i].JSON == nil {
									streams[i].JSON = map[string]string{}
								}
								streams[i].JSON["BitRate"] = strconv.FormatInt(videoBps, 10)
								streams[i].JSON["StreamSize"] = strconv.FormatInt(videoSS, 10)
								streams[i].Fields = setFieldValue(streams[i].Fields, "Bit rate", formatBitrate(float64(videoBps)))
								break
							}
							generalSS := fileSize - videoSS - audioSum
							if generalSS > 0 {
								general.JSON["StreamSize"] = strconv.FormatInt(generalSS, 10)
							}
						}
					}
				}
			} else {
				overhead := info.StreamOverheadBytes
				if overhead > 0 {
					general.JSON["StreamSize"] = strconv.FormatInt(overhead, 10)
				}
				if fileSize > 0 && overhead > 0 {
					videoSS := fileSize - overhead - audioSum
					if videoSS > 0 {
						for i := range streams {
							if streams[i].Kind != StreamVideo {
								continue
							}
							if streams[i].JSON == nil {
								streams[i].JSON = map[string]string{}
							}
							streams[i].JSON["StreamSize"] = strconv.FormatInt(videoSS, 10)
							if info.DurationSeconds > 0 {
								br := int64(math.Round((float64(videoSS) * 8) / info.DurationSeconds))
								if br > 0 {
									streams[i].JSON["BitRate"] = strconv.FormatInt(br, 10)
									streams[i].Fields = setFieldValue(streams[i].Fields, "Bit rate", formatBitrate(float64(br)))
								}
							}
							break
						}
					}
				}
			}
		}
	case "MPEG-PS":
		psSize := stat.Size()
		psPaths := []string{path}
		var completeNameLast string
		dvdExtras := false
		dvdParsing := false
		if strings.EqualFold(filepath.Ext(path), ".vob") && strings.EqualFold(filepath.Base(filepath.Dir(path)), "VIDEO_TS") {
			// Keep VOB-as-file behavior aligned with official mediainfo: treat it as plain MPEG-PS.
			// DVD title-set aggregation is handled when parsing IFOs.
			_ = completeNameLast
		}
		parseSpeed := opts.ParseSpeed
		if dvdParsing && parseSpeed < 1 {
			// DVD-Video VOB aggregation needs full parsing for stable duration/stream stats.
			parseSpeed = 1
		}
		var parsedInfo ContainerInfo
		var parsedStreams []Stream
		var ok bool
		if len(psPaths) > 1 {
			parsedInfo, parsedStreams, ok = ParseMPEGPSFiles(psPaths, psSize, mpegPSOptions{dvdExtras: dvdExtras, dvdParsing: dvdParsing, parseSpeed: parseSpeed})
		} else {
			parsedInfo, parsedStreams, ok = ParseMPEGPSWithOptions(file, psSize, mpegPSOptions{dvdExtras: dvdExtras, dvdParsing: dvdParsing, parseSpeed: parseSpeed})
		}
		if ok {
			info = parsedInfo
			streams = parsedStreams
			if general.JSON == nil {
				general.JSON = map[string]string{}
			}
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				if jsonDuration > 0 {
					general.JSON["Duration"] = formatJSONSeconds(jsonDuration)
					overall := (float64(psSize) * 8) / jsonDuration
					general.JSON["OverallBitRate"] = strconv.FormatInt(int64(math.Round(overall)), 10)
				}
			}
			var frameCount string
			hasAudio := false
			for _, stream := range streams {
				if stream.Kind == StreamAudio {
					hasAudio = true
					break
				}
			}
			audioIndex := 0
			textIndex := 0
			for i := range streams {
				if streams[i].Kind == StreamMenu {
					streams[i].JSONSkipStreamOrder = true
					continue
				}
				if streams[i].Kind == StreamAudio {
					streams[i].JSON["StreamOrder"] = strconv.Itoa(audioIndex)
					audioIndex++
				}
				if streams[i].Kind == StreamText {
					streams[i].JSON["StreamOrder"] = strconv.Itoa(textIndex)
					textIndex++
				}
				if streams[i].Kind == StreamVideo {
					if streams[i].JSON != nil {
						if value := streams[i].JSON["FrameCount"]; value != "" {
							frameCount = value
						}
					}
					if frameCount == "" && findField(streams[i].Fields, "Format") != "" {
						if count, ok := frameCountFromFields(streams[i].Fields); ok {
							frameCount = count
						}
					}
					if hasAudio {
						streams[i].JSONSkipStreamOrder = true
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			streamSizeSum := sumStreamSizes(streams, false)
			setRemainingStreamSize(general.JSON, psSize, streamSizeSum)
		}
	case "MPEG Audio":
		if parsedInfo, parsedStreams, ok := ParseMP3(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			// For audio-only formats, the Field-based duration formatting drops milliseconds
			// (e.g. "23 min 34 s"). Override JSON Duration to keep MediaInfo parity.
			general.JSON = map[string]string{}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = formatJSONSeconds(info.DurationSeconds)
			}
			// Official mediainfo overall bitrate excludes tags (ID3v2/ID3v1).
			payloadSize := stat.Size() - info.StreamOverheadBytes
			if payloadSize < 0 {
				payloadSize = stat.Size()
			}
			setOverallBitRate(general.JSON, payloadSize, info.DurationSeconds)
			if info.StreamOverheadBytes > 0 {
				general.JSON["StreamSize"] = strconv.FormatInt(info.StreamOverheadBytes, 10)
			}
		}
	case "FLAC":
		if parsedInfo, parsedStreams, ok := ParseFLAC(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			general.JSON = map[string]string{}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = formatJSONSeconds(info.DurationSeconds)
			}
			// Official mediainfo overall bitrate uses total file size for FLAC.
			setOverallBitRate(general.JSON, stat.Size(), info.DurationSeconds)
			// Official mediainfo sets General StreamSize=0 for FLAC.
			general.JSON["StreamSize"] = "0"
		}
	case "Wave":
		if parsedInfo, parsedStreams, ok := ParseWAV(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
		}
	case "Ogg":
		if parsedInfo, parsedStreams, ok := ParseOgg(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
		}
	case "MPEG Video":
		if parsedInfo, parsedStreams, ok := ParseMPEGVideo(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			general.JSON = map[string]string{}
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Format version", Value: "Version 2"})
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "FileExtension_Invalid", Value: "mpgv mpv mp1v m1v mp2v m2v"})
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Conformance warnings", Value: "Yes"})
			general.Fields = appendFieldUnique(general.Fields, Field{Name: " General compliance", Value: "File name extension is not expected for this file format (actual mpg, expected mpgv mpv mp1v m1v mp2v m2v)"})
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				setOverallBitRate(general.JSON, stat.Size(), jsonDuration)
			}
			var frameCount string
			for i := range streams {
				streams[i].JSONSkipStreamOrder = true
				if streams[i].Kind == StreamVideo {
					if count, ok := frameCountFromFields(streams[i].Fields); ok {
						frameCount = count
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			streamSizeSum := sumStreamSizes(streams, false)
			setRemainingStreamSize(general.JSON, stat.Size(), streamSizeSum)
			general.JSONRaw = map[string]string{
				"extra": "{\"FileExtension_Invalid\":\"mpgv mpv mp1v m1v mp2v m2v\",\"ConformanceWarnings\":[{\"GeneralCompliance\":\"File name extension is not expected for this file format (actual mpg, expected mpgv mpv mp1v m1v mp2v m2v)\"}]}",
			}
		}
	case "AVI":
		if parsedInfo, parsedStreams, generalFields, ok := ParseAVIWithOptions(file, stat.Size(), opts); ok {
			info = parsedInfo
			general.JSON = map[string]string{}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				setOverallBitRate(general.JSON, stat.Size(), jsonDuration)
			}
			var frameCount string
			for _, stream := range streams {
				if stream.Kind == StreamVideo {
					if count, ok := frameCountFromFields(stream.Fields); ok {
						frameCount = count
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			streamSizeSum := sumStreamSizes(streams, false)
			setRemainingStreamSize(general.JSON, stat.Size(), streamSizeSum)
		}
	case "DVD Video":
		if parsed, ok := parseDVDVideo(path, file, stat.Size(), opts); ok {
			info = parsed.Container
			if parsed.FileSize > 0 {
				general.Fields = setFieldValue(general.Fields, "File size", formatBytes(parsed.FileSize))
			}
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Streams...)
			if parsed.GeneralJSON != nil {
				general.JSON = parsed.GeneralJSON
			} else {
				general.JSON = map[string]string{}
			}
			if parsed.GeneralJSONRaw != nil {
				general.JSONRaw = parsed.GeneralJSONRaw
			}
			if parsed.FileSize > 0 {
				general.JSON["FileSize"] = strconv.FormatInt(parsed.FileSize, 10)
			}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = formatJSONSeconds(info.DurationSeconds)
			}
			if value := extractLeadingNumber(findField(general.Fields, "Frame rate")); value != "" {
				general.JSON["FrameRate"] = value
			}
			if mode := findField(general.Fields, "Overall bit rate mode"); mode != "" {
				general.JSON["OverallBitRate_Mode"] = mapBitrateMode(mode)
			}
		}
	}

	for _, stream := range streams {
		if stream.Kind != StreamVideo {
			continue
		}
		if rate := findField(stream.Fields, "Frame rate"); rate != "" {
			if (format == "MPEG-PS" || format == "MPEG Video" || format == "Matroska") && strings.Contains(rate, "(") {
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
			if mode != "" && format != "Matroska" && format != "AVI" && format != "MPEG Audio" && format != "MPEG-4" && format != "QuickTime" {
				general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: mode})
			}
			if mode == "" && format != "Matroska" && format != "AVI" && format != "MPEG Audio" && format != "MPEG-4" && format != "QuickTime" {
				if inferred := bitrateMode(bitrate); inferred != "" {
					general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: inferred})
				}
			}
			general.Fields = append(general.Fields, Field{Name: "Overall bit rate", Value: formatBitrate(bitrate)})
		}
	}

	sortFields(StreamGeneral, general.Fields)
	for i := range streams {
		sortFields(streams[i].Kind, streams[i].Fields)
	}
	sortStreams(streams)
	return Report{
		Ref:     path,
		General: general,
		Streams: streams,
	}, nil
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

type x264InfoOptions struct {
	skipWritingLibIfExists bool
	skipEncodingIfExists   bool
	addNominalBitrate      bool
	addBitsPerPixel        bool
}

func applyX264Info(file io.ReadSeeker, streams []Stream, opts x264InfoOptions) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return
	}
	sniff := make([]byte, 1<<20)
	n, _ := io.ReadFull(file, sniff)
	writingLib, encoding := findX264Info(sniff[:n])
	if writingLib == "" && encoding == "" {
		return
	}
	for i := range streams {
		if streams[i].Kind != StreamVideo || findField(streams[i].Fields, "Format") != "AVC" {
			continue
		}
		if writingLib != "" {
			if !opts.skipWritingLibIfExists || findField(streams[i].Fields, "Writing library") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: writingLib})
			}
		}
		if encoding != "" {
			if !opts.skipEncodingIfExists || findField(streams[i].Fields, "Encoding settings") == "" {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: encoding})
			}
		}
		if opts.addNominalBitrate && encoding != "" && findField(streams[i].Fields, "Nominal bit rate") == "" {
			if bitrate, ok := findX264Bitrate(encoding); ok {
				streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
				if opts.addBitsPerPixel {
					width, _ := parsePixels(findField(streams[i].Fields, "Width"))
					height, _ := parsePixels(findField(streams[i].Fields, "Height"))
					fps, _ := parseFPS(findField(streams[i].Fields, "Frame rate"))
					if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
						streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
					}
				}
			}
		}
		break
	}
}
