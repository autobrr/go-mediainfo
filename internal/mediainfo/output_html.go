package mediainfo

import (
	"bytes"
	"html"
)

func RenderHTML(reports []Report) string {
	var buf bytes.Buffer
	buf.WriteString("<html><head><meta charset=\"utf-8\"/></head><body>")
	for _, report := range reports {
		buf.WriteString("<table>")
		buf.WriteString(renderHTMLStream("General", report.General))
		for _, entry := range enumerateStreams(report.Streams) {
			buf.WriteString(renderHTMLStream(entry.Title, entry.Stream))
		}
		buf.WriteString("</table>")
	}
	buf.WriteString("</body></html>")
	return buf.String()
}

func renderHTMLStream(title string, stream Stream) string {
	fields := orderFieldsForJSON(stream.Kind, stream.Fields)
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
