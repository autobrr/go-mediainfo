package mediainfo_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mediainfo "github.com/autobrr/go-mediainfo"
)

func samplePath() string {
	return filepath.Join("samples", "sample.ts")
}

func findField(fields []mediainfo.Field, name string) (string, bool) {
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
	report, err := mediainfo.AnalyzeFile(samplePath())
	if err != nil {
		t.Fatalf("AnalyzeFile(sample): %v", err)
	}
	if report.Ref == "" {
		t.Fatal("report.Ref is empty")
	}
	if report.General.Kind != mediainfo.StreamGeneral {
		t.Fatalf("report.General.Kind=%q, want %q", report.General.Kind, mediainfo.StreamGeneral)
	}
	if len(report.General.Fields) == 0 {
		t.Fatal("report.General.Fields is empty")
	}

	out := mediainfo.RenderJSON([]mediainfo.Report{report})
	if !json.Valid([]byte(out)) {
		t.Fatalf("RenderJSON output is invalid JSON: %q", out)
	}
}

func TestAnalyzeFileWithOptionsHonorsContinuousFileNames(t *testing.T) {
	first, last := writeContinuousSampleSet(t)

	reportDefault, err := mediainfo.AnalyzeFileWithOptions(first, mediainfo.AnalyzeOptions{})
	if err != nil {
		t.Fatalf("AnalyzeFileWithOptions default: %v", err)
	}
	if _, ok := findField(reportDefault.General.Fields, "CompleteName_Last"); ok {
		t.Fatal("unexpected CompleteName_Last without options")
	}

	testContinuous := true
	reportWithOpts, err := mediainfo.AnalyzeFileWithOptions(first, mediainfo.AnalyzeOptions{
		TestContinuousFileNames: &testContinuous,
	})
	if err != nil {
		t.Fatalf("AnalyzeFileWithOptions with opts: %v", err)
	}
	got, ok := findField(reportWithOpts.General.Fields, "CompleteName_Last")
	if !ok {
		t.Fatal("missing CompleteName_Last with TestContinuousFileNames=true")
	}
	if got != last {
		t.Fatalf("CompleteName_Last=%q, want %q", got, last)
	}
}

func TestAnalyzeFilesAndAnalyzeFilesWithOptionsSmoke(t *testing.T) {
	reports, count, err := mediainfo.AnalyzeFiles([]string{samplePath()})
	if err != nil {
		t.Fatalf("AnalyzeFiles(sample): %v", err)
	}
	if count != 1 {
		t.Fatalf("AnalyzeFiles count=%d, want 1", count)
	}
	if len(reports) != 1 {
		t.Fatalf("AnalyzeFiles len=%d, want 1", len(reports))
	}

	first, last := writeContinuousSampleSet(t)
	testContinuous := true
	reports, count, err = mediainfo.AnalyzeFilesWithOptions([]string{first}, mediainfo.AnalyzeOptions{
		TestContinuousFileNames: &testContinuous,
	})
	if err != nil {
		t.Fatalf("AnalyzeFilesWithOptions: %v", err)
	}
	if count != 1 || len(reports) != 1 {
		t.Fatalf("AnalyzeFilesWithOptions count/len=%d/%d, want 1/1", count, len(reports))
	}
	got, ok := findField(reports[0].General.Fields, "CompleteName_Last")
	if !ok {
		t.Fatal("missing CompleteName_Last in AnalyzeFilesWithOptions")
	}
	if got != last {
		t.Fatalf("CompleteName_Last=%q, want %q", got, last)
	}
}

func TestDetectFormatSmoke(t *testing.T) {
	header, err := os.ReadFile(samplePath())
	if err != nil {
		t.Fatalf("os.ReadFile(sample): %v", err)
	}
	if len(header) > 512 {
		header = header[:512]
	}
	format := mediainfo.DetectFormat(header, samplePath())
	if format != "MPEG-TS" {
		t.Fatalf("DetectFormat=%q, want %q", format, "MPEG-TS")
	}
}

func TestFacadeRenderersSmoke(t *testing.T) {
	report, err := mediainfo.AnalyzeFile(samplePath())
	if err != nil {
		t.Fatalf("AnalyzeFile(sample): %v", err)
	}
	reports := []mediainfo.Report{report}

	rendered := []struct {
		name string
		out  string
	}{
		{name: "text", out: mediainfo.RenderText(reports)},
		{name: "json", out: mediainfo.RenderJSON(reports)},
		{name: "xml", out: mediainfo.RenderXML(reports)},
		{name: "html", out: mediainfo.RenderHTML(reports)},
		{name: "csv", out: mediainfo.RenderCSV(reports)},
		{name: "ebucore", out: mediainfo.RenderEBUCore(reports)},
		{name: "pbcore", out: mediainfo.RenderPBCore(reports)},
		{name: "graph-svg", out: mediainfo.RenderGraphSVG(reports)},
		{name: "graph-dot", out: mediainfo.RenderGraphDOT(reports)},
	}
	for _, item := range rendered {
		if strings.TrimSpace(item.out) == "" {
			t.Fatalf("%s renderer output is empty", item.name)
		}
	}
	if !json.Valid([]byte(rendered[1].out)) {
		t.Fatalf("RenderJSON output is invalid JSON: %q", rendered[1].out)
	}
	if !strings.Contains(rendered[7].out, "<svg") {
		t.Fatalf("RenderGraphSVG output=%q, want SVG tag", rendered[7].out)
	}
	if !strings.Contains(rendered[8].out, "digraph") {
		t.Fatalf("RenderGraphDOT output=%q, want digraph", rendered[8].out)
	}
}

func TestFacadeMetadataExports(t *testing.T) {
	if mediainfo.AppName == "" {
		t.Fatal("AppName is empty")
	}
	if mediainfo.AppURL == "" {
		t.Fatal("AppURL is empty")
	}
	if got := mediainfo.InfoParameters(); !strings.Contains(got, "General") {
		t.Fatalf("InfoParameters()=%q, want General section", got)
	}
}

func TestStreamKindAlias(t *testing.T) {
	var kind mediainfo.StreamKind = mediainfo.StreamVideo
	if kind != mediainfo.StreamVideo {
		t.Fatalf("kind=%q, want %q", kind, mediainfo.StreamVideo)
	}
}
