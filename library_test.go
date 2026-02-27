package mediainfo

import "testing"

func TestAnalyzeFile(t *testing.T) {
	report, err := AnalyzeFile("samples/sample.mp4")
	if err != nil {
		t.Fatalf("AnalyzeFile error = %v", err)
	}
	if report.General.Kind != StreamGeneral {
		t.Fatalf("General.Kind = %q, want %q", report.General.Kind, StreamGeneral)
	}
}

func TestAnalyzeFilesWithCount(t *testing.T) {
	reports, count, err := AnalyzeFilesWithCount(
		[]string{"samples/sample.mp4"},
		WithParseSpeed(0),
		WithTestContinuousFileNames(true),
	)
	if err != nil {
		t.Fatalf("AnalyzeFilesWithCount error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if len(reports) != 1 {
		t.Fatalf("len(reports) = %d, want 1", len(reports))
	}
}

func TestRenderUnknownFormat(t *testing.T) {
	if _, err := Render(nil, OutputFormat("UNKNOWN")); err == nil {
		t.Fatal("Render error = nil, want non-nil")
	}
}
