package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	mediainfo "github.com/autobrr/go-mediainfo"
)

func TestRunPreservesRawOutputFraming(t *testing.T) {
	sample := filepath.Join("..", "..", "samples", "sample.mkv")
	report, err := mediainfo.AnalyzeFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	want, err := mediainfo.RenderWithOptions([]mediainfo.Report{report}, mediainfo.OutputText, mediainfo.RenderOptions{Language: "raw"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"mediainfo", "--Language=raw", sample}, &stdout, &stderr); code != exitOK {
		t.Fatalf("Run exit = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != want {
		t.Fatalf("raw stdout framing changed:\ngot  %q\nwant %q", got, want)
	}
}

func TestRunAddsNewlineForNonRawOutput(t *testing.T) {
	sample := filepath.Join("..", "..", "samples", "sample.mkv")
	report, err := mediainfo.AnalyzeFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	want, err := mediainfo.RenderWithOptions([]mediainfo.Report{report}, mediainfo.OutputJSON, mediainfo.RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"mediainfo", "--Output=JSON", sample}, &stdout, &stderr); code != exitOK {
		t.Fatalf("Run exit = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != want+"\n" {
		t.Fatalf("non-raw stdout framing changed:\ngot  %q\nwant %q", got, want+"\n")
	}
}
