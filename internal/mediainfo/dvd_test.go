package mediainfo

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDVDHasConstantVideoBitRateUsesFirstVideoStream(t *testing.T) {
	streamWithMode := func(kind StreamKind, mode string) Stream {
		builder := newCanonicalStreamBuilder(kind)
		if mode != "" {
			builder.DirectStructured("BitRate_Mode", mode)
		}
		return builder.Snapshot(canonicalStreamPolicy{})
	}
	tests := []struct {
		name    string
		streams []Stream
		want    bool
	}{
		{name: "no streams"},
		{name: "audio only", streams: []Stream{streamWithMode(StreamAudio, "CBR")}},
		{name: "CBR", streams: []Stream{streamWithMode(StreamVideo, "CBR")}, want: true},
		{name: "Constant", streams: []Stream{streamWithMode(StreamVideo, "Constant")}, want: true},
		{name: "VBR", streams: []Stream{streamWithMode(StreamVideo, "VBR")}},
		{
			name: "skip audio before video",
			streams: []Stream{
				streamWithMode(StreamAudio, "CBR"),
				streamWithMode(StreamVideo, "Constant"),
			},
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dvdHasConstantVideoBitRate(test.streams); got != test.want {
				t.Fatalf("dvdHasConstantVideoBitRate() = %v; want %v", got, test.want)
			}
		})
	}
}

func TestDVDTitleSetBitRateDurationFlagsMuxRateAboveDVDLimit(t *testing.T) {
	const duration = 10.0
	atLimitSize := int64(dvdMaxMuxBitRate * duration / 8)

	gotDuration, mismatch := dvdTitleSetBitRateDuration(atLimitSize, 8, 9, duration)
	if gotDuration != duration || mismatch {
		t.Fatalf("at-limit result=%.3f/%v; want %.3f/false", gotDuration, mismatch, duration)
	}
	gotDuration, mismatch = dvdTitleSetBitRateDuration(atLimitSize+1, 8, 9, duration)
	if gotDuration != duration || !mismatch {
		t.Fatalf("above-limit result=%.3f/%v; want %.3f/true", gotDuration, mismatch, duration)
	}
}

func TestDVDPointerUses2048ByteSectorsAndBounds(t *testing.T) {
	data := make([]byte, 3*dvdSectorSize)
	binary.BigEndian.PutUint32(data[4:8], 2)
	if got := dvdPointer(data, 4); got != 2*dvdSectorSize {
		t.Fatalf("dvdPointer()=%d; want sector 2 offset %d", got, 2*dvdSectorSize)
	}
	binary.BigEndian.PutUint32(data[4:8], 3)
	if got := dvdPointer(data, 4); got != 0 {
		t.Fatalf("out-of-bounds dvdPointer()=%d; want 0", got)
	}
}

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

func TestAnalyzeVTSIFOAppliesLanguageToParsedAudio(t *testing.T) {
	root := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	ifoData := make([]byte, 0x0300)
	copy(ifoData[:12], []byte("DVDVIDEO-VTS"))
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	binary.BigEndian.PutUint16(ifoData[dvdAudioCountVTSOffset:], 1)
	ifoData[dvdAudioAttrVTSOffset+1] = 1
	ifoData[dvdAudioAttrVTSOffset+2] = 'e'
	ifoData[dvdAudioAttrVTSOffset+3] = 'n'
	if err := os.WriteFile(ifoPath, ifoData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write IFO: %v", err)
	}

	vobData, err := os.ReadFile(filepath.Join("samples", "sample_ac3.vob"))
	if err != nil {
		t.Fatalf("read VOB fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "VTS_01_1.VOB"), vobData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write VOB: %v", err)
	}

	report, err := AnalyzeFile(ifoPath)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if text := RenderText([]Report{report}); !strings.Contains(text, padRight("Language", textLabelWidth)+": English") {
		t.Fatal("text output lacks the IFO-declared audio language")
	}
	for _, stream := range report.Streams {
		if stream.Kind != StreamAudio || findField(stream.Fields, "ID") != "189 (0xBD)-128 (0x80)" {
			continue
		}
		if got := findField(stream.Fields, "Bit rate"); got == "" {
			t.Fatal("matched audio stream lacks VOB-derived details")
		}
		return
	}
	t.Fatal("missing parsed AC-3 audio stream")
}

