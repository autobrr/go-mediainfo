package mediainfo

import (
	"fmt"
	"io"
	"math"
	"os"
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

	header := make([]byte, maxSniffBytes)
	file, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer file.Close()

	n, _ := io.ReadFull(file, header)
	header = header[:n]

	format := DetectFormat(header, path)

	general := Stream{Kind: StreamGeneral}
	general.Fields = append(general.Fields,
		Field{Name: "Complete name", Value: path},
		Field{Name: "Format", Value: format},
		Field{Name: "File size", Value: formatBytes(stat.Size())},
	)

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
			if info.DurationSeconds > 0 {
				overall := (float64(stat.Size()) * 8) / info.DurationSeconds
				general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
			}
			if headerSize, dataSize, footerSize, mdatCount, moovBeforeMdat, ok := mp4TopLevelSizes(file, stat.Size()); ok {
				general.JSON["HeaderSize"] = fmt.Sprintf("%d", headerSize)
				general.JSON["DataSize"] = fmt.Sprintf("%d", dataSize)
				general.JSON["FooterSize"] = fmt.Sprintf("%d", footerSize)
				streamSize := stat.Size() - (dataSize - int64(mdatCount*8))
				if streamSize > 0 {
					general.JSON["StreamSize"] = fmt.Sprintf("%d", streamSize)
				}
				if moovBeforeMdat {
					general.JSON["IsStreamable"] = "Yes"
				} else {
					general.JSON["IsStreamable"] = "No"
				}
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
					fields = appendFieldUnique(fields, Field{Name: "ID", Value: fmt.Sprintf("%d", track.ID)})
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
									fields = appendFieldUnique(fields, Field{Name: "Source_Duration_LastFrame", Value: fmt.Sprintf("%d ms", diffMs)})
								}
							}
						}
					}
					if bitrate > 0 {
						if track.Kind != StreamVideo {
							if mode := bitrateMode(bitrate); mode != "" {
								fields = appendFieldUnique(fields, Field{Name: "Bit rate mode", Value: mode})
							}
						}
						fields = addStreamBitrate(fields, bitrate)
						jsonExtras["BitRate"] = fmt.Sprintf("%d", int64(math.Round(bitrate)))
					}
				}
				if track.SampleBytes > 0 {
					streamBytes := int64(track.SampleBytes)
					displaySamples := 0.0
					if sourceDuration > 0 && displayDuration > 0 {
						if track.SampleDelta > 0 && track.SampleCount > 0 && track.Timescale > 0 {
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
						jsonStreamBytes := streamBytes
						if track.Kind == StreamAudio && sourceDuration > 0 {
							jsonStreamBytes++
						}
						jsonExtras["StreamSize"] = fmt.Sprintf("%d", jsonStreamBytes)
					}
					if sourceDuration > 0 {
						if sourceSize := formatStreamSize(int64(track.SampleBytes), stat.Size()); sourceSize != "" {
							fields = appendFieldUnique(fields, Field{Name: "Source stream size", Value: sourceSize})
						}
						jsonExtras["Source_StreamSize"] = fmt.Sprintf("%d", int64(track.SampleBytes))
						if track.Kind == StreamAudio {
							if displaySamples > 0 {
								jsonExtras["FrameCount"] = fmt.Sprintf("%d", int64(math.Round(displaySamples)))
							}
							if track.SampleCount > 0 {
								jsonExtras["Source_FrameCount"] = fmt.Sprintf("%d", track.SampleCount)
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
					jsonExtras["FrameCount"] = fmt.Sprintf("%d", track.SampleCount)
					jsonExtras["FrameRate_Mode_Original"] = "VFR"
					if generalFrameCount == "" {
						generalFrameCount = fmt.Sprintf("%d", track.SampleCount)
					}
				}
				if track.Default && track.Kind != StreamVideo {
					fields = appendFieldUnique(fields, Field{Name: "Default", Value: "Yes"})
				}
				if track.AlternateGroup > 0 {
					fields = appendFieldUnique(fields, Field{Name: "Alternate group", Value: fmt.Sprintf("%d", track.AlternateGroup)})
				}
				if track.Kind == StreamAudio && track.EditMediaTime > 0 && track.Timescale > 0 {
					delayMs := int64(math.Round(float64(track.EditMediaTime) * 1000 / float64(track.Timescale)))
					if delayMs != 0 {
						jsonRaw["extra"] = fmt.Sprintf("{\"Source_Delay\":\"-%d\",\"Source_Delay_Source\":\"Container\"}", delayMs)
					}
				}
				streams = append(streams, Stream{Kind: track.Kind, Fields: fields, JSON: jsonExtras, JSONRaw: jsonRaw})
			}
			if generalFrameCount != "" {
				general.JSON["FrameCount"] = generalFrameCount
			}
			if _, err := file.Seek(0, io.SeekStart); err == nil {
				sniff := make([]byte, 1<<20)
				n, _ := io.ReadFull(file, sniff)
				writingLib, encoding := findX264Info(sniff[:n])
				if writingLib != "" || encoding != "" {
					for i := range streams {
						if streams[i].Kind != StreamVideo || findField(streams[i].Fields, "Format") != "AVC" {
							continue
						}
						if writingLib != "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: writingLib})
						}
						if encoding != "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: encoding})
						}
						break
					}
				}
			}
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
			if rawWritingApp != "" {
				general.JSON["Encoded_Application"] = rawWritingApp
				if name, version, rawVersion := splitWritingApplication(rawWritingApp); name != "" && rawVersion != "" {
					general.JSON["Encoded_Application_Name"] = name
					if version != "" {
						general.JSON["Encoded_Application_Version"] = version
					}
				}
			}
			if info.DurationSeconds > 0 {
				general.JSON["Duration"] = formatJSONFloat(info.DurationSeconds)
			}
			if info.DurationSeconds > 0 {
				overall := (float64(stat.Size()) * 8) / info.DurationSeconds
				general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
			}
			general.JSON["IsStreamable"] = "Yes"
			var streamSizeSum int64
			for _, stream := range streams {
				if stream.JSON != nil {
					if value, ok := stream.JSON["StreamSize"]; ok {
						if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
							streamSizeSum += parsed
						}
					}
				} else if sizeValue := findField(stream.Fields, "Stream size"); sizeValue != "" {
					if parsed, ok := parseSizeBytes(sizeValue); ok {
						streamSizeSum += parsed
					}
				}
			}
			if streamSizeSum > 0 {
				remaining := stat.Size() - streamSizeSum
				if remaining >= 0 {
					general.JSON["StreamSize"] = fmt.Sprintf("%d", remaining)
				}
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
						general.JSON["FrameCount"] = fmt.Sprintf("%d", int(math.Round(duration*fps)))
					}
				}
				break
			}
			if _, err := file.Seek(0, io.SeekStart); err == nil {
				sniff := make([]byte, 1<<20)
				n, _ := io.ReadFull(file, sniff)
				writingLib, encoding := findX264Info(sniff[:n])
				if writingLib != "" || encoding != "" {
					for i := range streams {
						if streams[i].Kind != StreamVideo || findField(streams[i].Fields, "Format") != "AVC" {
							continue
						}
						if writingLib != "" && findField(streams[i].Fields, "Writing library") == "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: writingLib})
						}
						if encoding != "" && findField(streams[i].Fields, "Encoding settings") == "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: encoding})
						}
						if encoding != "" && findField(streams[i].Fields, "Nominal bit rate") == "" {
							if bitrate, ok := findX264Bitrate(encoding); ok {
								streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
								width, _ := parsePixels(findField(streams[i].Fields, "Width"))
								height, _ := parsePixels(findField(streams[i].Fields, "Height"))
								fps, _ := parseFPS(findField(streams[i].Fields, "Frame rate"))
								if bits := formatBitsPerPixelFrame(bitrate, width, height, fps); bits != "" {
									streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
								}
							}
						}
						break
					}
				}
			}
		}
	case "MPEG-TS":
		if parsedInfo, parsedStreams, generalFields, ok := ParseMPEGTS(file, stat.Size()); ok {
			info = parsedInfo
			general.JSON = map[string]string{}
			general.JSONRaw = map[string]string{}
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
				overall := (float64(stat.Size()) * 8) / info.DurationSeconds
				general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
			}
			if info.OverallBitrateMin > 0 && info.OverallBitrateMax > 0 {
				min := int64(math.Round(info.OverallBitrateMin))
				max := int64(math.Round(info.OverallBitrateMax))
				general.JSONRaw["extra"] = fmt.Sprintf("{\"OverallBitRate_Precision_Min\":\"%d\",\"OverallBitRate_Precision_Max\":\"%d\"}", min, max)
			}
			if _, err := file.Seek(0, io.SeekStart); err == nil {
				sniff := make([]byte, 1<<20)
				n, _ := io.ReadFull(file, sniff)
				writingLib, encoding := findX264Info(sniff[:n])
				if writingLib != "" || encoding != "" {
					for i := range streams {
						if streams[i].Kind != StreamVideo || findField(streams[i].Fields, "Format") != "AVC" {
							continue
						}
						if writingLib != "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Writing library", Value: writingLib})
						}
						if encoding != "" {
							streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Encoding settings", Value: encoding})
							if findField(streams[i].Fields, "Nominal bit rate") == "" {
								if bitrate, ok := findX264Bitrate(encoding); ok {
									streams[i].Fields = appendFieldUnique(streams[i].Fields, Field{Name: "Nominal bit rate", Value: formatBitrate(bitrate)})
								}
							}
						}
						break
					}
				}
			}
		}
	case "MPEG-PS":
		if parsedInfo, parsedStreams, ok := ParseMPEGPS(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
			general.JSON = map[string]string{}
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				if jsonDuration > 0 {
					overall := (float64(stat.Size()) * 8) / jsonDuration
					general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
				}
			}
			var frameCount string
			var streamSizeSum int64
			hasAudio := false
			for _, stream := range streams {
				if stream.Kind == StreamAudio {
					hasAudio = true
					break
				}
			}
			audioIndex := 0
			for i := range streams {
				if streams[i].Kind == StreamMenu {
					streams[i].JSONSkipStreamOrder = true
					continue
				}
				if streams[i].Kind == StreamAudio {
					streams[i].JSON["StreamOrder"] = fmt.Sprintf("%d", audioIndex)
					audioIndex++
				}
				if streams[i].Kind == StreamVideo {
					if findField(streams[i].Fields, "Format") != "" {
						duration, durOk := parseDurationSeconds(findField(streams[i].Fields, "Duration"))
						fps, fpsOk := parseFPS(findField(streams[i].Fields, "Frame rate"))
						if durOk && fpsOk {
							frameCount = fmt.Sprintf("%d", int(math.Round(duration*fps)))
						}
					}
					if hasAudio {
						streams[i].JSONSkipStreamOrder = true
					}
				}
				if streams[i].JSON != nil {
					if value, ok := streams[i].JSON["StreamSize"]; ok {
						if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
							streamSizeSum += parsed
						}
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			if streamSizeSum > 0 {
				remaining := stat.Size() - streamSizeSum
				if remaining >= 0 {
					general.JSON["StreamSize"] = fmt.Sprintf("%d", remaining)
				}
			}
		}
	case "MPEG Audio":
		if parsedInfo, parsedStreams, ok := ParseMP3(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
		}
	case "FLAC":
		if parsedInfo, parsedStreams, ok := ParseFLAC(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
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
				if jsonDuration > 0 {
					overall := (float64(stat.Size()) * 8) / jsonDuration
					general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
				}
			}
			var frameCount string
			var streamSizeSum int64
			for i := range streams {
				streams[i].JSONSkipStreamOrder = true
				if streams[i].Kind == StreamVideo {
					duration, durOk := parseDurationSeconds(findField(streams[i].Fields, "Duration"))
					fps, fpsOk := parseFPS(findField(streams[i].Fields, "Frame rate"))
					if durOk && fpsOk {
						frameCount = fmt.Sprintf("%d", int(math.Round(duration*fps)))
					}
				}
				if streams[i].JSON != nil {
					if value, ok := streams[i].JSON["StreamSize"]; ok {
						if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
							streamSizeSum += parsed
						}
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			if streamSizeSum > 0 {
				remaining := stat.Size() - streamSizeSum
				if remaining >= 0 {
					general.JSON["StreamSize"] = fmt.Sprintf("%d", remaining)
				}
			}
			general.JSONRaw = map[string]string{
				"extra": "{\"FileExtension_Invalid\":\"mpgv mpv mp1v m1v mp2v m2v\",\"ConformanceWarnings\":[{\"GeneralCompliance\":\"File name extension is not expected for this file format (actual mpg, expected mpgv mpv mp1v m1v mp2v m2v)\"}]}",
			}
		}
	case "AVI":
		if parsedInfo, parsedStreams, generalFields, ok := ParseAVI(file, stat.Size()); ok {
			info = parsedInfo
			general.JSON = map[string]string{}
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
			if info.DurationSeconds > 0 {
				jsonDuration := math.Round(info.DurationSeconds*1000) / 1000
				if jsonDuration > 0 {
					overall := (float64(stat.Size()) * 8) / jsonDuration
					general.JSON["OverallBitRate"] = fmt.Sprintf("%d", int64(math.Round(overall)))
				}
			}
			var frameCount string
			var streamSizeSum int64
			for _, stream := range streams {
				if stream.Kind == StreamVideo {
					duration, durOk := parseDurationSeconds(findField(stream.Fields, "Duration"))
					fps, fpsOk := parseFPS(findField(stream.Fields, "Frame rate"))
					if durOk && fpsOk {
						frameCount = fmt.Sprintf("%d", int(math.Round(duration*fps)))
					}
				}
				if stream.JSON != nil {
					if value, ok := stream.JSON["StreamSize"]; ok {
						if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
							streamSizeSum += parsed
						}
					}
				}
			}
			if frameCount != "" {
				general.JSON["FrameCount"] = frameCount
			}
			if streamSizeSum > 0 {
				remaining := stat.Size() - streamSizeSum
				if remaining >= 0 {
					general.JSON["StreamSize"] = fmt.Sprintf("%d", remaining)
				}
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
					general.Fields = appendFieldUnique(general.Fields, Field{Name: "Frame rate", Value: fmt.Sprintf("%s FPS", parts[0])})
					break
				}
			}
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Frame rate", Value: rate})
			break
		}
	}

	if info.HasDuration() {
		general.Fields = append(general.Fields, Field{Name: "Duration", Value: formatDuration(info.DurationSeconds)})
		bitrate := float64(stat.Size()*8) / info.DurationSeconds
		if bitrate > 0 {
			mode := info.BitrateMode
			if mode != "" && format != "Matroska" && format != "AVI" {
				general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: mode})
			}
			if mode == "" && format != "Matroska" && format != "AVI" {
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
	reports := make([]Report, 0, len(paths))
	for _, path := range paths {
		report, err := AnalyzeFileWithOptions(path, opts)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path, err)
		}
		reports = append(reports, report)
	}
	return reports, len(reports), nil
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
	return parsed, true
}
