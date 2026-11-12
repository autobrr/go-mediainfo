package mediainfo

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTextEscapesMetadataControls(t *testing.T) {
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Title", Value: "cover\n\x1b[31m\x00\u009b" + string([]byte{0x9B}) + ".jpg"}}}}
	output := RenderText([]Report{report})
	if strings.ContainsRune(output, '\x1b') || strings.ContainsRune(output, '\x00') || strings.ContainsRune(output, '\u009B') {
		t.Fatalf("text contains raw controls: %q", output)
	}
	if !strings.Contains(output, `cover\n\x1B[31m\x00\u009B\x9B.jpg`) {
		t.Fatalf("text did not visibly escape controls: %q", output)
	}
}

func TestRenderCSVNeutralizesFormulasAndPreservesMeasurements(t *testing.T) {
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{
		{Name: "Title", Value: "=HYPERLINK(\"https://invalid.example\")"},
		{Name: "Comment", Value: "  @SUM(1,1)"},
		{Name: "Encoded by", Value: "\uFEFF=1+1"},
		{Name: "Description", Value: "-cmd|' /C calc'!A0"},
		{Name: "Source", Value: "safe,=1+1"},
		{Name: "Delay", Value: "-83 ms"},
		{Name: "Gain", Value: "+1.5 dB"},
		{Name: "Signed expression", Value: "+1 ^ HYPERLINK(B1)"},
		{Name: "Attachment", Value: "cover\nname.jpg"},
	}}}
	output := RenderCSV([]Report{report})
	for _, want := range []string{
		`Title,"'=HYPERLINK(""https://invalid.example"")"`,
		`Comment,"'  @SUM(1,1)"`,
		"Encoded by,'\uFEFF=1+1",
		"Description,'-cmd",
		`Source,"safe,=1+1"`,
		"Delay,-83 ms",
		"Gain,+1.5 dB",
		"Signed expression,'+1 ^ HYPERLINK(B1)",
		`Attachment,cover\nname.jpg`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("CSV missing %q in %q", want, output)
		}
	}
	if strings.Count(output, "\n") != 11 {
		t.Fatalf("metadata forged CSV rows: %q", output)
	}
	var titleLine string
	var sourceLine string
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "Title,") {
			titleLine = line
		}
		if strings.HasPrefix(line, "Source,") {
			sourceLine = line
		}
	}
	titleReader := csv.NewReader(strings.NewReader(titleLine))
	titleRecords, err := titleReader.ReadAll()
	if err != nil {
		t.Fatalf("quoted Title CSV is not parseable: %v", err)
	}
	if len(titleRecords) != 1 || len(titleRecords[0]) != 2 || titleRecords[0][0] != "Title" || titleRecords[0][1] != `'=HYPERLINK("https://invalid.example")` {
		t.Fatalf("quoted Title did not round-trip: %#v", titleRecords)
	}
	reader := csv.NewReader(strings.NewReader(sourceLine))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	for _, record := range records {
		if len(record) == 2 && record[0] == "Source" && record[1] != "safe,=1+1" {
			t.Fatalf("delimiter-bearing value split or changed: %#v", record)
		}
	}
	if len(records) != 1 || len(records[0]) != 2 || records[0][0] != "Source" {
		t.Fatalf("delimiter-bearing source row missing: %#v", records)
	}
}

func TestSafeCSVOutputValuePreservesOrdinaryCommasAndContainsInjectedCells(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "ordinary comma", value: "English, French", want: "English, French"},
		{name: "formula with comma", value: "=SUM(1,1)", want: `"'=SUM(1,1)"`},
		{name: "injected cell", value: "safe,=1+1", want: `"safe,=1+1"`},
		{name: "ordinary quote", value: `Director's "Cut"`, want: `"Director's ""Cut"""`},
		{name: "quoted injected cell", value: `safe,"=1+1"`, want: `"safe,""=1+1"""`},
		{name: "signed expression", value: "+1 ^ HYPERLINK(B1)", want: "'+1 ^ HYPERLINK(B1)"},
		{name: "signed percent attached", value: "+12.5%", want: "+12.5%"},
		{name: "signed percent separated", value: "-12.5 %", want: "-12.5 %"},
		{name: "compound duration", value: "-1 s 250 ms", want: "-1 s 250 ms"},
		{name: "tab separated measurement", value: "+1.5\tdB", want: `+1.5\tdB`},
		{name: "arbitrary word is not a unit", value: "+1 HYPERLINK", want: "'+1 HYPERLINK"},
		{name: "operator as unit", value: "+1 / A1", want: "'+1 / A1"},
		{name: "hex float", value: "+0x1p2 ms", want: "'+0x1p2 ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeCSVOutputValue(tt.value); got != tt.want {
				t.Fatalf("safeCSVOutputValue(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestRenderJSONEscapesHostileMetadata(t *testing.T) {
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: "Title", Value: "cover\n\x1b.jpg"}}}}
	output := RenderJSON([]Report{report})
	if !json.Valid([]byte(output)) {
		t.Fatalf("invalid JSON: %q", output)
	}
	if strings.ContainsRune(output, '\x1b') || !strings.Contains(output, `cover\n\u001b.jpg`) {
		t.Fatalf("JSON did not escape hostile metadata: %q", output)
	}
}