func remappedSubpictureDVDFixture() ([]byte, []byte) {
	ifoData := make([]byte, 3*dvdSectorSize)
	copy(ifoData[:12], []byte("DVDVIDEO-VTS"))
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	binary.BigEndian.PutUint16(ifoData[dvdSubpicCountVTSOff:], 1)
	subpicture := dvdSubpicCountVTSOff + 2
	ifoData[subpicture+2] = 'e'
	ifoData[subpicture+3] = 'n'

	binary.BigEndian.PutUint32(ifoData[dvdPTTSRPTPointerOff:], 1)
	binary.BigEndian.PutUint32(ifoData[dvdPGCIPointerOff:], 2)
	pttOffset := dvdSectorSize
	binary.BigEndian.PutUint32(ifoData[pttOffset+8:], 12)
	binary.BigEndian.PutUint16(ifoData[pttOffset+12:], 1)
	pgcOffset := 2 * dvdSectorSize
	binary.BigEndian.PutUint16(ifoData[pgcOffset:], 1)
	binary.BigEndian.PutUint32(ifoData[pgcOffset+12:], 16)
	pgcBase := pgcOffset + 16
	binary.BigEndian.PutUint32(ifoData[pgcBase+0x1C:], 0x80000102)

	vobData := []byte{0x00, 0x00, 0x01, 0xBD, 0x00, 0x05, 0x80, 0x00, 0x00, 0x21, 0x00}
	return ifoData, vobData
}

func TestAnalyzeVTSIFOAppliesLanguageToRemappedSubpicture(t *testing.T) {
	root := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ifoData, vobData := remappedSubpictureDVDFixture()
	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	if err := os.WriteFile(ifoPath, ifoData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write IFO: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "VTS_01_1.VOB"), vobData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write VOB: %v", err)
	}

	report, err := AnalyzeFile(ifoPath)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if text := RenderText([]Report{report}); !strings.Contains(text, padRight("Language", textLabelWidth)+": English") {
		t.Fatalf("text output lacks the IFO-declared subpicture language:\n%s", text)
	}
	for _, stream := range report.Streams {
		if stream.Kind != StreamText || findField(stream.Fields, "ID") != "189 (0xBD)-33 (0x21)" {
			continue
		}
		if got, _ := canonicalSeedValue(stream, "Language"); got != "en" {
			t.Fatalf("canonical Language = %q, want en", got)
		}
		return
	}
	t.Fatal("missing parsed RLE subpicture stream")
}

