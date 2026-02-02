package mediainfo

import (
	"strings"
	"testing"
)

func TestRenderJSONSingle(t *testing.T) {
	report := Report{
		Ref: "file.mp4",
		General: Stream{
			Kind:   StreamGeneral,
			Fields: []Field{{Name: "Format", Value: "MPEG-4"}},
		},
		Streams: []Stream{{Kind: StreamVideo, Fields: []Field{{Name: "Format", Value: "AVC"}}}},
	}

	output := RenderJSON([]Report{report})
	if !strings.Contains(output, "\"@ref\": \"file.mp4\"") {
		t.Fatalf("missing ref: %s", output)
	}
	if !strings.Contains(output, "\"@type\": \"General\"") {
		t.Fatalf("missing general type: %s", output)
	}
	if !strings.Contains(output, "\"@type\": \"Video\"") {
		t.Fatalf("missing video type: %s", output)
	}
}

func TestRenderJSONMultiple(t *testing.T) {
	report := Report{
		Ref: "file.mp4",
		General: Stream{
			Kind:   StreamGeneral,
			Fields: []Field{{Name: "Format", Value: "MPEG-4"}},
		},
	}

	output := RenderJSON([]Report{report, report})
	if strings.Contains(output, "\"@type\": \"Video\"") {
		t.Fatalf("unexpected video type in multi output")
	}
	if strings.Count(output, "\"@ref\"") != 2 {
		t.Fatalf("expected two refs")
	}
}
