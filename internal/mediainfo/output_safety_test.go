package mediainfo

import (
	"encoding/csv"
	"encoding/json"
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
