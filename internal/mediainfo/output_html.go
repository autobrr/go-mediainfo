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
		buf.WriteString(renderHTMLStream(report.General))
		for _, stream := range orderTracks(report.Streams) {
			buf.WriteString(renderHTMLStream(stream))
		}
		buf.WriteString("</table>")
	}
	buf.WriteString("</body></html>")
	return buf.String()
}

func renderHTMLStream(stream Stream) string {
	fields := orderFieldsForJSON(stream.Kind, stream.Fields)
	var buf bytes.Buffer
	buf.WriteString("<tr><th colspan=\"2\">")
	buf.WriteString(html.EscapeString(string(stream.Kind)))
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
