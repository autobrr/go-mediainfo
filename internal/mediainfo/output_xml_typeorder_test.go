package mediainfo

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestRenderXMLTypeOrderIsAttributeOnly(t *testing.T) {
	report := Report{
		Ref: "multi.mkv",
		General: Stream{Kind: StreamGeneral, Fields: []Field{
			{Name: "Format", Value: "Matroska"},
		}},
		Streams: []Stream{
			{Kind: StreamVideo, Fields: []Field{{Name: "Format", Value: "AVC"}}},
			{Kind: StreamVideo, Fields: []Field{{Name: "Format", Value: "HEVC"}}},
			{Kind: StreamAudio, Fields: []Field{{Name: "Format", Value: "AAC"}}},
			{Kind: StreamAudio, Fields: []Field{{Name: "Format", Value: "AC-3"}}},
		},
	}

	output := RenderXML([]Report{report})
	if strings.Contains(output, "<_typeorder>") {
		t.Fatalf("XML leaked generated type order as a child: %s", output)
	}
	if strings.Count(output, `typeorder="1"`) != 2 || strings.Count(output, `typeorder="2"`) != 2 {
		t.Fatalf("XML type-order attributes = %q", output)
	}
	if err := xml.Unmarshal([]byte(output), new(any)); err != nil {
		t.Fatalf("XML is not well formed: %v\n%s", err, output)
	}
}

func TestRenderXMLSingleStreamOmitsTypeOrder(t *testing.T) {
	report := Report{
		General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Format", Value: "MPEG-4"}}},
		Streams: []Stream{{Kind: StreamAudio, Fields: []Field{{Name: "Format", Value: "AAC"}}}},
	}
	output := RenderXML([]Report{report})
	if strings.Contains(output, "typeorder=") || strings.Contains(output, "<_typeorder>") {
		t.Fatalf("single stream emitted type order: %s", output)
	}
}

func TestRenderXMLSuppressesOnlyTransportAttributes(t *testing.T) {
	fields := []structuredField{
		{Key: "@type", Value: structuredNode{Kind: structuredString, Text: "Audio"}},
		{Key: "@typeorder", Value: structuredNode{Kind: structuredString, Text: "2"}},
		{Key: "@custom", Value: structuredNode{Kind: structuredString, Text: "kept"}},
		{Key: "Format", Value: structuredNode{Kind: structuredString, Text: "AAC"}},
	}
	output := renderXMLStructuredTrack("Audio", 2, fields)
	if strings.Contains(output, "<_type>") || strings.Contains(output, "<_typeorder>") {
		t.Fatalf("XML leaked transport metadata: %s", output)
	}
	if !strings.Contains(output, "<_custom>kept</_custom>") {
		t.Fatalf("XML suppressed a non-transport dynamic @ field: %s", output)
	}
}
