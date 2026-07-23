package mediainfo

import (
	"bytes"
	"fmt"
)

// RenderCSV renders reports as MediaInfo-style comma-separated field sections.
func RenderCSV(reports []Report) string {
	var buf bytes.Buffer
	for _, report := range reports {
		projected := projectTextReport(report)
		if len(projected.Streams) == 0 {
			continue
		}
		writeCSVFields(&buf, string(projected.Streams[0].Kind), projected.Streams[0].Fields)
		streams := projected.Streams[1:]
		counts := make(map[StreamKind]int)
		for _, stream := range streams {
			counts[stream.Kind]++
		}
		kindIndex := make(map[StreamKind]int)
		for _, stream := range streams {
			kindIndex[stream.Kind]++
			title := csvStreamTitle(stream.Kind, kindIndex[stream.Kind], counts[stream.Kind])
			writeCSVFields(&buf, title, stream.Fields)
		}
	}
	return buf.String()
}

func csvStreamTitle(kind StreamKind, index int, total int) string {
	if total > 1 {
		return fmt.Sprintf("%s,%d", kind, index)
	}
	return string(kind)
}

func writeCSVTrack(buf *bytes.Buffer, trackType string, stream Stream) {
	writeCSVFields(buf, trackType, stream.Fields)
}

// writeCSVFields appends one field section using CSV-safe values.
func writeCSVFields(buf *bytes.Buffer, trackType string, fields []Field) {
	buf.WriteString(trackType)
	buf.WriteString("\n")
	for _, field := range fields {
		buf.WriteString(safeCSVOutputValue(field.Name))
		buf.WriteString(",")
		buf.WriteString(safeCSVOutputValue(field.Value))
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}
