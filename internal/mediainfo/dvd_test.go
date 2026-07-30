package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeVTSIFOUsesIFOProgramMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	ifoData := make([]byte, 0x0300)
	copy(ifoData[:12], []byte("DVDVIDEO-VTS"))
	// Version 2, NTSC, 16:9 at 720x480.
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	ifoData[dvdVideoAttrVTSOffset+1] = 0x00
	// One AC-3 stereo audio stream, language "en".
	ifoData[dvdAudioCountVTSOffset] = 0x00
	ifoData[dvdAudioCountVTSOffset+1] = 0x01
	ifoData[dvdAudioAttrVTSOffset] = 0x00
	ifoData[dvdAudioAttrVTSOffset+1] = 0x01
	ifoData[dvdAudioAttrVTSOffset+2] = 'e'
	ifoData[dvdAudioAttrVTSOffset+3] = 'n'
	if err := os.WriteFile(ifoPath, ifoData, 0o644); err != nil { //nolint:gosec // test fixture file
		t.Fatalf("write ifo: %v", err)
	}

	// Sibling VOB should not be aggregated for VTS IFO reporting.
	vobPath := filepath.Join(root, "VTS_01_1.VOB")
	if err := os.WriteFile(vobPath, make([]byte, 2<<20), 0o644); err != nil { //nolint:gosec // test fixture file
		t.Fatalf("write vob: %v", err)
	}

	report, err := AnalyzeFile(ifoPath)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}

	if got := findField(report.General.Fields, "Format profile"); got != "Program" {
		t.Fatalf("Format profile = %q, want Program", got)
	}
	if got := findField(report.General.Fields, "File size"); got != formatBytes(int64(len(ifoData))) {
		t.Fatalf("File size = %q, want %q", got, formatBytes(int64(len(ifoData))))
	}

	var audio *Stream
	for i := range report.Streams {
		if report.Streams[i].Kind == StreamAudio {
			audio = &report.Streams[i]
			break
		}
	}
	if audio == nil {
		t.Fatalf("missing audio stream")
	}
	if got := findField(audio.Fields, "ID"); got != "128 (0x80)" {
		t.Fatalf("audio ID = %q, want 128 (0x80)", got)
	}

	for _, stream := range report.Streams {
		if got := findField(stream.Fields, "Source"); got != "" {
			t.Fatalf("unexpected Source field in %s stream: %q", stream.Kind, got)
		}
		if len(stream.canonicalSeed) == 0 {
			t.Fatalf("%s stream has no canonical seed", stream.Kind)
		}
		for _, entry := range stream.canonicalSeed {
			key := firstNonEmpty(entry.StructuredKey, string(entry.Name))
			if key == "@type" || key == "@typeorder" || key == "StreamOrder" {
				continue
			}
			if entry.Projected {
				t.Fatalf("%s field %q came from the legacy projection", stream.Kind, entry.Name)
			}
		}
	}
}
