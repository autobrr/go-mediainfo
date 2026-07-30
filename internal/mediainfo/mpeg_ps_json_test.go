package mediainfo

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestMpegPsGOPHeaderJSONBitRate(t *testing.T) {
	path := filepath.Join("samples", "sample_ac3.vob")
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
	// Sample generated in samples/generate.sh (ffmpeg); fixed fixture, expect stable bitrate.
	if bitrate != "2216694" {
		t.Fatalf("unexpected video bitrate: %s", bitrate)
	}
}

func TestBuildMPEGPSCanonicalSnapshotProjectsCompatibilityState(t *testing.T) {
	facts := &mpegPSStructuredFacts{}
	facts.Set("Duration", "1.250")
	facts.Set("BitRate", "1000")
	facts.Set("BitRate", "2000")
	extra := structuredNode{Kind: structuredObject, Object: []structuredMember{{
		Key:   "proof",
		Value: structuredNode{Kind: structuredString, Text: "yes"},
	}}}
	builder := newCanonicalStreamBuilder(StreamVideo)
	builder.Fill("Format", "MPEG Video", "Format", "MPEG Video")
	stream := buildMPEGPSCanonicalSnapshot(
		builder,
		[]Field{{Name: "Format", Value: "MPEG Video"}, {Name: "Duration", Value: "1 s 250 ms"}},
		facts,
		&extra,
		canonicalStreamPolicy{SkipStreamOrder: true},
	)

	if value, found := canonicalSeedValue(stream, "Duration"); !found || value != "1250" {
		t.Fatalf("canonical duration = %q, found = %v", value, found)
	}
	if value, found := canonicalSeedValue(stream, "BitRate"); !found || value != "2000" {
		t.Fatalf("canonical bitrate = %q, found = %v", value, found)
	}
	if stream.JSON["Duration"] != "1.250" || stream.JSON["BitRate"] != "2000" {
		t.Fatalf("legacy JSON = %#v", stream.JSON)
	}
	if stream.JSONRaw["extra"] != `{"proof":"yes"}` {
		t.Fatalf("legacy raw extra = %q", stream.JSONRaw["extra"])
	}
	if !stream.JSONSkipStreamOrder || !stream.canonicalPolicy.SkipStreamOrder {
		t.Fatalf("projection policy was not preserved: %#v", stream.canonicalPolicy)
	}
}
