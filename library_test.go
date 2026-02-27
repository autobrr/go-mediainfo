package mediainfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func samplePath() string {
	return filepath.Join("samples", "sample.ts")
}

func findField(fields []Field, name string) (string, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return "", false
}

func writeContinuousSampleSet(t *testing.T) (string, string) {
	t.Helper()

	data, err := os.ReadFile(samplePath())
	if err != nil {
		t.Fatalf("os.ReadFile(sample): %v", err)
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "clip001.ts")
	last := filepath.Join(dir, "clip002.ts")
	if err := os.WriteFile(first, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", first, err)
	}
	if err := os.WriteFile(last, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", last, err)
	}
	return first, last
}

func TestAnalyzeFileAndRenderJSONSmoke(t *testing.T) {
	report, err := AnalyzeFile(samplePath())
	if err != nil {
		t.Fatalf("AnalyzeFile(sample): %v", err)
	}
	if report.Ref == "" {
		t.Fatal("report.Ref is empty")
	}
	if report.General.Kind != StreamGeneral {
		t.Fatalf("report.General.Kind=%q, want %q", report.General.Kind, StreamGeneral)
	}
	if len(report.General.Fields) == 0 {
		t.Fatal("report.General.Fields is empty")
	}

	out, err := Render([]Report{report}, OutputJSON)
	if err != nil {
		t.Fatalf("Render(JSON) error: %v", err)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("Render(JSON) output is invalid JSON: %q", out)
	}
}

func TestAnalyzeFileWithOptionsHonorsContinuousFileNames(t *testing.T) {
	first, last := writeContinuousSampleSet(t)

	reportDefault, err := AnalyzeFile(first)
	if err != nil {
		t.Fatalf("AnalyzeFile default: %v", err)
	}
	if _, ok := findField(reportDefault.General.Fields, "CompleteName_Last"); ok {
		t.Fatal("unexpected CompleteName_Last without options")
	}

	reportWithOpts, err := AnalyzeFile(first, WithTestContinuousFileNames(true))
	if err != nil {
		t.Fatalf("AnalyzeFile with opts: %v", err)
	}
	got, ok := findField(reportWithOpts.General.Fields, "CompleteName_Last")
	if !ok {
		t.Fatal("missing CompleteName_Last with WithTestContinuousFileNames(true)")
	}
	if got != last {
		t.Fatalf("CompleteName_Last=%q, want %q", got, last)
	}
}

func TestAnalyzeFilesAndAnalyzeFilesWithCountSmoke(t *testing.T) {
	reports, err := AnalyzeFiles([]string{samplePath()})
	if err != nil {
		t.Fatalf("AnalyzeFiles(sample): %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("AnalyzeFiles len=%d, want 1", len(reports))
	}

	first, last := writeContinuousSampleSet(t)
	reports, count, err := AnalyzeFilesWithCount(
		[]string{first},
		WithTestContinuousFileNames(true),
	)
	if err != nil {
		t.Fatalf("AnalyzeFilesWithCount: %v", err)
	}
	if count != 1 || len(reports) != 1 {
		t.Fatalf("AnalyzeFilesWithCount count/len=%d/%d, want 1/1", count, len(reports))
	}
	got, ok := findField(reports[0].General.Fields, "CompleteName_Last")
	if !ok {
		t.Fatal("missing CompleteName_Last in AnalyzeFilesWithCount")
	}
	if got != last {
		t.Fatalf("CompleteName_Last=%q, want %q", got, last)
	}
}

func TestRenderFormatsSmoke(t *testing.T) {
	report, err := AnalyzeFile(samplePath())
	if err != nil {
		t.Fatalf("AnalyzeFile(sample): %v", err)
	}
	reports := []Report{report}

	formats := map[OutputFormat]string{
		OutputText:        "OutputText",
		OutputJSON:        "OutputJSON",
		OutputXML:         "OutputXML",
		OutputOldXML:      "OutputOldXML",
		OutputHTML:        "OutputHTML",
		OutputCSV:         "OutputCSV",
		OutputEBUCore:     "OutputEBUCore",
		OutputEBUCoreJSON: "OutputEBUCoreJSON",
		OutputPBCore:      "OutputPBCore",
		OutputPBCore2:     "OutputPBCore2",
		OutputGraphSVG:    "OutputGraphSVG",
		OutputGraphDOT:    "OutputGraphDOT",
	}

	rendered := make(map[OutputFormat]string, len(formats))
	for format, name := range formats {
		out, err := Render(reports, format)
		if err != nil {
			t.Fatalf("%s render error: %v", name, err)
		}
		if strings.TrimSpace(out) == "" {
			t.Fatalf("%s renderer output is empty", name)
		}
		rendered[format] = out
	}

	if !json.Valid([]byte(rendered[OutputJSON])) {
		t.Fatalf("OutputJSON invalid JSON: %q", rendered[OutputJSON])
	}
	if !strings.Contains(rendered[OutputGraphSVG], "<svg") {
		t.Fatalf("OutputGraphSVG output=%q, want SVG tag", rendered[OutputGraphSVG])
	}
	if !strings.Contains(rendered[OutputGraphDOT], "digraph") {
		t.Fatalf("OutputGraphDOT output=%q, want digraph", rendered[OutputGraphDOT])
	}
}

func TestLibraryMetadataExports(t *testing.T) {
	if got := InfoParameters(); !strings.Contains(got, "General") {
		t.Fatalf("InfoParameters()=%q, want General section", got)
	}
	if got := FormatVersion("1.2.3"); got != "v1.2.3" {
		t.Fatalf("FormatVersion(1.2.3)=%q, want %q", got, "v1.2.3")
	}
}

func TestStreamKindAlias(t *testing.T) {
	if reflect.TypeOf(StreamVideo) != reflect.TypeOf(StreamKind("")) {
		t.Fatalf("StreamVideo type=%v, want %v", reflect.TypeOf(StreamVideo), reflect.TypeOf(StreamKind("")))
	}
	if StreamVideo != StreamKind("Video") {
		t.Fatalf("StreamVideo=%q, want %q", StreamVideo, StreamKind("Video"))
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	if _, err := Render(nil, OutputFormat("UNKNOWN")); err == nil {
		t.Fatal("Render error = nil, want non-nil")
	}
}
