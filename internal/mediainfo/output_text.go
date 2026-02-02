package mediainfo

import (
	"bytes"
	"fmt"
	"strings"
)

func RenderText(reports []Report) string {
	var buf bytes.Buffer
	for i, report := range reports {
		if i > 0 {
			buf.WriteString("\n")
		}
		writeStream(&buf, string(report.General.Kind), report.General)
		kindCounts := map[StreamKind]int{}
		for _, stream := range report.Streams {
			kindCounts[stream.Kind]++
		}
		kindIndex := map[StreamKind]int{}
		for _, stream := range report.Streams {
			buf.WriteString("\n")
			kindIndex[stream.Kind]++
			title := streamTitle(stream.Kind, kindIndex[stream.Kind], kindCounts[stream.Kind])
			writeStream(&buf, title, stream)
		}
	}
	return strings.TrimRight(buf.String(), "\n")
}

func writeStream(buf *bytes.Buffer, title string, stream Stream) {
	buf.WriteString(title)
	buf.WriteString("\n")
	for _, field := range stream.Fields {
		buf.WriteString(padRight(field.Name, 36))
		buf.WriteString(": ")
		buf.WriteString(field.Value)
		buf.WriteString("\n")
	}
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func streamTitle(kind StreamKind, index, total int) string {
	if total <= 1 || kind == StreamGeneral {
		return string(kind)
	}
	return fmt.Sprintf("%s #%d", kind, index)
}
