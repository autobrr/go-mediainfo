package mediainfo

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestMpegPsGOPHeaderJSONBitRate(t *testing.T) {
	path := filepath.Join("..", "..", "samples", "sample_ac3.vob")
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatalf("analyze sample: %v", err)
	}

	output := RenderJSON([]Report{report})
	var root map[string]any
	if err := json.Unmarshal([]byte(output), &root); err != nil {
		t.Fatalf("parse json: %v", err)
	}

	media, ok := root["media"].(map[string]any)
	if !ok {
		t.Fatalf("missing media object")
	}
	tracks, ok := media["track"].([]any)
	if !ok {
		t.Fatalf("missing track list")
	}

	var bitrate string
	for _, item := range tracks {
		track, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if track["@type"] == "Video" {
			if value, ok := track["BitRate"].(string); ok {
				bitrate = value
			}
			break
		}
	}
	if bitrate == "" {
		t.Fatalf("missing video bitrate")
	}
	if bitrate != "51402" {
		t.Fatalf("unexpected video bitrate: %s", bitrate)
	}
}
