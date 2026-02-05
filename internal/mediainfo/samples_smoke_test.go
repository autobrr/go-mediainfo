package mediainfo

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSamplesSmoke(t *testing.T) {
	cases := []string{
		"sample.mp4",
		"sample.mkv",
		"sample.ts",
		"sample.avi",
		"sample.mpg",
		"sample.vob",
		"sample_ac3.vob",
		"sample.mp3",
		"sample.flac",
		"sample.wav",
		"sample.ogg",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "samples", name)
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatalf("analyze sample: %v", err)
			}

			if out := RenderText([]Report{report}); out == "" {
				t.Fatalf("empty text output")
			}

			jsonOut := RenderJSON([]Report{report})
			var root any
			if err := json.Unmarshal([]byte(jsonOut), &root); err != nil {
				t.Fatalf("parse json output: %v", err)
			}

			if out := RenderXML([]Report{report}); out == "" {
				t.Fatalf("empty xml output")
			}
			if out := RenderHTML([]Report{report}); out == "" {
				t.Fatalf("empty html output")
			}
			if out := RenderCSV([]Report{report}); out == "" {
				t.Fatalf("empty csv output")
			}
		})
	}
}
