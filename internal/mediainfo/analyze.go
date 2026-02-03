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
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
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
					}
				}
				if track.SampleBytes > 0 {
					streamBytes := int64(track.SampleBytes)
					if sourceDuration > 0 && displayDuration > 0 {
						if track.SampleDelta > 0 && track.SampleCount > 0 && track.Timescale > 0 {
							displaySamples := (displayDuration * float64(track.Timescale)) / float64(track.SampleDelta)
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
					if sourceDuration > 0 {
						if sourceSize := formatStreamSize(int64(track.SampleBytes), stat.Size()); sourceSize != "" {
							fields = appendFieldUnique(fields, Field{Name: "Source stream size", Value: sourceSize})
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
				}
				if track.Default && track.Kind != StreamVideo {
					fields = appendFieldUnique(fields, Field{Name: "Default", Value: "Yes"})
				}
				if track.AlternateGroup > 0 {
					fields = appendFieldUnique(fields, Field{Name: "Alternate group", Value: fmt.Sprintf("%d", track.AlternateGroup)})
				}
				streams = append(streams, Stream{Kind: track.Kind, Fields: fields})
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
		if parsed, ok := ParseMatroska(file, stat.Size()); ok {
			info = parsed.Container
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Tracks...)
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
			for _, field := range generalFields {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = parsedStreams
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
	}

	for _, stream := range streams {
		if stream.Kind != StreamVideo {
			continue
		}
		if rate := findField(stream.Fields, "Frame rate"); rate != "" {
			general.Fields = appendFieldUnique(general.Fields, Field{Name: "Frame rate", Value: rate})
			break
		}
	}

	if info.HasDuration() {
		general.Fields = append(general.Fields, Field{Name: "Duration", Value: formatDuration(info.DurationSeconds)})
		bitrate := float64(stat.Size()*8) / info.DurationSeconds
		if bitrate > 0 {
			mode := info.BitrateMode
			if mode != "" && format != "Matroska" {
				general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: mode})
			}
			if mode == "" && format != "Matroska" {
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
	reports := make([]Report, 0, len(paths))
	for _, path := range paths {
		report, err := AnalyzeFile(path)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path, err)
		}
		reports = append(reports, report)
	}
	return reports, len(reports), nil
}

func parsePixels(value string) (uint64, bool) {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return 0, false
	}
	parsed, err := strconv.ParseUint(parts[0], 10, 64)
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
