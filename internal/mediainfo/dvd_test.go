package mediainfo

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// dvdComparableOutputs renders the JSON and text reports of path with the path
// and the file dates removed, so that reports of copies at other paths compare.
func dvdComparableOutputs(t *testing.T, path string) (string, string) {
	t.Helper()
	report, err := AnalyzeFile(path)
	if err != nil {
		t.Fatalf("AnalyzeFile(%s): %v", path, err)
	}
	escaped, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	jsonLines := []string{}
	for line := range strings.SplitSeq(RenderJSON([]Report{report}), "\n") {
		if !strings.HasPrefix(line, `"File_`) {
			jsonLines = append(jsonLines, strings.ReplaceAll(line, string(escaped[1:len(escaped)-1]), "INPUT"))
		}
	}
	return strings.Join(jsonLines, "\n"), strings.ReplaceAll(RenderText([]Report{report}), path, "INPUT")
}

func writeDVDFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestAnalyzeTitleVOBReportsOnlyTheNamedFile(t *testing.T) {
	vobData, err := os.ReadFile(filepath.Join("samples", "sample_ac3.vob"))
	if err != nil {
		t.Fatalf("read VOB fixture: %v", err)
	}
	second := vobData[:len(vobData)/2]

	set := filepath.Join(t.TempDir(), "VIDEO_TS")
	writeDVDFixture(t, filepath.Join(set, "VTS_01_1.VOB"), vobData)
	writeDVDFixture(t, filepath.Join(set, "VTS_01_2.VOB"), second)
	writeDVDFixture(t, filepath.Join(set, "VTS_01_3.VOB"), vobData)
	alone := filepath.Join(t.TempDir(), "VIDEO_TS", "VTS_01_2.VOB")
	writeDVDFixture(t, alone, second)

	gotJSON, gotText := dvdComparableOutputs(t, filepath.Join(set, "VTS_01_2.VOB"))
	size := strconv.Itoa(len(second))
	if !strings.Contains(gotJSON, `"FileSize":"`+size+`"`) {
		t.Errorf("JSON FileSize is not the size of the named VOB (%s)", size)
	}
	if !strings.Contains(gotText, padRight("File size", textLabelWidth)+": "+formatBytes(int64(len(second)))+"\n") {
		t.Errorf("text File size is not the size of the named VOB (%s)", formatBytes(int64(len(second))))
	}
	if strings.Contains(gotJSON, "CompleteName_Last") || strings.Contains(gotText, "VTS_01_3.VOB") {
		t.Error("report names the last VOB of the title set")
	}

	wantJSON, wantText := dvdComparableOutputs(t, alone)
	if gotJSON != wantJSON {
		t.Errorf("JSON of a title VOB in a set differs from the same VOB alone:\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
	if gotText != wantText {
		t.Errorf("text of a title VOB in a set differs from the same VOB alone:\n got: %s\nwant: %s", gotText, wantText)
	}
}

func TestAnalyzeVTSIFOReportsOnlyTheIFO(t *testing.T) {
	ifoData := make([]byte, 0x0300)
	copy(ifoData[:12], "DVDVIDEO-VTS")
	ifoData[dvdVideoAttrVTSOffset] = 0x4C
	binary.BigEndian.PutUint16(ifoData[dvdAudioCountVTSOffset:], 1)
	ifoData[dvdAudioAttrVTSOffset+1] = 1
	ifoData[dvdAudioAttrVTSOffset+2] = 'e'
	ifoData[dvdAudioAttrVTSOffset+3] = 'n'
	vobData, err := os.ReadFile(filepath.Join("samples", "sample_ac3.vob"))
	if err != nil {
		t.Fatalf("read VOB fixture: %v", err)
	}

	videoTS := filepath.Join(t.TempDir(), "VIDEO_TS")
	ifoPath := filepath.Join(videoTS, "VTS_01_0.IFO")
	writeDVDFixture(t, ifoPath, ifoData)
	writeDVDFixture(t, filepath.Join(videoTS, "VTS_01_1.VOB"), vobData)
	writeDVDFixture(t, filepath.Join(videoTS, "VTS_01_2.VOB"), vobData)
	outside := filepath.Join(t.TempDir(), "VTS_01_0.IFO")
	writeDVDFixture(t, outside, ifoData)
	wantJSON, wantText := dvdComparableOutputs(t, outside)

	for _, withBUP := range []bool{false, true} {
		t.Run(fmt.Sprintf("BUP=%v", withBUP), func(t *testing.T) {
			if withBUP {
				writeDVDFixture(t, filepath.Join(videoTS, "VTS_01_0.BUP"), ifoData)
			}
			gotJSON, gotText := dvdComparableOutputs(t, ifoPath)
			if !strings.Contains(gotJSON, `"FileSize":"`+strconv.Itoa(len(ifoData))+`"`) {
				t.Error("JSON FileSize is not the IFO size")
			}
			if !strings.Contains(gotText, padRight("File size", textLabelWidth)+": "+formatBytes(int64(len(ifoData)))+"\n") {
				t.Error("text File size is not the IFO size")
			}
			if !strings.Contains(gotText, padRight("Format profile", textLabelWidth)+": Program\n") {
				t.Error("Format profile is not Program")
			}
			for _, vobOnly := range []string{"ServiceKind", "dialnorm", "StreamSize", "BitRate_Maximum"} {
				if strings.Contains(gotJSON, vobOnly) {
					t.Errorf("JSON has the VOB value %s", vobOnly)
				}
			}
			if gotJSON != wantJSON {
				t.Errorf("JSON differs from the same IFO outside VIDEO_TS:\n got: %s\nwant: %s", gotJSON, wantJSON)
			}
			if gotText != wantText {
				t.Errorf("text differs from the same IFO outside VIDEO_TS:\n got: %s\nwant: %s", gotText, wantText)
			}
		})
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

func remappedSubpictureDVDFixture() []byte {
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
	return ifoData
}

func TestAnalyzeVTSIFOAppliesLanguageToRemappedSubpicture(t *testing.T) {
	ifoData := remappedSubpictureDVDFixture()
	ifoPath := filepath.Join(t.TempDir(), "VIDEO_TS", "VTS_01_0.IFO")
	writeDVDFixture(t, ifoPath, ifoData)

	report, err := AnalyzeFile(ifoPath)
	if err != nil {
		t.Fatalf("AnalyzeFile: %v", err)
	}
	if text := RenderText([]Report{report}); !strings.Contains(text, padRight("Language", textLabelWidth)+": English") {
		t.Fatalf("text output lacks the IFO-declared subpicture language:\n%s", text)
	}
	for _, stream := range report.Streams {
		if stream.Kind != StreamText {
			continue
		}
		if got, _ := canonicalSeedValue(stream, "Language"); got != "en" {
			t.Fatalf("canonical Language = %q, want en", got)
		}
		return
	}
	t.Fatal("missing declared RLE subpicture stream")
}

func TestApplyDVDPGCStreamControlsKeepsSubpictureEnabledByLaterProgram(t *testing.T) {
	const (
		pttOffset = 0x20
		pgcOffset = 0x80
		pgc1Base  = 0xC0
		pgc2Base  = 0x1C0
	)
	data := make([]byte, 0x300)
	binary.BigEndian.PutUint16(data[pttOffset:], 2)
	binary.BigEndian.PutUint32(data[pttOffset+4:], 23)
	binary.BigEndian.PutUint32(data[pttOffset+8:], 16)
	binary.BigEndian.PutUint32(data[pttOffset+12:], 20)
	binary.BigEndian.PutUint16(data[pttOffset+16:], 1)
	binary.BigEndian.PutUint16(data[pttOffset+18:], 1)
	binary.BigEndian.PutUint16(data[pttOffset+20:], 2)
	binary.BigEndian.PutUint16(data[pttOffset+22:], 1)

	binary.BigEndian.PutUint16(data[pgcOffset:], 2)
	data[pgcOffset+8] = 0x80
	binary.BigEndian.PutUint32(data[pgcOffset+12:], pgc1Base-pgcOffset)
	data[pgcOffset+16] = 0x80
	binary.BigEndian.PutUint32(data[pgcOffset+20:], pgc2Base-pgcOffset)
	data[pgc1Base+6] = 0x10
	data[pgc2Base+6] = 0x10
	binary.BigEndian.PutUint32(data[pgc2Base+0x1C:], 0x81010101)

	_, subpics, _, known := applyDVDPGCStreamControls(data, pttOffset, pgcOffset, dvdVideoAttrs{AspectRatio: "16:9"}, nil, []dvdSubpicAttrs{{
		Language:     "English",
		LanguageCode: "en",
	}})
	if !known {
		t.Fatal("PGC controls not detected")
	}
	if len(subpics) != 1 || subpics[0].StreamID != 1 {
		t.Fatalf("subpictures = %+v, want stream 1 enabled by the second PGC", subpics)
	}
	if subpics[0].LanguageCode != "en" {
		t.Fatalf("subtitle language = %q, want en", subpics[0].LanguageCode)
	}
}

func FuzzAnalyzeVTSIFO(f *testing.F) {
	ifoData := remappedSubpictureDVDFixture()
	f.Add(ifoData)
	f.Add(ifoData[:len(ifoData)/2])

	root := filepath.Join(f.TempDir(), "VIDEO_TS")
	if err := os.MkdirAll(root, 0o755); err != nil {
		f.Fatalf("mkdir: %v", err)
	}
	ifoPath := filepath.Join(root, "VTS_01_0.IFO")

	f.Fuzz(func(t *testing.T, ifoData []byte) {
		if err := os.WriteFile(ifoPath, fuzzLimit(ifoData), 0o600); err != nil { //nolint:gosec // bounded fuzz fixture
			t.Fatalf("write IFO: %v", err)
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

func TestDVDProgramStartDelaysDeduplicateFirstSector(t *testing.T) {
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

	delays := dvdProgramStartDelays(data, pgcOffset, make([]dvdProgram, 3))
	if delays[1] != 0 || delays[2] != 120 {
		t.Fatalf("timeline delays = %v, want replacement at 0 and next PGC at 120", delays)
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
	expandDVDPrimaryAudioDuration(streams)

	if got, _ := canonicalSeedValue(streams[1], "Duration"); got != "120000" {
		t.Fatalf("canonical Duration = %q, want milliseconds", got)
	}
	if got, _ := projectedCanonicalSeedValue(streams[1], "Duration"); got != "120.000" {
		t.Fatalf("projected Duration = %q, want seconds", got)
	}
}
