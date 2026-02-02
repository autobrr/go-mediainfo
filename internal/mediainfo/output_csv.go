package mediainfo

import (
	"bytes"
	"strings"
)

func RenderCSV(reports []Report) string {
	var buf bytes.Buffer
	buf.WriteString("ref,track_type,field,value\n")
	for _, report := range reports {
		writeCSVTrack(&buf, report.Ref, "General", report.General)
		for _, entry := range enumerateStreams(report.Streams) {
			writeCSVTrack(&buf, report.Ref, entry.Title, entry.Stream)
		}
	}
	return buf.String()
}

func writeCSVTrack(buf *bytes.Buffer, ref string, trackType string, stream Stream) {
	for _, field := range orderFieldsForJSON(stream.Kind, stream.Fields) {
		buf.WriteString(csvEscape(ref))
		buf.WriteString(",")
		buf.WriteString(csvEscape(trackType))
		buf.WriteString(",")
		buf.WriteString(csvEscape(field.Name))
		buf.WriteString(",")
		buf.WriteString(csvEscape(field.Value))
		buf.WriteString("\n")
	}
}

func csvEscape(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, ",\n\"") {
		return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
	}
	return value
}
