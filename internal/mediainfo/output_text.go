package mediainfo

import (
	"bytes"
	"fmt"
	"strings"
)

const textLabelWidth = 41

// TextRenderOptions configures text-specific projection behavior.
type TextRenderOptions struct {
	// Language selects MediaInfo raw labels and values for "raw". The zero value
	// and other languages select the friendly text projection.
	Language string
}

// RenderText renders reports in MediaInfo-style aligned text from text projections.
func RenderText(reports []Report) string {
	return RenderTextWithOptions(reports, TextRenderOptions{})
}

// RenderTextWithOptions renders reports through the projection selected by
// options. Raw matching is case-insensitive and ignores surrounding spaces.
func RenderTextWithOptions(reports []Report, options TextRenderOptions) string {
	if strings.EqualFold(strings.TrimSpace(options.Language), "raw") {
		return renderRawText(reports)
	}
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
		buf.WriteString(reportByLine(textLabelWidth))
		buf.WriteString("\n")
	}
	output := strings.TrimRight(buf.String(), "\n")
	return output + "\n\n"
}

func reportByLine(labelWidth int) string {
	return fmt.Sprintf("%s: %s - %s", padRight("ReportBy", labelWidth), AppName, FormatVersion(AppVersion))
}

func writeStream(buf *bytes.Buffer, title string, stream Stream) {
	writeFields(buf, title, stream.Fields)
}

// writeFields appends one text stream section with fixed-width labels.
func writeFields(buf *bytes.Buffer, title string, fields []Field) {
	buf.WriteString(title)
	buf.WriteString("\n")
	for _, field := range fields {
		buf.WriteString(padRight(field.Name, textLabelWidth))
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
