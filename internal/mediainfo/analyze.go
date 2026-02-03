package mediainfo

import (
	"fmt"
	"io"
	"os"
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
				if track.ID > 0 {
					fields = appendFieldUnique(fields, Field{Name: "ID", Value: formatID(uint64(track.ID))})
				}
				if track.Format != "" {
					fields = appendFieldUnique(fields, Field{Name: "Format", Value: track.Format})
				}
				for _, field := range track.Fields {
					fields = appendFieldUnique(fields, field)
				}
				if track.DurationSeconds > 0 {
					bits := 0.0
					if track.SampleBytes > 0 {
						bits = (float64(track.SampleBytes) * 8) / track.DurationSeconds
					}
					fields = addStreamCommon(fields, track.DurationSeconds, bits)
				}
				if track.SampleBytes > 0 {
					if streamSize := formatStreamSize(int64(track.SampleBytes), stat.Size()); streamSize != "" {
						fields = appendFieldUnique(fields, Field{Name: "Stream size", Value: streamSize})
					}
				}
				if track.Kind == StreamVideo && track.SampleCount > 0 && track.DurationSeconds > 0 {
					fields = appendFieldUnique(fields, Field{Name: "Frame rate mode", Value: "Constant"})
					rate := float64(track.SampleCount) / track.DurationSeconds
					if rate > 0 {
						fields = appendFieldUnique(fields, Field{Name: "Frame rate", Value: formatFrameRate(rate)})
					}
					if track.Width > 0 && track.Height > 0 && track.SampleBytes > 0 {
						bitrate := (float64(track.SampleBytes) * 8) / track.DurationSeconds
						if bits := formatBitsPerPixelFrame(bitrate, track.Width, track.Height, rate); bits != "" {
							fields = appendFieldUnique(fields, Field{Name: "Bits/(Pixel*Frame)", Value: bits})
						}
					}
				}
				streams = append(streams, Stream{Kind: track.Kind, Fields: fields})
			}
		}
	case "Matroska":
		if parsed, ok := ParseMatroska(file, stat.Size()); ok {
			info = parsed.Container
			for _, field := range parsed.General {
				general.Fields = appendFieldUnique(general.Fields, field)
			}
			streams = append(streams, parsed.Tracks...)
		}
	case "MPEG-TS":
		if parsedInfo, parsedStreams, ok := ParseMPEGTS(file, stat.Size()); ok {
			info = parsedInfo
			streams = parsedStreams
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
			if mode == "" {
				mode = bitrateMode(bitrate)
			}
			if mode != "" {
				general.Fields = append(general.Fields, Field{Name: "Overall bit rate mode", Value: mode})
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
