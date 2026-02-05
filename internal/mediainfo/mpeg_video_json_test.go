package mediainfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestMpegVideoJSONFields(t *testing.T) {
	path := filepath.Join("samples", "sample.mpg")
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sample: %v", err)
	}
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

	var general map[string]any
	var video map[string]any
	for _, item := range tracks {
		track, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch track["@type"] {
		case "General":
			general = track
		case "Video":
			video = track
		}
	}
	if general == nil || video == nil {
		t.Fatalf("missing general or video track")
	}

	if got, _ := general["StreamSize"].(string); got != "0" {
		t.Fatalf("unexpected general stream size: %v", got)
	}

	genDuration, _ := general["Duration"].(string)
	if genDuration == "" {
		t.Fatalf("missing general duration")
	}
	videoDuration, _ := video["Duration"].(string)
	if videoDuration == "" {
		t.Fatalf("missing video duration")
	}
	if genDuration != videoDuration {
		t.Fatalf("duration mismatch: general=%v video=%v", genDuration, videoDuration)
	}

	streamSizeStr, _ := video["StreamSize"].(string)
	if streamSizeStr == "" {
		t.Fatalf("missing video stream size")
	}
	streamSize, err := strconv.ParseInt(streamSizeStr, 10, 64)
	if err != nil {
		t.Fatalf("parse video stream size: %v", err)
	}
	if streamSize != stat.Size() {
		t.Fatalf("unexpected video stream size: got=%d want=%d", streamSize, stat.Size())
	}

	if got, _ := general["OverallBitRate"].(string); got == "" {
		t.Fatalf("missing overall bitrate")
	} else if got, _ := video["BitRate"].(string); got == "" {
		t.Fatalf("missing video bitrate")
	} else if general["OverallBitRate"] != video["BitRate"] {
		t.Fatalf("bitrate mismatch: general=%v video=%v", general["OverallBitRate"], video["BitRate"])
	}

	if got, _ := video["Delay_Settings"].(string); got != "drop_frame_flag=0 / closed_gop=1 / broken_link=0" {
		t.Fatalf("unexpected delay settings: %v", got)
	}
	if got, _ := video["BufferSize"].(string); got != "6144" {
		t.Fatalf("unexpected buffer size: %v", got)
	}
	if extra, ok := video["extra"].(map[string]any); !ok {
		t.Fatalf("missing extra")
	} else if got, _ := extra["intra_dc_precision"].(string); got != "8" {
		t.Fatalf("unexpected intra_dc_precision: %v", got)
	}
}
