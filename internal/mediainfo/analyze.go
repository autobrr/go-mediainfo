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
			for _, track := range parsed.Tracks {
				streams = append(streams, Stream{
					Kind:   track.Kind,
					Fields: []Field{{Name: "Format", Value: track.Format}},
				})
			}
		}
	case "Matroska":
		if parsed, ok := ParseMatroska(file, stat.Size()); ok {
			info = parsed.Container
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
	}

	if info.HasDuration() {
		general.Fields = append(general.Fields, Field{Name: "Duration", Value: formatDuration(info.DurationSeconds)})
		bitrate := float64(stat.Size()*8) / info.DurationSeconds
		if bitrate > 0 {
			general.Fields = append(general.Fields, Field{Name: "Overall bit rate", Value: formatBitrate(bitrate)})
		}
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
