package mediainfo

import (
	"bytes"
	"html"
)

// RenderHTML renders reports as an escaped HTML table using the text projection.
func RenderHTML(reports []Report) string {
	var buf bytes.Buffer
	buf.WriteString("<html><head><meta charset=\"utf-8\"/></head><body>")
	for _, report := range reports {
		projected := projectTextReport(report)
		buf.WriteString("<table>")
		if len(projected.Streams) > 0 {
			buf.WriteString(renderHTMLFields("General", projected.Streams[0].Kind, projected.Streams[0].Fields))
		}
		streams := projected.Streams[1:]
		counts := make(map[StreamKind]int)
		for _, stream := range streams {
			counts[stream.Kind]++
		}
		kindIndex := make(map[StreamKind]int)
		for _, stream := range streams {
			kindIndex[stream.Kind]++
			title := streamTitle(stream.Kind, kindIndex[stream.Kind], counts[stream.Kind])
			buf.WriteString(renderHTMLFields(title, stream.Kind, stream.Fields))
		}
		buf.WriteString("</table>")
	}
	buf.WriteString("</body></html>")
	return buf.String()
}

func renderHTMLStream(title string, stream Stream) string {
	return renderHTMLFields(title, stream.Kind, stream.Fields)
}

// renderHTMLFields renders one ordered text-projection stream as table rows.
func renderHTMLFields(title string, kind StreamKind, sourceFields []Field) string {
	fields := orderFieldsForJSON(kind, sourceFields)
	var buf bytes.Buffer
	buf.WriteString("<tr><th colspan=\"2\">")
	buf.WriteString(html.EscapeString(title))
	buf.WriteString("</th></tr>")
	for _, field := range fields {
		buf.WriteString("<tr><td>")
		buf.WriteString(html.EscapeString(field.Name))
		buf.WriteString("</td><td>")
		buf.WriteString(html.EscapeString(field.Value))
		buf.WriteString("</td></tr>")
	}
	return buf.String()
}