func TestRenderersEscapeHostileDynamicLabels(t *testing.T) {
	label := "Safe\x1B[31m\a\r\n\t\x00\x7F\u0085\u2028\u2029Field"
	report := Report{General: Stream{Kind: StreamGeneral, Fields: []Field{{Name: label, Value: "value"}}}}
	wantVisible := `Safe\x1B[31m\x07\r\n\t\x00\x7F\u0085\u2028\u2029Field`

	for name, output := range map[string]string{
		"text": RenderText([]Report{report}),
		"raw":  RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"}),
		"CSV":  RenderCSV([]Report{report}),
		"HTML": RenderHTML([]Report{report}),
	} {
		if !strings.Contains(output, wantVisible) {
			t.Fatalf("%s omitted visibly escaped label %q: %q", name, wantVisible, output)
		}
		for _, unsafe := range []rune{'\x1B', '\a', '\x00', '\x7F', '\u0085', '\u2028', '\u2029'} {
			if strings.ContainsRune(output, unsafe) {
				t.Fatalf("%s retained unsafe label rune U+%04X: %q", name, unsafe, output)
			}
		}
		if strings.Contains(output, "\nField") || strings.Contains(output, "\r\nField") {
			t.Fatalf("%s label forged a new field: %q", name, output)
		}
	}

	jsonOutput := RenderJSON([]Report{report})
	if !json.Valid([]byte(jsonOutput)) || strings.ContainsRune(jsonOutput, '\x1B') {
		t.Fatalf("JSON key escaping failed: %q", jsonOutput)
	}
	xmlOutput := RenderXML([]Report{report})
	if strings.ContainsRune(xmlOutput, '\x1B') {
		t.Fatalf("XML name sanitization failed: %q", xmlOutput)
	}
	if err := xml.Unmarshal([]byte(xmlOutput), new(any)); err != nil {
		t.Fatalf("XML is not well formed: %v", err)
	}
}

func TestParsedID3AndFLACDynamicLabelsCannotForgeRawText(t *testing.T) {
	tests := []struct {
		name      string
		extension string
		data      []byte
		wantLabel string
	}{
		{
			name:      "ID3 TXXX",
			extension: ".mp3",
			data:      buildHostileID3MP3("Tag\u2028Next\nForged\x1B[31m"),
			wantLabel: `Tag\u2028Next\nForged\x1B[31m`,
		},
		{
			name:      "FLAC Vorbis comment",
			extension: ".flac",
			data:      buildHostileFLAC("Tag\u2028Next\nForged\x1B[31m"),
			wantLabel: `TAG\u2028NEXT\nFORGED\x1B[31M`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hostile"+test.extension)
			if err := os.WriteFile(path, test.data, 0o644); err != nil { //nolint:gosec // test fixture
				t.Fatalf("write fixture: %v", err)
			}
			report, err := AnalyzeFile(path)
			if err != nil {
				t.Fatalf("AnalyzeFile: %v", err)
			}
			output := RenderTextWithOptions([]Report{report}, TextRenderOptions{Language: "raw"})
			if !strings.Contains(output, test.wantLabel) {
				t.Fatalf("raw text omitted safe dynamic label %q: %q", test.wantLabel, output)
			}
			if strings.ContainsRune(output, '\x1B') || strings.ContainsRune(output, '\u2028') || strings.Contains(output, "\r\nForged") || strings.Contains(output, "\r\nFORGED") {
				t.Fatalf("dynamic label escaped its field: %q", output)
			}
		})
	}
}

func TestEscapeOutputControlsPreservesSafeLabels(t *testing.T) {
	for _, label := range []string{"Title", "Language, more info", "Recorded/Location", "日本語"} {
		if got := escapeOutputControls(label); got != label {
			t.Fatalf("safe label %q changed to %q", label, got)
		}
	}
}

func buildHostileID3MP3(description string) []byte {
	frameData := append([]byte{0x03}, description...)
	frameData = append(frameData, 0)
	frameData = append(frameData, "value"...)
	frame := make([]byte, 10, 10+len(frameData))
	copy(frame, "TXXX")
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(frameData)))
	frame = append(frame, frameData...)

	tag := make([]byte, 10, 10+len(frame))
	copy(tag, "ID3")
	tag[3] = 3
	writeSynchsafe32ForTest(tag[6:10], uint32(len(frame)))
	tag = append(tag, frame...)

	const frameLength = 313
	audio := make([]byte, frameLength*2)
	copy(audio, []byte{0xFF, 0xFB, 0x70, 0x40})
	copy(audio[frameLength:], []byte{0xFF, 0xFB, 0x70, 0x40})
	return append(tag, audio...)
}

func writeSynchsafe32ForTest(dst []byte, value uint32) {
	dst[0] = byte(value >> 21 & 0x7F)
	dst[1] = byte(value >> 14 & 0x7F)
	dst[2] = byte(value >> 7 & 0x7F)
	dst[3] = byte(value & 0x7F)
}

func buildHostileFLAC(key string) []byte {
	streamInfo, err := hex.DecodeString("1000100000001000210c0bb802f00e5b6540864d55f003143d8bad47d3b997fae64c")
	if err != nil {
		panic(err)
	}
	vendor := []byte("test")
	comment := []byte(key + "=value")
	vorbis := make([]byte, 4+len(vendor)+4+4+len(comment))
	binary.LittleEndian.PutUint32(vorbis[:4], uint32(len(vendor)))
	copy(vorbis[4:], vendor)
	countOffset := 4 + len(vendor)
	binary.LittleEndian.PutUint32(vorbis[countOffset:countOffset+4], 1)
	commentOffset := countOffset + 4
	binary.LittleEndian.PutUint32(vorbis[commentOffset:commentOffset+4], uint32(len(comment)))
	copy(vorbis[commentOffset+4:], comment)

	var fixture bytes.Buffer
	fixture.WriteString("fLaC")
	fixture.Write([]byte{0x00, 0x00, 0x00, byte(len(streamInfo))})
	fixture.Write(streamInfo)
	fixture.Write([]byte{0x84, byte(len(vorbis) >> 16), byte(len(vorbis) >> 8), byte(len(vorbis))})
	fixture.Write(vorbis)
	return fixture.Bytes()
}
