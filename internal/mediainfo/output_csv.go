package mediainfo

import (
	"bytes"
	"fmt"
)

func RenderCSV(reports []Report) string {
	var buf bytes.Buffer
	for _, report := range reports {
		writeCSVTrack(&buf, string(report.General.Kind), report.General)
		forEachStreamWithKindIndex(report.Streams, func(stream Stream, index, total, _ int) {
			title := csvStreamTitle(stream.Kind, index, total)
			writeCSVTrack(&buf, title, stream)
		})
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
	buf.WriteString(trackType)
	buf.WriteString("\n")
	for _, field := range stream.Fields {
		buf.WriteString(field.Name)
		buf.WriteString(",")
		buf.WriteString(field.Value)
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}
