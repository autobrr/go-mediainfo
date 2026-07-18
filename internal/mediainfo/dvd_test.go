package mediainfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeVTSIFOFallsBackToProgramMetadataForInvalidVOB(t *testing.T) {
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

	// An invalid sibling VOB must not replace valid IFO metadata.
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
	if got := findField(audio.Fields, "ID"); got != "189 (0xBD)-128 (0x80)" {
		t.Fatalf("audio ID = %q, want private stream identity", got)
	}
	if got := findField(audio.Fields, "Language"); got != "English" {
		t.Fatalf("audio language = %q, want English", got)
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

func TestAnalyzeVTSIFOUsesBUPOnlyForMissingLanguage(t *testing.T) {
	tests := []struct {
		name            string
		primaryLanguage string
		wantLanguage    string
	}{
		{name: "fallback", wantLanguage: "French"},
		{name: "primary wins", primaryLanguage: "en", wantLanguage: "English"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "VIDEO_TS")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			fixture := func(language string) []byte {
				data := make([]byte, 0x300)
				copy(data[:12], "DVDVIDEO-VTS")
				data[dvdVideoAttrVTSOffset] = 0x4C
				data[dvdAudioCountVTSOffset+1] = 1
				data[dvdAudioAttrVTSOffset+1] = 1
				if len(language) == 2 {
					data[dvdAudioAttrVTSOffset+2] = language[0]
					data[dvdAudioAttrVTSOffset+3] = language[1]
				}
				return data
			}
			ifoPath := filepath.Join(root, "VTS_01_0.IFO")
			if err := os.WriteFile(ifoPath, fixture(tt.primaryLanguage), 0o644); err != nil { //nolint:gosec // test fixture
				t.Fatalf("write IFO: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, "vts_01_0.bup"), fixture("fr"), 0o644); err != nil { //nolint:gosec // test fixture
				t.Fatalf("write BUP: %v", err)
			}

			report, err := AnalyzeFile(ifoPath)
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}
			for _, stream := range report.Streams {
				if stream.Kind == StreamAudio {
					if got := findField(stream.Fields, "Language"); got != tt.wantLanguage {
						t.Fatalf("Language = %q, want %q", got, tt.wantLanguage)
					}
					return
				}
			}
			t.Fatal("missing audio stream")
		})
	}
}

func TestDVDTitleSetVOBsUsesExactSparseCaseInsensitiveMembers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, size := range map[string]int{
		"VTS_07_0.VOB":     11,
		"vts_07_1.vob":     13,
		"VTS_07_3.VOB":     17,
		"VTS_08_1.VOB":     19,
		"VTS_07_1.VOB.bak": 23,
	} {
		if err := os.WriteFile(filepath.Join(root, name), make([]byte, size), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatalf("write %s: %v", name, err)
		}
	}
	paths, size := dvdTitleSetVOBs(filepath.Join(root, "VTS_07_0.IFO"))
	if len(paths) != 2 || filepath.Base(paths[0]) != "vts_07_1.vob" || filepath.Base(paths[1]) != "VTS_07_3.VOB" {
		t.Fatalf("members = %v, want sparse title members 1 and 3", paths)
	}
	if size != 30 {
		t.Fatalf("size = %d, want 30", size)
	}
}

func TestOverlayDVDDeclaredLanguagesMatchesPayloadIdentity(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		format string
	}{
		{name: "MPEG audio PES", id: "192", format: "MPEG Audio"},
		{name: "AC-3 private stream", id: "189-128", format: "AC-3"},
		{name: "DTS private stream", id: "189-136", format: "DTS"},
		{name: "PCM private stream", id: "189-160", format: "PCM"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := Stream{Kind: StreamAudio}
			replaceCanonicalSeedFill(&stream, "ID", test.id, "ID", test.id)
			streams := []Stream{stream}
			overlayDVDDeclaredLanguages(streams, []dvdAudioAttrs{{
				Format:       test.format,
				Language:     "English",
				LanguageCode: "en",
				StreamID:     0,
			}}, nil)
			got, found := canonicalSeedValue(streams[0], "Language")
			if !found || got != "en" {
				t.Fatalf("Language = %q, found %v; want en", got, found)
			}
		})
	}
}

func TestOverlayDVDDeclaredLanguagesRejectsPositionalMatch(t *testing.T) {
	stream := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&stream, "ID", "193", "ID", "193")
	streams := []Stream{stream}
	overlayDVDDeclaredLanguages(streams, []dvdAudioAttrs{{
		Format:       "MPEG Audio",
		Language:     "English",
		LanguageCode: "en",
		StreamID:     0,
	}}, nil)
	if got, found := canonicalSeedValue(streams[0], "Language"); found {
		t.Fatalf("mismatched payload received positional language %q", got)
	}
}
