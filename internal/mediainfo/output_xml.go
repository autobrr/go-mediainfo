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
	var buf bytes.Buffer
	buf.WriteString("<media")
	if report.Ref != "" {
		buf.WriteString(fmt.Sprintf(" ref=\"%s\"", xmlEscapeAttr(report.Ref)))
	}
	buf.WriteString(">\n")

	buf.WriteString(renderXMLTrack("General", 0, buildJSONGeneralFields(report)))

	containerFormat := findField(report.General.Fields, "Format")
	sorted := orderTracks(report.Streams)
	forEachStreamWithKindIndex(sorted, func(stream Stream, index, total, order int) {
		typeOrder := 0
		if total > 1 {
			typeOrder = index
		}
		// typeorder is rendered as a track attribute in XML, so omit @typeorder from fields.
		fields := buildJSONStreamFields(stream, order, 0, containerFormat)
		buf.WriteString(renderXMLTrack(string(stream.Kind), typeOrder, fields))
	})

	buf.WriteString("</media>\n")
	return buf.String()
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