func TestAnalyzeVTSIFORejectsConflictingPGCLanguages(t *testing.T) {
	root := filepath.Join(t.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ifoData := make([]byte, 3*dvdSectorSize)
	copy(ifoData[:12], "DVDVIDEO-VTS")
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	binary.BigEndian.PutUint16(ifoData[dvdAudioCountVTSOffset:], 2)
	ifoData[dvdAudioAttrVTSOffset+1] = 1
	copy(ifoData[dvdAudioAttrVTSOffset+2:], "en")
	ifoData[dvdAudioAttrVTSOffset+8+1] = 1
	copy(ifoData[dvdAudioAttrVTSOffset+8+2:], "fr")
	binary.BigEndian.PutUint16(ifoData[dvdSubpicCountVTSOff:], 2)
	copy(ifoData[dvdSubpicCountVTSOff+2+2:], "en")
	copy(ifoData[dvdSubpicCountVTSOff+2+6+2:], "fr")

	binary.BigEndian.PutUint32(ifoData[dvdPTTSRPTPointerOff:], 1)
	binary.BigEndian.PutUint32(ifoData[dvdPGCIPointerOff:], 2)
	pttOffset := dvdSectorSize
	binary.BigEndian.PutUint16(ifoData[pttOffset:], 2)
	binary.BigEndian.PutUint32(ifoData[pttOffset+4:], 23)
	binary.BigEndian.PutUint32(ifoData[pttOffset+8:], 16)
	binary.BigEndian.PutUint32(ifoData[pttOffset+12:], 20)
	binary.BigEndian.PutUint16(ifoData[pttOffset+16:], 1)
	binary.BigEndian.PutUint16(ifoData[pttOffset+18:], 1)
	binary.BigEndian.PutUint16(ifoData[pttOffset+20:], 2)
	binary.BigEndian.PutUint16(ifoData[pttOffset+22:], 1)

	pgcOffset := 2 * dvdSectorSize
	binary.BigEndian.PutUint16(ifoData[pgcOffset:], 2)
	pgcBases := []int{pgcOffset + 0x20, pgcOffset + 0x120}
	for i, base := range pgcBases {
		entry := pgcOffset + 8 + i*8
		ifoData[entry] = 0x80
		binary.BigEndian.PutUint32(ifoData[entry+4:], uint32(base-pgcOffset))
		copy(ifoData[base+4:], []byte{0x00, 0x01, 0x00, 0x40})
	}
	binary.BigEndian.PutUint16(ifoData[pgcBases[0]+0x0C:], 0x8000)
	binary.BigEndian.PutUint32(ifoData[pgcBases[0]+0x1C:], 0x80000000)
	binary.BigEndian.PutUint16(ifoData[pgcBases[1]+0x0C+2:], 0x8000)
	binary.BigEndian.PutUint32(ifoData[pgcBases[1]+0x1C+4:], 0x80000000)

	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	if err := os.WriteFile(ifoPath, ifoData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write IFO: %v", err)
	}
	vobData, err := os.ReadFile(filepath.Join("samples", "sample_ac3.vob"))
	if err != nil {
		t.Fatalf("read VOB fixture: %v", err)
	}
	vobData = append(vobData, 0x00, 0x00, 0x01, 0xBD, 0x00, 0x05, 0x80, 0x00, 0x00, 0x20, 0x00)
	if err := os.WriteFile(filepath.Join(root, "VTS_01_1.VOB"), vobData, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write VOB: %v", err)
	}

	report, err := AnalyzeFile(ifoPath)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	wantIDs := map[StreamKind]string{StreamAudio: "189-128", StreamText: "189-32"}
	for kind, wantID := range wantIDs {
		found := false
		for _, stream := range report.Streams {
			id, _ := canonicalSeedValue(stream, "ID")
			if stream.Kind != kind || id != wantID {
				continue
			}
			found = true
			if got, ok := canonicalSeedValue(stream, "Language"); ok {
				t.Errorf("%s payload received conflicting PGC language %q", kind, got)
			}
		}
		if !found {
			t.Errorf("missing parsed %s payload %s", kind, wantID)
		}
	}
}

func FuzzAnalyzeVTSIFO(f *testing.F) {
	ifoData, vobData := remappedSubpictureDVDFixture()
	f.Add(ifoData, vobData)
	f.Add(ifoData[:len(ifoData)/2], vobData)
	f.Add(ifoData, vobData[:len(vobData)/2])

	root := filepath.Join(f.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		f.Fatalf("mkdir: %v", err)
	}
	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	vobPath := filepath.Join(root, "VTS_01_1.VOB")

	f.Fuzz(func(t *testing.T, ifoData, vobData []byte) {
		ifoData = fuzzLimit(ifoData)
		vobData = fuzzLimit(vobData)
		if err := os.WriteFile(ifoPath, ifoData, 0o600); err != nil { //nolint:gosec // bounded fuzz fixture
			t.Fatalf("write IFO: %v", err)
		}
		if err := os.WriteFile(vobPath, vobData, 0o600); err != nil { //nolint:gosec // bounded fuzz fixture
			t.Fatalf("write VOB: %v", err)
		}
		_, _ = AnalyzeFile(ifoPath)
	})
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

func TestDVDLanguageMoreTablesMatchMediaInfo(t *testing.T) {
	audio := []string{
		"",
		"",
		"For visually impaired",
		"Director's comments",
		"Director's comments",
		"",
		"",
		"",
	}
	for code, want := range audio {
		if got := dvdAudioLanguageMore(byte(code)); got != want {
			t.Errorf("audio code %d = %q, want %q", code, got, want)
		}
	}
	if got := dvdAudioLanguageMore(8); got != "" {
		t.Errorf("reserved audio code = %q, want empty", got)
	}

	subpicture := []string{
		"",
		"Normal",
		"Large",
		"Children",
		"",
		"",
		"Large",
		"Children",
		"",
		"Forced",
		"",
		"",
		"",
		"Director comments",
		"Director comments large",
		"Director comments children",
	}
	for code, want := range subpicture {
		if got := dvdLanguageMore(byte(code)); got != want {
			t.Errorf("subpicture code %d = %q, want %q", code, got, want)
		}
	}
	if got := dvdLanguageMore(16); got != "" {
		t.Errorf("reserved subpicture code = %q, want empty", got)
	}
}

func TestAnalyzeDVDProjectsDistinctLanguageMoreTables(t *testing.T) {
	for _, extension := range []string{".IFO", ".BUP"} {
		t.Run(extension, func(t *testing.T) {
			data := make([]byte, 0x300)
			copy(data[:12], "DVDVIDEO-VTS")
			data[dvdVideoAttrVTSOffset] = 0x4C

			binary.BigEndian.PutUint16(data[dvdAudioCountVTSOffset:], 1)
			audio := dvdAudioAttrVTSOffset
			data[audio+1] = 1
			data[audio+2] = 'e'
			data[audio+3] = 'n'
			data[audio+5] = 3

			binary.BigEndian.PutUint16(data[dvdSubpicCountVTSOff:], 1)
			subpicture := dvdSubpicCountVTSOff + 2
			data[subpicture+2] = 'e'
			data[subpicture+3] = 'n'
			data[subpicture+5] = 14

			path := filepath.Join(t.TempDir(), "VTS_01_0"+extension)
			if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // test fixture
				t.Fatalf("write DVD fixture: %v", err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}

			gotAudio := ""
			gotSubpicture := ""
			for _, stream := range report.Streams {
				switch stream.Kind {
				case StreamAudio:
					gotAudio = findField(stream.Fields, "Language, more info")
				case StreamText:
					gotSubpicture = findField(stream.Fields, "Language, more info")
				case StreamGeneral, StreamVideo, StreamImage, StreamMenu:
				}
			}
			if gotAudio != "Director's comments" {
				t.Fatalf("audio Language, more info = %q", gotAudio)
			}
			if gotSubpicture != "Director comments large" {
				t.Fatalf("subpicture Language, more info = %q", gotSubpicture)
			}
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

func TestOverlayDVDDeclaredLanguagesRejectsConflictingAudioIdentity(t *testing.T) {
	stream := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&stream, "ID", "189-128", "ID", "189-128")
	streams := []Stream{stream}
	overlayDVDDeclaredLanguages(streams, []dvdAudioAttrs{
		{Format: "AC-3", Language: "English", LanguageCode: "en", StreamID: 0},
		{Format: "AC-3", Language: "French", LanguageCode: "fr", StreamID: 0},
	}, nil)
	if got, found := canonicalSeedValue(streams[0], "Language"); found {
		t.Fatalf("ambiguous payload received language %q", got)
	}
}

func TestOverlayDVDDeclaredLanguagesMatchesRemappedSubpictureIdentity(t *testing.T) {
	textStream := func(id, languageCode, language string) Stream {
		stream := Stream{Kind: StreamText}
		replaceCanonicalSeedFill(&stream, "ID", id, "ID", id)
		replaceCanonicalSeedFill(&stream, "Language", languageCode, "Language", language)
		return stream
	}
	streams := []Stream{
		textStream("189-32", "", ""),
		textStream("189-33", "", ""),
		textStream("189-34", "it", "Italian"),
		textStream("189-35", "nl", "Dutch"),
		textStream("189-36", "", ""),
	}
	overlayDVDDeclaredLanguages(streams, nil, []dvdSubpicAttrs{
		{Language: "English", LanguageCode: "en", StreamID: 1, AlternateStreamIDs: []int{4}},
		{Language: "French", LanguageCode: "fr", StreamID: 0, AlternateStreamIDs: []int{2}},
		{Language: "German", LanguageCode: "de", StreamID: 3},
		{Language: "Spanish", LanguageCode: "es", StreamID: 3},
		{Language: "Italian", LanguageCode: "it", StreamID: 2},
	})

	wants := []struct {
		name     string
		language string
	}{
		{name: "reordered primary zero", language: "fr"},
		{name: "reordered primary one", language: "en"},
		{name: "primary alternate collision preserves payload", language: "it"},
		{name: "duplicate primary collision preserves payload", language: "nl"},
		{name: "unique alternate", language: "en"},
	}
	for index, want := range wants {
		got, _ := canonicalSeedValue(streams[index], "Language")
		if got != want.language {
			t.Errorf("%s stream Language = %q, want %q", want.name, got, want.language)
		}
	}
}

func TestDVDTitleSetBitRateDurationRetainsBoundedPayloadClock(t *testing.T) {
	if got, mismatch := dvdTitleSetBitRateDuration(7_272_744_960, 8855, 8771.8, 83.3); got != 83.3 || !mismatch {
		t.Fatalf("duration = %v, mismatch = %v; want mismatched bounded payload timing", got, mismatch)
	}
	if got, mismatch := dvdTitleSetBitRateDuration(1_000_000_000, 8855, 8771.8, 1000); got != 1000 || mismatch {
		t.Fatalf("plausible duration = %v, mismatch = %v; want bounded timing retained", got, mismatch)
	}
	if got, mismatch := dvdTitleSetBitRateDuration(7_272_744_960, 0, 8771.8, 83.3); got != 83.3 || !mismatch {
		t.Fatalf("IFO fallback duration = %v, mismatch = %v; want mismatched bounded payload timing", got, mismatch)
	}
	if got, mismatch := dvdTitleSetBitRateDuration(7_272_744_960, 0, 0, 83.3); got != 83.3 || !mismatch {
		t.Fatalf("payload fallback duration = %v, mismatch = %v; want mismatched bounded payload timing", got, mismatch)
	}
}

func BenchmarkDVDTitleSetParsing(b *testing.B) {
	root := filepath.Join(b.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		b.Fatal(err)
	}
	ifoPath := filepath.Join(root, "VTS_01_0.IFO")
	ifoData := make([]byte, 0x0300)
	copy(ifoData[:12], []byte("DVDVIDEO-VTS"))
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	if err := os.WriteFile(ifoPath, ifoData, 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VTS_01_1.VOB"), make([]byte, 2<<20), 0o600); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(2 << 20)
	b.ResetTimer()
	for range b.N {
		if _, err := AnalyzeFile(ifoPath); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDVDPGCTimelineDurationDeduplicatesFirstSector(t *testing.T) {
	const pgcOffset = 0x100
	data := make([]byte, 0x1000)
	binary.BigEndian.PutUint16(data[pgcOffset:], 3)
	setPGC := func(index, relative int, duration [4]byte, firstSector, lastSector uint32) {
		entry := pgcOffset + 8 + index*8
		data[entry] = 0x80
		binary.BigEndian.PutUint32(data[entry+4:], uint32(relative))
		base := pgcOffset + relative
		data[base+3] = 1
		copy(data[base+4:base+8], duration[:])
		binary.BigEndian.PutUint16(data[base+0xE8:], 0xF0)
		cell := base + 0xF0
		binary.BigEndian.PutUint32(data[cell+8:], firstSector)
		binary.BigEndian.PutUint32(data[cell+20:], lastSector)
	}
	setPGC(0, 0x100, [4]byte{0x00, 0x01, 0x00, 0x40}, 100, 199)
	setPGC(1, 0x300, [4]byte{0x00, 0x02, 0x00, 0x40}, 100, 199)
	setPGC(2, 0x500, [4]byte{0x00, 0x00, 0x30, 0x40}, 200, 299)

	if got := dvdPGCTimelineDuration(data, pgcOffset); got != 150 {
		t.Fatalf("timeline duration = %v, want 150 seconds", got)
	}
	delays := dvdProgramStartDelays(data, pgcOffset, make([]dvdProgram, 3))
	if delays[1] != 0 || delays[2] != 120 {
		t.Fatalf("timeline delays = %v, want replacement at 0 and next PGC at 120", delays)
	}
}

func TestDeriveDVDPSVideoBitRateAndSizeUsesBoundedPayloadClock(t *testing.T) {
	video := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&video, "BitRate_Mode", "CBR", "Bit rate mode", "Constant")
	replaceCanonicalSeedFill(&video, "BitRate", "6000000", "Bit rate", "6000 kb/s")
	replaceCanonicalSeedFill(&video, "Duration", "83300", "Duration", "1 min 23 s")
	replaceCanonicalSeedFill(&video, "FrameCount", "2496", "", "")
	replaceCanonicalSeedFill(&video, "FrameRate", "29.970", "Frame rate", "29.970 FPS")

	audio := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&audio, "BitRate", "384000", "Bit rate", "384 kb/s")
	streams := []Stream{video, audio}

	const (
		payloadSize    = int64(7272744960)
		sampledSeconds = 83.3
	)
	deriveDVDPSVideoBitRateAndSize(streams, payloadSize, sampledSeconds, true)

	overall := math.Round(float64(payloadSize) * 8 / sampledSeconds)
	derivedBitRate := (overall*0.99 - 384000/0.99) * 0.99
	wantBitRate := strconv.FormatInt(int64(math.Round(derivedBitRate)), 10)
	if got, _ := canonicalSeedValue(streams[0], "BitRate"); got != wantBitRate {
		t.Fatalf("video bit rate = %q, want bounded-clock rate %s", got, wantBitRate)
	}
	if got, _ := canonicalSeedValue(streams[0], "BitRate_Maximum"); got != "6000000" {
		t.Fatalf("maximum video bit rate = %q, want retained sequence-header rate", got)
	}
	wantSize := int64(math.Round(derivedBitRate / 8 * (2496 / 29.970)))
	gotSizeValue, _ := canonicalSeedValue(streams[0], "StreamSize")
	gotSize, err := strconv.ParseInt(gotSizeValue, 10, 64)
	if err != nil {
		t.Fatalf("parse StreamSize %q: %v", gotSizeValue, err)
	}
	if gotSize != wantSize {
		t.Fatalf("video stream size = %d, want %d from bounded frame clock", gotSize, wantSize)
	}
}

func TestExpandDVDPrimaryAudioDurationUsesCanonicalMilliseconds(t *testing.T) {
	video := Stream{Kind: StreamVideo}
	replaceCanonicalSeedFill(&video, "Duration", "120000", "Duration", "2 min 0 s")
	audio := Stream{Kind: StreamAudio}
	replaceCanonicalSeedFill(&audio, "Duration", "1000", "Duration", "1 s 0 ms")
	replaceCanonicalSeedFill(&audio, "SamplingRate", "48000", "Sampling rate", "48.0 kHz")
	replaceCanonicalSeedFill(&audio, "SamplingCount", "48000", "", "")

	streams := []Stream{video, audio}
	expandDVDPrimaryAudioDuration(streams, false)

	if got, _ := canonicalSeedValue(streams[1], "Duration"); got != "120000" {
		t.Fatalf("canonical Duration = %q, want milliseconds", got)
	}
	if got, _ := projectedCanonicalSeedValue(streams[1], "Duration"); got != "120.000" {
		t.Fatalf("projected Duration = %q, want seconds", got)
	}
}
