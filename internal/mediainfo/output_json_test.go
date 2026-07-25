package mediainfo

import (
	"encoding/json"
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
	if !strings.Contains(output, "\"creatingLibrary\"") {
		t.Fatalf("missing creating library: %s", output)
	}
	if !strings.Contains(output, "\"@ref\":\"file.mp4\"") {
		t.Fatalf("missing ref: %s", output)
	}
	if !strings.Contains(output, "\"@type\":\"General\"") {
		t.Fatalf("missing general type: %s", output)
	}
	if !strings.Contains(output, "\"@type\":\"Video\"") {
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
	if !strings.HasPrefix(strings.TrimSpace(output), "[") {
		t.Fatalf("expected array")
	}
	if strings.Count(output, "\"@ref\"") < 2 {
		t.Fatalf("expected refs in list")
	}
}

func TestRenderJSONObjectEscapesDynamicKeys(t *testing.T) {
	keys := []string{
		`quote"key`,
		`back\slash`,
		"line\nbreak",
		"Unicode 雪",
		"punctuation.![]{}",
	}
	fields := make([]jsonKV, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, jsonKV{Key: key, Val: key})
	}

	output := renderJSONObject(fields, false)
	var decoded map[string]string
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
	for _, key := range keys {
		if got := decoded[key]; got != key {
			t.Errorf("decoded[%q] = %q, want %q", key, got, key)
		}
	}
}
