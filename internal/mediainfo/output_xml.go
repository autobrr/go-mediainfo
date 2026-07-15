package mediainfo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	mediaInfoXMLNS      = "https://mediaarea.net/mediainfo"
	mediaInfoXMLSchema  = "https://mediaarea.net/mediainfo/mediainfo_2_0.xsd"
	mediaInfoXMLVersion = "2.0"
)

type orderedValueKind int

const (
	orderedString orderedValueKind = iota
	orderedObject
	orderedArray
)

type orderedValue struct {
	kind orderedValueKind
	str  string
	obj  []orderedKV
	arr  []orderedValue
}

type orderedKV struct {
	key string
	val orderedValue
}

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
		if field.Key == "@type" {
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

func renderXMLTrack(trackType string, typeOrder int, fields []jsonKV) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<track type=\"%s\"", xmlEscapeAttr(trackType)))
	if typeOrder > 0 {
		buf.WriteString(fmt.Sprintf(" typeorder=\"%d\"", typeOrder))
	}
	buf.WriteString(">\n")
	for _, field := range fields {
		if field.Key == "@type" {
			continue
		}
		if field.Key == "extra" {
			buf.WriteString(renderXMLExtra(field.Val))
			continue
		}
		buf.WriteString(renderXMLField(field.Key, field.Val))
	}
	buf.WriteString("</track>\n")
	return buf.String()
}

func renderXMLField(key, value string) string {
	name := xmlFieldName(key)
	return fmt.Sprintf("<%s>%s</%s>\n", name, xmlEscape(value), name)
}

func renderXMLExtra(raw string) string {
	value, err := parseOrderedJSON(raw)
	if err != nil || value.kind != orderedObject {
		return renderXMLField("extra", raw)
	}
	var buf bytes.Buffer
	buf.WriteString("<extra>\n")
	for _, kv := range value.obj {
		buf.WriteString(renderOrderedXML(kv.key, kv.val))
	}
	buf.WriteString("</extra>\n")
	return buf.String()
}

func renderOrderedXML(key string, value orderedValue) string {
	name := xmlFieldName(key)
	switch value.kind {
	case orderedString:
		return fmt.Sprintf("<%s>%s</%s>\n", name, xmlEscape(value.str), name)
	case orderedObject:
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("<%s>\n", name))
		for _, kv := range value.obj {
			buf.WriteString(renderOrderedXML(kv.key, kv.val))
		}
		buf.WriteString(fmt.Sprintf("</%s>\n", name))
		return buf.String()
	case orderedArray:
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("<%s>\n", name))
		for _, item := range value.arr {
			switch item.kind {
			case orderedObject:
				for _, kv := range item.obj {
					buf.WriteString(renderOrderedXML(kv.key, kv.val))
				}
			case orderedString:
				buf.WriteString(xmlEscape(item.str))
			case orderedArray:
			}
		}
		buf.WriteString(fmt.Sprintf("</%s>\n", name))
		return buf.String()
	default:
		return fmt.Sprintf("<%s>%s</%s>\n", name, xmlEscape(value.str), name)
	}
}

func parseOrderedJSON(value string) (orderedValue, error) {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.UseNumber()
	return parseOrderedValue(dec)
}

func parseOrderedValue(dec *json.Decoder) (orderedValue, error) {
	tok, err := dec.Token()
	if err != nil {
		return orderedValue{}, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			var kvs []orderedKV
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return orderedValue{}, err
				}
				key, _ := keyTok.(string)
				val, err := parseOrderedValue(dec)
				if err != nil {
					return orderedValue{}, err
				}
				kvs = append(kvs, orderedKV{key: key, val: val})
			}
			if _, err := dec.Token(); err != nil {
				return orderedValue{}, err
			}
			return orderedValue{kind: orderedObject, obj: kvs}, nil
		case '[':
			var arr []orderedValue
			for dec.More() {
				val, err := parseOrderedValue(dec)
				if err != nil {
					return orderedValue{}, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil {
				return orderedValue{}, err
			}
			return orderedValue{kind: orderedArray, arr: arr}, nil
		}
	case string:
		return orderedValue{kind: orderedString, str: t}, nil
	case json.Number:
		return orderedValue{kind: orderedString, str: t.String()}, nil
	case bool:
		if t {
			return orderedValue{kind: orderedString, str: "true"}, nil
		}
		return orderedValue{kind: orderedString, str: "false"}, nil
	case nil:
		return orderedValue{kind: orderedString, str: ""}, nil
	}
	return orderedValue{kind: orderedString, str: fmt.Sprint(tok)}, nil
}

func xmlEscape(value string) string {
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
