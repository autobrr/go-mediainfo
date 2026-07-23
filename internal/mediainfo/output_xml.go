package mediainfo

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	mediaInfoXMLNS      = "https://mediaarea.net/mediainfo"
	mediaInfoXMLSchema  = "https://mediaarea.net/mediainfo/mediainfo_2_0.xsd"
	mediaInfoXMLVersion = "2.0"
)

// RenderXML renders reports as MediaInfo XML from the shared structured projection.
func RenderXML(reports []Report) string {
	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	buf.WriteString("<MediaInfo\n")
	buf.WriteString(fmt.Sprintf("    xmlns=\"%s\"\n", mediaInfoXMLNS))
	buf.WriteString("    xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"\n")
	buf.WriteString(fmt.Sprintf("    xsi:schemaLocation=\"%s %s\"\n", mediaInfoXMLNS, mediaInfoXMLSchema))
	buf.WriteString(fmt.Sprintf("    version=\"%s\">\n", mediaInfoXMLVersion))
	buf.WriteString(fmt.Sprintf("<creatingLibrary version=\"%s\" url=\"%s\">%s</creatingLibrary>\n", FormatVersion(AppVersion), AppURL, AppName))

	for _, report := range reports {
		buf.WriteString(renderXMLMedia(report))
	}
	buf.WriteString("</MediaInfo>\n")
	return buf.String()
}

func renderXMLMedia(report Report) string {
	projected := projectStructuredReportFor(report, structuredProjectionXML)
	var buf bytes.Buffer
	buf.WriteString("<media")
	if projected.Ref != "" {
		fmt.Fprintf(&buf, " ref=\"%s\"", xmlEscapeAttr(projected.Ref))
	}
	buf.WriteString(">\n")
	for _, stream := range projected.Streams {
		buf.WriteString(renderXMLStructuredTrack(string(stream.Kind), stream.TypeOrder, stream.Fields))
	}

	buf.WriteString("</media>\n")
	return buf.String()
}

// renderXMLStructuredTrack renders one structured stream, preserving projected field order.
func renderXMLStructuredTrack(trackType string, typeOrder int, fields []structuredField) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<track type=\"%s\"", xmlEscapeAttr(trackType))
	if typeOrder > 0 {
		fmt.Fprintf(&buf, " typeorder=\"%d\"", typeOrder)
	}
	buf.WriteString(">\n")
	for _, field := range fields {
		if field.Key == "@type" || field.Key == "@typeorder" {
			continue
		}
		if field.Key == "extra" {
			buf.WriteString(renderXMLStructuredExtra(field.Value))
			continue
		}
		buf.WriteString(renderXMLField(field.Key, structuredNodeText(field.Value)))
	}
	buf.WriteString("</track>\n")
	return buf.String()
}

// renderXMLStructuredExtra expands an object-valued extra field into XML elements.
func renderXMLStructuredExtra(value structuredNode) string {
	if value.Kind != structuredObject {
		return renderXMLField("extra", structuredNodeText(value))
	}
	var buf bytes.Buffer
	buf.WriteString("<extra>\n")
	for _, member := range value.Object {
		buf.WriteString(renderStructuredXML(member.Key, member.Value))
	}
	buf.WriteString("</extra>\n")
	return buf.String()
}

// renderStructuredXML recursively renders a structured node beneath key.
func renderStructuredXML(key string, value structuredNode) string {
	name := xmlFieldName(key)
	switch value.Kind {
	case structuredObject:
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "<%s>\n", name)
		for _, member := range value.Object {
			buf.WriteString(renderStructuredXML(member.Key, member.Value))
		}
		fmt.Fprintf(&buf, "</%s>\n", name)
		return buf.String()
	case structuredArray:
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "<%s>\n", name)
		for _, item := range value.Array {
			switch item.Kind {
			case structuredObject:
				for _, member := range item.Object {
					buf.WriteString(renderStructuredXML(member.Key, member.Value))
				}
			case structuredArray:
			case structuredString, structuredNumber, structuredBool, structuredNull, structuredRaw:
				buf.WriteString(xmlEscape(structuredNodeText(item)))
			}
		}
		fmt.Fprintf(&buf, "</%s>\n", name)
		return buf.String()
	case structuredString, structuredNumber, structuredBool, structuredNull, structuredRaw:
		return fmt.Sprintf("<%s>%s</%s>\n", name, xmlEscape(structuredNodeText(value)), name)
	}
	return ""
}

func renderXMLField(key, value string) string {
	name := xmlFieldName(key)
	return fmt.Sprintf("<%s>%s</%s>\n", name, xmlEscape(value), name)
}

func xmlEscape(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF {
			return r
		}
		return '\uFFFD'
	}, value)
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	value = strings.ReplaceAll(value, "'", "&apos;")
	return value
}

func xmlEscapeAttr(value string) string {
	return xmlEscape(value)
}
