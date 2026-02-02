package mediainfo

import (
	"bytes"
	"encoding/xml"
)

type xmlMedia struct {
	XMLName xml.Name    `xml:"MediaInfo"`
	Media   []xmlReport `xml:"media"`
}

type xmlReport struct {
	Ref   string     `xml:"ref,attr,omitempty"`
	Track []xmlTrack `xml:"track"`
}

type xmlTrack struct {
	Type   string     `xml:"type,attr"`
	Fields []xmlField `xml:",any"`
}

type xmlField struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func RenderXML(reports []Report) string {
	media := xmlMedia{}
	for _, report := range reports {
		media.Media = append(media.Media, buildXMLReport(report))
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	_ = enc.Encode(media)
	return buf.String()
}

func buildXMLReport(report Report) xmlReport {
	tracks := make([]xmlTrack, 0, len(report.Streams)+1)
	tracks = append(tracks, buildXMLTrack(report.General))
	for _, stream := range orderTracks(report.Streams) {
		tracks = append(tracks, buildXMLTrack(stream))
	}
	return xmlReport{Ref: report.Ref, Track: tracks}
}

func buildXMLTrack(stream Stream) xmlTrack {
	fields := orderFieldsForJSON(stream.Kind, stream.Fields)
	xmlFields := make([]xmlField, 0, len(fields))
	for _, field := range fields {
		xmlFields = append(xmlFields, xmlField{XMLName: xml.Name{Local: field.Name}, Value: field.Value})
	}
	return xmlTrack{Type: string(stream.Kind), Fields: xmlFields}
}
