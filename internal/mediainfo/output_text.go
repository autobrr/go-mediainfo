package mediainfo

import (
	"bytes"
	"fmt"
	"strings"
)

// RenderText renders reports in MediaInfo-style aligned text from text projections.
func RenderText(reports []Report) string {
	var buf bytes.Buffer
	for i, report := range reports {
		if i > 0 {
			buf.WriteString("\n")
		}
		projected := projectTextReport(report)
		if len(projected.Streams) == 0 {
			continue
		}
		writeFields(&buf, string(projected.Streams[0].Kind), projected.Streams[0].Fields)
		streams := projected.Streams[1:]
		counts := make(map[StreamKind]int)
		for _, stream := range streams {
			counts[stream.Kind]++
		}
		kindIndex := make(map[StreamKind]int)
		for _, stream := range streams {
			kindIndex[stream.Kind]++
			buf.WriteString("\n")
			title := streamTitle(stream.Kind, kindIndex[stream.Kind], counts[stream.Kind])
			writeFields(&buf, title, stream.Fields)
		}
		buf.WriteString("\n")
		buf.WriteString(reportByLine())
		buf.WriteString("\n")
	}
	output := strings.TrimRight(buf.String(), "\n")
	return output + "\n\n"
}

func reportByLine() string {
	return fmt.Sprintf("ReportBy : %s - %s", AppName, FormatVersion(AppVersion))
}

func writeStream(buf *bytes.Buffer, title string, stream Stream) {
	writeFields(buf, title, stream.Fields)
}

// writeFields appends one text stream section with fixed-width labels.
func writeFields(buf *bytes.Buffer, title string, fields []Field) {
	buf.WriteString(title)
	buf.WriteString("\n")
	for _, field := range fields {
		buf.WriteString(padRight(field.Name, 41))
		buf.WriteString(": ")
		buf.WriteString(escapeOutputControls(field.Value))
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
